package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
)

func TestLoadEmbeddedManifests(t *testing.T) {
	t.Parallel()

	loaded, err := LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("manifest count = %d, want 1", len(loaded))
	}
	if got := loaded[0].Manifest.Catalog.ID; got != "disney-ooshies-2026" {
		t.Fatalf("published catalog = %q, want disney-ooshies-2026", got)
	}

	again, err := LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("second LoadEmbeddedManifests() error = %v", err)
	}
	loaded[0].Manifest.Items[0].Name = "mutated caller copy"
	if again[0].Manifest.Items[0].Name == "mutated caller copy" {
		t.Fatal("LoadEmbeddedManifests returned shared mutable manifest state")
	}
}

func TestLoadEmbeddedDemoManifests(t *testing.T) {
	t.Parallel()

	loaded, err := LoadEmbeddedDemoManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedDemoManifests() error = %v", err)
	}
	wantCatalogs := []string{
		"disney-ooshies-2026",
		"pocket-constellations-2026",
		"tiny-weather-2026",
		"harbor-signal-pins-2026",
		"garden-sprites-2026",
	}
	if len(loaded) != len(wantCatalogs) {
		t.Fatalf("manifest count = %d, want %d", len(loaded), len(wantCatalogs))
	}

	itemIDs := make(map[string]string)
	fictionalBrands := make(map[string]struct{})
	for index, loadedManifest := range loaded {
		manifest := loadedManifest.Manifest
		if manifest.Catalog.ID != wantCatalogs[index] {
			t.Errorf("catalog %d = %q, want %q", index, manifest.Catalog.ID, wantCatalogs[index])
		}
		if manifest.ManifestVersion != 1 {
			t.Errorf("%s version = %d, want 1", manifest.Catalog.ID, manifest.ManifestVersion)
		}
		if len(loadedManifest.SHA256) != sha256.Size*2 {
			t.Errorf("%s digest length = %d, want %d", manifest.Catalog.ID, len(loadedManifest.SHA256), sha256.Size*2)
		}
		raw, err := embeddedManifestFS.ReadFile(loadedManifest.Path)
		if err != nil {
			t.Fatalf("read embedded %s: %v", loadedManifest.Path, err)
		}
		digest := sha256.Sum256(raw)
		if want := hex.EncodeToString(digest[:]); loadedManifest.SHA256 != want {
			t.Errorf("%s digest = %q, want %q", manifest.Catalog.ID, loadedManifest.SHA256, want)
		}
		for itemIndex, item := range manifest.Items {
			if itemIndex > 0 && manifest.Items[itemIndex-1].Number >= item.Number {
				t.Errorf("%s items are not in increasing number order", manifest.Catalog.ID)
			}
			if otherCatalog, duplicate := itemIDs[item.ID]; duplicate {
				t.Errorf("item %q appears in %s and %s", item.ID, otherCatalog, manifest.Catalog.ID)
			}
			itemIDs[item.ID] = manifest.Catalog.ID
		}
		if manifest.Brand.ID != "woolworths-ooshies" {
			fictionalBrands[manifest.Brand.ID] = struct{}{}
			assertDemoPlaceholderCatalog(t, manifest)
		}
	}
	if len(fictionalBrands) != 3 {
		t.Errorf("fictional brand count = %d, want 3", len(fictionalBrands))
	}

	again, err := LoadEmbeddedDemoManifests()
	if err != nil {
		t.Fatalf("second LoadEmbeddedDemoManifests() error = %v", err)
	}
	loaded[0].Manifest.Items[0].Name = "mutated caller copy"
	if again[0].Manifest.Items[0].Name == "mutated caller copy" {
		t.Fatal("LoadEmbeddedDemoManifests returned shared mutable manifest state")
	}
}

