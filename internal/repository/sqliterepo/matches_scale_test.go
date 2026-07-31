package sqliterepo

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func TestHighFanoutUnfulfillableFeedIsSplitBoundedAndStable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := Open(ctx, Config{
		Path: filepath.Join(t.TempDir(), "high-fanout.sqlite"),
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
	current := domain.User{ID: "fanout-current", DisplayName: "Fanout Current"}
	if err := store.UpsertUser(ctx, current); err != nil {
		t.Fatalf("UpsertUser current: %v", err)
	}
	receiveID := loaded[0].Manifest.Items[0].ID
	requestID := loaded[0].Manifest.Items[1].ID
	rankingAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return rankingAt.Add(-30 * 24 * time.Hour) }
	if _, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
		UserID: current.ID,
		ItemID: receiveID,
		Wanted: 1,
	}); err != nil {
		t.Fatalf("SaveCollectionEntry current: %v", err)
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer transaction.Rollback()
	userStatement, err := transaction.PrepareContext(
		ctx,
		`INSERT INTO users(id, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		t.Fatalf("prepare users: %v", err)
	}
	defer userStatement.Close()
	entryStatement, err := transaction.PrepareContext(
		ctx,
		`INSERT INTO collection_entries(
			user_id, item_id, owned, available, available_since_at,
			wanted, wanted_since_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		t.Fatalf("prepare entries: %v", err)
	}
	defer entryStatement.Close()
	const counterparts = 320
	stateSince := rankingAt.Add(-120 * 24 * time.Hour).Format(time.RFC3339Nano)
	for index := 0; index < counterparts; index++ {
		userID := fmt.Sprintf("fanout-%03d", index)
		if _, err := userStatement.ExecContext(
			ctx,
			userID,
			fmt.Sprintf("Fanout %03d", index),
			stateSince,
			stateSince,
		); err != nil {
			t.Fatalf("insert user %d: %v", index, err)
		}
		if _, err := entryStatement.ExecContext(
			ctx,
			userID,
			receiveID,
			1,
			1,
			stateSince,
			0,
			nil,
			stateSince,
		); err != nil {
			t.Fatalf("insert offer %d: %v", index, err)
		}
		if _, err := entryStatement.ExecContext(
			ctx,
			userID,
			requestID,
			0,
			0,
			nil,
			1,
			stateSince,
			stateSince,
		); err != nil {
			t.Fatalf("insert want %d: %v", index, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	actionable, err := store.FindMatches(ctx, repository.MatchQuery{
		UserID:    current.ID,
		Readiness: repository.MatchReadinessActionable,
		RankingAt: rankingAt,
		Limit:     repository.MaxMatchPageSize,
	})
	if err != nil {
		t.Fatalf("FindMatches actionable: %v", err)
	}
	if len(actionable.Matches) != 0 || actionable.Next != nil {
		t.Fatalf("all-unfulfillable fanout leaked into actionable: %#v", actionable)
	}

	query := repository.MatchQuery{
		UserID:    current.ID,
		Readiness: repository.MatchReadinessUnfulfillable,
		RankingAt: rankingAt,
		Limit:     37,
	}
	seen := make(map[string]struct{}, counterparts)
	var firstCounterpart string
	for {
		page, err := store.FindMatches(ctx, query)
		if err != nil {
			t.Fatalf("FindMatches unfulfillable: %v", err)
		}
		if len(page.Matches) > query.Limit {
			t.Fatalf("unfulfillable page returned %d, limit %d", len(page.Matches), query.Limit)
		}
		for _, match := range page.Matches {
			if firstCounterpart == "" {
				firstCounterpart = match.OtherUser.ID
			}
			if match.TheyRequest == nil ||
				match.TheyRequest.Fulfillable ||
				match.TheyRequest.Quantity != 0 ||
				match.TheyRequest.CatalogItem.Item.ID != requestID {
				t.Fatalf("unfulfillable match = %#v", match)
			}
			if _, duplicate := seen[match.OtherUser.ID]; duplicate {
				t.Fatalf("duplicate counterpart %q", match.OtherUser.ID)
			}
			seen[match.OtherUser.ID] = struct{}{}
		}
		if page.Next == nil {
			break
		}
		query.After = page.Next
	}
	if len(seen) != counterparts {
		t.Fatalf("unfulfillable counterparts = %d, want %d", len(seen), counterparts)
	}
	if firstCounterpart != "fanout-000" {
		t.Fatalf("first stable counterpart = %q, want fanout-000", firstCounterpart)
	}
}
