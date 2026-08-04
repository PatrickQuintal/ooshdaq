package sqliterepo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

func TestSQLiteCollectionPagesSummaryAndSparseWrites(t *testing.T) {
	store := preparedBehaviorStore(t)
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
	if page.Catalog.MemberSummary != (repository.MemberSummary{
		AvailableDesigns: 1,
		WantedDesigns:    1,
	}) {
		t.Fatalf("catalog member summary = %#v", page.Catalog.MemberSummary)
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
	if summary.PossibleSwapCount != 4 ||
		summary.MemberSummary != (repository.MemberSummary{
			AvailableDesigns: 2,
			WantedDesigns:    3,
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
