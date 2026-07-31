// Package conformance contains backend-neutral repository contract tests.
package conformance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

// Store is the complete set of independently consumable ports exercised by
// this suite. Application code should continue to accept only the narrow port
// it needs.
type Store interface {
	repository.HealthChecker
	repository.UserStore
	repository.CatalogReader
	repository.CollectionReader
	repository.CollectionWriter
	repository.MatchFinder
	repository.ManifestImporter
}

// Factory returns a fresh isolated store. Implementations may register cleanup
// with t.
type Factory func(t *testing.T) Store

// Run executes the behavior shared by memory and durable adapters.
func Run(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("health users and failures", func(t *testing.T) {
		testHealthUsersAndFailures(t, factory(t))
	})
	t.Run("manifest import and catalog pages", func(t *testing.T) {
		testManifestImportAndCatalogPages(t, factory(t))
	})
	t.Run("collection pages summary and sparse writes", func(t *testing.T) {
		testCollectionBehavior(t, preparedStore(t, factory(t)))
	})
	t.Run("same and cross catalog matches", func(t *testing.T) {
		testMatchBehavior(t, preparedStore(t, factory(t)))
	})
	t.Run("cancellation", func(t *testing.T) {
		testCancellation(t, preparedStore(t, factory(t)))
	})
}

func testHealthUsersAndFailures(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	user := domain.User{ID: "member-current", DisplayName: "Current Member"}
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatalf("UpsertUser create: %v", err)
	}
	got, err := store.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got != user {
		t.Fatalf("GetUser = %#v, want %#v", got, user)
	}
	user.DisplayName = "Renamed Member"
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatalf("UpsertUser update: %v", err)
	}
	got, err = store.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser after update: %v", err)
	}
	if got != user {
		t.Fatalf("updated user = %#v, want %#v", got, user)
	}

	_, err = store.GetUser(ctx, "missing-member")
	assertFailure(t, err, repository.ErrNotFound, repository.FailureNotFound)
}

