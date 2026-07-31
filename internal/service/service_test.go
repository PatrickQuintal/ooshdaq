package service

import (
	"context"
	"errors"
	"testing"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func preparedService(t *testing.T) (*Service, *repository.MemoryStore, []catalogdata.LoadedManifest) {
	t.Helper()
	store := repository.NewMemoryStore()
	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	for _, candidate := range loaded {
		if _, err := store.ImportManifest(context.Background(), repository.ManifestImport{
			Manifest: candidate.Manifest,
			SHA256:   candidate.SHA256,
		}); err != nil {
			t.Fatalf("ImportManifest: %v", err)
		}
	}
	for _, user := range []domain.User{
		{ID: "member-current", DisplayName: "Current Member"},
		{ID: "member-other", DisplayName: "Other Member"},
	} {
		if err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatalf("UpsertUser: %v", err)
		}
	}
	service, err := New(Dependencies{
		Health:      store,
		Users:       store,
		Activity:    store,
		Catalogs:    store,
		Collections: store,
		Writes:      store,
		Matches:     store,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return service, store, loaded
}

func TestServiceUsesNarrowCatalogCollectionAndMatchPorts(t *testing.T) {
	t.Parallel()

	service, store, loaded := preparedService(t)
	ctx := context.Background()
	if err := service.Ready(ctx); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if err := service.TouchUserActivity(ctx, "member-current"); err != nil {
		t.Fatalf("TouchUserActivity: %v", err)
	}
	if err := service.TouchUserActivity(ctx, "missing-member"); !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf("TouchUserActivity missing error = %v", err)
	}
	catalogs, err := service.Catalogs(ctx, repository.CatalogQuery{
		UserID:            "member-current",
		PublicationStatus: repository.PublicationFilterAll,
	})
	if err != nil {
		t.Fatalf("Catalogs: %v", err)
	}
	if len(catalogs.Catalogs) != 5 {
		t.Fatalf("catalog count = %d, want 5", len(catalogs.Catalogs))
	}

	give := loaded[0].Manifest.Items[0]
	receive := loaded[1].Manifest.Items[0]
	for _, write := range []repository.CollectionWrite{
		{UserID: "member-current", ItemID: give.ID, Owned: 2, Available: 1},
		{UserID: "member-current", ItemID: receive.ID, Wanted: 1},
		{UserID: "member-other", ItemID: give.ID, Wanted: 1},
		{UserID: "member-other", ItemID: receive.ID, Owned: 1, Available: 1},
	} {
		if _, err := store.SaveCollectionEntry(ctx, write); err != nil {
			t.Fatalf("SaveCollectionEntry: %v", err)
		}
	}
	collection, err := service.Collection(ctx, repository.CollectionQuery{
		UserID:    "member-current",
		CatalogID: give.CatalogID,
	})
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if len(collection.Entries) == 0 {
		t.Fatal("Collection returned no entries")
	}
	summary, err := service.CollectionSummary(ctx, "member-current")
	if err != nil {
		t.Fatalf("CollectionSummary: %v", err)
	}
	if summary.MemberSummary.AvailableQuantity != 1 ||
		summary.MemberSummary.WantedQuantity != 1 ||
		summary.PossibleSwapCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	matches, err := service.Matches(ctx, repository.MatchQuery{UserID: "member-current"})
	if err != nil {
		t.Fatalf("Matches: %v", err)
	}
	if len(matches.Matches) != 1 ||
		matches.Matches[0].TheyRequest == nil ||
		!matches.Matches[0].TheyRequest.Fulfillable ||
		matches.Matches[0].TheyRequest.CatalogItem.Item.ID != give.ID ||
		matches.Matches[0].YouReceive.CatalogItem.Item.ID != receive.ID {
		t.Fatalf("matches = %#v", matches.Matches)
	}
}

func TestEnsureUserAndUpdateCollectionPreserveValidationAndFailures(t *testing.T) {
	t.Parallel()

	service, _, loaded := preparedService(t)
	ctx := context.Background()
	updated, err := service.EnsureUser(ctx, "member-current", "Renamed")
	if err != nil {
		t.Fatalf("EnsureUser existing: %v", err)
	}
	if updated.DisplayName != "Renamed" {
		t.Fatalf("updated display name = %q", updated.DisplayName)
	}
	created, err := service.EnsureUser(ctx, "new-member", "")
	if err != nil {
		t.Fatalf("EnsureUser new: %v", err)
	}
	if created.DisplayName != "$OOSHDAQ user" {
		t.Fatalf("default display name = %q", created.DisplayName)
	}

	itemID := loaded[0].Manifest.Items[0].ID
	if _, err := service.UpdateCollection(ctx, "member-current", itemID, 1, 2, 0); err == nil {
		t.Fatal("UpdateCollection accepted available above owned")
	}
	if _, err := service.UpdateCollection(ctx, "member-current", itemID, 3, 2, 1); err == nil {
		t.Fatal("UpdateCollection accepted wanted while available")
	}
	saved, err := service.UpdateCollection(ctx, "member-current", itemID, 3, 0, 1)
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if saved.Owned != 3 || saved.Available != 0 || saved.Wanted != 1 {
		t.Fatalf("saved record = %#v", saved)
	}
	_, err = service.UpdateCollection(ctx, "member-current", "missing-item", 0, 0, 0)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing item error = %v, want ErrNotFound", err)
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if _, err := New(Dependencies{}); err == nil {
		t.Fatal("New accepted empty dependencies")
	}
}
