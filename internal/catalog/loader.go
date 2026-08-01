// Package catalog loads the reviewed, versioned catalog manifests embedded in
// the application. It does not persist or import them.
package catalog

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"

	"github.com/ooshdaq/ooshdaq/internal/domain"
)

const (
	embeddedManifestPattern = "manifests/*.json"
	minimumCatalogCount     = 5
	maxManifestBytes        = 1 << 20
)

//go:embed manifests/*.json
var embeddedManifestFS embed.FS

// LoadedManifest is one validated catalog manifest plus the digest of the exact
// reviewed JSON bytes embedded in the binary.
type LoadedManifest struct {
	Path     string
	SHA256   string
	Manifest domain.CatalogManifest
}

// LoadEmbeddedManifests decodes and validates the complete embedded set before
// returning any data. Results and items are sorted deterministically.
func LoadEmbeddedManifests() ([]LoadedManifest, error) {
	manifests, err := loadManifestFiles(embeddedManifestFS, embeddedManifestPattern)
	if err != nil {
		return nil, err
	}
	if len(manifests) < minimumCatalogCount {
		return nil, fmt.Errorf(
			"load embedded catalog manifests: got %d catalogs, need at least %d",
			len(manifests),
			minimumCatalogCount,
		)
	}
	return manifests, nil
}

// ValidateManifestSet enforces identities which must be unique or consistent
// across independently versioned catalogs.
func ValidateManifestSet(manifests []domain.CatalogManifest) error {
	if len(manifests) == 0 {
		return errors.New("catalog manifest set must not be empty")
	}

	brands := make(map[string]domain.Brand)
	catalogs := make(map[string]struct{}, len(manifests))
	items := make(map[string]string)
	for index, manifest := range manifests {
		if err := manifest.Validate(); err != nil {
			return fmt.Errorf("manifest %d: %w", index, err)
		}
		if prior, exists := brands[manifest.Brand.ID]; exists {
			if prior != manifest.Brand {
				return fmt.Errorf(
					"manifest %d: brand %q conflicts with another manifest",
					index,
					manifest.Brand.ID,
				)
			}
		} else {
			brands[manifest.Brand.ID] = manifest.Brand
		}
		if _, duplicate := catalogs[manifest.Catalog.ID]; duplicate {
			return fmt.Errorf("manifest %d: duplicate catalog id %q", index, manifest.Catalog.ID)
		}
		catalogs[manifest.Catalog.ID] = struct{}{}
		for _, item := range manifest.Items {
			if otherCatalog, duplicate := items[item.ID]; duplicate {
				return fmt.Errorf(
					"manifest %d: item id %q is already used by catalog %q",
					index,
					item.ID,
					otherCatalog,
				)
			}
			items[item.ID] = manifest.Catalog.ID
		}
	}
	return nil
}

func loadManifestFiles(fsys fs.FS, pattern string) ([]LoadedManifest, error) {
	paths, err := fs.Glob(fsys, pattern)
	if err != nil {
		return nil, fmt.Errorf("find catalog manifests: %w", err)
	}
	if len(paths) == 0 {
		return nil, errors.New("find catalog manifests: no files matched")
	}

	loaded := make([]LoadedManifest, 0, len(paths))
	manifests := make([]domain.CatalogManifest, 0, len(paths))
	for _, path := range paths {
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read catalog manifest %q: %w", path, err)
		}
		if len(raw) > maxManifestBytes {
			return nil, fmt.Errorf(
				"read catalog manifest %q: exceeds %d bytes",
				path,
				maxManifestBytes,
			)
		}
		manifest, err := decodeManifest(raw)
		if err != nil {
			return nil, fmt.Errorf("decode catalog manifest %q: %w", path, err)
		}
		digest := sha256.Sum256(raw)
		loaded = append(loaded, LoadedManifest{
			Path:     path,
			SHA256:   hex.EncodeToString(digest[:]),
			Manifest: manifest,
		})
		manifests = append(manifests, manifest)
	}

	if err := ValidateManifestSet(manifests); err != nil {
		return nil, fmt.Errorf("validate catalog manifests: %w", err)
	}

	for index := range loaded {
		sort.Slice(loaded[index].Manifest.Items, func(left, right int) bool {
			leftItem := loaded[index].Manifest.Items[left]
			rightItem := loaded[index].Manifest.Items[right]
			if leftItem.Number != rightItem.Number {
				return leftItem.Number < rightItem.Number
			}
			return leftItem.ID < rightItem.ID
		})
	}
	sort.Slice(loaded, func(left, right int) bool {
		leftCatalog := loaded[left].Manifest.Catalog
		rightCatalog := loaded[right].Manifest.Catalog
		if leftCatalog.SortOrder != rightCatalog.SortOrder {
			return leftCatalog.SortOrder < rightCatalog.SortOrder
		}
		return leftCatalog.ID < rightCatalog.ID
	})

	return loaded, nil
}

func decodeManifest(raw []byte) (domain.CatalogManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var manifest domain.CatalogManifest
	if err := decoder.Decode(&manifest); err != nil {
		return domain.CatalogManifest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.CatalogManifest{}, errors.New("multiple JSON values")
		}
		return domain.CatalogManifest{}, fmt.Errorf("trailing JSON: %w", err)
	}
	return manifest, nil
}
