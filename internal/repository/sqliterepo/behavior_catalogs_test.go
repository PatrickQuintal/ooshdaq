package sqliterepo_test

import (
	"context"
	"fmt"
	"testing"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

func TestSQLiteManifestImportAndCatalogPages(t *testing.T) {
	store := openBehaviorStore(t)
	ctx := context.Background()
	manifests := fixtureManifests()

	importFixtureManifest(t, store, manifests[0], digest("1"))
	importFixtureManifest(t, store, manifests[0], digest("1"))
	err := store.ImportManifests(ctx, []catalogdata.LoadedManifest{{
		Path:     "testdata/" + manifests[0].Catalog.ID,
		Manifest: manifests[0],
		SHA256:   digest("f"),
	}})
	assertFailure(t, err, repository.ErrConflict, repository.FailureConflict)

	for index, manifest := range manifests[1:] {
		importFixtureManifest(t, store, manifest, digest(fmt.Sprint(index+2)))
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
