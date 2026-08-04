package sqliterepo

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

func TestUnfulfillableMatchRankingUsesCappedWaitAndStrictStaleness(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, Config{
		Path: filepath.Join(t.TempDir(), "match-ranking.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	loaded, err := catalogdata.LoadEmbeddedDemoManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedDemoManifests: %v", err)
	}
	if err := store.ImportManifests(ctx, loaded); err != nil {
		t.Fatalf("ImportManifests: %v", err)
	}

	rankingAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return rankingAt.Add(-24 * time.Hour) }
	current := domain.User{
		ID:          "rank-current",
		DisplayName: "Rank Current",
	}
	if err := store.UpsertUser(ctx, current); err != nil {
		t.Fatalf("UpsertUser current: %v", err)
	}
	receiveID := loaded[0].Manifest.Items[0].ID
	requestID := loaded[0].Manifest.Items[1].ID
	if _, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: current.ID,
		ItemID: receiveID,
		Wanted: 1,
	}); err != nil {
		t.Fatalf("SaveCollectionEntry current: %v", err)
	}

	counterparts := []struct {
		id          string
		wantedSince time.Time
		lastSeen    any
	}{
		{
			id:          "rank-01-maintained",
			wantedSince: rankingAt.Add(-120 * 24 * time.Hour),
			lastSeen:    rankingAt.Add(-24 * time.Hour).Format(time.RFC3339Nano),
		},
		{
			id:          "rank-02-exact-90",
			wantedSince: rankingAt.Add(-90 * 24 * time.Hour),
			lastSeen:    nil,
		},
		{
			id:          "rank-03-stale",
			wantedSince: rankingAt.Add(-120 * 24 * time.Hour),
			lastSeen:    nil,
		},
		{
			id:          "rank-04-recent",
			wantedSince: rankingAt.Add(-30 * 24 * time.Hour),
			lastSeen:    nil,
		},
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer transaction.Rollback()
	for _, counterpart := range counterparts {
		timestamp := counterpart.wantedSince.Format(time.RFC3339Nano)
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO users(
				id, display_name, created_at, updated_at, last_seen_at
			 ) VALUES (?, ?, ?, ?, ?)`,
			counterpart.id,
			counterpart.id,
			timestamp,
			timestamp,
			counterpart.lastSeen,
		); err != nil {
			t.Fatalf("insert user %q: %v", counterpart.id, err)
		}
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO collection_entries(
				user_id, item_id, owned, available, available_since_at,
				wanted, wanted_since_at, updated_at
			 ) VALUES (?, ?, 1, 1, ?, 0, NULL, ?)`,
			counterpart.id,
			receiveID,
			timestamp,
			timestamp,
		); err != nil {
			t.Fatalf("insert offer %q: %v", counterpart.id, err)
		}
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO collection_entries(
				user_id, item_id, owned, available, available_since_at,
				wanted, wanted_since_at, updated_at
			 ) VALUES (?, ?, 0, 0, NULL, 1, ?, ?)`,
			counterpart.id,
			requestID,
			timestamp,
			timestamp,
		); err != nil {
			t.Fatalf("insert want %q: %v", counterpart.id, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	query := repository.MatchQuery{
		UserID:    current.ID,
		Readiness: repository.MatchReadinessUnfulfillable,
		RankingAt: rankingAt,
		Limit:     2,
	}
	var got []string
	for {
		page, err := store.FindMatches(ctx, query)
		if err != nil {
			t.Fatalf("FindMatches: %v", err)
		}
		if !page.RankingAt.Equal(rankingAt) {
			t.Fatalf("page ranking time = %s, want %s", page.RankingAt, rankingAt)
		}
		for _, match := range page.Matches {
			if match.TheyRequest == nil ||
				match.TheyRequest.Fulfillable ||
				match.TheyRequest.CatalogItem.Item.ID != requestID {
				t.Fatalf("match request = %#v", match.TheyRequest)
			}
			got = append(got, match.OtherUser.ID)
		}
		if page.Next == nil {
			break
		}
		query.After = page.Next
	}
	want := []string{
		"rank-01-maintained",
		"rank-02-exact-90",
		"rank-03-stale",
		"rank-04-recent",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ranked counterpart IDs = %q, want %q", got, want)
	}
}