func TestDisneyOoshiesManifestPreservesReviewedCatalog(t *testing.T) {
	t.Parallel()

	loaded, err := LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests() error = %v", err)
	}
	manifest := loaded[0].Manifest
	wantIDs := []string{
		"disney-2026-01-moana",
		"disney-2026-02-maui",
		"disney-2026-03-pua",
		"disney-2026-04-hei-hei",
		"disney-2026-05-kakamora",
		"disney-2026-06-elsa",
		"disney-2026-07-anna",
		"disney-2026-08-stitch",
		"disney-2026-09-lilo",
		"disney-2026-10-judy-hopps",
		"pixar-2026-01-jessie",
		"pixar-2026-02-woody",
		"pixar-2026-03-buzz-lightyear",
		"pixar-2026-04-bullseye",
		"pixar-2026-05-lilypad",
		"pixar-2026-06-mike",
		"pixar-2026-07-anxiety",
		"pixar-2026-08-sulley",
		"pixar-2026-09-joy",
		"pixar-2026-10-lightning-mcqueen",
		"marvel-2026-01-spider-man",
		"marvel-2026-02-hulk",
		"marvel-2026-03-tombstone",
		"marvel-2026-04-scorpion",
		"marvel-2026-05-tarantula",
		"marvel-2026-06-the-thing",
		"marvel-2026-07-wolverine",
		"marvel-2026-08-doctor-doom",
		"marvel-2026-09-thor",
		"marvel-2026-10-gambit",
		"star-wars-2026-01-mandalorian",
		"star-wars-2026-02-grogu",
		"star-wars-2026-03-anzellan",
		"star-wars-2026-04-zeb",
		"star-wars-2026-05-stormtrooper",
		"star-wars-2026-06-darth-vader",
		"star-wars-2026-07-c-3po",
		"star-wars-2026-08-princess-leia",
		"star-wars-2026-09-luke-skywalker",
		"star-wars-2026-10-r2-d2",
	}
	if len(manifest.Items) != len(wantIDs) {
		t.Fatalf("Disney Ooshies items = %d, want %d", len(manifest.Items), len(wantIDs))
	}

	const (
		assetPrefix = "https://cdn0.woolworths.media/content/content/dot/"
		sourcePage  = "https://www.woolworths.com.au/shop/discover/disney-ooshies"
	)
	for index, item := range manifest.Items {
		if item.ID != wantIDs[index] {
			t.Errorf("item %d id = %q, want %q", index, item.ID, wantIDs[index])
		}
		if !strings.HasPrefix(item.Artwork.AssetURL, assetPrefix) {
			t.Errorf("%s asset URL = %q, want reviewed Woolworths host", item.ID, item.Artwork.AssetURL)
		}
		if item.Artwork.Placeholder {
			t.Errorf("%s unexpectedly uses placeholder", item.ID)
		}
		if item.Artwork.Rights.Status != domain.RightsSourceHostedLink {
			t.Errorf("%s rights = %q, want source-hosted-link", item.ID, item.Artwork.Rights.Status)
		}
		if item.Artwork.Provenance.SourcePageURL != sourcePage ||
			item.Artwork.Provenance.ReviewReference != "catalog-maintainers" {
			t.Errorf("%s provenance does not reference the reviewed source", item.ID)
		}
	}

	assertAssetURL(t, manifest, "disney-2026-07-anna", assetPrefix+"ooshies-2026-character-disney-ana-v3.png")
	assertAssetURL(t, manifest, "star-wars-2026-07-c-3po", assetPrefix+"ooshies-2026-character-star-wars-c3po-v3.png")
	assertAssetURL(t, manifest, "star-wars-2026-10-r2-d2", assetPrefix+"ooshies-2026-character-star-wars-r2d2-v3.png")
}

func TestLoadManifestFilesRejectsPartialOrLooseInput(t *testing.T) {
	t.Parallel()

	valid := marshalManifest(t, testManifest("catalog-one", "item-one", 1))
	tests := []struct {
		name string
		fsys fstest.MapFS
	}{
		{
			name: "unknown field",
			fsys: fstest.MapFS{
				"one.json": &fstest.MapFile{
					Data: []byte(strings.Replace(
						string(valid),
						`"schema_version":1`,
						`"unexpected":true,"schema_version":1`,
						1,
					)),
				},
			},
		},
		{
			name: "multiple JSON values",
			fsys: fstest.MapFS{
				"one.json": &fstest.MapFile{Data: append(append([]byte(nil), valid...), []byte("\n{}")...)},
			},
		},
		{
			name: "invalid second manifest returns no partial set",
			fsys: fstest.MapFS{
				"one.json": &fstest.MapFile{Data: valid},
				"two.json": &fstest.MapFile{Data: []byte(`{"schema_version":1}`)},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadManifestFiles(tt.fsys, "*.json")
			if err == nil {
				t.Fatal("loadManifestFiles() succeeded, want error")
			}
			if got != nil {
				t.Fatalf("loadManifestFiles() returned %d partial manifests, want nil", len(got))
			}
		})
	}
}

