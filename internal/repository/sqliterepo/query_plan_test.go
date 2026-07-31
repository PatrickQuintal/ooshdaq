package sqliterepo

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func TestPossibleSwapQueryPlanUsesBoundedIndexes(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), Config{
		Path: filepath.Join(t.TempDir(), "query-plan.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("ImportManifests: %v", err)
	}
	for _, user := range []domain.User{
		{ID: "plan-current", DisplayName: "Plan Current"},
		{ID: "plan-other", DisplayName: "Plan Other"},
	} {
		if err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatalf("UpsertUser: %v", err)
		}
	}
	giveID := loaded[0].Manifest.Items[0].ID
	receiveID := loaded[1].Manifest.Items[0].ID
	for _, write := range []repository.CollectionWrite{
		{UserID: "plan-current", ItemID: giveID, Owned: 2, Available: 1},
		{UserID: "plan-current", ItemID: receiveID, Wanted: 1},
		{UserID: "plan-other", ItemID: giveID, Wanted: 1},
		{UserID: "plan-other", ItemID: receiveID, Owned: 1, Available: 1},
	} {
		if _, err := store.SaveCollectionEntry(context.Background(), write); err != nil {
			t.Fatalf("SaveCollectionEntry: %v", err)
		}
	}

	rankingAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		query repository.MatchQuery
	}{
		{
			name: "actionable",
			query: repository.MatchQuery{
				UserID:    "plan-current",
				Readiness: repository.MatchReadinessActionable,
				RankingAt: rankingAt,
				Scope:     repository.MatchScopeCrossCatalog,
				Limit:     20,
			},
		},
		{
			name: "unfulfillable",
			query: repository.MatchQuery{
				UserID:    "plan-current",
				Readiness: repository.MatchReadinessUnfulfillable,
				RankingAt: rankingAt,
				Limit:     20,
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			details, err := store.ExplainMatchQueryPlan(
				context.Background(),
				test.query,
			)
			if err != nil {
				t.Fatalf("ExplainMatchQueryPlan: %v", err)
			}
			plan := strings.Join(details, "\n")
			t.Logf("%s possible-swap query plan:\n%s", test.name, plan)
			for _, required := range []string{
				"current_want USING",
				"other_offer USING",
				"candidate_offer USING",
				"candidate_want USING",
			} {
				if !strings.Contains(plan, required) {
					t.Fatalf("query plan does not contain %q:\n%s", required, plan)
				}
			}
			for _, prohibited := range []string{
				"SCAN current_want",
				"SCAN other_offer",
				"SCAN candidate_offer",
				"SCAN candidate_want",
			} {
				if strings.Contains(plan, prohibited) {
					t.Fatalf("query plan contains unbounded %q:\n%s", prohibited, plan)
				}
			}
		})
	}
}

func TestCatalogItemSearchQueryPlanUsesCatalogIndex(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), Config{
		Path: filepath.Join(t.TempDir(), "catalog-search-plan.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	rows, err := store.database.Query(`
		EXPLAIN QUERY PLAN
		SELECT c.id
		FROM catalogs c
		JOIN brands b ON b.id = c.brand_id
		WHERE b.status <> 'draft'
			AND c.status = 'active'
			AND (
				instr(ooshdaq_unicode_lower(b.name), ?) > 0
				OR instr(ooshdaq_unicode_lower(c.name), ?) > 0
				OR instr(ooshdaq_unicode_lower(c.edition), ?) > 0
				OR EXISTS (
					SELECT 1
					FROM catalog_items search_item
					WHERE search_item.catalog_id = c.id
						AND search_item.status <> 'draft'
						AND (
							instr(ooshdaq_unicode_lower(search_item.name), ?) > 0
							OR instr(ooshdaq_unicode_lower(search_item.universe), ?) > 0
							OR instr(ooshdaq_unicode_lower(search_item.variant), ?) > 0
						)
				)
			)
		ORDER BY c.sort_order, c.id
		LIMIT 21`,
		"needle",
		"needle",
		"needle",
		"needle",
		"needle",
		"needle",
	)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate query plan: %v", err)
	}
	plan := strings.Join(details, "\n")
	t.Logf("catalog item search query plan:\n%s", plan)
	if !strings.Contains(plan, "SEARCH search_item USING") ||
		!strings.Contains(plan, "(catalog_id=?)") {
		t.Fatalf("item search is not constrained by a catalog index:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN search_item") {
		t.Fatalf("item search performs an unbounded item scan:\n%s", plan)
	}
}
