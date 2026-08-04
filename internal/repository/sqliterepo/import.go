package sqliterepo

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
)

// ImportManifests validates the complete reviewed set, then imports every
// changed manifest in one transaction. Repeating identical input is a no-op.
func (store *Store) ImportManifests(
	ctx context.Context,
	loaded []catalogdata.LoadedManifest,
) error {
	if err := validateLoadedManifests(loaded); err != nil {
		return err
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	_, err := store.importManifestsLocked(ctx, loaded)
	return err
}

func (store *Store) importManifestsLocked(
	ctx context.Context,
	loaded []catalogdata.LoadedManifest,
) (map[string]bool, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, classify("begin catalog import", err)
	}
	defer transaction.Rollback()

	unchanged, err := validateImportAgainstDatabase(ctx, transaction, loaded)
	if err != nil {
		return nil, err
	}
	importedAt := store.now().UTC().Format(time.RFC3339Nano)
	for _, candidate := range loaded {
		if unchanged[candidate.Manifest.Catalog.ID] {
			continue
		}
		if err := importManifest(ctx, transaction, candidate, importedAt); err != nil {
			return nil, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return nil, classify("commit catalog import", err)
	}
	return unchanged, nil
}

func validateLoadedManifests(loaded []catalogdata.LoadedManifest) error {
	manifests := make([]domain.CatalogManifest, 0, len(loaded))
	for index, candidate := range loaded {
		if strings.TrimSpace(candidate.Path) == "" {
			return fmt.Errorf("catalog manifest %d has no reviewed path", index)
		}
		decoded, err := hex.DecodeString(candidate.SHA256)
		if err != nil || len(decoded) != 32 || strings.ToLower(candidate.SHA256) != candidate.SHA256 {
			return fmt.Errorf("catalog manifest %d has an invalid SHA-256 digest", index)
		}
		manifests = append(manifests, candidate.Manifest)
	}
	if err := catalogdata.ValidateManifestSet(manifests); err != nil {
		return fmt.Errorf("validate catalog import set: %w", err)
	}
	return nil
}

func importManifest(
	ctx context.Context,
	transaction *sql.Tx,
	candidate catalogdata.LoadedManifest,
	importedAt string,
) error {
	manifest := candidate.Manifest
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO brands(id, name, status) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, status = excluded.status`,
		manifest.Brand.ID,
		manifest.Brand.Name,
		manifest.Brand.Status,
	); err != nil {
		return fmt.Errorf("import brand %q: %w", manifest.Brand.ID, err)
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO catalogs(id, brand_id, name, edition, status, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			edition = excluded.edition,
			status = excluded.status`,
		manifest.Catalog.ID,
		manifest.Catalog.BrandID,
		manifest.Catalog.Name,
		manifest.Catalog.Edition,
		manifest.Catalog.Status,
		manifest.Catalog.SortOrder,
	); err != nil {
		return fmt.Errorf("import catalog %q: %w", manifest.Catalog.ID, err)
	}
	if err := importCatalogSource(ctx, transaction, manifest); err != nil {
		return err
	}
	for _, item := range manifest.Items {
		if err := importItem(ctx, transaction, item); err != nil {
			return err
		}
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO catalog_imports(catalog_id, manifest_version, manifest_digest, imported_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(catalog_id) DO UPDATE SET
			manifest_version = excluded.manifest_version,
			manifest_digest = excluded.manifest_digest,
			imported_at = excluded.imported_at`,
		manifest.Catalog.ID,
		manifest.ManifestVersion,
		candidate.SHA256,
		importedAt,
	); err != nil {
		return fmt.Errorf("record catalog import %q: %w", manifest.Catalog.ID, err)
	}
	return nil
}

func importCatalogSource(
	ctx context.Context,
	transaction *sql.Tx,
	manifest domain.CatalogManifest,
) error {
	artwork := manifest.Items[0].Artwork
	provenance := artwork.Provenance
	rights := artwork.Rights
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO catalog_sources(
			catalog_id, source_page_url, retrieved_at, reviewed_at,
			review_reference, attribution, rights_status, license
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(catalog_id) DO UPDATE SET
			source_page_url = excluded.source_page_url,
			retrieved_at = excluded.retrieved_at,
			reviewed_at = excluded.reviewed_at,
			review_reference = excluded.review_reference,
			attribution = excluded.attribution,
			rights_status = excluded.rights_status,
			license = excluded.license`,
		manifest.Catalog.ID,
		provenance.SourcePageURL,
		provenance.RetrievedAt,
		provenance.ReviewedAt,
		provenance.ReviewReference,
		rights.Attribution,
		rights.Status,
		rights.License,
	); err != nil {
		return fmt.Errorf("import catalog source %q: %w", manifest.Catalog.ID, err)
	}
	return nil
}

func importItem(ctx context.Context, transaction *sql.Tx, item domain.CatalogItem) error {
	retirementDate, retirementReason := "", ""
	if item.Retirement != nil {
		retirementDate = item.Retirement.EffectiveDate
		retirementReason = item.Retirement.Reason
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO catalog_items(
			id, catalog_id, number, name, universe, variant, status,
			retirement_effective_date, retirement_reason
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			universe = excluded.universe,
			variant = excluded.variant,
			status = excluded.status,
			retirement_effective_date = excluded.retirement_effective_date,
			retirement_reason = excluded.retirement_reason`,
		item.ID,
		item.CatalogID,
		item.Number,
		item.Name,
		item.Universe,
		item.Variant,
		item.Status,
		retirementDate,
		retirementReason,
	); err != nil {
		return fmt.Errorf("import catalog item %q: %w", item.ID, err)
	}
	artwork := item.Artwork
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO item_assets(
			item_id, asset_url, placeholder, rights_status, license, attribution,
			permission_reference, source_page_url, retrieved_at, reviewed_at,
			review_reference, transformation, sha256
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(item_id) DO UPDATE SET
			asset_url = excluded.asset_url,
			placeholder = excluded.placeholder,
			rights_status = excluded.rights_status,
			license = excluded.license,
			attribution = excluded.attribution,
			permission_reference = excluded.permission_reference,
			source_page_url = excluded.source_page_url,
			retrieved_at = excluded.retrieved_at,
			reviewed_at = excluded.reviewed_at,
			review_reference = excluded.review_reference,
			transformation = excluded.transformation,
			sha256 = excluded.sha256`,
		item.ID,
		artwork.AssetURL,
		artwork.Placeholder,
		artwork.Rights.Status,
		artwork.Rights.License,
		artwork.Rights.Attribution,
		artwork.Rights.PermissionReference,
		artwork.Provenance.SourcePageURL,
		artwork.Provenance.RetrievedAt,
		artwork.Provenance.ReviewedAt,
		artwork.Provenance.ReviewReference,
		artwork.Provenance.Transformation,
		artwork.Provenance.SHA256,
	); err != nil {
		return fmt.Errorf("import artwork for item %q: %w", item.ID, err)
	}
	return nil
}

func queryOptionalString(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	query string,
	arguments ...any,
) (string, bool, error) {
	var value string
	err := queryer.QueryRowContext(ctx, query, arguments...).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}