func TestLoadManifestFilesSortsCatalogsAndItems(t *testing.T) {
	t.Parallel()

	later := testManifest("catalog-later", "later-item-02", 20)
	later.Items = append(later.Items, later.Items[0])
	later.Items[0].ID = "later-item-02"
	later.Items[0].Number = 2
	later.Items[1].ID = "later-item-01"
	later.Items[1].Number = 1
	earlier := testManifest("catalog-earlier", "earlier-item", 10)

	loaded, err := loadManifestFiles(fstest.MapFS{
		"a-later.json":   &fstest.MapFile{Data: marshalManifest(t, later)},
		"z-earlier.json": &fstest.MapFile{Data: marshalManifest(t, earlier)},
	}, "*.json")
	if err != nil {
		t.Fatalf("loadManifestFiles() error = %v", err)
	}
	if loaded[0].Manifest.Catalog.ID != "catalog-earlier" ||
		loaded[1].Manifest.Catalog.ID != "catalog-later" {
		t.Fatalf("catalog order = %q, %q", loaded[0].Manifest.Catalog.ID, loaded[1].Manifest.Catalog.ID)
	}
	if loaded[1].Manifest.Items[0].Number != 1 || loaded[1].Manifest.Items[1].Number != 2 {
		t.Fatalf("item order = %d, %d", loaded[1].Manifest.Items[0].Number, loaded[1].Manifest.Items[1].Number)
	}
}

func TestValidateManifestSetCrossCatalogRules(t *testing.T) {
	t.Parallel()

	first := testManifest("catalog-one", "item-one", 1)
	second := testManifest("catalog-two", "item-two", 2)

	if err := ValidateManifestSet([]domain.CatalogManifest{first, second}); err != nil {
		t.Fatalf("valid shared-brand set rejected: %v", err)
	}

	tests := []struct {
		name   string
		change func(*domain.CatalogManifest)
		match  string
	}{
		{
			name: "duplicate catalog id",
			change: func(m *domain.CatalogManifest) {
				m.Catalog.ID = first.Catalog.ID
				m.Items[0].CatalogID = first.Catalog.ID
			},
			match: "duplicate catalog id",
		},
		{
			name:   "globally duplicate item id",
			change: func(m *domain.CatalogManifest) { m.Items[0].ID = first.Items[0].ID },
			match:  "already used by catalog",
		},
		{
			name:   "conflicting brand definition",
			change: func(m *domain.CatalogManifest) { m.Brand.Name = "Changed Brand Name" },
			match:  "conflicts with another manifest",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			changed := second
			changed.Items = append([]domain.CatalogItem(nil), second.Items...)
			tt.change(&changed)
			err := ValidateManifestSet([]domain.CatalogManifest{first, changed})
			if err == nil || !strings.Contains(err.Error(), tt.match) {
				t.Fatalf("ValidateManifestSet() error = %v, want text %q", err, tt.match)
			}
		})
	}

	if err := ValidateManifestSet(nil); err == nil {
		t.Fatal("ValidateManifestSet(nil) succeeded")
	}
}

func assertDemoPlaceholderCatalog(t *testing.T, manifest domain.CatalogManifest) {
	t.Helper()
	for _, item := range manifest.Items {
		if !item.Artwork.Placeholder || item.Artwork.AssetURL != "" ||
			item.Artwork.Rights.Status != domain.RightsUnknown ||
			item.Artwork.Provenance.ReviewReference != "fictional-fixture-no-artwork" {
			t.Errorf("%s does not use explicit unknown-rights placeholder artwork", item.ID)
		}
	}
}

func assertAssetURL(t *testing.T, manifest domain.CatalogManifest, id, want string) {
	t.Helper()
	for _, item := range manifest.Items {
		if item.ID == id {
			if item.Artwork.AssetURL != want {
				t.Errorf("%s asset URL = %q, want %q", id, item.Artwork.AssetURL, want)
			}
			return
		}
	}
	t.Errorf("item %q not found", id)
}

func testManifest(catalogID, itemID string, sortOrder int) domain.CatalogManifest {
	return domain.CatalogManifest{
		SchemaVersion:   domain.CatalogManifestSchemaVersion,
		ManifestVersion: 1,
		Brand: domain.Brand{
			ID:     "test-brand",
			Name:   "Test Brand",
			Status: domain.PublicationActive,
		},
		Catalog: domain.Catalog{
			ID:        catalogID,
			BrandID:   "test-brand",
			Name:      fmt.Sprintf("Catalog %d", sortOrder),
			Edition:   "2026",
			Status:    domain.PublicationActive,
			SortOrder: sortOrder,
		},
		Items: []domain.CatalogItem{
			{
				ID:        itemID,
				CatalogID: catalogID,
				Number:    1,
				Name:      "Test Item",
				Status:    domain.PublicationActive,
				Artwork: domain.Artwork{
					Placeholder: true,
					Rights: domain.ArtworkRights{
						Status: domain.RightsUnknown,
					},
					Provenance: domain.ArtworkProvenance{
						ReviewedAt:      "2026-07-30",
						ReviewReference: "test-fixture",
						Transformation:  "none",
					},
				},
			},
		},
	}
}

func marshalManifest(t *testing.T, manifest domain.CatalogManifest) []byte {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return raw
}