func testManifestImportAndCatalogPages(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	manifests := fixtureManifests()

	first := importManifest(t, store, manifests[0], digest("1"))
	if !first.Applied {
		t.Fatal("first manifest import was reported as a no-op")
	}
	again := importManifest(t, store, manifests[0], digest("1"))
	if again.Applied {
		t.Fatal("identical manifest import was applied twice")
	}
	_, err := store.ImportManifest(ctx, repository.ManifestImport{
		Manifest: manifests[0],
		SHA256:   digest("f"),
	})
	assertFailure(t, err, repository.ErrConflict, repository.FailureConflict)

	for index, manifest := range manifests[1:] {
		importManifest(t, store, manifest, digest(fmt.Sprint(index+2)))
	}
	upsertFixtureUsers(t, store)

	page, err := store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID:            currentUserID,
		PublicationStatus: repository.PublicationFilterAll,
		Limit:             1,
	})
	if err != nil {
		t.Fatalf("ListCatalogs page one: %v", err)
	}
	if len(page.Catalogs) != 1 || page.Catalogs[0].Catalog.ID != alphaCatalogID {
		t.Fatalf("first catalog page = %#v, want alpha catalog", page.Catalogs)
	}
	if page.Next == nil ||
		*page.Next != (repository.CatalogPosition{SortOrder: 10, CatalogID: alphaCatalogID}) {
		t.Fatalf("first catalog next position = %#v", page.Next)
	}
	page, err = store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID:            currentUserID,
		PublicationStatus: repository.PublicationFilterAll,
		Limit:             1,
		After:             page.Next,
	})
	if err != nil {
		t.Fatalf("ListCatalogs page two: %v", err)
	}
	if len(page.Catalogs) != 1 || page.Catalogs[0].Catalog.ID != betaCatalogID {
		t.Fatalf("second catalog page = %#v, want beta catalog", page.Catalogs)
	}

	active, err := store.ListCatalogs(ctx, repository.CatalogQuery{UserID: currentUserID})
	if err != nil {
		t.Fatalf("ListCatalogs defaults: %v", err)
	}
	if active.Limit != repository.DefaultCatalogPageSize || len(active.Catalogs) != 2 {
		t.Fatalf("default active page = %#v", active)
	}
	if active.Catalogs[0].ItemCount != 3 {
		t.Fatalf("alpha published item count = %d, want 3", active.Catalogs[0].ItemCount)
	}
	if got := active.Catalogs[0].UniverseOptions; !equalStrings(got, []string{"Earth", "Sky"}) {
		t.Fatalf("alpha universes = %q, want [Earth Sky]", got)
	}

	searched, err := store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID: currentUserID,
		Search: "  BETA  ",
	})
	if err != nil {
		t.Fatalf("ListCatalogs search: %v", err)
	}
	if len(searched.Catalogs) != 1 || searched.Catalogs[0].Catalog.ID != betaCatalogID {
		t.Fatalf("search page = %#v, want beta catalog", searched.Catalogs)
	}

	for _, test := range []struct {
		name      string
		search    string
		catalogID string
	}{
		{name: "item name", search: "  ZERO  ", catalogID: betaCatalogID},
		{name: "item universe", search: "Sky", catalogID: alphaCatalogID},
		{name: "item variant", search: "glitter", catalogID: betaCatalogID},
		{name: "retired item", search: "Retired", catalogID: alphaCatalogID},
	} {
		test := test
		t.Run("catalog search by "+test.name, func(t *testing.T) {
			found, err := store.ListCatalogs(ctx, repository.CatalogQuery{
				UserID: currentUserID,
				Search: test.search,
			})
			if err != nil {
				t.Fatalf("ListCatalogs: %v", err)
			}
			if len(found.Catalogs) != 1 ||
				found.Catalogs[0].Catalog.ID != test.catalogID {
				t.Fatalf(
					"catalogs = %#v, want only %s",
					found.Catalogs,
					test.catalogID,
				)
			}
		})
	}

	draftItemSearch, err := store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID: currentUserID,
		Search: "Hidden",
	})
	if err != nil {
		t.Fatalf("ListCatalogs draft item search: %v", err)
	}
	if len(draftItemSearch.Catalogs) != 0 {
		t.Fatalf("draft item search exposed catalogs: %#v", draftItemSearch.Catalogs)
	}

	itemSearchPage, err := store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID: currentUserID,
		Search: "Give",
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("ListCatalogs item search page one: %v", err)
	}
	if len(itemSearchPage.Catalogs) != 1 ||
		itemSearchPage.Catalogs[0].Catalog.ID != alphaCatalogID ||
		itemSearchPage.Next == nil {
		t.Fatalf("item search page one = %#v", itemSearchPage)
	}
	itemSearchPage, err = store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID: currentUserID,
		Search: "Give",
		Limit:  1,
		After:  itemSearchPage.Next,
	})
	if err != nil {
		t.Fatalf("ListCatalogs item search page two: %v", err)
	}
	if len(itemSearchPage.Catalogs) != 1 ||
		itemSearchPage.Catalogs[0].Catalog.ID != betaCatalogID ||
		itemSearchPage.Next != nil {
		t.Fatalf("item search page two = %#v", itemSearchPage)
	}

	_, err = store.ListCatalogs(ctx, repository.CatalogQuery{
		UserID: currentUserID,
		Limit:  repository.MaxCatalogPageSize + 1,
	})
	assertFailure(t, err, repository.ErrInvalidArgument, repository.FailureInvalidArgument)

	// Returned slices and retirement pointers must not alias repository state.
	active.Catalogs[0].UniverseOptions[0] = "mutated"
	collection, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
	})
	if err != nil {
		t.Fatalf("ListCollectionPage for copy check: %v", err)
	}
	collection.Entries[2].CatalogItem.Item.Retirement.Reason = "mutated"
	againCollection, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
	})
	if err != nil {
		t.Fatalf("ListCollectionPage after mutation: %v", err)
	}
	if againCollection.Catalog.UniverseOptions[0] != "Earth" ||
		againCollection.Entries[2].CatalogItem.Item.Retirement.Reason == "mutated" {
		t.Fatal("repository returned mutable storage")
	}
}

