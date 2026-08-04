package sqliterepo_test

import (
	"context"
	"errors"
	"testing"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

func TestSQLiteHealthUsersAndFailures(t *testing.T) {
	store := openBehaviorStore(t)
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

func TestSQLiteCancellation(t *testing.T) {
	store := preparedBehaviorStore(t)
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
	manifest := fixtureManifests()[0]
	if err := store.ImportManifests(ctx, []catalogdata.LoadedManifest{{
		Path:     "testdata/" + manifest.Catalog.ID,
		Manifest: manifest,
		SHA256:   digest("a"),
	}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ImportManifests cancellation = %v", err)
	}
}
