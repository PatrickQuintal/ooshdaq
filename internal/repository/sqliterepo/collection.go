package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

var (
	_ repository.CollectionReader = (*Store)(nil)
	_ repository.CollectionWriter = (*Store)(nil)
)

func (store *Store) ListCollectionPage(
	ctx context.Context,
	query repository.CollectionQuery,
) (repository.CollectionPage, error) {
	query, err := normalizeCollectionQuery(query)
	if err != nil {
		return repository.CollectionPage{}, invalidArgument("list collection", err)
	}
	if _, err := store.GetUser(ctx, query.UserID); err != nil {
		return repository.CollectionPage{}, err
	}
	catalog, err := store.getCatalog(ctx, query.UserID, query.CatalogID)
	if err != nil {
		return repository.CollectionPage{}, err
	}
	if query.Universe != "" && !contains(catalog.UniverseOptions, query.Universe) {
		return repository.CollectionPage{}, invalidArgument(
			"list collection",
			domain.ValidationError{
				Field:  "universe",
				Reason: "is not available in this catalog",
			},
		)
	}

	statement := strings.Builder{}
	statement.WriteString("SELECT ")
	statement.WriteString(catalogItemColumns)
	statement.WriteString(`,
		COALESCE(entry.owned, 0),
		COALESCE(entry.available, 0),
		COALESCE(entry.wanted, 0)
		FROM catalog_items i
		JOIN catalogs c ON c.id = i.catalog_id
		JOIN brands b ON b.id = c.brand_id
		JOIN item_assets a ON a.item_id = i.id
		LEFT JOIN collection_entries entry
			ON entry.item_id = i.id AND entry.user_id = ?
		WHERE b.status <> 'draft' AND c.status <> 'draft'
			AND i.status <> 'draft' AND c.id = ?`)
	arguments := []any{query.UserID, query.CatalogID}
	if query.Search != "" {
		statement.WriteString(`
			AND (
				instr(ooshdaq_unicode_lower(i.name), ?) > 0
				OR instr(ooshdaq_unicode_lower(i.universe), ?) > 0
				OR instr(ooshdaq_unicode_lower(i.variant), ?) > 0
			)`)
		folded := strings.ToLower(query.Search)
		arguments = append(arguments, folded, folded, folded)
	}
	if query.Universe != "" {
		statement.WriteString(" AND i.universe = ?")
		arguments = append(arguments, query.Universe)
	}
	switch query.State {
	case repository.CollectionStateOwned:
		statement.WriteString(" AND COALESCE(entry.owned, 0) > 0")
	case repository.CollectionStateAvailable:
		statement.WriteString(" AND COALESCE(entry.available, 0) > 0")
	case repository.CollectionStateWanted:
		statement.WriteString(" AND COALESCE(entry.wanted, 0) > 0")
	}
	if query.After != nil {
		statement.WriteString(`
			AND (i.number > ? OR (i.number = ? AND i.id > ?))`)
		arguments = append(
			arguments,
			query.After.ItemNumber,
			query.After.ItemNumber,
			query.After.ItemID,
		)
	}
	statement.WriteString(" ORDER BY i.number, i.id LIMIT ?")
	arguments = append(arguments, query.Limit+1)

	rows, err := store.database.QueryContext(ctx, statement.String(), arguments...)
	if err != nil {
		return repository.CollectionPage{}, classify("list collection", err)
	}
	defer rows.Close()
	records := make([]repository.CollectionRecord, 0, query.Limit+1)
	for rows.Next() {
		var raw rawCatalogItem
		var record repository.CollectionRecord
		targets := append(
			raw.scanTargets(),
			&record.Owned,
			&record.Available,
			&record.Wanted,
		)
		if err := rows.Scan(targets...); err != nil {
			return repository.CollectionPage{}, classify("scan collection", err)
		}
		record.CatalogItem = raw.record()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return repository.CollectionPage{}, classify("iterate collection", err)
	}

	var next *repository.CollectionPosition
	if len(records) > query.Limit {
		records = records[:query.Limit]
		last := records[len(records)-1].CatalogItem.Item
		next = &repository.CollectionPosition{
			ItemNumber: last.Number,
			ItemID:     last.ID,
		}
	}
	return repository.CollectionPage{
		Catalog: catalog,
		Entries: records,
		Limit:   query.Limit,
		Next:    next,
	}, nil
}