func testCollectionBehavior(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()

	for _, write := range fixtureWrites() {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry(%s): %v", write.ItemID, err)
		}
	}

	page, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("ListCollectionPage one: %v", err)
	}
	if len(page.Entries) != 2 ||
		page.Entries[0].CatalogItem.Item.ID != alphaGiveID ||
		page.Entries[1].CatalogItem.Item.ID != alphaReceiveID {
		t.Fatalf("collection page one = %#v", page.Entries)
	}
	if page.Next == nil ||
		*page.Next != (repository.CollectionPosition{ItemNumber: 2, ItemID: alphaReceiveID}) {
		t.Fatalf("collection next position = %#v", page.Next)
	}
	page, err = store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
		Limit:     2,
		After:     page.Next,
	})
	if err != nil {
		t.Fatalf("ListCollectionPage two: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].CatalogItem.Item.ID != alphaRetiredID {
		t.Fatalf("collection page two = %#v", page.Entries)
	}

	wanted, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
		State:     repository.CollectionStateWanted,
	})
	if err != nil {
		t.Fatalf("wanted collection page: %v", err)
	}
	if len(wanted.Entries) != 1 || wanted.Entries[0].CatalogItem.Item.ID != alphaReceiveID {
		t.Fatalf("wanted collection page = %#v", wanted.Entries)
	}
	_, err = store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
		Universe:  "Not Reviewed",
	})
	assertFailure(t, err, repository.ErrInvalidArgument, repository.FailureInvalidArgument)
	_, err = store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
		Limit:     repository.MaxCollectionPageSize + 1,
	})
	assertFailure(t, err, repository.ErrInvalidArgument, repository.FailureInvalidArgument)

	for _, write := range possibleSwapOfferWrites() {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry possible offer(%s): %v", write.ItemID, err)
		}
	}
	summary, err := store.GetCollectionSummary(ctx, currentUserID)
	if err != nil {
		t.Fatalf("GetCollectionSummary: %v", err)
	}
	if summary.ActiveCatalogCount != 2 ||
		summary.ActiveItemCount != 5 ||
		summary.CatalogsStarted != 3 ||
		summary.PossibleSwapCount != 4 ||
		summary.MemberSummary != (repository.MemberSummary{
			OwnedQuantity:     7,
			OwnedDesigns:      2,
			AvailableQuantity: 7,
			AvailableDesigns:  2,
			WantedQuantity:    6,
			WantedDesigns:     3,
		}) {
		t.Fatalf("collection summary = %#v", summary)
	}

	saved, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: currentUserID,
		ItemID: betaZeroID,
		Owned:  1,
		Wanted: 1,
	})
	if err != nil {
		t.Fatalf("save owned-and-wanted sparse entry: %v", err)
	}
	if saved.Owned != 1 || saved.Available != 0 || saved.Wanted != 1 {
		t.Fatalf("saved owned-and-wanted entry = %#v", saved)
	}
	_, err = store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID:    currentUserID,
		ItemID:    betaZeroID,
		Owned:     1,
		Available: 1,
		Wanted:    1,
	})
	assertFailure(t, err, repository.ErrInvalidArgument, repository.FailureInvalidArgument)
	var validationError domain.ValidationError
	if !errors.As(err, &validationError) || validationError.Field != "wanted" {
		t.Fatalf("exclusive-state error = %v, want wanted validation error", err)
	}
	saved, err = store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: currentUserID,
		ItemID: betaZeroID,
		Wanted: 1,
	})
	if err != nil {
		t.Fatalf("save wanted-only sparse entry: %v", err)
	}
	if saved.Owned != 0 || saved.Wanted != 1 {
		t.Fatalf("saved entry = %#v", saved)
	}
	zero, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: currentUserID,
		ItemID: betaZeroID,
	})
	if err != nil {
		t.Fatalf("delete sparse entry: %v", err)
	}
	if zero.Owned != 0 || zero.Available != 0 || zero.Wanted != 0 {
		t.Fatalf("canonical zero entry = %#v", zero)
	}
	owned, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: betaCatalogID,
		State:     repository.CollectionStateOwned,
	})
	if err != nil {
		t.Fatalf("owned page after sparse delete: %v", err)
	}
	for _, entry := range owned.Entries {
		if entry.CatalogItem.Item.ID == betaZeroID {
			t.Fatal("all-zero sparse row remained owned")
		}
	}
}

