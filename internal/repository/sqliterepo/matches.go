package sqliterepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

var _ repository.MatchFinder = (*Store)(nil)

func (store *Store) FindMatches(
	ctx context.Context,
	query repository.MatchQuery,
) (repository.MatchPage, error) {
	query, err := normalizeMatchQuery(query)
	if err != nil {
		return repository.MatchPage{}, invalidArgument("find matches", err)
	}
	if _, err := store.GetUser(ctx, query.UserID); err != nil {
		return repository.MatchPage{}, err
	}
	if query.RankingAt.IsZero() {
		query.RankingAt = store.now().UTC().Truncate(time.Second)
	} else {
		query.RankingAt = query.RankingAt.UTC().Truncate(time.Second)
	}

	var statement string
	var arguments []any
	switch query.Readiness {
	case repository.MatchReadinessActionable:
		statement, arguments = actionableMatchStatement(query)
	case repository.MatchReadinessUnfulfillable:
		statement, arguments = unfulfillableMatchStatement(query)
	}
	return store.queryMatchPage(ctx, query, statement, arguments)
}

// ExplainMatchQueryPlan returns SQLite's plan for the exact production match
// statement. It is used by qualification and repository plan regression tests.
func (store *Store) ExplainMatchQueryPlan(
	ctx context.Context,
	query repository.MatchQuery,
) ([]string, error) {
	query, err := normalizeMatchQuery(query)
	if err != nil {
		return nil, invalidArgument("explain matches", err)
	}
	if query.RankingAt.IsZero() {
		query.RankingAt = store.now().UTC().Truncate(time.Second)
	} else {
		query.RankingAt = query.RankingAt.UTC().Truncate(time.Second)
	}
	var statement string
	var arguments []any
	switch query.Readiness {
	case repository.MatchReadinessActionable:
		statement, arguments = actionableMatchStatement(query)
	case repository.MatchReadinessUnfulfillable:
		statement, arguments = unfulfillableMatchStatement(query)
	}
	rows, err := store.database.QueryContext(
		ctx,
		"EXPLAIN QUERY PLAN "+statement,
		arguments...,
	)
	if err != nil {
		return nil, classify("explain matches", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return nil, classify("scan match plan", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, classify("iterate match plan", err)
	}
	return details, nil
}

func actionableMatchStatement(query repository.MatchQuery) (string, []any) {
	statement := strings.Builder{}
	statement.WriteString(`WITH candidate_matches AS (
		SELECT
			other_offer.user_id AS other_user_id,
			current_want.item_id AS receive_item_id,
			COALESCE(other_want.item_id, '') AS request_item_id,
			min(current_want.wanted, other_offer.available) AS receive_quantity,
			COALESCE(min(current_offer.available, other_want.wanted), 0)
				AS request_quantity,
			CASE WHEN other_want.item_id IS NULL THEN 1 ELSE 0 END
				AS request_class,
			max(
				COALESCE(
					unixepoch(
						CASE
							WHEN other_want.item_id IS NULL
								THEN other_offer.available_since_at
							ELSE other_want.wanted_since_at
						END
					),
					@ranking_at
				),
				@cap_cutoff
			) AS capped_since,
			CASE
				WHEN COALESCE(
					unixepoch(
						CASE
							WHEN other_want.item_id IS NULL
								THEN other_offer.available_since_at
							ELSE other_want.wanted_since_at
						END
					),
					@ranking_at
				) < @stale_cutoff
				AND (
					other_user.last_seen_at IS NULL
					OR unixepoch(other_user.last_seen_at) IS NULL
					OR unixepoch(other_user.last_seen_at) < @stale_cutoff
				)
				THEN 1
				ELSE 0
			END AS stale
		FROM collection_entries current_want
		JOIN catalog_items receive_item ON receive_item.id = current_want.item_id
		JOIN catalogs receive_catalog ON receive_catalog.id = receive_item.catalog_id
		JOIN brands receive_brand ON receive_brand.id = receive_catalog.brand_id
		JOIN collection_entries other_offer
			ON other_offer.item_id = current_want.item_id
			AND other_offer.available > 0
			AND other_offer.user_id <> current_want.user_id
		JOIN users other_user ON other_user.id = other_offer.user_id
		LEFT JOIN collection_entries other_want
			ON other_want.user_id = other_offer.user_id
			AND other_want.wanted > 0
			AND other_want.item_id = (
				SELECT candidate_want.item_id
				FROM collection_entries candidate_want
				JOIN catalog_items candidate_item
					ON candidate_item.id = candidate_want.item_id
				JOIN catalogs candidate_catalog
					ON candidate_catalog.id = candidate_item.catalog_id
				JOIN brands candidate_brand
					ON candidate_brand.id = candidate_catalog.brand_id
				JOIN collection_entries candidate_offer
					ON candidate_offer.user_id = current_want.user_id
					AND candidate_offer.item_id = candidate_want.item_id
					AND candidate_offer.available > 0
				WHERE candidate_want.user_id = other_offer.user_id
					AND candidate_want.wanted > 0
					AND candidate_want.item_id <> current_want.item_id
					AND candidate_item.status <> 'draft'
					AND candidate_catalog.status <> 'draft'
					AND candidate_brand.status <> 'draft'`)
	if query.GiveCatalogID != "" {
		statement.WriteString(" AND candidate_catalog.id = @give_catalog_id")
	}
	switch query.Scope {
	case repository.MatchScopeSameCatalog:
		statement.WriteString(" AND candidate_catalog.id = receive_catalog.id")
	case repository.MatchScopeCrossCatalog:
		statement.WriteString(" AND candidate_catalog.id <> receive_catalog.id")
	}
	statement.WriteString(`
				ORDER BY
					max(
						COALESCE(
							unixepoch(candidate_want.wanted_since_at),
							@ranking_at
						),
						@cap_cutoff
					),
					CASE
						WHEN COALESCE(
							unixepoch(candidate_want.wanted_since_at),
							@ranking_at
						) < @stale_cutoff
						AND (
							other_user.last_seen_at IS NULL
							OR unixepoch(other_user.last_seen_at) IS NULL
							OR unixepoch(other_user.last_seen_at) < @stale_cutoff
						)
						THEN 1
						ELSE 0
					END,
					candidate_want.item_id
				LIMIT 1
			)
		LEFT JOIN collection_entries current_offer
			ON current_offer.user_id = current_want.user_id
			AND current_offer.item_id = other_want.item_id
			AND current_offer.available > 0
		WHERE current_want.user_id = @user_id
			AND current_want.wanted > 0
			AND receive_item.status <> 'draft'
			AND receive_catalog.status <> 'draft'
			AND receive_brand.status <> 'draft'`)
	if query.ReceiveCatalogID != "" {
		statement.WriteString(" AND receive_catalog.id = @receive_catalog_id")
	}
	if query.GiveCatalogID != "" || query.Scope != repository.MatchScopeAll {
		statement.WriteString(" AND other_want.item_id IS NOT NULL")
	} else {
		statement.WriteString(`
			AND (
				other_want.item_id IS NOT NULL
				OR NOT EXISTS (
					SELECT 1
					FROM collection_entries published_want
					JOIN catalog_items published_item
						ON published_item.id = published_want.item_id
					JOIN catalogs published_catalog
						ON published_catalog.id = published_item.catalog_id
					JOIN brands published_brand
						ON published_brand.id = published_catalog.brand_id
					WHERE published_want.user_id = other_offer.user_id
						AND published_want.wanted > 0
						AND published_item.status <> 'draft'
						AND published_catalog.status <> 'draft'
						AND published_brand.status <> 'draft'
				)
			)`)
	}
	statement.WriteString("\n)")
	writeMatchPageCTE(&statement, query)
	writeMatchProjectionQuery(&statement)
	return statement.String(), matchStatementArguments(query)
}

func unfulfillableMatchStatement(query repository.MatchQuery) (string, []any) {
	statement := strings.Builder{}
	statement.WriteString(`WITH candidate_matches AS (
		SELECT
			other_offer.user_id AS other_user_id,
			current_want.item_id AS receive_item_id,
			other_want.item_id AS request_item_id,
			min(current_want.wanted, other_offer.available) AS receive_quantity,
			0 AS request_quantity,
			0 AS request_class,
			max(
				COALESCE(unixepoch(other_want.wanted_since_at), @ranking_at),
				@cap_cutoff
			) AS capped_since,
			CASE
				WHEN COALESCE(
					unixepoch(other_want.wanted_since_at),
					@ranking_at
				) < @stale_cutoff
				AND (
					other_user.last_seen_at IS NULL
					OR unixepoch(other_user.last_seen_at) IS NULL
					OR unixepoch(other_user.last_seen_at) < @stale_cutoff
				)
				THEN 1
				ELSE 0
			END AS stale
		FROM collection_entries current_want
		JOIN catalog_items receive_item ON receive_item.id = current_want.item_id
		JOIN catalogs receive_catalog ON receive_catalog.id = receive_item.catalog_id
		JOIN brands receive_brand ON receive_brand.id = receive_catalog.brand_id
		JOIN collection_entries other_offer
			ON other_offer.item_id = current_want.item_id
			AND other_offer.available > 0
			AND other_offer.user_id <> current_want.user_id
		JOIN users other_user ON other_user.id = other_offer.user_id
		JOIN collection_entries other_want
			ON other_want.user_id = other_offer.user_id
			AND other_want.wanted > 0
			AND other_want.item_id = (
				SELECT candidate_want.item_id
				FROM collection_entries candidate_want
				JOIN catalog_items candidate_item
					ON candidate_item.id = candidate_want.item_id
				JOIN catalogs candidate_catalog
					ON candidate_catalog.id = candidate_item.catalog_id
				JOIN brands candidate_brand
					ON candidate_brand.id = candidate_catalog.brand_id
				LEFT JOIN collection_entries candidate_offer
					ON candidate_offer.user_id = current_want.user_id
					AND candidate_offer.item_id = candidate_want.item_id
					AND candidate_offer.available > 0
				WHERE candidate_want.user_id = other_offer.user_id
					AND candidate_want.wanted > 0
					AND candidate_want.item_id <> current_want.item_id
					AND candidate_offer.item_id IS NULL
					AND candidate_item.status <> 'draft'
					AND candidate_catalog.status <> 'draft'
					AND candidate_brand.status <> 'draft'
				ORDER BY
					max(
						COALESCE(
							unixepoch(candidate_want.wanted_since_at),
							@ranking_at
						),
						@cap_cutoff
					),
					CASE
						WHEN COALESCE(
							unixepoch(candidate_want.wanted_since_at),
							@ranking_at
						) < @stale_cutoff
						AND (
							other_user.last_seen_at IS NULL
							OR unixepoch(other_user.last_seen_at) IS NULL
							OR unixepoch(other_user.last_seen_at) < @stale_cutoff
						)
						THEN 1
						ELSE 0
					END,
					candidate_want.item_id
				LIMIT 1
			)
		WHERE current_want.user_id = @user_id
			AND current_want.wanted > 0
			AND receive_item.status <> 'draft'
			AND receive_catalog.status <> 'draft'
			AND receive_brand.status <> 'draft'
			AND NOT EXISTS (
				SELECT 1
				FROM collection_entries fulfill_want
				JOIN catalog_items fulfill_item
					ON fulfill_item.id = fulfill_want.item_id
				JOIN catalogs fulfill_catalog
					ON fulfill_catalog.id = fulfill_item.catalog_id
				JOIN brands fulfill_brand
					ON fulfill_brand.id = fulfill_catalog.brand_id
				JOIN collection_entries fulfill_offer
					ON fulfill_offer.user_id = current_want.user_id
					AND fulfill_offer.item_id = fulfill_want.item_id
					AND fulfill_offer.available > 0
				WHERE fulfill_want.user_id = other_offer.user_id
					AND fulfill_want.wanted > 0
					AND fulfill_want.item_id <> current_want.item_id
					AND fulfill_item.status <> 'draft'
					AND fulfill_catalog.status <> 'draft'
					AND fulfill_brand.status <> 'draft'
			)`)
	if query.ReceiveCatalogID != "" {
		statement.WriteString(" AND receive_catalog.id = @receive_catalog_id")
	}
	statement.WriteString("\n)")
	writeMatchPageCTE(&statement, query)
	writeMatchProjectionQuery(&statement)
	return statement.String(), matchStatementArguments(query)
}

func writeMatchPageCTE(statement *strings.Builder, query repository.MatchQuery) {
	statement.WriteString(`,
	page AS (
		SELECT *
		FROM candidate_matches`)
	if query.After != nil {
		statement.WriteString(`
		WHERE (
			request_class,
			capped_since,
			stale,
			request_item_id,
			other_user_id,
			receive_item_id
		) > (
			@after_class,
			@after_since,
			@after_stale,
			@after_request_item,
			@after_counterpart,
			@after_receive_item
		)`)
	}
	statement.WriteString(`
		ORDER BY
			request_class,
			capped_since,
			stale,
			request_item_id,
			other_user_id,
			receive_item_id
		LIMIT @page_limit
	)`)
}

func writeMatchProjectionQuery(statement *strings.Builder) {
	statement.WriteString(`
	SELECT page.other_user_id, other_user.display_name,`)
	statement.WriteString(optionalCatalogItemColumnsFor(
		"request_brand",
		"request_catalog",
		"request_item",
		"request_asset",
	))
	statement.WriteString(",")
	statement.WriteString(catalogItemColumnsFor(
		"receive_brand",
		"receive_catalog",
		"receive_item",
		"receive_asset",
	))
	statement.WriteString(`,
		page.receive_quantity,
		page.request_quantity,
		page.request_class,
		page.capped_since,
		page.stale,
		page.request_item_id
	FROM page
	JOIN users other_user ON other_user.id = page.other_user_id
	LEFT JOIN catalog_items request_item
		ON request_item.id = NULLIF(page.request_item_id, '')
	LEFT JOIN catalogs request_catalog ON request_catalog.id = request_item.catalog_id
	LEFT JOIN brands request_brand ON request_brand.id = request_catalog.brand_id
	LEFT JOIN item_assets request_asset ON request_asset.item_id = request_item.id
	JOIN catalog_items receive_item ON receive_item.id = page.receive_item_id
	JOIN catalogs receive_catalog ON receive_catalog.id = receive_item.catalog_id
	JOIN brands receive_brand ON receive_brand.id = receive_catalog.brand_id
	JOIN item_assets receive_asset ON receive_asset.item_id = receive_item.id
	ORDER BY
		page.request_class,
		page.capped_since,
		page.stale,
		page.request_item_id,
		page.other_user_id,
		page.receive_item_id`)
}

func matchStatementArguments(query repository.MatchQuery) []any {
	arguments := []any{
		sql.Named("user_id", query.UserID),
		sql.Named("ranking_at", query.RankingAt.Unix()),
		sql.Named("cap_cutoff", query.RankingAt.Add(-repository.MatchWaitCap).Unix()),
		sql.Named("stale_cutoff", query.RankingAt.Add(-repository.MatchStaleAfter).Unix()),
		sql.Named("page_limit", query.Limit+1),
	}
	if query.GiveCatalogID != "" {
		arguments = append(arguments, sql.Named("give_catalog_id", query.GiveCatalogID))
	}
	if query.ReceiveCatalogID != "" {
		arguments = append(arguments, sql.Named("receive_catalog_id", query.ReceiveCatalogID))
	}
	if query.After != nil {
		stale := 0
		if query.After.Stale {
			stale = 1
		}
		arguments = append(
			arguments,
			sql.Named("after_class", query.After.RequestClass),
			sql.Named("after_since", query.After.CappedSince),
			sql.Named("after_stale", stale),
			sql.Named("after_request_item", query.After.RequestItemID),
			sql.Named("after_counterpart", query.After.CounterpartID),
			sql.Named("after_receive_item", query.After.ReceiveItemID),
		)
	}
	return arguments
}

type rankedMatchRow struct {
	record   repository.MatchRecord
	position repository.MatchPosition
}

func (store *Store) queryMatchPage(
	ctx context.Context,
	query repository.MatchQuery,
	statement string,
	arguments []any,
) (repository.MatchPage, error) {
	rows, err := store.database.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return repository.MatchPage{}, classify("find matches", err)
	}
	defer rows.Close()

	ranked := make([]rankedMatchRow, 0, query.Limit+1)
	for rows.Next() {
		var other domain.User
		var request, receive rawCatalogItem
		var receiveQuantity, requestQuantity int
		var requestClass, stale int
		var cappedSince int64
		var requestItemID string
		targets := []any{&other.ID, &other.DisplayName}
		targets = append(targets, request.scanTargets()...)
		targets = append(targets, receive.scanTargets()...)
		targets = append(
			targets,
			&receiveQuantity,
			&requestQuantity,
			&requestClass,
			&cappedSince,
			&stale,
			&requestItemID,
		)
		if err := rows.Scan(targets...); err != nil {
			return repository.MatchPage{}, classify("scan match", err)
		}
		receiveRecord := receive.record()
		record := repository.MatchRecord{
			ID:        matchID(query.UserID, other.ID, receiveRecord.Item.ID),
			OtherUser: other,
			YouReceive: repository.MatchLeg{
				CatalogItem: receiveRecord,
				Quantity:    receiveQuantity,
			},
		}
		if request.itemID != "" {
			record.TheyRequest = &repository.MatchRequest{
				CatalogItem: request.record(),
				Quantity:    requestQuantity,
				Fulfillable: requestQuantity > 0,
			}
		}
		ranked = append(ranked, rankedMatchRow{
			record: record,
			position: repository.MatchPosition{
				RequestClass:  requestClass,
				CappedSince:   cappedSince,
				Stale:         stale == 1,
				RequestItemID: requestItemID,
				CounterpartID: other.ID,
				ReceiveItemID: receiveRecord.Item.ID,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return repository.MatchPage{}, classify("iterate matches", err)
	}

	var next *repository.MatchPosition
	if len(ranked) > query.Limit {
		ranked = ranked[:query.Limit]
		position := ranked[len(ranked)-1].position
		next = &position
	}
	matches := make([]repository.MatchRecord, 0, len(ranked))
	for _, row := range ranked {
		matches = append(matches, row.record)
	}
	return repository.MatchPage{
		Matches:   matches,
		Limit:     query.Limit,
		RankingAt: query.RankingAt,
		Next:      next,
	}, nil
}

func normalizeMatchQuery(query repository.MatchQuery) (repository.MatchQuery, error) {
	if query.GiveCatalogID != "" && !validSlug(query.GiveCatalogID) {
		return query, errors.New("give catalog id must be a lowercase slug")
	}
	if query.ReceiveCatalogID != "" && !validSlug(query.ReceiveCatalogID) {
		return query, errors.New("receive catalog id must be a lowercase slug")
	}
	if query.Scope == "" {
		query.Scope = repository.MatchScopeAll
	}
	switch query.Scope {
	case repository.MatchScopeAll,
		repository.MatchScopeSameCatalog,
		repository.MatchScopeCrossCatalog:
	default:
		return query, errors.New("match scope is invalid")
	}
	if query.Readiness == "" {
		query.Readiness = repository.MatchReadinessActionable
	}
	switch query.Readiness {
	case repository.MatchReadinessActionable:
	case repository.MatchReadinessUnfulfillable:
		if query.GiveCatalogID != "" || query.Scope != repository.MatchScopeAll {
			return query, errors.New(
				"unfulfillable readiness cannot use give catalog or reciprocal scope",
			)
		}
	default:
		return query, errors.New("match readiness is invalid")
	}
	if !query.RankingAt.IsZero() && query.RankingAt.Unix() <= 0 {
		return query, errors.New("ranking time is invalid")
	}
	if query.Limit == 0 {
		query.Limit = repository.DefaultMatchPageSize
	}
	if query.Limit < 1 || query.Limit > repository.MaxMatchPageSize {
		return query, fmt.Errorf(
			"limit must be between 1 and %d",
			repository.MaxMatchPageSize,
		)
	}
	if query.After != nil {
		requestPositionValid := (query.After.RequestClass == 0 &&
			validSlug(query.After.RequestItemID)) ||
			(query.After.RequestClass == 1 &&
				query.After.RequestItemID == "")
		if query.After.CappedSince <= 0 ||
			!requestPositionValid ||
			validateUserID(query.After.CounterpartID) != nil ||
			!validSlug(query.After.ReceiveItemID) {
			return query, errors.New("match position is invalid")
		}
		if query.RankingAt.IsZero() {
			return query, errors.New("ranking time is required for a continuation")
		}
	}
	return query, nil
}

func matchID(userID, otherUserID, receiveItemID string) string {
	digest := sha256.Sum256([]byte(
		userID + "\x00" + otherUserID + "\x00" + receiveItemID,
	))
	return "match-" + hex.EncodeToString(digest[:12])
}
