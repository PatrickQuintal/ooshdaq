package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

var _ repository.CatalogReader = (*Store)(nil)

type rawCatalogRecord struct {
	brandID       string
	brandName     string
	brandStatus   string
	catalogID     string
	catalogName   string
	edition       string
	catalogStatus string
	sortOrder     int
	itemCount     int
	summary       repository.MemberSummary
}

func (raw *rawCatalogRecord) scanTargets() []any {
	return []any{
		&raw.brandID,
		&raw.brandName,
		&raw.brandStatus,
		&raw.catalogID,
		&raw.catalogName,
		&raw.edition,
		&raw.catalogStatus,
		&raw.sortOrder,
		&raw.itemCount,
		&raw.summary.OwnedQuantity,
		&raw.summary.OwnedDesigns,
		&raw.summary.AvailableQuantity,
		&raw.summary.AvailableDesigns,
		&raw.summary.WantedQuantity,
		&raw.summary.WantedDesigns,
	}
}

func (raw rawCatalogRecord) record() repository.CatalogRecord {
	return repository.CatalogRecord{
		Brand: domain.Brand{
			ID:     raw.brandID,
			Name:   raw.brandName,
			Status: domain.PublicationStatus(raw.brandStatus),
		},
		Catalog: domain.Catalog{
			ID:        raw.catalogID,
			BrandID:   raw.brandID,
			Name:      raw.catalogName,
			Edition:   raw.edition,
			Status:    domain.PublicationStatus(raw.catalogStatus),
			SortOrder: raw.sortOrder,
		},
		ItemCount:     raw.itemCount,
		MemberSummary: raw.summary,
	}
}

func (store *Store) ListCatalogs(
	ctx context.Context,
	query repository.CatalogQuery,
) (repository.CatalogPage, error) {
	query, err := normalizeCatalogQuery(query)
	if err != nil {
		return repository.CatalogPage{}, invalidArgument("list catalogs", err)
	}
	if _, err := store.GetUser(ctx, query.UserID); err != nil {
		return repository.CatalogPage{}, err
	}

	statement := strings.Builder{}
	statement.WriteString(catalogSummarySelect)
	statement.WriteString(`
		WHERE b.status <> 'draft' AND c.status <> 'draft'`)
	arguments := []any{query.UserID}
	switch query.PublicationStatus {
	case repository.PublicationFilterActive:
		statement.WriteString(" AND c.status = 'active'")
	case repository.PublicationFilterRetired:
		statement.WriteString(" AND c.status = 'retired'")
	}
	if query.BrandID != "" {
		statement.WriteString(" AND b.id = ?")
		arguments = append(arguments, query.BrandID)
	}
	if query.Search != "" {
		statement.WriteString(`
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
			)`)
		folded := strings.ToLower(query.Search)
		arguments = append(
			arguments,
			folded,
			folded,
			folded,
			folded,
			folded,
			folded,
		)
	}
	if query.After != nil {
		statement.WriteString(`
			AND (c.sort_order > ? OR (c.sort_order = ? AND c.id > ?))`)
		arguments = append(
			arguments,
			query.After.SortOrder,
			query.After.SortOrder,
			query.After.CatalogID,
		)
	}
	statement.WriteString(" ORDER BY c.sort_order, c.id LIMIT ?")
	arguments = append(arguments, query.Limit+1)

	rows, err := store.database.QueryContext(ctx, statement.String(), arguments...)
	if err != nil {
		return repository.CatalogPage{}, classify("list catalogs", err)
	}
	defer rows.Close()
	records := make([]repository.CatalogRecord, 0, query.Limit+1)
	for rows.Next() {
		var raw rawCatalogRecord
		if err := rows.Scan(raw.scanTargets()...); err != nil {
			return repository.CatalogPage{}, classify("scan catalog", err)
		}
		records = append(records, raw.record())
	}
	if err := rows.Err(); err != nil {
		return repository.CatalogPage{}, classify("iterate catalogs", err)
	}

	var next *repository.CatalogPosition
	if len(records) > query.Limit {
		records = records[:query.Limit]
		last := records[len(records)-1].Catalog
		next = &repository.CatalogPosition{
			SortOrder: last.SortOrder,
			CatalogID: last.ID,
		}
	}
	for index := range records {
		if err := store.hydrateCatalog(ctx, &records[index]); err != nil {
			return repository.CatalogPage{}, err
		}
	}
	return repository.CatalogPage{
		Catalogs: records,
		Limit:    query.Limit,
		Next:     next,
	}, nil
}

func (store *Store) getCatalog(
	ctx context.Context,
	userID string,
	catalogID string,
) (repository.CatalogRecord, error) {
	var raw rawCatalogRecord
	err := store.database.QueryRowContext(
		ctx,
		catalogSummarySelect+`
		 WHERE b.status <> 'draft' AND c.status <> 'draft' AND c.id = ?`,
		userID,
		catalogID,
	).Scan(raw.scanTargets()...)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.CatalogRecord{}, notFound("get catalog", "catalog", catalogID)
	}
	if err != nil {
		return repository.CatalogRecord{}, classify("get catalog", err)
	}
	record := raw.record()
	if err := store.hydrateCatalog(ctx, &record); err != nil {
		return repository.CatalogRecord{}, err
	}
	return record, nil
}