func (store *Store) GetCollectionSummary(
	ctx context.Context,
	userID string,
) (repository.OverallCollectionSummary, error) {
	if _, err := store.GetUser(ctx, userID); err != nil {
		return repository.OverallCollectionSummary{}, err
	}
	var summary repository.OverallCollectionSummary
	err := store.database.QueryRowContext(
		ctx,
		`SELECT
			(SELECT count(*) FROM catalogs c JOIN brands b ON b.id = c.brand_id
			 WHERE c.status = 'active' AND b.status = 'active'),
			(SELECT count(*) FROM catalog_items i
			 JOIN catalogs c ON c.id = i.catalog_id
			 JOIN brands b ON b.id = c.brand_id
			 WHERE i.status = 'active' AND c.status = 'active' AND b.status = 'active'),
			(SELECT count(DISTINCT i.catalog_id)
			 FROM collection_entries e
			 JOIN catalog_items i ON i.id = e.item_id
			 JOIN catalogs c ON c.id = i.catalog_id
			 JOIN brands b ON b.id = c.brand_id
			 WHERE e.user_id = ? AND i.status <> 'draft'
				AND c.status <> 'draft' AND b.status <> 'draft'),
			(SELECT count(*)
			 FROM collection_entries current_want
			 JOIN collection_entries other_offer
				ON other_offer.item_id = current_want.item_id
				AND other_offer.available > 0
				AND other_offer.user_id <> current_want.user_id
			 JOIN catalog_items possible_item ON possible_item.id = current_want.item_id
			 JOIN catalogs possible_catalog ON possible_catalog.id = possible_item.catalog_id
			 JOIN brands possible_brand ON possible_brand.id = possible_catalog.brand_id
			 WHERE current_want.user_id = ? AND current_want.wanted > 0
				AND possible_item.status <> 'draft'
				AND possible_catalog.status <> 'draft'
				AND possible_brand.status <> 'draft'),
			COALESCE(sum(e.owned), 0),
			COALESCE(sum(CASE WHEN e.owned > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(sum(e.available), 0),
			COALESCE(sum(CASE WHEN e.available > 0 THEN 1 ELSE 0 END), 0),
			COALESCE(sum(e.wanted), 0),
			COALESCE(sum(CASE WHEN e.wanted > 0 THEN 1 ELSE 0 END), 0)
		 FROM collection_entries e
		 JOIN catalog_items i ON i.id = e.item_id
		 JOIN catalogs c ON c.id = i.catalog_id
		 JOIN brands b ON b.id = c.brand_id
		 WHERE e.user_id = ? AND i.status <> 'draft'
			AND c.status <> 'draft' AND b.status <> 'draft'`,
		userID,
		userID,
		userID,
	).Scan(
		&summary.ActiveCatalogCount,
		&summary.ActiveItemCount,
		&summary.CatalogsStarted,
		&summary.PossibleSwapCount,
		&summary.MemberSummary.OwnedQuantity,
		&summary.MemberSummary.OwnedDesigns,
		&summary.MemberSummary.AvailableQuantity,
		&summary.MemberSummary.AvailableDesigns,
		&summary.MemberSummary.WantedQuantity,
		&summary.MemberSummary.WantedDesigns,
	)
	if err != nil {
		return repository.OverallCollectionSummary{}, classify("get collection summary", err)
	}
	return summary, nil
}

