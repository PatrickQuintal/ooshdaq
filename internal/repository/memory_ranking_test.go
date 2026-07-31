package repository

import (
	"context"
	"testing"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/domain"
)

func TestMatchWaitRankCapsAgeAndUsesStrictStaleConjunction(t *testing.T) {
	t.Parallel()

	rankingAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	capEpoch := rankingAt.Add(-MatchWaitCap).Unix()
	oldState := rankingAt.Add(-120 * 24 * time.Hour)
	freshActivity := rankingAt.Add(-time.Hour)

	capped, stale := matchWaitRank(oldState, time.Time{}, rankingAt)
	if capped != capEpoch || !stale {
		t.Fatalf("old state with absent activity rank = %d/%t", capped, stale)
	}
	capped, stale = matchWaitRank(oldState, freshActivity, rankingAt)
	if capped != capEpoch || stale {
		t.Fatalf("fresh activity did not clear stale flag = %d/%t", capped, stale)
	}
	exactCap := rankingAt.Add(-MatchWaitCap)
	capped, stale = matchWaitRank(exactCap, time.Time{}, rankingAt)
	if capped != exactCap.Unix() || stale {
		t.Fatalf("exact cap rank = %d/%t", capped, stale)
	}
	exactStale := rankingAt.Add(-MatchStaleAfter)
	capped, stale = matchWaitRank(exactStale, time.Time{}, rankingAt)
	if capped != capEpoch || stale {
		t.Fatalf("exact stale boundary rank = %d/%t", capped, stale)
	}
	olderThanStale := exactStale.Add(-time.Second)
	_, stale = matchWaitRank(olderThanStale, exactStale.Add(-time.Second), rankingAt)
	if !stale {
		t.Fatal("state and activity older than stale boundary were not stale")
	}
	_, stale = matchWaitRank(olderThanStale, exactStale, rankingAt)
	if stale {
		t.Fatal("activity exactly at stale boundary was treated as stale")
	}
	recentState := rankingAt.Add(-30 * 24 * time.Hour)
	capped, stale = matchWaitRank(recentState, time.Time{}, rankingAt)
	if capped != recentState.Unix() || stale {
		t.Fatalf("recent state rank = %d/%t", capped, stale)
	}
}

func TestMemoryUserActivityIsExplicitAndRateLimited(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	base := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	store.now = func() time.Time { return base }
	user := domain.User{ID: "activity-member", DisplayName: "Activity Member"}
	if err := store.UpsertUser(context.Background(), user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if !store.lastSeen[user.ID].IsZero() {
		t.Fatalf("profile upsert fabricated activity = %s", store.lastSeen[user.ID])
	}
	if err := store.TouchUserLastSeen(context.Background(), user.ID); err != nil {
		t.Fatalf("TouchUserLastSeen: %v", err)
	}
	if !store.lastSeen[user.ID].Equal(base) {
		t.Fatalf("first activity = %s, want %s", store.lastSeen[user.ID], base)
	}
	store.now = func() time.Time { return base.Add(59 * time.Minute) }
	if err := store.TouchUserLastSeen(context.Background(), user.ID); err != nil {
		t.Fatalf("rate-limited TouchUserLastSeen: %v", err)
	}
	if !store.lastSeen[user.ID].Equal(base) {
		t.Fatalf("sub-hour activity changed to %s", store.lastSeen[user.ID])
	}
	exactHour := base.Add(LastSeenTouchInterval)
	store.now = func() time.Time { return exactHour }
	if err := store.TouchUserLastSeen(context.Background(), user.ID); err != nil {
		t.Fatalf("exact-hour TouchUserLastSeen: %v", err)
	}
	if !store.lastSeen[user.ID].Equal(exactHour) {
		t.Fatalf("exact-hour activity = %s, want %s", store.lastSeen[user.ID], exactHour)
	}
}

func TestMatchPositionOrderingUsesCompletePrivateTuple(t *testing.T) {
	t.Parallel()

	base := MatchPosition{
		RequestClass:  0,
		CappedSince:   100,
		RequestItemID: "item-a",
		CounterpartID: "member-a",
		ReceiveItemID: "receive-a",
	}
	tests := []MatchPosition{
		{RequestClass: 1, CappedSince: 1, CounterpartID: "member-a", ReceiveItemID: "receive-a"},
		{
			RequestClass:  0,
			CappedSince:   101,
			RequestItemID: "item-a",
			CounterpartID: "member-a",
			ReceiveItemID: "receive-a",
		},
		{
			RequestClass:  0,
			CappedSince:   100,
			Stale:         true,
			RequestItemID: "item-a",
			CounterpartID: "member-a",
			ReceiveItemID: "receive-a",
		},
		{
			RequestClass:  0,
			CappedSince:   100,
			RequestItemID: "item-b",
			CounterpartID: "member-a",
			ReceiveItemID: "receive-a",
		},
		{
			RequestClass:  0,
			CappedSince:   100,
			RequestItemID: "item-a",
			CounterpartID: "member-b",
			ReceiveItemID: "receive-a",
		},
		{
			RequestClass:  0,
			CappedSince:   100,
			RequestItemID: "item-a",
			CounterpartID: "member-a",
			ReceiveItemID: "receive-b",
		},
	}
	for _, later := range tests {
		if !matchPositionLess(base, later) {
			t.Fatalf("position did not sort before later tuple: %#v", later)
		}
	}
}
