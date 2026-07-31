package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func validateImportAgainstDatabase(
	ctx context.Context,
	transaction *sql.Tx,
	loaded []catalogdata.LoadedManifest,
) (map[string]bool, error) {
	unchanged := make(map[string]bool, len(loaded))
	incomingCatalogs := make(
		map[string]catalogdata.LoadedManifest,
		len(loaded),
	)
	for _, candidate := range loaded {
		incomingCatalogs[candidate.Manifest.Catalog.ID] = candidate
	}
	for _, candidate := range loaded {
		manifest := candidate.Manifest
		var currentVersion int
		var currentDigest string
		err := transaction.QueryRowContext(
			ctx,
			`SELECT manifest_version, manifest_digest
			 FROM catalog_imports WHERE catalog_id = ?`,
			manifest.Catalog.ID,
		).Scan(&currentVersion, &currentDigest)
		if errors.Is(err, sql.ErrNoRows) {
			exists, err := catalogExists(ctx, transaction, manifest.Catalog.ID)
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, repository.NewFailure(
					repository.FailureIncompatible,
					"import manifest",
					fmt.Errorf(
						"catalog %q exists without catalog import history",
						manifest.Catalog.ID,
					),
				)
			}
			if err := ensureNewItemIDs(ctx, transaction, manifest); err != nil {
				return nil, err
			}
			if err := validateSharedBrandUpdate(
				ctx,
				transaction,
				manifest,
				incomingCatalogs,
			); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read catalog import %q: %w", manifest.Catalog.ID, err)
		}
		if currentVersion == manifest.ManifestVersion && currentDigest == candidate.SHA256 {
			unchanged[manifest.Catalog.ID] = true
			continue
		}
		if manifest.ManifestVersion <= currentVersion {
			return nil, importConflict(
				"catalog %q manifest version %d does not advance stored version %d",
				manifest.Catalog.ID,
				manifest.ManifestVersion,
				currentVersion,
			)
		}
		if err := validateSharedBrandUpdate(
			ctx,
			transaction,
			manifest,
			incomingCatalogs,
		); err != nil {
			return nil, err
		}
		if err := validateCatalogSuccessor(ctx, transaction, manifest); err != nil {
			return nil, err
		}
	}
	return unchanged, nil
}

func validateSharedBrandUpdate(
	ctx context.Context,
	transaction *sql.Tx,
	manifest domain.CatalogManifest,
	incomingCatalogs map[string]catalogdata.LoadedManifest,
) error {
	var currentName, currentStatus string
	err := transaction.QueryRowContext(
		ctx,
		"SELECT name, status FROM brands WHERE id = ?",
		manifest.Brand.ID,
	).Scan(&currentName, &currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return classify("read existing brand", err)
	}
	if currentName == manifest.Brand.Name &&
		currentStatus == string(manifest.Brand.Status) {
		return nil
	}

	rows, err := transaction.QueryContext(
		ctx,
		`SELECT id FROM catalogs
		 WHERE brand_id = ? AND id <> ?`,
		manifest.Brand.ID,
		manifest.Catalog.ID,
	)
	if err != nil {
		return classify("read catalogs sharing brand", err)
	}
	defer rows.Close()
	for rows.Next() {
		var catalogID string
		if err := rows.Scan(&catalogID); err != nil {
			return classify("scan catalog sharing brand", err)
		}
		candidate, included := incomingCatalogs[catalogID]
		if !included {
			return importConflict(
				"brand %q is shared with catalog %q, which is absent from this import",
				manifest.Brand.ID,
				catalogID,
			)
		}
		var currentVersion int
		if err := transaction.QueryRowContext(
			ctx,
			"SELECT manifest_version FROM catalog_imports WHERE catalog_id = ?",
			catalogID,
		).Scan(&currentVersion); err != nil {
			return classify("read shared catalog import", err)
		}
		if candidate.Manifest.ManifestVersion <= currentVersion {
			return importConflict(
				"brand %q update requires catalog %q to advance beyond manifest version %d",
				manifest.Brand.ID,
				catalogID,
				currentVersion,
			)
		}
	}
	if err := rows.Err(); err != nil {
		return classify("iterate catalogs sharing brand", err)
	}
	return nil
}

