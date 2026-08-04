package sqliterepo_test

import "github.com/PatrickQuintal/ooshdaq/internal/domain"

func fixtureManifests() []domain.CatalogManifest {
	return []domain.CatalogManifest{
		manifest(
			"beta-brand",
			"Beta Brand",
			betaCatalogID,
			"Beta Catalog",
			domain.PublicationActive,
			10,
			item(betaCatalogID, betaGiveID, 1, "Beta Give", "Water", domain.PublicationActive),
			item(betaCatalogID, betaReceiveID, 2, "Beta Receive", "Water", domain.PublicationActive),
			itemWithVariant(
				betaCatalogID,
				betaZeroID,
				3,
				"Beta Zero",
				"Stone",
				"Glitter finish",
				domain.PublicationActive,
			),
		),
		manifest(
			"alpha-brand",
			"Alpha Brand",
			alphaCatalogID,
			"Alpha Catalog",
			domain.PublicationActive,
			10,
			item(alphaCatalogID, alphaGiveID, 1, "Alpha Give", "Sky", domain.PublicationActive),
			item(alphaCatalogID, alphaReceiveID, 2, "Alpha Receive", "Earth", domain.PublicationActive),
			retiredItem(alphaCatalogID, alphaRetiredID, 3, "Alpha Retired", "Earth"),
			item(alphaCatalogID, "alpha-catalog-004-draft", 4, "Alpha Draft", "Hidden", domain.PublicationDraft),
		),
		manifest(
			"alpha-brand",
			"Alpha Brand",
			retiredCatalogID,
			"Retired Catalog",
			domain.PublicationRetired,
			30,
			item(retiredCatalogID, retiredItemID, 1, "Retired Kept", "Archive", domain.PublicationActive),
		),
		manifest(
			"draft-brand",
			"Draft Brand",
			"draft-catalog",
			"Draft Catalog",
			domain.PublicationDraft,
			40,
			item("draft-catalog", "draft-catalog-001-hidden", 1, "Hidden", "Hidden", domain.PublicationDraft),
		),
	}
}

func manifest(
	brandID string,
	brandName string,
	catalogID string,
	catalogName string,
	status domain.PublicationStatus,
	sortOrder int,
	items ...domain.CatalogItem,
) domain.CatalogManifest {
	brandStatus := domain.PublicationActive
	if status == domain.PublicationDraft {
		brandStatus = domain.PublicationDraft
	}
	return domain.CatalogManifest{
		SchemaVersion:   domain.CatalogManifestSchemaVersion,
		ManifestVersion: 1,
		Brand: domain.Brand{
			ID:     brandID,
			Name:   brandName,
			Status: brandStatus,
		},
		Catalog: domain.Catalog{
			ID:        catalogID,
			BrandID:   brandID,
			Name:      catalogName,
			Edition:   "2026",
			Status:    status,
			SortOrder: sortOrder,
		},
		Items: items,
	}
}

func item(
	catalogID string,
	id string,
	number int,
	name string,
	universe string,
	status domain.PublicationStatus,
) domain.CatalogItem {
	return domain.CatalogItem{
		ID:        id,
		CatalogID: catalogID,
		Number:    number,
		Name:      name,
		Universe:  universe,
		Status:    status,
		Artwork: domain.Artwork{
			Placeholder: true,
			Rights: domain.ArtworkRights{
				Status: domain.RightsUnknown,
			},
			Provenance: domain.ArtworkProvenance{
				ReviewedAt:      "2026-07-30",
				ReviewReference: "repository-behavior",
				Transformation:  "none",
			},
		},
	}
}

func itemWithVariant(
	catalogID string,
	id string,
	number int,
	name string,
	universe string,
	variant string,
	status domain.PublicationStatus,
) domain.CatalogItem {
	result := item(catalogID, id, number, name, universe, status)
	result.Variant = variant
	return result
}

func retiredItem(
	catalogID string,
	id string,
	number int,
	name string,
	universe string,
) domain.CatalogItem {
	result := item(catalogID, id, number, name, universe, domain.PublicationRetired)
	result.Retirement = &domain.ItemRetirement{
		EffectiveDate: "2026-07-30",
		Reason:        "Retained by the repository behavior fixture.",
	}
	return result
}
