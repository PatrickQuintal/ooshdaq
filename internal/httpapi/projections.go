package httpapi

import (
	"net/url"
	"strings"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

type artworkProjection struct {
	ImageURL     string `json:"image_url,omitempty"`
	AltText      string `json:"alt_text"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	SourceURL    string `json:"source_url,omitempty"`
	Attribution  string `json:"attribution"`
	RightsStatus string `json:"rights_status"`
	License      string `json:"license,omitempty"`
	RetrievedAt  string `json:"retrieved_at"`
}

type brandProjection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type memberSummaryProjection struct {
	OwnedQuantity     int `json:"owned_quantity"`
	OwnedDesigns      int `json:"owned_designs"`
	AvailableQuantity int `json:"available_quantity"`
	AvailableDesigns  int `json:"available_designs"`
	WantedQuantity    int `json:"wanted_quantity"`
	WantedDesigns     int `json:"wanted_designs"`
}

type compactCatalogProjection struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Edition           string          `json:"edition,omitempty"`
	PublicationStatus string          `json:"publication_status"`
	Brand             brandProjection `json:"brand"`
}

type catalogProjection struct {
	ID                string                  `json:"id"`
	Brand             brandProjection         `json:"brand"`
	Name              string                  `json:"name"`
	Edition           string                  `json:"edition,omitempty"`
	PublicationStatus string                  `json:"publication_status"`
	SortOrder         int                     `json:"sort_order"`
	ItemCount         int                     `json:"item_count"`
	UniverseOptions   []string                `json:"universe_options"`
	Artwork           artworkProjection       `json:"artwork"`
	MemberSummary     memberSummaryProjection `json:"member_summary"`
}

type catalogItemProjection struct {
	ID                string                   `json:"id"`
	CatalogID         string                   `json:"catalog_id"`
	Catalog           compactCatalogProjection `json:"catalog"`
	Number            int                      `json:"number"`
	Name              string                   `json:"name"`
	Universe          string                   `json:"universe,omitempty"`
	Variant           string                   `json:"variant,omitempty"`
	PublicationStatus string                   `json:"publication_status"`
	Artwork           artworkProjection        `json:"artwork"`
}

type collectionEntryProjection struct {
	CatalogItem catalogItemProjection `json:"catalog_item"`
	Owned       int                   `json:"owned"`
	Available   int                   `json:"available"`
	Wanted      int                   `json:"wanted"`
}

type matchLegProjection struct {
	CatalogItem catalogItemProjection `json:"catalog_item"`
	Quantity    int                   `json:"quantity"`
}

type matchRequestProjection struct {
	CatalogItem catalogItemProjection `json:"catalog_item"`
	Quantity    int                   `json:"quantity"`
	Fulfillable bool                  `json:"fulfillable"`
}

type matchProjection struct {
	ID          string                  `json:"id"`
	OtherUser   domain.User             `json:"other_user"`
	TheyRequest *matchRequestProjection `json:"they_request,omitempty"`
	YouReceive  matchLegProjection      `json:"you_receive"`
}

type pageProjection struct {
	Limit      int     `json:"limit"`
	Returned   int     `json:"returned"`
	NextCursor *string `json:"next_cursor"`
}

type overallSummaryProjection struct {
	ActiveCatalogCount int `json:"active_catalog_count"`
	ActiveItemCount    int `json:"active_item_count"`
	CatalogsStarted    int `json:"catalogs_started"`
	PossibleSwapCount  int `json:"possible_swap_count"`
	OwnedQuantity      int `json:"owned_quantity"`
	OwnedDesigns       int `json:"owned_designs"`
	AvailableQuantity  int `json:"available_quantity"`
	AvailableDesigns   int `json:"available_designs"`
	WantedQuantity     int `json:"wanted_quantity"`
	WantedDesigns      int `json:"wanted_designs"`
}

func projectCatalog(record repository.CatalogRecord) catalogProjection {
	universes := append([]string{}, record.UniverseOptions...)
	return catalogProjection{
		ID: record.Catalog.ID,
		Brand: brandProjection{
			ID:   record.Brand.ID,
			Name: record.Brand.Name,
		},
		Name:              record.Catalog.Name,
		Edition:           record.Catalog.Edition,
		PublicationStatus: string(record.Catalog.Status),
		SortOrder:         record.Catalog.SortOrder,
		ItemCount:         record.ItemCount,
		UniverseOptions:   universes,
		Artwork: projectArtwork(
			record.Artwork,
			"Artwork for the "+record.Catalog.Name+" catalog.",
		),
		MemberSummary: projectMemberSummary(record.MemberSummary),
	}
}

func projectCatalogItem(record repository.CatalogItemRecord) catalogItemProjection {
	return catalogItemProjection{
		ID:        record.Item.ID,
		CatalogID: record.Item.CatalogID,
		Catalog: compactCatalogProjection{
			ID:                record.Catalog.ID,
			Name:              record.Catalog.Name,
			Edition:           record.Catalog.Edition,
			PublicationStatus: string(record.Catalog.Status),
			Brand: brandProjection{
				ID:   record.Brand.ID,
				Name: record.Brand.Name,
			},
		},
		Number:            record.Item.Number,
		Name:              record.Item.Name,
		Universe:          record.Item.Universe,
		Variant:           record.Item.Variant,
		PublicationStatus: string(record.Item.Status),
		Artwork: projectArtwork(
			record.Item.Artwork,
			"Artwork for "+record.Item.Name+".",
		),
	}
}

func projectCollectionEntry(
	record repository.CollectionRecord,
) collectionEntryProjection {
	return collectionEntryProjection{
		CatalogItem: projectCatalogItem(record.CatalogItem),
		Owned:       record.Owned,
		Available:   record.Available,
		Wanted:      record.Wanted,
	}
}

func projectMatch(record repository.MatchRecord) matchProjection {
	projected := matchProjection{
		ID:        record.ID,
		OtherUser: record.OtherUser,
		YouReceive: matchLegProjection{
			CatalogItem: projectCatalogItem(record.YouReceive.CatalogItem),
			Quantity:    record.YouReceive.Quantity,
		},
	}
	if record.TheyRequest != nil {
		projected.TheyRequest = &matchRequestProjection{
			CatalogItem: projectCatalogItem(record.TheyRequest.CatalogItem),
			Quantity:    record.TheyRequest.Quantity,
			Fulfillable: record.TheyRequest.Fulfillable,
		}
	}
	return projected
}

func projectMemberSummary(summary repository.MemberSummary) memberSummaryProjection {
	return memberSummaryProjection{
		OwnedQuantity:     summary.OwnedQuantity,
		OwnedDesigns:      summary.OwnedDesigns,
		AvailableQuantity: summary.AvailableQuantity,
		AvailableDesigns:  summary.AvailableDesigns,
		WantedQuantity:    summary.WantedQuantity,
		WantedDesigns:     summary.WantedDesigns,
	}
}

func projectArtwork(artwork domain.Artwork, altText string) artworkProjection {
	attribution := strings.TrimSpace(artwork.Rights.Attribution)
	if attribution == "" {
		attribution = "No third-party artwork is published; $OOSHDAQ uses its local placeholder."
	}
	retrievedAt := artwork.Provenance.RetrievedAt
	if retrievedAt == "" {
		retrievedAt = artwork.Provenance.ReviewedAt
	}
	projected := artworkProjection{
		AltText:      altText,
		SourceURL:    artwork.Provenance.SourcePageURL,
		Attribution:  attribution,
		RightsStatus: string(artwork.Rights.Status),
		License:      artwork.Rights.License,
		RetrievedAt:  retrievedAt,
	}
	if approvedArtworkURL(artwork) {
		projected.ImageURL = artwork.AssetURL
	}
	return projected
}

func approvedArtworkURL(artwork domain.Artwork) bool {
	if artwork.Placeholder ||
		artwork.Rights.Status == domain.RightsUnknown ||
		artwork.Rights.Status == domain.RightsRestricted {
		return false
	}
	parsed, err := url.Parse(artwork.AssetURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "cdn0.woolworths.media":
		return true
	default:
		return false
	}
}