func catalogExists(ctx context.Context, transaction *sql.Tx, catalogID string) (bool, error) {
	var count int
	if err := transaction.QueryRowContext(
		ctx,
		"SELECT count(*) FROM catalogs WHERE id = ?",
		catalogID,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("check catalog %q: %w", catalogID, err)
	}
	return count == 1, nil
}

func ensureNewItemIDs(
	ctx context.Context,
	transaction *sql.Tx,
	manifest domain.CatalogManifest,
) error {
	for _, item := range manifest.Items {
		var catalogID string
		err := transaction.QueryRowContext(
			ctx,
			"SELECT catalog_id FROM catalog_items WHERE id = ?",
			item.ID,
		).Scan(&catalogID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check globally unique item %q: %w", item.ID, err)
		}
		return importConflict(
			"catalog item id %q is already owned by catalog %q",
			item.ID,
			catalogID,
		)
	}
	return nil
}

func validateCatalogSuccessor(
	ctx context.Context,
	transaction *sql.Tx,
	manifest domain.CatalogManifest,
) error {
	var brandID, status string
	var sortOrder int
	if err := transaction.QueryRowContext(
		ctx,
		"SELECT brand_id, status, sort_order FROM catalogs WHERE id = ?",
		manifest.Catalog.ID,
	).Scan(&brandID, &status, &sortOrder); err != nil {
		return classify("read existing catalog", err)
	}
	if brandID != manifest.Catalog.BrandID {
		return importConflict("catalog %q brand identity is immutable", manifest.Catalog.ID)
	}
	if sortOrder != manifest.Catalog.SortOrder {
		return importConflict("catalog %q sort order is immutable", manifest.Catalog.ID)
	}
	if status == string(domain.PublicationRetired) &&
		manifest.Catalog.Status != domain.PublicationRetired {
		return importConflict("retired catalog %q cannot be reactivated", manifest.Catalog.ID)
	}
	var brandStatus string
	if err := transaction.QueryRowContext(
		ctx,
		"SELECT status FROM brands WHERE id = ?",
		manifest.Brand.ID,
	).Scan(&brandStatus); err != nil {
		return classify("read existing brand", err)
	}
	if brandStatus == string(domain.PublicationRetired) &&
		manifest.Brand.Status != domain.PublicationRetired {
		return importConflict("retired brand %q cannot be reactivated", manifest.Brand.ID)
	}

	incoming := make(map[string]domain.CatalogItem, len(manifest.Items))
	for _, item := range manifest.Items {
		incoming[item.ID] = item
	}
	rows, err := transaction.QueryContext(
		ctx,
		"SELECT id, number, status FROM catalog_items WHERE catalog_id = ?",
		manifest.Catalog.ID,
	)
	if err != nil {
		return classify("read existing catalog items", err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, itemStatus string
		var number int
		if err := rows.Scan(&itemID, &number, &itemStatus); err != nil {
			return classify("scan existing catalog item", err)
		}
		item, exists := incoming[itemID]
		if !exists {
			return importConflict(
				"catalog %q omits existing item %q instead of retiring it",
				manifest.Catalog.ID,
				itemID,
			)
		}
		if item.Number != number {
			return importConflict("catalog item %q number is immutable", itemID)
		}
		if itemStatus == string(domain.PublicationRetired) &&
			item.Status != domain.PublicationRetired {
			return importConflict("retired catalog item %q cannot be reactivated", itemID)
		}
	}
	if err := rows.Err(); err != nil {
		return classify("iterate existing catalog items", err)
	}
	return ensureNewItemIDsForSuccessor(ctx, transaction, manifest)
}

func ensureNewItemIDsForSuccessor(
	ctx context.Context,
	transaction *sql.Tx,
	manifest domain.CatalogManifest,
) error {
	for _, item := range manifest.Items {
		catalogID, exists, err := queryOptionalString(
			ctx,
			transaction,
			"SELECT catalog_id FROM catalog_items WHERE id = ?",
			item.ID,
		)
		if err != nil {
			return fmt.Errorf("check successor item %q: %w", item.ID, err)
		}
		if exists && catalogID != manifest.Catalog.ID {
			return importConflict(
				"catalog item id %q is already owned by catalog %q",
				item.ID,
				catalogID,
			)
		}
	}
	return nil
}

func importConflict(format string, arguments ...any) error {
	return repository.NewFailure(
		repository.FailureConflict,
		"import manifest",
		fmt.Errorf(format, arguments...),
	)
}