func (store *Store) hydrateCatalog(
	ctx context.Context,
	record *repository.CatalogRecord,
) error {
	rows, err := store.database.QueryContext(
		ctx,
		`SELECT DISTINCT universe
		 FROM catalog_items
		 WHERE catalog_id = ? AND status <> 'draft' AND universe <> ''
		 ORDER BY universe`,
		record.Catalog.ID,
	)
	if err != nil {
		return classify("list catalog universes", err)
	}
	for rows.Next() {
		var universe string
		if err := rows.Scan(&universe); err != nil {
			rows.Close()
			return classify("scan catalog universe", err)
		}
		record.UniverseOptions = append(record.UniverseOptions, universe)
	}
	if err := rows.Close(); err != nil {
		return classify("close catalog universes", err)
	}
	if err := rows.Err(); err != nil {
		return classify("iterate catalog universes", err)
	}

	var artwork rawArtwork
	err = store.database.QueryRowContext(
		ctx,
		`SELECT
			a.asset_url, a.placeholder, a.rights_status, a.license, a.attribution,
			a.permission_reference, a.source_page_url, a.retrieved_at, a.reviewed_at,
			a.review_reference, a.transformation, a.sha256
		 FROM catalog_items i
		 JOIN item_assets a ON a.item_id = i.id
		 WHERE i.catalog_id = ? AND i.status <> 'draft'
		 ORDER BY i.number, i.id
		 LIMIT 1`,
		record.Catalog.ID,
	).Scan(artwork.scanTargets()...)
	if err != nil {
		return classify("get representative catalog artwork", err)
	}
	record.Artwork = artwork.domain()
	return nil
}

func normalizeCatalogQuery(query repository.CatalogQuery) (repository.CatalogQuery, error) {
	query.Search = strings.TrimSpace(query.Search)
	if !utf8.ValidString(query.Search) ||
		utf8.RuneCountInString(query.Search) > repository.MaxSearchRunes {
		return query, errors.New("search must be valid UTF-8 with at most 100 Unicode code points")
	}
	if query.BrandID != "" && !validSlug(query.BrandID) {
		return query, errors.New("brand id must be a lowercase slug")
	}
	if query.PublicationStatus == "" {
		query.PublicationStatus = repository.PublicationFilterActive
	}
	switch query.PublicationStatus {
	case repository.PublicationFilterActive,
		repository.PublicationFilterRetired,
		repository.PublicationFilterAll:
	default:
		return query, errors.New("publication status is invalid")
	}
	if query.Limit == 0 {
		query.Limit = repository.DefaultCatalogPageSize
	}
	if query.Limit < 1 || query.Limit > repository.MaxCatalogPageSize {
		return query, fmt.Errorf("limit must be between 1 and %d", repository.MaxCatalogPageSize)
	}
	if query.After != nil &&
		(query.After.SortOrder < 1 || !validSlug(query.After.CatalogID)) {
		return query, errors.New("catalog position is invalid")
	}
	return query, nil
}

const catalogSummarySelect = `
	SELECT
		b.id, b.name, b.status,
		c.id, c.name, c.edition, c.status, c.sort_order,
		(SELECT count(*) FROM catalog_items count_item
		 WHERE count_item.catalog_id = c.id AND count_item.status <> 'draft'),
		COALESCE(summary.owned_quantity, 0),
		COALESCE(summary.owned_designs, 0),
		COALESCE(summary.available_quantity, 0),
		COALESCE(summary.available_designs, 0),
		COALESCE(summary.wanted_quantity, 0),
		COALESCE(summary.wanted_designs, 0)
	FROM catalogs c
	JOIN brands b ON b.id = c.brand_id
	LEFT JOIN (
		SELECT
			item.catalog_id,
			COALESCE(sum(entry.owned), 0) AS owned_quantity,
			sum(CASE WHEN entry.owned > 0 THEN 1 ELSE 0 END) AS owned_designs,
			COALESCE(sum(entry.available), 0) AS available_quantity,
			sum(CASE WHEN entry.available > 0 THEN 1 ELSE 0 END) AS available_designs,
			COALESCE(sum(entry.wanted), 0) AS wanted_quantity,
			sum(CASE WHEN entry.wanted > 0 THEN 1 ELSE 0 END) AS wanted_designs
		FROM catalog_items item
		LEFT JOIN collection_entries entry
			ON entry.item_id = item.id AND entry.user_id = ?
		WHERE item.status <> 'draft'
		GROUP BY item.catalog_id
	) summary ON summary.catalog_id = c.id`

func validSlug(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			(character == '-' && index > 0) {
			continue
		}
		return false
	}
	return value[len(value)-1] != '-'
}
