package sqliterepo

import (
	"fmt"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

type rawArtwork struct {
	assetURL            string
	placeholder         int
	rightsStatus        string
	license             string
	attribution         string
	permissionReference string
	sourcePageURL       string
	retrievedAt         string
	reviewedAt          string
	reviewReference     string
	transformation      string
	sha256              string
}

func (raw *rawArtwork) scanTargets() []any {
	return []any{
		&raw.assetURL,
		&raw.placeholder,
		&raw.rightsStatus,
		&raw.license,
		&raw.attribution,
		&raw.permissionReference,
		&raw.sourcePageURL,
		&raw.retrievedAt,
		&raw.reviewedAt,
		&raw.reviewReference,
		&raw.transformation,
		&raw.sha256,
	}
}

func (raw rawArtwork) domain() domain.Artwork {
	return domain.Artwork{
		AssetURL:    raw.assetURL,
		Placeholder: raw.placeholder == 1,
		Rights: domain.ArtworkRights{
			Status:              domain.RightsStatus(raw.rightsStatus),
			License:             raw.license,
			Attribution:         raw.attribution,
			PermissionReference: raw.permissionReference,
		},
		Provenance: domain.ArtworkProvenance{
			SourcePageURL:   raw.sourcePageURL,
			RetrievedAt:     raw.retrievedAt,
			ReviewedAt:      raw.reviewedAt,
			ReviewReference: raw.reviewReference,
			Transformation:  raw.transformation,
			SHA256:          raw.sha256,
		},
	}
}

type rawCatalogItem struct {
	brandID       string
	brandName     string
	brandStatus   string
	catalogID     string
	catalogName   string
	edition       string
	catalogStatus string
	sortOrder     int
	itemID        string
	itemNumber    int
	itemName      string
	universe      string
	variant       string
	itemStatus    string
	retiredAt     string
	retiredReason string
	artwork       rawArtwork
}

func (raw *rawCatalogItem) scanTargets() []any {
	targets := []any{
		&raw.brandID,
		&raw.brandName,
		&raw.brandStatus,
		&raw.catalogID,
		&raw.catalogName,
		&raw.edition,
		&raw.catalogStatus,
		&raw.sortOrder,
		&raw.itemID,
		&raw.itemNumber,
		&raw.itemName,
		&raw.universe,
		&raw.variant,
		&raw.itemStatus,
		&raw.retiredAt,
		&raw.retiredReason,
	}
	return append(targets, raw.artwork.scanTargets()...)
}

func (raw rawCatalogItem) record() repository.CatalogItemRecord {
	item := domain.CatalogItem{
		ID:        raw.itemID,
		CatalogID: raw.catalogID,
		Number:    raw.itemNumber,
		Name:      raw.itemName,
		Universe:  raw.universe,
		Variant:   raw.variant,
		Status:    domain.PublicationStatus(raw.itemStatus),
		Artwork:   raw.artwork.domain(),
	}
	if raw.retiredAt != "" || raw.retiredReason != "" {
		item.Retirement = &domain.ItemRetirement{
			EffectiveDate: raw.retiredAt,
			Reason:        raw.retiredReason,
		}
	}
	return repository.CatalogItemRecord{
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
		Item: item,
	}
}

var catalogItemColumns = catalogItemColumnsFor("b", "c", "i", "a")

func catalogItemColumnsFor(brand, catalog, item, asset string) string {
	return fmt.Sprintf(`
		%[1]s.id, %[1]s.name, %[1]s.status,
		%[2]s.id, %[2]s.name, %[2]s.edition, %[2]s.status, %[2]s.sort_order,
		%[3]s.id, %[3]s.number, %[3]s.name, %[3]s.universe, %[3]s.variant, %[3]s.status,
		%[3]s.retirement_effective_date, %[3]s.retirement_reason,
		%[4]s.asset_url, %[4]s.placeholder, %[4]s.rights_status, %[4]s.license,
		%[4]s.attribution, %[4]s.permission_reference, %[4]s.source_page_url,
		%[4]s.retrieved_at, %[4]s.reviewed_at, %[4]s.review_reference,
		%[4]s.transformation, %[4]s.sha256`,
		brand,
		catalog,
		item,
		asset,
	)
}

func optionalCatalogItemColumnsFor(brand, catalog, item, asset string) string {
	return fmt.Sprintf(`
		COALESCE(%[1]s.id, ''), COALESCE(%[1]s.name, ''), COALESCE(%[1]s.status, ''),
		COALESCE(%[2]s.id, ''), COALESCE(%[2]s.name, ''), COALESCE(%[2]s.edition, ''),
		COALESCE(%[2]s.status, ''), COALESCE(%[2]s.sort_order, 0),
		COALESCE(%[3]s.id, ''), COALESCE(%[3]s.number, 0), COALESCE(%[3]s.name, ''),
		COALESCE(%[3]s.universe, ''), COALESCE(%[3]s.variant, ''),
		COALESCE(%[3]s.status, ''), COALESCE(%[3]s.retirement_effective_date, ''),
		COALESCE(%[3]s.retirement_reason, ''),
		COALESCE(%[4]s.asset_url, ''), COALESCE(%[4]s.placeholder, 0),
		COALESCE(%[4]s.rights_status, ''), COALESCE(%[4]s.license, ''),
		COALESCE(%[4]s.attribution, ''), COALESCE(%[4]s.permission_reference, ''),
		COALESCE(%[4]s.source_page_url, ''), COALESCE(%[4]s.retrieved_at, ''),
		COALESCE(%[4]s.reviewed_at, ''), COALESCE(%[4]s.review_reference, ''),
		COALESCE(%[4]s.transformation, ''), COALESCE(%[4]s.sha256, '')`,
		brand,
		catalog,
		item,
		asset,
	)
}
