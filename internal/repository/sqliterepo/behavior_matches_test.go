package sqliterepo_test

import (
	"context"
	"testing"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

func TestSQLiteSameAndCrossCatalogMatches(t *testing.T) {
	store := preparedBehaviorStore(t)
	ctx := context.Background()
	for _, write := range fixtureWrites() {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry(%s): %v", write.ItemID, err)
		}
	}
	for _, write := range counterpartWrites() {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry counterpart(%s): %v", write.ItemID, err)
		}
	}
	for _, write := range oneWayWrites() {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry one-way(%s): %v", write.ItemID, err)
		}
	}
	for _, write := range unfulfillableWrites() {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry unfulfillable(%s): %v", write.ItemID, err)
		}
	}

	rankingAt := time.Now().UTC().Truncate(time.Second)
	actionable := collectMatchPages(t, store, repository.MatchQuery{
		UserID:    currentUserID,
		Readiness: repository.MatchReadinessActionable,
		RankingAt: rankingAt,
		Limit:     1,
	})
	if len(actionable) != 3 {
		t.Fatalf(
			"actionable matches = %d, want 3: %#v",
			len(actionable),
			actionable,
		)
	}
	if actionable[len(actionable)-1].TheyRequest != nil {
		t.Fatalf("genuine giveaway did not sort after requests: %#v", actionable)
	}
	counterpart := matchForCounterpart(actionable, counterpartUserID)
	if counterpart == nil ||
		counterpart.TheyRequest == nil ||
		!counterpart.TheyRequest.Fulfillable ||
		counterpart.TheyRequest.Quantity != 1 ||
		counterpart.YouReceive.Quantity != 2 {
		t.Fatalf(
			"independent quantities = %#v, want request 1 and receive 2",
			counterpart,
		)
	}
	oneWay := matchForCounterpart(actionable, "member-without-matches")
	if oneWay == nil ||
		oneWay.TheyRequest != nil ||
		oneWay.YouReceive.CatalogItem.Item.ID != alphaReceiveID {
		t.Fatalf("one-way possible swap = %#v", oneWay)
	}
	unfulfillableFeed := collectMatchPages(t, store, repository.MatchQuery{
		UserID:    currentUserID,
		Readiness: repository.MatchReadinessUnfulfillable,
		RankingAt: rankingAt,
		Limit:     1,
	})
	if len(unfulfillableFeed) != 1 {
		t.Fatalf(
			"unfulfillable matches = %d, want 1: %#v",
			len(unfulfillableFeed),
			unfulfillableFeed,
		)
	}
	unfulfillable := matchForCounterpart(
		unfulfillableFeed,
		"member-unfulfillable",
	)
	if unfulfillable == nil ||
		unfulfillable.TheyRequest == nil ||
		unfulfillable.TheyRequest.Fulfillable ||
		unfulfillable.TheyRequest.Quantity != 0 ||
		unfulfillable.TheyRequest.CatalogItem.Item.ID != alphaRetiredID ||
		unfulfillable.YouReceive.CatalogItem.Item.ID != alphaReceiveID {
		t.Fatalf("unfulfillable counterpart request = %#v", unfulfillable)
	}

	same, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID: currentUserID,
		Scope:  repository.MatchScopeSameCatalog,
	})
	if err != nil {
		t.Fatalf("FindMatches same catalog: %v", err)
	}
	if len(same.Matches) != 2 {
		t.Fatalf("same-catalog matches = %d, want 2", len(same.Matches))
	}
	for _, match := range same.Matches {
		if match.TheyRequest == nil || !match.TheyRequest.Fulfillable {
			t.Fatalf("same-catalog filter returned one-way lead: %#v", match)
		}
		if match.TheyRequest.CatalogItem.Item.CatalogID !=
			match.YouReceive.CatalogItem.Item.CatalogID {
			t.Fatalf("same-catalog query returned cross match: %#v", match)
		}
	}

	cross, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID: currentUserID,
		Scope:  repository.MatchScopeCrossCatalog,
	})
	if err != nil {
		t.Fatalf("FindMatches cross catalog: %v", err)
	}
	if len(cross.Matches) != 2 {
		t.Fatalf("cross-catalog matches = %d, want 2", len(cross.Matches))
	}
	for _, match := range cross.Matches {
		if match.TheyRequest == nil || !match.TheyRequest.Fulfillable {
			t.Fatalf("cross-catalog filter returned one-way lead: %#v", match)
		}
		if match.TheyRequest.CatalogItem.Item.CatalogID ==
			match.YouReceive.CatalogItem.Item.CatalogID {
			t.Fatalf("cross-catalog query returned same match: %#v", match)
		}
	}

	filtered, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID:           currentUserID,
		GiveCatalogID:    alphaCatalogID,
		ReceiveCatalogID: betaCatalogID,
	})
	if err != nil {
		t.Fatalf("FindMatches catalog filters: %v", err)
	}
	if len(filtered.Matches) != 1 ||
		filtered.Matches[0].TheyRequest == nil ||
		!filtered.Matches[0].TheyRequest.Fulfillable ||
		filtered.Matches[0].TheyRequest.CatalogItem.Item.ID != alphaGiveID ||
		filtered.Matches[0].YouReceive.CatalogItem.Item.ID != betaReceiveID {
		t.Fatalf("filtered matches = %#v", filtered.Matches)
	}
	if _, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: "member-unfulfillable",
		ItemID: betaGiveID,
		Wanted: 2,
	}); err != nil {
		t.Fatalf("SaveCollectionEntry fulfillable preference: %v", err)
	}
	withPreferredRequest, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID:    currentUserID,
		Readiness: repository.MatchReadinessActionable,
		RankingAt: rankingAt,
	})
	if err != nil {
		t.Fatalf("FindMatches fulfillable preference: %v", err)
	}
	preferred := matchForCounterpart(
		withPreferredRequest.Matches,
		"member-unfulfillable",
	)
	if preferred == nil ||
		preferred.TheyRequest == nil ||
		!preferred.TheyRequest.Fulfillable ||
		preferred.TheyRequest.Quantity != 2 ||
		preferred.TheyRequest.CatalogItem.Item.ID != betaGiveID {
		t.Fatalf("preferred fulfillable counterpart request = %#v", preferred)
	}
	afterPreference, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID:    currentUserID,
		Readiness: repository.MatchReadinessUnfulfillable,
		RankingAt: rankingAt,
	})
	if err != nil {
		t.Fatalf("FindMatches unfulfillable after preference: %v", err)
	}
	if matchForCounterpart(
		afterPreference.Matches,
		"member-unfulfillable",
	) != nil {
		t.Fatalf("lead duplicated across readiness feeds: %#v", afterPreference.Matches)
	}
	_, err = store.FindMatches(ctx, repository.MatchQuery{
		UserID:    currentUserID,
		Readiness: repository.MatchReadinessUnfulfillable,
		Scope:     repository.MatchScopeCrossCatalog,
	})
	assertFailure(t, err, repository.ErrInvalidArgument, repository.FailureInvalidArgument)
	_, err = store.FindMatches(ctx, repository.MatchQuery{
		UserID: currentUserID,
		Limit:  repository.MaxMatchPageSize + 1,
	})
	assertFailure(t, err, repository.ErrInvalidArgument, repository.FailureInvalidArgument)
}
