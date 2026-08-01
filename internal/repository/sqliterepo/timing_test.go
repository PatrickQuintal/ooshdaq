package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func TestCollectionStateSinceTimestampsFollowPositiveTransitions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, Config{
		Path: filepath.Join(t.TempDir(), "collection-timing.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	if err := store.ImportManifests(ctx, loaded); err != nil {
		t.Fatalf("ImportManifests: %v", err)
	}
	if err := store.UpsertUser(ctx, domain.User{
		ID:          "timing-member",
		DisplayName: "Timing Member",
	}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	itemID := loaded[0].Manifest.Items[0].ID
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	store.now = func() time.Time { return base }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID:    "timing-member",
		ItemID:    itemID,
		Owned:     2,
		Available: 1,
	})
	assertEntryTiming(t, store, "timing-member", itemID, base, time.Time{})

	store.now = func() time.Time { return base.Add(12 * time.Hour) }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID:    "timing-member",
		ItemID:    itemID,
		Owned:     3,
		Available: 2,
	})
	assertEntryTiming(t, store, "timing-member", itemID, base, time.Time{})

	store.now = func() time.Time { return base.Add(24 * time.Hour) }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID: "timing-member",
		ItemID: itemID,
		Owned:  3,
	})
	assertEntryTiming(t, store, "timing-member", itemID, time.Time{}, time.Time{})

	wantedSince := base.Add(36 * time.Hour)
	store.now = func() time.Time { return wantedSince }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID: "timing-member",
		ItemID: itemID,
		Owned:  3,
		Wanted: 1,
	})
	assertEntryTiming(t, store, "timing-member", itemID, time.Time{}, wantedSince)

	store.now = func() time.Time { return base.Add(48 * time.Hour) }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID: "timing-member",
		ItemID: itemID,
		Owned:  4,
		Wanted: 2,
	})
	assertEntryTiming(t, store, "timing-member", itemID, time.Time{}, wantedSince)

	store.now = func() time.Time { return base.Add(60 * time.Hour) }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID: "timing-member",
		ItemID: itemID,
		Owned:  4,
	})
	assertEntryTiming(t, store, "timing-member", itemID, time.Time{}, time.Time{})

	reactivated := base.Add(72 * time.Hour)
	store.now = func() time.Time { return reactivated }
	saveTimingEntry(t, store, repository.CollectionWrite{
		UserID:    "timing-member",
		ItemID:    itemID,
		Owned:     4,
		Available: 1,
	})
	assertEntryTiming(t, store, "timing-member", itemID, reactivated, time.Time{})
}

func TestUserActivityTouchIsExplicitAndHourly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, Config{
		Path: filepath.Join(t.TempDir(), "activity.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	base := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	store.now = func() time.Time { return base }
	if err := store.UpsertUser(ctx, domain.User{
		ID:          "activity-member",
		DisplayName: "Activity Member",
	}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	var updatedAt string
	var lastSeen sql.NullString
	if err := store.database.QueryRowContext(
		ctx,
		`SELECT updated_at, last_seen_at FROM users WHERE id = 'activity-member'`,
	).Scan(&updatedAt, &lastSeen); err != nil {
		t.Fatalf("read new user timing: %v", err)
	}
	if lastSeen.Valid {
		t.Fatalf("profile upsert fabricated last_seen_at = %q", lastSeen.String)
	}

	if err := store.TouchUserLastSeen(ctx, "activity-member"); err != nil {
		t.Fatalf("first TouchUserLastSeen: %v", err)
	}
	assertLastSeen(t, store, "activity-member", base)

	store.now = func() time.Time { return base.Add(30 * time.Minute) }
	if err := store.UpsertUser(ctx, domain.User{
		ID:          "activity-member",
		DisplayName: "Renamed Activity Member",
	}); err != nil {
		t.Fatalf("UpsertUser rename: %v", err)
	}
	assertLastSeen(t, store, "activity-member", base)

	store.now = func() time.Time { return base.Add(59 * time.Minute) }
	if err := store.TouchUserLastSeen(ctx, "activity-member"); err != nil {
		t.Fatalf("rate-limited TouchUserLastSeen: %v", err)
	}
	assertLastSeen(t, store, "activity-member", base)

	exactHour := base.Add(repository.LastSeenTouchInterval)
	store.now = func() time.Time { return exactHour }
	if err := store.TouchUserLastSeen(ctx, "activity-member"); err != nil {
		t.Fatalf("exact-hour TouchUserLastSeen: %v", err)
	}
	assertLastSeen(t, store, "activity-member", exactHour)

	if err := store.TouchUserLastSeen(ctx, "missing-member"); !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf("missing user touch error = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.TouchUserLastSeen(
		cancelled,
		"activity-member",
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled user touch error = %v", err)
	}

	var updatedAfter string
	if err := store.database.QueryRowContext(
		ctx,
		"SELECT updated_at FROM users WHERE id = 'activity-member'",
	).Scan(&updatedAfter); err != nil {
		t.Fatalf("read updated_at after activity: %v", err)
	}
	if updatedAfter == exactHour.Format(time.RFC3339Nano) ||
		updatedAfter == updatedAt {
		t.Fatalf(
			"activity/profile timestamps not independent: before=%q after=%q",
			updatedAt,
			updatedAfter,
		)
	}
}

func saveTimingEntry(
	t *testing.T,
	store *Store,
	write repository.CollectionWrite,
) {
	t.Helper()
	if _, err := store.SaveCollectionEntry(context.Background(), write); err != nil {
		t.Fatalf("SaveCollectionEntry: %v", err)
	}
}

func assertEntryTiming(
	t *testing.T,
	store *Store,
	userID string,
	itemID string,
	wantAvailable time.Time,
	wantWanted time.Time,
) {
	t.Helper()
	var availableSince, wantedSince sql.NullString
	if err := store.database.QueryRow(
		`SELECT available_since_at, wanted_since_at
		 FROM collection_entries
		 WHERE user_id = ? AND item_id = ?`,
		userID,
		itemID,
	).Scan(&availableSince, &wantedSince); err != nil {
		t.Fatalf("read collection timing: %v", err)
	}
	assertNullTime(t, "available_since_at", availableSince, wantAvailable)
	assertNullTime(t, "wanted_since_at", wantedSince, wantWanted)
}

func assertLastSeen(t *testing.T, store *Store, userID string, want time.Time) {
	t.Helper()
	var value sql.NullString
	if err := store.database.QueryRow(
		"SELECT last_seen_at FROM users WHERE id = ?",
		userID,
	).Scan(&value); err != nil {
		t.Fatalf("read last_seen_at: %v", err)
	}
	assertNullTime(t, "last_seen_at", value, want)
}

func assertNullTime(
	t *testing.T,
	field string,
	got sql.NullString,
	want time.Time,
) {
	t.Helper()
	if want.IsZero() {
		if got.Valid {
			t.Fatalf("%s = %q, want NULL", field, got.String)
		}
		return
	}
	if !got.Valid {
		t.Fatalf("%s = NULL, want %s", field, want)
	}
	parsed, err := time.Parse(time.RFC3339Nano, got.String)
	if err != nil {
		t.Fatalf("parse %s %q: %v", field, got.String, err)
	}
	if !parsed.Equal(want) {
		t.Fatalf("%s = %s, want %s", field, parsed, want)
	}
}
