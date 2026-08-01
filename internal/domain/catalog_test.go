package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestCatalogManifestValidate(t *testing.T) {
	t.Parallel()

	if err := validCatalogManifest().Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	tests := []struct {
		name   string
		change func(*CatalogManifest)
		field  string
	}{
		{
			name:   "unsupported schema",
			change: func(m *CatalogManifest) { m.SchemaVersion = 2 },
			field:  "schema_version",
		},
		{
			name:   "nonpositive manifest version",
			change: func(m *CatalogManifest) { m.ManifestVersion = 0 },
			field:  "manifest_version",
		},
		{
			name:   "invalid brand id",
			change: func(m *CatalogManifest) { m.Brand.ID = "Brand" },
			field:  "brand.id",
		},
		{
			name:   "invalid brand status",
			change: func(m *CatalogManifest) { m.Brand.Status = "" },
			field:  "brand.status",
		},
		{
			name:   "catalog brand mismatch",
			change: func(m *CatalogManifest) { m.Catalog.BrandID = "other-brand" },
			field:  "catalog.brand_id",
		},
		{
			name:   "nonpositive catalog sort order",
			change: func(m *CatalogManifest) { m.Catalog.SortOrder = 0 },
			field:  "catalog.sort_order",
		},
		{
			name:   "empty items",
			change: func(m *CatalogManifest) { m.Items = nil },
			field:  "items",
		},
		{
			name:   "item catalog mismatch",
			change: func(m *CatalogManifest) { m.Items[0].CatalogID = "other-catalog" },
			field:  "items[0].catalog_id",
		},
		{
			name: "duplicate globally unique item id",
			change: func(m *CatalogManifest) {
				duplicate := m.Items[0]
				duplicate.Number = 2
				m.Items = append(m.Items, duplicate)
			},
			field: "items[1].id",
		},
		{
			name: "duplicate catalog-local number",
			change: func(m *CatalogManifest) {
				duplicate := m.Items[0]
				duplicate.ID = "test-catalog-2026-02-nova"
				m.Items = append(m.Items, duplicate)
			},
			field: "items[1].number",
		},
		{
			name:   "invalid item UTF-8",
			change: func(m *CatalogManifest) { m.Items[0].Name = string([]byte{0xff}) },
			field:  "items[0].name",
		},
		{
			name: "overlong item name",
			change: func(m *CatalogManifest) {
				m.Items[0].Name = strings.Repeat("x", maxCatalogNameBytes+1)
			},
			field: "items[0].name",
		},
		{
			name:   "control character",
			change: func(m *CatalogManifest) { m.Items[0].Universe = "line\nbreak" },
			field:  "items[0].universe",
		},
		{
			name:   "surrounding whitespace",
			change: func(m *CatalogManifest) { m.Catalog.Edition = " 2026" },
			field:  "catalog.edition",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := validCatalogManifest()
			tt.change(&manifest)
			assertValidationField(t, manifest.Validate(), tt.field)
		})
	}
}