func (store *Store) SaveCollectionEntry(
	ctx context.Context,
	write repository.CollectionWrite,
) (repository.CollectionRecord, error) {
	if err := write.Validate(); err != nil {
		return repository.CollectionRecord{}, invalidArgument("save collection entry", err)
	}
	if _, err := store.GetUser(ctx, write.UserID); err != nil {
		return repository.CollectionRecord{}, err
	}

	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return repository.CollectionRecord{}, classify("begin collection write", err)
	}
	defer transaction.Rollback()

	record, err := getCollectionItem(ctx, transaction, write.UserID, write.ItemID)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.CollectionRecord{}, notFound(
			"save collection entry",
			"catalog item",
			write.ItemID,
		)
	}
	if err != nil {
		return repository.CollectionRecord{}, classify("get collection item", err)
	}
	var previousAvailableSince, previousWantedSince sql.NullString
	err = transaction.QueryRowContext(
		ctx,
		`SELECT available_since_at, wanted_since_at
		 FROM collection_entries
		 WHERE user_id = ? AND item_id = ?`,
		write.UserID,
		write.ItemID,
	).Scan(&previousAvailableSince, &previousWantedSince)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return repository.CollectionRecord{}, classify("get collection timing", err)
	}
	if write.Owned == 0 && write.Available == 0 && write.Wanted == 0 {
		_, err = transaction.ExecContext(
			ctx,
			"DELETE FROM collection_entries WHERE user_id = ? AND item_id = ?",
			write.UserID,
			write.ItemID,
		)
	} else {
		now := store.now().UTC().Format(time.RFC3339Nano)
		availableSince := transitionTimestamp(
			record.Available,
			write.Available,
			previousAvailableSince,
			now,
		)
		wantedSince := transitionTimestamp(
			record.Wanted,
			write.Wanted,
			previousWantedSince,
			now,
		)
		_, err = transaction.ExecContext(
			ctx,
			`INSERT INTO collection_entries(
				user_id, item_id, owned, available, available_since_at,
				wanted, wanted_since_at, updated_at
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, item_id) DO UPDATE SET
				owned = excluded.owned,
				available = excluded.available,
				available_since_at = excluded.available_since_at,
				wanted = excluded.wanted,
				wanted_since_at = excluded.wanted_since_at,
				updated_at = excluded.updated_at`,
			write.UserID,
			write.ItemID,
			write.Owned,
			write.Available,
			availableSince,
			write.Wanted,
			wantedSince,
			now,
		)
	}
	if err != nil {
		return repository.CollectionRecord{}, classify("write collection entry", err)
	}
	if err := transaction.Commit(); err != nil {
		return repository.CollectionRecord{}, classify("commit collection entry", err)
	}
	record.Owned = write.Owned
	record.Available = write.Available
	record.Wanted = write.Wanted
	return record, nil
}

func transitionTimestamp(
	previousQuantity int,
	nextQuantity int,
	previousSince sql.NullString,
	now string,
) any {
	switch {
	case nextQuantity <= 0:
		return nil
	case previousQuantity > 0 && previousSince.Valid:
		return previousSince.String
	default:
		return now
	}
}

func getCollectionItem(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	userID string,
	itemID string,
) (repository.CollectionRecord, error) {
	statement := "SELECT " + catalogItemColumns + `,
		COALESCE(entry.owned, 0),
		COALESCE(entry.available, 0),
		COALESCE(entry.wanted, 0)
		FROM catalog_items i
		JOIN catalogs c ON c.id = i.catalog_id
		JOIN brands b ON b.id = c.brand_id
		JOIN item_assets a ON a.item_id = i.id
		LEFT JOIN collection_entries entry
			ON entry.item_id = i.id AND entry.user_id = ?
		WHERE i.id = ? AND i.status <> 'draft'
			AND c.status <> 'draft' AND b.status <> 'draft'`
	var raw rawCatalogItem
	var record repository.CollectionRecord
	targets := append(raw.scanTargets(), &record.Owned, &record.Available, &record.Wanted)
	err := queryer.QueryRowContext(ctx, statement, userID, itemID).Scan(targets...)
	if err != nil {
		return repository.CollectionRecord{}, err
	}
	record.CatalogItem = raw.record()
	return record, nil
}

func normalizeCollectionQuery(
	query repository.CollectionQuery,
) (repository.CollectionQuery, error) {
	if !validSlug(query.CatalogID) {
		return query, errors.New("catalog id must be a lowercase slug")
	}
	query.Search = strings.TrimSpace(query.Search)
	if !utf8.ValidString(query.Search) ||
		utf8.RuneCountInString(query.Search) > repository.MaxSearchRunes {
		return query, errors.New("search must be valid UTF-8 with at most 100 Unicode code points")
	}
	if !utf8.ValidString(query.Universe) {
		return query, errors.New("universe must be valid UTF-8")
	}
	if query.State == "" {
		query.State = repository.CollectionStateAll
	}
	switch query.State {
	case repository.CollectionStateAll,
		repository.CollectionStateOwned,
		repository.CollectionStateAvailable,
		repository.CollectionStateWanted:
	default:
		return query, errors.New("collection state is invalid")
	}
	if query.Limit == 0 {
		query.Limit = repository.DefaultCollectionPageSize
	}
	if query.Limit < 1 || query.Limit > repository.MaxCollectionPageSize {
		return query, fmt.Errorf("limit must be between 1 and %d", repository.MaxCollectionPageSize)
	}
	if query.After != nil &&
		(query.After.ItemNumber < 1 || !validSlug(query.After.ItemID)) {
		return query, errors.New("collection position is invalid")
	}
	return query, nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
