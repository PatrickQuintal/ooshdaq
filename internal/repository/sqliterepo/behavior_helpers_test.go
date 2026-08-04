package sqliterepo_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	"github.com/PatrickQuintal/ooshdaq/internal/repository/sqliterepo"
)

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

func openBehaviorStore(t *testing.T) *sqliterepo.Store {
	t.Helper()
	store, err := sqliterepo.Open(context.Background(), sqliterepo.Config{
		Path: filepath.Join(t.TempDir(), "behavior.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open behavior store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close behavior store: %v", err)
		}
	})
	return store
}

func preparedBehaviorStore(t *testing.T) *sqliterepo.Store {
	t.Helper()
	store := openBehaviorStore(t)
	for index, manifest := range fixtureManifests() {
		importFixtureManifest(t, store, manifest, digest(fmt.Sprint(index+1)))
	}
	upsertFixtureUsers(t, store)
	return store
}

func upsertFixtureUsers(t *testing.T, store *sqliterepo.Store) {
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

func importFixtureManifest(
	t *testing.T,
	store *sqliterepo.Store,
	manifest domain.CatalogManifest,
	sha256 string,
) {
	t.Helper()
	err := store.ImportManifests(context.Background(), []catalogdata.LoadedManifest{{
		Path:     "testdata/" + manifest.Catalog.ID,
		Manifest: manifest,
		SHA256:   sha256,
	}})
	if err != nil {
		t.Fatalf("ImportManifests(%s): %v", manifest.Catalog.ID, err)
	}
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

func collectMatchPages(
	t *testing.T,
	store *sqliterepo.Store,
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