func TestArtworkValidateRightsAndSafeURLs(t *testing.T) {
	t.Parallel()

	for name, artwork := range map[string]Artwork{
		"explicit unknown placeholder": unknownArtwork(),
		"reviewed source-hosted link":  sourceHostedArtwork(),
	} {
		name, artwork := name, artwork
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := artwork.Validate(); err != nil {
				t.Fatalf("valid artwork rejected: %v", err)
			}
		})
	}

	tests := []struct {
		name    string
		artwork func() Artwork
		change  func(*Artwork)
		field   string
	}{
		{
			name:    "HTTP asset",
			artwork: sourceHostedArtwork,
			change:  func(a *Artwork) { a.AssetURL = "http://example.invalid/item.png" },
			field:   "asset_url",
		},
		{
			name:    "asset credentials",
			artwork: sourceHostedArtwork,
			change:  func(a *Artwork) { a.AssetURL = "https://user@example.invalid/item.png" },
			field:   "asset_url",
		},
		{
			name:    "asset fragment",
			artwork: sourceHostedArtwork,
			change:  func(a *Artwork) { a.AssetURL = "https://example.invalid/item.png#fragment" },
			field:   "asset_url",
		},
		{
			name:    "unsafe source page",
			artwork: sourceHostedArtwork,
			change:  func(a *Artwork) { a.Provenance.SourcePageURL = "javascript:alert(1)" },
			field:   "provenance.source_page_url",
		},
		{
			name:    "unknown rights publish URL",
			artwork: unknownArtwork,
			change:  func(a *Artwork) { a.AssetURL = "https://example.invalid/item.png" },
			field:   "asset_url",
		},
		{
			name:    "unknown rights without placeholder",
			artwork: unknownArtwork,
			change:  func(a *Artwork) { a.Placeholder = false },
			field:   "placeholder",
		},
		{
			name:    "publishable artwork without URL",
			artwork: sourceHostedArtwork,
			change:  func(a *Artwork) { a.AssetURL = "" },
			field:   "asset_url",
		},
		{
			name:    "source-hosted link without license statement",
			artwork: sourceHostedArtwork,
			change:  func(a *Artwork) { a.Rights.License = "" },
			field:   "rights.license",
		},
		{
			name:    "permissioned link without permission reference",
			artwork: sourceHostedArtwork,
			change: func(a *Artwork) {
				a.Rights.Status = RightsPermissionedLink
				a.Rights.License = ""
			},
			field: "rights.permission_reference",
		},
		{
			name:    "invalid reviewed date",
			artwork: unknownArtwork,
			change:  func(a *Artwork) { a.Provenance.ReviewedAt = "30 July 2026" },
			field:   "provenance.reviewed_at",
		},
		{
			name:    "retrieval date without source",
			artwork: unknownArtwork,
			change:  func(a *Artwork) { a.Provenance.RetrievedAt = "2026-07-30" },
			field:   "provenance.retrieved_at",
		},
		{
			name:    "invalid digest",
			artwork: unknownArtwork,
			change:  func(a *Artwork) { a.Provenance.SHA256 = strings.Repeat("A", 64) },
			field:   "provenance.sha256",
		},
		{
			name:    "overlong attribution",
			artwork: sourceHostedArtwork,
			change: func(a *Artwork) {
				a.Rights.Attribution = strings.Repeat("x", maxRightsTextBytes+1)
			},
			field: "rights.attribution",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			artwork := tt.artwork()
			tt.change(&artwork)
			assertValidationField(t, artwork.Validate(), tt.field)
		})
	}
}

func TestCatalogItemRetirementIsExplicit(t *testing.T) {
	t.Parallel()

	valid := validCatalogManifest().Items[0]
	retired := valid
	retired.Status = PublicationRetired
	retired.Retirement = &ItemRetirement{
		EffectiveDate: "2026-08-01",
		Reason:        "Retained after the catalog publisher withdrew the item.",
	}
	if err := retired.Validate(); err != nil {
		t.Fatalf("valid retired item rejected: %v", err)
	}

	missingDirective := valid
	missingDirective.Status = PublicationRetired
	assertValidationField(t, missingDirective.Validate(), "retirement")

	activeWithDirective := valid
	activeWithDirective.Retirement = retired.Retirement
	assertValidationField(t, activeWithDirective.Validate(), "retirement")

	invalidDate := retired
	invalidDate.Retirement = &ItemRetirement{EffectiveDate: "2026-02-30", Reason: "Withdrawn"}
	assertValidationField(t, invalidDate.Validate(), "retirement.effective_date")
}