func testMatchBehavior(t *testing.T, store Store) {
	t.Helper()
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

func testCancellation(t *testing.T, store Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Ping(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ping cancellation = %v", err)
	}
	if _, err := store.GetUser(ctx, currentUserID); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetUser cancellation = %v", err)
	}
	if err := store.UpsertUser(
		ctx,
		domain.User{ID: "cancelled", DisplayName: "Cancelled"},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpsertUser cancellation = %v", err)
	}
	if _, err := store.ListCatalogs(
		ctx,
		repository.CatalogQuery{UserID: currentUserID},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListCatalogs cancellation = %v", err)
	}
	if _, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
		UserID:    currentUserID,
		CatalogID: alphaCatalogID,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListCollectionPage cancellation = %v", err)
	}
	if _, err := store.GetCollectionSummary(ctx, currentUserID); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetCollectionSummary cancellation = %v", err)
	}
	if _, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: currentUserID,
		ItemID: alphaGiveID,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveCollectionEntry cancellation = %v", err)
	}
	if _, err := store.FindMatches(
		ctx,
		repository.MatchQuery{UserID: currentUserID},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindMatches cancellation = %v", err)
	}
	if _, err := store.ImportManifest(ctx, repository.ManifestImport{
		Manifest: fixtureManifests()[0],
		SHA256:   digest("a"),
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ImportManifest cancellation = %v", err)
	}
}

const (
	currentUserID     = "member-current"
	counterpartUserID = "member-counterpart"
	alphaCatalogID    = "alpha-catalog"
	betaCatalogID     = "beta-catalog"
	retiredCatalogID  = "retired-catalog"
	alphaGiveID       = "alpha-catalog-001-give"
	alphaReceiveID    = "alpha-catalog-002-receive"
	alphaRetiredID    = "alpha-catalog-003-retired"
	betaGiveID        = "beta-catalog-001-give"
	betaReceiveID     = "beta-catalog-002-receive"
	betaZeroID        = "beta-catalog-003-zero"
	retiredItemID     = "retired-catalog-001-kept"
)

func preparedStore(t *testing.T, store Store) Store {
	t.Helper()
	for index, manifest := range fixtureManifests() {
		importManifest(t, store, manifest, digest(fmt.Sprint(index+1)))
	}
	upsertFixtureUsers(t, store)
	return store
}

func upsertFixtureUsers(t *testing.T, store Store) {
	t.Helper()
	for _, user := range []domain.User{
		{ID: currentUserID, DisplayName: "Current Member"},
		{ID: counterpartUserID, DisplayName: "Counterpart Member"},
		{ID: "member-unfulfillable", DisplayName: "Unfulfillable Request"},
		{ID: "member-without-matches", DisplayName: "No Matches"},
	} {
		if err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatalf("UpsertUser(%s): %v", user.ID, err)
		}
	}
}

func importManifest(
	t *testing.T,
	store Store,
	manifest domain.CatalogManifest,
	sha256 string,
) repository.ManifestImportResult {
	t.Helper()
	result, err := store.ImportManifest(context.Background(), repository.ManifestImport{
		Manifest: manifest,
		SHA256:   sha256,
	})
	if err != nil {
		t.Fatalf("ImportManifest(%s): %v", manifest.Catalog.ID, err)
	}
	return result
}

