package qualification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math/rand"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
)

type reciprocalPair struct {
	left  int
	right int
	cross bool
}

func generatedManifests(
	config Config,
	digest hash.Hash,
) ([]catalogdata.LoadedManifest, []string, []string, error) {
	brands := make([]domain.Brand, config.Brands)
	for index := range brands {
		brands[index] = domain.Brand{
			ID:     fmt.Sprintf("fictional-qualification-brand-%02d", index+1),
			Name:   fmt.Sprintf("Fictional Qualification Brand %02d", index+1),
			Status: domain.PublicationActive,
		}
	}
	loaded := make([]catalogdata.LoadedManifest, 0, config.Catalogs)
	catalogIDs := make([]string, 0, config.Catalogs)
	itemIDs := make([]string, 0, config.Catalogs*config.ItemsPerCatalog)
	for catalogIndex := range config.Catalogs {
		catalogID := fmt.Sprintf("fictional-qualification-catalog-%02d", catalogIndex+1)
		manifest := domain.CatalogManifest{
			SchemaVersion:   domain.CatalogManifestSchemaVersion,
			ManifestVersion: 1,
			Brand:           brands[catalogIndex%len(brands)],
			Catalog: domain.Catalog{
				ID:        catalogID,
				BrandID:   brands[catalogIndex%len(brands)].ID,
				Name:      fmt.Sprintf("Fictional Qualification Catalog %02d", catalogIndex+1),
				Edition:   "Synthetic 2026",
				Status:    domain.PublicationActive,
				SortOrder: catalogIndex + 1,
			},
			Items: make([]domain.CatalogItem, 0, config.ItemsPerCatalog),
		}
		for itemIndex := range config.ItemsPerCatalog {
			itemID := fmt.Sprintf("%s-item-%03d", catalogID, itemIndex+1)
			manifest.Items = append(manifest.Items, domain.CatalogItem{
				ID:        itemID,
				CatalogID: catalogID,
				Number:    itemIndex + 1,
				Name:      fmt.Sprintf("Synthetic Collectible %02d-%03d", catalogIndex+1, itemIndex+1),
				Universe:  fmt.Sprintf("Fictional Universe %02d", (itemIndex%4)+1),
				Variant:   fmt.Sprintf("Synthetic Variant %02d", (itemIndex%3)+1),
				Status:    domain.PublicationActive,
				Artwork: domain.Artwork{
					Placeholder: true,
					Rights: domain.ArtworkRights{
						Status: domain.RightsUnknown,
					},
					Provenance: domain.ArtworkProvenance{
						ReviewedAt:      "2026-01-01",
						ReviewReference: "synthetic-qualification-fixture",
						Transformation:  "generated-no-artwork",
					},
				},
			})
			itemIDs = append(itemIDs, itemID)
		}
		raw, err := json.Marshal(manifest)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("encode qualification manifest: %w", err)
		}
		manifestDigest := sha256.Sum256(raw)
		loaded = append(loaded, catalogdata.LoadedManifest{
			Path:     "qualification/generated/" + catalogID + ".json",
			SHA256:   hex.EncodeToString(manifestDigest[:]),
			Manifest: manifest,
		})
		writeDigestRecord(digest, "manifest", raw)
		catalogIDs = append(catalogIDs, catalogID)
	}
	return loaded, catalogIDs, itemIDs, nil
}

func reciprocalPairs(config Config, itemIDs []string) ([]reciprocalPair, int, int, error) {
	var same, cross []reciprocalPair
	for left := 0; left < len(itemIDs); left++ {
		leftCatalog := left / config.ItemsPerCatalog
		for right := left + 1; right < len(itemIDs); right++ {
			pair := reciprocalPair{
				left:  left,
				right: right,
				cross: leftCatalog != right/config.ItemsPerCatalog,
			}
			if pair.cross {
				cross = append(cross, pair)
			} else {
				same = append(same, pair)
			}
		}
	}
	random := rand.New(rand.NewSource(config.Seed))
	random.Shuffle(len(same), func(left, right int) {
		same[left], same[right] = same[right], same[left]
	})
	random.Shuffle(len(cross), func(left, right int) {
		cross[left], cross[right] = cross[right], cross[left]
	})

	pairCount := config.Users / 2
	sameCount := max(1, pairCount/5)
	if sameCount > len(same) {
		sameCount = len(same)
	}
	crossCount := pairCount - sameCount
	if crossCount > len(cross) {
		shift := crossCount - len(cross)
		sameCount += shift
		crossCount -= shift
	}
	if sameCount < 1 || crossCount < 1 ||
		sameCount > len(same) || crossCount > len(cross) {
		return nil, 0, 0, fmt.Errorf(
			"cannot create both reciprocal scopes from %d items and %d users",
			len(itemIDs),
			config.Users,
		)
	}
	pairs := make([]reciprocalPair, 0, pairCount)
	sameIndex, crossIndex := 0, 0
	for len(pairs) < pairCount {
		if len(pairs)%5 == 4 && sameIndex < sameCount {
			pairs = append(pairs, same[sameIndex])
			sameIndex++
			continue
		}
		if crossIndex < crossCount {
			pairs = append(pairs, cross[crossIndex])
			crossIndex++
			continue
		}
		pairs = append(pairs, same[sameIndex])
		sameIndex++
	}
	return pairs, sameCount, crossCount, nil
}