func TestCatalogManifestValidateSuccessor(t *testing.T) {
	t.Parallel()

	previous := validCatalogManifest()
	successor := validCatalogManifest()
	successor.ManifestVersion = 2
	successor.Items[0].Name = "Comet Cat, renamed"
	successor.Items[0].Status = PublicationRetired
	successor.Items[0].Retirement = &ItemRetirement{
		EffectiveDate: "2026-08-01",
		Reason:        "Explicitly retired while preserving the item ID.",
	}
	if err := successor.ValidateSuccessor(previous); err != nil {
		t.Fatalf("valid successor rejected: %v", err)
	}

	t.Run("missing item is not an implicit retirement", func(t *testing.T) {
		next := validCatalogManifest()
		next.ManifestVersion = 2
		next.Items = append(next.Items, CatalogItem{
			ID:        "test-catalog-2026-02-nova",
			CatalogID: next.Catalog.ID,
			Number:    2,
			Name:      "Nova Newt",
			Status:    PublicationActive,
			Artwork:   unknownArtwork(),
		})
		next.Items = next.Items[1:]
		assertValidationField(t, next.ValidateSuccessor(previous), "items")
	})

	t.Run("version must increase", func(t *testing.T) {
		next := validCatalogManifest()
		assertValidationField(t, next.ValidateSuccessor(previous), "manifest_version")
	})

	t.Run("retirement is permanent", func(t *testing.T) {
		retiredPrevious := successor
		next := validCatalogManifest()
		next.ManifestVersion = 3
		assertValidationField(t, next.ValidateSuccessor(retiredPrevious), "items")
	})

	t.Run("catalog identity is immutable", func(t *testing.T) {
		next := validCatalogManifest()
		next.ManifestVersion = 2
		next.Catalog.ID = "replacement-catalog"
		next.Items[0].CatalogID = next.Catalog.ID
		assertValidationField(t, next.ValidateSuccessor(previous), "catalog.id")
	})

	t.Run("published catalog order is immutable", func(t *testing.T) {
		next := validCatalogManifest()
		next.ManifestVersion = 2
		next.Catalog.SortOrder = 2
		assertValidationField(t, next.ValidateSuccessor(previous), "catalog.sort_order")
	})

	t.Run("published item number is immutable", func(t *testing.T) {
		next := validCatalogManifest()
		next.ManifestVersion = 2
		next.Items[0].Number = 2
		assertValidationField(t, next.ValidateSuccessor(previous), "items")
	})

	t.Run("draft ordering is still stable across reviewed versions", func(t *testing.T) {
		draft := validCatalogManifest()
		draft.Catalog.Status = PublicationDraft
		draft.Items[0].Status = PublicationDraft
		next := validCatalogManifest()
		next.ManifestVersion = 2
		next.Catalog.SortOrder = 2
		next.Items[0].Number = 2
		assertValidationField(t, next.ValidateSuccessor(draft), "catalog.sort_order")
	})
}

func TestPublishedCatalogRequiresPublishedItem(t *testing.T) {
	t.Parallel()

	manifest := validCatalogManifest()
	manifest.Items[0].Status = PublicationDraft
	assertValidationField(t, manifest.Validate(), "items")

	manifest.Catalog.Status = PublicationDraft
	if err := manifest.Validate(); err != nil {
		t.Fatalf("draft catalog with only draft items rejected: %v", err)
	}
}

func validCatalogManifest() CatalogManifest {
	return CatalogManifest{
		SchemaVersion:   CatalogManifestSchemaVersion,
		ManifestVersion: 1,
		Brand: Brand{
			ID:     "test-brand",
			Name:   "Test Brand",
			Status: PublicationActive,
		},
		Catalog: Catalog{
			ID:        "test-catalog-2026",
			BrandID:   "test-brand",
			Name:      "Test Catalog",
			Edition:   "2026",
			Status:    PublicationActive,
			SortOrder: 1,
		},
		Items: []CatalogItem{
			{
				ID:        "test-catalog-2026-01-comet",
				CatalogID: "test-catalog-2026",
				Number:    1,
				Name:      "Comet Cat",
				Universe:  "Night Sky",
				Status:    PublicationActive,
				Artwork:   unknownArtwork(),
			},
		},
	}
}

func unknownArtwork() Artwork {
	return Artwork{
		Placeholder: true,
		Rights: ArtworkRights{
			Status: RightsUnknown,
		},
		Provenance: ArtworkProvenance{
			ReviewedAt:      "2026-07-30",
			ReviewReference: "fictional-fixture-policy",
			Transformation:  "none",
		},
	}
}

func sourceHostedArtwork() Artwork {
	return Artwork{
		AssetURL: "https://cdn.example.invalid/catalog/item.png",
		Rights: ArtworkRights{
			Status:      RightsSourceHostedLink,
			License:     "No copying or redistribution license identified",
			Attribution: "Source-hosted example artwork",
		},
		Provenance: ArtworkProvenance{
			SourcePageURL:   "https://example.invalid/catalog",
			RetrievedAt:     "2026-07-30",
			ReviewedAt:      "2026-07-30",
			ReviewReference: "review-example",
			Transformation:  "none",
		},
	}
}

func assertValidationField(t *testing.T, err error, want string) {
	t.Helper()

	var validationError ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error = %T %v, want ValidationError for %q", err, err, want)
	}
	if validationError.Field != want {
		t.Fatalf("validation field = %q, want %q (error: %v)", validationError.Field, want, err)
	}
}