func fixtureWrites() []repository.CollectionWrite {
	return []repository.CollectionWrite{
		{
			UserID: currentUserID,
			ItemID: alphaGiveID,
			Owned:  3, Available: 3,
		},
		{
			UserID: currentUserID,
			ItemID: alphaReceiveID,
			Wanted: 2,
		},
		{
			UserID: currentUserID,
			ItemID: betaGiveID,
			Owned:  4, Available: 4,
		},
		{
			UserID: currentUserID,
			ItemID: betaReceiveID,
			Wanted: 3,
		},
		{
			UserID: currentUserID,
			ItemID: retiredItemID,
			Wanted: 1,
		},
	}
}

func counterpartWrites() []repository.CollectionWrite {
	return []repository.CollectionWrite{
		{
			UserID: counterpartUserID,
			ItemID: alphaGiveID,
			Wanted: 1,
		},
		{
			UserID: counterpartUserID,
			ItemID: alphaReceiveID,
			Owned:  4, Available: 4,
		},
		{
			UserID: counterpartUserID,
			ItemID: betaGiveID,
			Wanted: 2,
		},
		{
			UserID: counterpartUserID,
			ItemID: betaReceiveID,
			Owned:  2, Available: 2,
		},
	}
}

func possibleSwapOfferWrites() []repository.CollectionWrite {
	writes := append(counterpartOfferWrites(), oneWayWrites()...)
	return append(writes, unfulfillableWrites()...)
}

func counterpartOfferWrites() []repository.CollectionWrite {
	return []repository.CollectionWrite{
		{
			UserID: counterpartUserID,
			ItemID: alphaReceiveID,
			Owned:  4, Available: 4,
		},
		{
			UserID: counterpartUserID,
			ItemID: betaReceiveID,
			Owned:  2, Available: 2,
		},
	}
}

func oneWayWrites() []repository.CollectionWrite {
	return []repository.CollectionWrite{
		{
			UserID: "member-without-matches",
			ItemID: alphaReceiveID,
			Owned:  2, Available: 2,
		},
	}
}

func unfulfillableWrites() []repository.CollectionWrite {
	return []repository.CollectionWrite{
		{
			UserID: "member-unfulfillable",
			ItemID: alphaReceiveID,
			Owned:  1, Available: 1,
		},
		{
			UserID: "member-unfulfillable",
			ItemID: alphaRetiredID,
			Wanted: 1,
		},
		{
			UserID: "member-unfulfillable",
			ItemID: betaZeroID,
			Wanted: 2,
		},
	}
}

func fixtureManifests() []domain.CatalogManifest {
	return []domain.CatalogManifest{
		manifest(
			"beta-brand",
			"Beta Brand",
			betaCatalogID,
			"Beta Catalog",
			domain.PublicationActive,
			10,
			item(betaCatalogID, betaGiveID, 1, "Beta Give", "Water", domain.PublicationActive),
			item(betaCatalogID, betaReceiveID, 2, "Beta Receive", "Water", domain.PublicationActive),
			itemWithVariant(
				betaCatalogID,
				betaZeroID,
				3,
				"Beta Zero",
				"Stone",
				"Glitter finish",
				domain.PublicationActive,
			),
		),
		manifest(
			"alpha-brand",
			"Alpha Brand",
			alphaCatalogID,
			"Alpha Catalog",
			domain.PublicationActive,
			10,
			item(alphaCatalogID, alphaGiveID, 1, "Alpha Give", "Sky", domain.PublicationActive),
			item(alphaCatalogID, alphaReceiveID, 2, "Alpha Receive", "Earth", domain.PublicationActive),
			retiredItem(alphaCatalogID, alphaRetiredID, 3, "Alpha Retired", "Earth"),
			item(alphaCatalogID, "alpha-catalog-004-draft", 4, "Alpha Draft", "Hidden", domain.PublicationDraft),
		),
		manifest(
			"alpha-brand",
			"Alpha Brand",
			retiredCatalogID,
			"Retired Catalog",
			domain.PublicationRetired,
			30,
			item(retiredCatalogID, retiredItemID, 1, "Retired Kept", "Archive", domain.PublicationActive),
		),
		manifest(
			"draft-brand",
			"Draft Brand",
			"draft-catalog",
			"Draft Catalog",
			domain.PublicationDraft,
			40,
			item("draft-catalog", "draft-catalog-001-hidden", 1, "Hidden", "Hidden", domain.PublicationDraft),
		),
	}
}

