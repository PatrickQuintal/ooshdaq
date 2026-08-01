package sqliterepo

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
)

func TestImportEmbeddedManifestsIsAtomicIdempotentAndDurable(t *testing.T) {
	t.Parallel()

	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	path := filepath.Join(t.TempDir(), "catalogs.sqlite")
	store, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	firstImportTime := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	store.now = func() time.Time { return firstImportTime }
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("ImportManifests: %v", err)
	}
	assertTableCount(t, store, "brands", 4)
	assertTableCount(t, store, "catalogs", 5)
	assertTableCount(t, store, "catalog_items", 56)
	assertTableCount(t, store, "item_assets", 56)
	assertTableCount(t, store, "catalog_imports", 5)

	var importedAt string
	if err := store.database.QueryRow(
		"SELECT imported_at FROM catalog_imports WHERE catalog_id = ?",
		loaded[0].Manifest.Catalog.ID,
	).Scan(&importedAt); err != nil {
		t.Fatalf("read imported_at: %v", err)
	}
	store.now = func() time.Time { return firstImportTime.Add(time.Hour) }
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("repeat ImportManifests: %v", err)
	}
	var repeatedAt string
	if err := store.database.QueryRow(
		"SELECT imported_at FROM catalog_imports WHERE catalog_id = ?",
		loaded[0].Manifest.Catalog.ID,
	).Scan(&repeatedAt); err != nil {
		t.Fatalf("read repeated imported_at: %v", err)
	}
	if repeatedAt != importedAt {
		t.Fatalf("idempotent import changed imported_at from %q to %q", importedAt, repeatedAt)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	assertTableCount(t, reopened, "catalog_items", 56)
}

func TestImportRejectsPartialInvalidSetBeforeMutation(t *testing.T) {
	t.Parallel()

	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	loaded[1].Manifest.Items[0].ID = loaded[0].Manifest.Items[0].ID
	store, err := Open(context.Background(), Config{
		Path: filepath.Join(t.TempDir(), "atomic.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if err := store.ImportManifests(context.Background(), loaded); err == nil {
		t.Fatal("ImportManifests succeeded with a cross-catalog duplicate")
	}
	assertTableCount(t, store, "catalogs", 0)
	assertTableCount(t, store, "catalog_items", 0)
}

func TestImportEnforcesExplicitPermanentRetirement(t *testing.T) {
	t.Parallel()

	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	loaded = loaded[:1]
	store, err := Open(context.Background(), Config{
		Path: filepath.Join(t.TempDir(), "successor.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("initial ImportManifests: %v", err)
	}

	missing := cloneLoadedManifest(loaded[0])
	missing.Manifest.ManifestVersion = 2
	missing.SHA256 = strings.Repeat("1", 64)
	missing.Manifest.Items = missing.Manifest.Items[1:]
	if err := store.ImportManifests(context.Background(), []catalogdata.LoadedManifest{missing}); err == nil ||
		!strings.Contains(err.Error(), "omits existing item") {
		t.Fatalf("missing-item error = %v", err)
	}
	assertImportVersion(t, store, loaded[0].Manifest.Catalog.ID, 1)

	retired := cloneLoadedManifest(loaded[0])
	retired.Manifest.ManifestVersion = 2
	retired.SHA256 = strings.Repeat("2", 64)
	retired.Manifest.Items[0].Status = domain.PublicationRetired
	retired.Manifest.Items[0].Retirement = &domain.ItemRetirement{
		EffectiveDate: "2026-08-01",
		Reason:        "Fixture retirement.",
	}
	if err := store.ImportManifests(context.Background(), []catalogdata.LoadedManifest{retired}); err != nil {
		t.Fatalf("retirement ImportManifests: %v", err)
	}
	assertImportVersion(t, store, loaded[0].Manifest.Catalog.ID, 2)

	reactivated := cloneLoadedManifest(retired)
	reactivated.Manifest.ManifestVersion = 3
	reactivated.SHA256 = strings.Repeat("3", 64)
	reactivated.Manifest.Items[0].Status = domain.PublicationActive
	reactivated.Manifest.Items[0].Retirement = nil
	if err := store.ImportManifests(context.Background(), []catalogdata.LoadedManifest{reactivated}); err == nil ||
		!strings.Contains(err.Error(), "cannot be reactivated") {
		t.Fatalf("reactivation error = %v", err)
	}
	assertImportVersion(t, store, loaded[0].Manifest.Catalog.ID, 2)
}

func TestImportRequiresEverySharedBrandCatalogForBrandChanges(t *testing.T) {
	t.Parallel()

	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	store, err := Open(context.Background(), Config{
		Path: filepath.Join(t.TempDir(), "shared-brand.sqlite"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("initial ImportManifests: %v", err)
	}

	var shared []catalogdata.LoadedManifest
	for _, candidate := range loaded {
		if candidate.Manifest.Brand.ID == "northstar-novelties" {
			updated := cloneLoadedManifest(candidate)
			updated.Manifest.ManifestVersion++
			updated.Manifest.Brand.Name = "Northstar Novelties Studio (fictional)"
			updated.SHA256 = strings.Repeat(
				string(rune('a'+len(shared))),
				64,
			)
			shared = append(shared, updated)
		}
	}
	if len(shared) != 2 {
		t.Fatalf("shared brand fixture catalogs = %d, want 2", len(shared))
	}
	if err := store.ImportManifests(
		context.Background(),
		shared[:1],
	); err == nil || !strings.Contains(err.Error(), "absent from this import") {
		t.Fatalf("partial shared-brand update error = %v", err)
	}
	if err := store.ImportManifests(context.Background(), shared); err != nil {
		t.Fatalf("complete shared-brand update: %v", err)
	}
	var name string
	if err := store.database.QueryRow(
		"SELECT name FROM brands WHERE id = ?",
		"northstar-novelties",
	).Scan(&name); err != nil {
		t.Fatalf("read updated brand: %v", err)
	}
	if name != shared[0].Manifest.Brand.Name {
		t.Fatalf("updated brand name = %q, want %q", name, shared[0].Manifest.Brand.Name)
	}
}

func cloneLoadedManifest(input catalogdata.LoadedManifest) catalogdata.LoadedManifest {
	cloned := input
	cloned.Manifest.Items = append([]domain.CatalogItem(nil), input.Manifest.Items...)
	for index := range cloned.Manifest.Items {
		if input.Manifest.Items[index].Retirement != nil {
			retirement := *input.Manifest.Items[index].Retirement
			cloned.Manifest.Items[index].Retirement = &retirement
		}
	}
	return cloned
}

func assertTableCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.database.QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func assertImportVersion(t *testing.T, store *Store, catalogID string, want int) {
	t.Helper()
	var got int
	if err := store.database.QueryRow(
		"SELECT manifest_version FROM catalog_imports WHERE catalog_id = ?",
		catalogID,
	).Scan(&got); err != nil {
		t.Fatalf("read import version: %v", err)
	}
	if got != want {
		t.Fatalf("import version = %d, want %d", got, want)
	}
}