func manifest(
	brandID string,
	brandName string,
	catalogID string,
	catalogName string,
	status domain.PublicationStatus,
	sortOrder int,
	items ...domain.CatalogItem,
) domain.CatalogManifest {
	brandStatus := domain.PublicationActive
	if status == domain.PublicationDraft {
		brandStatus = domain.PublicationDraft
	}
	return domain.CatalogManifest{
		SchemaVersion:   domain.CatalogManifestSchemaVersion,
		ManifestVersion: 1,
		Brand: domain.Brand{
			ID:     brandID,
			Name:   brandName,
			Status: brandStatus,
		},
		Catalog: domain.Catalog{
			ID:        catalogID,
			BrandID:   brandID,
			Name:      catalogName,
			Edition:   "2026",
			Status:    status,
			SortOrder: sortOrder,
		},
		Items: items,
	}
}

func item(
	catalogID string,
	id string,
	number int,
	name string,
	universe string,
	status domain.PublicationStatus,
) domain.CatalogItem {
	return domain.CatalogItem{
		ID:        id,
		CatalogID: catalogID,
		Number:    number,
		Name:      name,
		Universe:  universe,
		Status:    status,
		Artwork: domain.Artwork{
			Placeholder: true,
			Rights: domain.ArtworkRights{
				Status: domain.RightsUnknown,
			},
			Provenance: domain.ArtworkProvenance{
				ReviewedAt:      "2026-07-30",
				ReviewReference: "repository-conformance",
				Transformation:  "none",
			},
		},
	}
}

func itemWithVariant(
	catalogID string,
	id string,
	number int,
	name string,
	universe string,
	variant string,
	status domain.PublicationStatus,
) domain.CatalogItem {
	result := item(catalogID, id, number, name, universe, status)
	result.Variant = variant
	return result
}

func retiredItem(
	catalogID string,
	id string,
	number int,
	name string,
	universe string,
) domain.CatalogItem {
	result := item(catalogID, id, number, name, universe, domain.PublicationRetired)
	result.Retirement = &domain.ItemRetirement{
		EffectiveDate: "2026-07-30",
		Reason:        "Retained by the repository conformance fixture.",
	}
	return result
}

func collectMatchPages(
	t *testing.T,
	store Store,
	query repository.MatchQuery,
) []repository.MatchRecord {
	t.Helper()
	var result []repository.MatchRecord
	for {
		page, err := store.FindMatches(context.Background(), query)
		if err != nil {
			t.Fatalf("FindMatches: %v", err)
		}
		result = append(result, page.Matches...)
		if page.Next == nil {
			return result
		}
		query.After = page.Next
	}
}

func matchForCounterpart(
	matches []repository.MatchRecord,
	counterpartID string,
) *repository.MatchRecord {
	for index := range matches {
		if matches[index].OtherUser.ID == counterpartID {
			return &matches[index]
		}
	}
	return nil
}

func assertFailure(
	t *testing.T,
	err error,
	sentinel error,
	kind repository.FailureKind,
) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want errors.Is(%v)", err, sentinel)
	}
	if got := repository.Classify(err); got != kind {
		t.Fatalf("failure class = %q, want %q", got, kind)
	}
}

func digest(character string) string {
	return strings.Repeat(character, 64)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
