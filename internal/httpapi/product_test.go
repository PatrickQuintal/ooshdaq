package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func TestBoundedAuthenticatedProductJourney(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	server := httptest.NewServer(environment.api)
	defer server.Close()
	client, session := newAuthenticatedClient(t, server.URL, "mock-alex")

	response := doRequest(
		t,
		client,
		http.MethodGet,
		queryURL(server.URL, "/api/v1/catalogs", url.Values{
			"publication_status": {"all"},
			"limit":              {"2"},
		}),
		nil,
		nil,
	)
	var firstCatalogs struct {
		Catalogs []catalogProjection `json:"catalogs"`
		Page     pageProjection      `json:"page"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &firstCatalogs)
	if len(firstCatalogs.Catalogs) != 2 || firstCatalogs.Page.Limit != 2 ||
		firstCatalogs.Page.Returned != 2 || firstCatalogs.Page.NextCursor == nil {
		t.Fatalf("catalog page one = %#v", firstCatalogs)
	}
	if firstCatalogs.Catalogs[0].Brand.Name == "" ||
		firstCatalogs.Catalogs[0].Artwork.Attribution == "" {
		t.Fatalf("catalog lacks brand/provenance: %#v", firstCatalogs.Catalogs[0])
	}
	response = doRequest(
		t,
		client,
		http.MethodGet,
		queryURL(server.URL, "/api/v1/catalogs", url.Values{
			"cursor": {*firstCatalogs.Page.NextCursor},
		}),
		nil,
		nil,
	)
	var secondCatalogs struct {
		Catalogs []catalogProjection `json:"catalogs"`
		Page     pageProjection      `json:"page"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &secondCatalogs)
	if len(secondCatalogs.Catalogs) != 2 ||
		secondCatalogs.Catalogs[0].ID == firstCatalogs.Catalogs[0].ID {
		t.Fatalf("catalog page two = %#v", secondCatalogs)
	}

	catalogID := environment.manifests[0].Manifest.Catalog.ID
	response = doRequest(
		t,
		client,
		http.MethodGet,
		queryURL(
			server.URL,
			"/api/v1/catalogs/"+catalogID+"/collection",
			url.Values{"limit": {"2"}},
		),
		nil,
		nil,
	)
	var collection struct {
		Catalog catalogProjection           `json:"catalog"`
		Entries []collectionEntryProjection `json:"entries"`
		Page    pageProjection              `json:"page"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &collection)
	if collection.Catalog.ID != catalogID || len(collection.Entries) != 2 ||
		collection.Page.NextCursor == nil {
		t.Fatalf("collection page = %#v", collection)
	}
	if !strings.HasPrefix(
		collection.Entries[0].CatalogItem.Artwork.ImageURL,
		"https://cdn0.woolworths.media/",
	) {
		t.Fatalf("reviewed Woolworths artwork URL missing: %#v", collection.Entries[0])
	}

	response = doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/collection/summary",
		nil,
		nil,
	)
	var summary struct {
		Summary overallSummaryProjection `json:"summary"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &summary)
	if summary.Summary.ActiveCatalogCount != 5 ||
		summary.Summary.AvailableQuantity != 2 ||
		summary.Summary.WantedQuantity != 2 ||
		summary.Summary.PossibleSwapCount != 2 {
		t.Fatalf("overall summary = %#v", summary.Summary)
	}

	itemID := environment.manifests[0].Manifest.Items[1].ID
	response = doRequest(
		t,
		client,
		http.MethodPut,
		server.URL+"/api/v1/collection/"+itemID,
		map[string]any{"owned": 1, "available": 2, "wanted": 0},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusUnprocessableEntity, "validation_failed")
	response = doRequest(
		t,
		client,
		http.MethodPut,
		server.URL+"/api/v1/collection/"+itemID,
		map[string]any{"owned": 3, "available": 2, "wanted": 1},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusUnprocessableEntity, "validation_failed")
	response = doRequest(
		t,
		client,
		http.MethodPut,
		server.URL+"/api/v1/collection/"+itemID,
		map[string]any{"owned": 3, "available": 0, "wanted": 1},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	var saved struct {
		Entry collectionEntryProjection `json:"entry"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &saved)
	if saved.Entry.CatalogItem.ID != itemID ||
		saved.Entry.Owned != 3 ||
		saved.Entry.Available != 0 ||
		saved.Entry.Wanted != 1 {
		t.Fatalf("saved entry = %#v", saved.Entry)
	}

	response = doRequest(
		t,
		client,
		http.MethodGet,
		queryURL(server.URL, "/api/v1/matches", url.Values{
			"scope": {"cross_catalog"},
			"limit": {"1"},
		}),
		nil,
		nil,
	)
	var matches struct {
		Matches []matchProjection `json:"matches"`
		Page    pageProjection    `json:"page"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &matches)
	if len(matches.Matches) != 1 ||
		matches.Matches[0].TheyRequest == nil ||
		!matches.Matches[0].TheyRequest.Fulfillable ||
		matches.Matches[0].TheyRequest.CatalogItem.Catalog.Brand.Name == "" ||
		matches.Matches[0].YouReceive.CatalogItem.Catalog.Brand.Name == "" ||
		matches.Page.NextCursor == nil {
		t.Fatalf("cross-catalog match page = %#v", matches)
	}
	firstMatchID := matches.Matches[0].ID
	cursorState, err := environment.api.cursors.Decode(
		*matches.Page.NextCursor,
		matchesCursorRoute,
		"mock-alex",
	)
	if err != nil {
		t.Fatalf("decode match cursor: %v", err)
	}
	if cursorState.Filters["readiness"] != "actionable" ||
		cursorState.Filters["ranking_at"] == "" ||
		len(cursorState.Position) != 6 {
		t.Fatalf("match cursor state = %#v", cursorState)
	}
	environment.api.now = func() time.Time {
		return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	response = doRequest(
		t,
		client,
		http.MethodGet,
		queryURL(server.URL, "/api/v1/matches", url.Values{
			"cursor": {*matches.Page.NextCursor},
		}),
		nil,
		nil,
	)
	var nextMatches struct {
		Matches []matchProjection `json:"matches"`
		Page    pageProjection    `json:"page"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &nextMatches)
	if len(nextMatches.Matches) != 1 ||
		nextMatches.Matches[0].ID == firstMatchID ||
		nextMatches.Page.NextCursor != nil {
		t.Fatalf("continued match page = %#v", nextMatches)
	}

	response = doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/catalog",
		nil,
		nil,
	)
	assertJSONStatus(t, response, http.StatusNotFound, "not_found")

	response = doRequest(
		t,
		client,
		http.MethodPost,
		server.URL+"/auth/logout",
		nil,
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("logout status = %d; body=%s", response.StatusCode, body)
	}
	response.Body.Close()
	response = doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/catalogs",
		nil,
		nil,
	)
	assertJSONStatus(t, response, http.StatusUnauthorized, "authentication_required")

	response = doRequest(
		t,
		http.DefaultClient,
		http.MethodPost,
		server.URL+"/auth/logout",
		nil,
		nil,
	)
	assertJSONStatus(t, response, http.StatusUnauthorized, "authentication_required")
}

func TestProductQueryAndMutationValidation(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	server := httptest.NewServer(environment.api)
	defer server.Close()
	client, session := newAuthenticatedClient(t, server.URL, "mock-alex")

	tests := []struct {
		path string
		code string
	}{
		{path: "/api/v1/catalogs?sort=name", code: "unknown_query_parameter"},
		{path: "/api/v1/catalogs?limit=1&limit=2", code: "invalid_query"},
		{path: "/api/v1/catalogs?limit=51", code: "invalid_query"},
		{path: "/api/v1/catalogs?cursor=broken", code: "invalid_cursor"},
		{path: "/api/v1/catalogs?cursor=broken&limit=1", code: "cursor_conflict"},
		{path: "/api/v1/catalogs?q=one;limit=1", code: "invalid_query"},
		{path: "/api/v1/collection/summary?ignored;value", code: "invalid_query"},
		{path: "/api/v1/matches?scope=nearby", code: "invalid_query"},
		{path: "/api/v1/matches?scope=all;limit=1", code: "invalid_query"},
		{path: "/api/v1/matches?readiness=waiting", code: "invalid_query"},
		{
			path: "/api/v1/matches?readiness=unfulfillable&scope=cross_catalog",
			code: "invalid_query",
		},
		{
			path: "/api/v1/matches?readiness=unfulfillable&give_catalog_id=example",
			code: "invalid_query",
		},
		{
			path: "/api/v1/catalogs/" +
				environment.manifests[0].Manifest.Catalog.ID +
				"/collection?universe=NotReviewed",
			code: "invalid_query",
		},
		{
			path: "/api/v1/catalogs/" +
				environment.manifests[0].Manifest.Catalog.ID +
				"/collection?q=one;limit=1",
			code: "invalid_query",
		},
	}
	for _, test := range tests {
		response := doRequest(
			t,
			client,
			http.MethodGet,
			server.URL+test.path,
			nil,
			nil,
		)
		assertJSONStatus(t, response, http.StatusBadRequest, test.code)
	}

	itemID := environment.manifests[0].Manifest.Items[0].ID
	response := doRequest(
		t,
		client,
		http.MethodPut,
		server.URL+"/api/v1/collection/"+itemID,
		map[string]any{"owned": 1, "available": 0},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusBadRequest, "invalid_request")
	response = doRequest(
		t,
		client,
		http.MethodPut,
		server.URL+"/api/v1/collection/"+itemID,
		map[string]any{"owned": 1, "available": 0, "wanted": 0, "extra": true},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusBadRequest, "invalid_request")

	request, err := http.NewRequest(
		http.MethodPut,
		server.URL+"/api/v1/collection/"+itemID,
		strings.NewReader(`{"owned":1,"available":0,"wanted":0}`),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	response, err = client.Do(request)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	assertJSONStatus(t, response, http.StatusUnsupportedMediaType, "unsupported_media_type")

	response = doRequest(
		t,
		client,
		http.MethodPost,
		server.URL+"/api/v1/activity",
		nil,
		nil,
	)
	assertJSONStatus(t, response, http.StatusForbidden, "csrf_failed")
	response = doRequest(
		t,
		client,
		http.MethodPost,
		server.URL+"/api/v1/activity?unexpected=1",
		nil,
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusBadRequest, "unknown_query_parameter")
	response = doRequest(
		t,
		client,
		http.MethodPost,
		server.URL+"/api/v1/activity",
		map[string]any{"unexpected": true},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusBadRequest, "invalid_request")
	response = doRequest(
		t,
		client,
		http.MethodPost,
		server.URL+"/api/v1/activity",
		nil,
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("activity status = %d; body=%s", response.StatusCode, body)
	}
	response.Body.Close()

	response = doRequest(
		t,
		client,
		http.MethodPost,
		server.URL+"/api/v1/catalogs",
		nil,
		nil,
	)
	if response.Header.Get("Allow") != http.MethodGet {
		t.Errorf("Allow = %q, want GET", response.Header.Get("Allow"))
	}
	assertJSONStatus(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestCatalogSearchIncludesPublishedItemMetadata(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	server := httptest.NewServer(environment.api)
	defer server.Close()
	client, _ := newAuthenticatedClient(t, server.URL, "mock-alex")

	for _, test := range []struct {
		name      string
		search    string
		catalogID string
	}{
		{
			name:      "item name",
			search:    "  KAKAMORA  ",
			catalogID: "disney-ooshies-2026",
		},
		{
			name:      "item universe",
			search:    "Night Sky",
			catalogID: "pocket-constellations-2026",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := doRequest(
				t,
				client,
				http.MethodGet,
				queryURL(
					server.URL,
					"/api/v1/catalogs",
					url.Values{"q": {test.search}},
				),
				nil,
				nil,
			)
			var result struct {
				Catalogs []catalogProjection `json:"catalogs"`
				Page     pageProjection      `json:"page"`
			}
			decodeJSONResponse(t, response, http.StatusOK, &result)
			if len(result.Catalogs) != 1 ||
				result.Catalogs[0].ID != test.catalogID ||
				result.Page.Returned != 1 {
				t.Fatalf("catalog item search = %#v", result)
			}
		})
	}
}

func TestPossibleSwapOmitsRequestWhenCounterpartWantsNothing(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	receiveID := environment.manifests[1].Manifest.Items[0].ID
	if _, err := environment.store.SaveCollectionEntry(
		context.Background(),
		repository.CollectionWrite{
			UserID:    "mock-devon",
			ItemID:    receiveID,
			Owned:     2,
			Available: 2,
		},
	); err != nil {
		t.Fatalf("SaveCollectionEntry one-way offer: %v", err)
	}
	server := httptest.NewServer(environment.api)
	defer server.Close()
	client, _ := newAuthenticatedClient(t, server.URL, "mock-alex")

	response := doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/matches?receive_catalog_id="+
			environment.manifests[1].Manifest.Catalog.ID,
		nil,
		nil,
	)
	var result struct {
		Matches []struct {
			OtherUser struct {
				ID string `json:"id"`
			} `json:"other_user"`
			TheyRequest json.RawMessage    `json:"they_request"`
			YouReceive  matchLegProjection `json:"you_receive"`
		} `json:"matches"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &result)
	for _, match := range result.Matches {
		if match.OtherUser.ID != "mock-devon" {
			continue
		}
		if len(match.TheyRequest) != 0 {
			t.Fatalf(
				"one-way lead encoded they_request = %s, want omitted",
				match.TheyRequest,
			)
		}
		if match.YouReceive.CatalogItem.ID != receiveID ||
			match.YouReceive.Quantity != 2 {
			t.Fatalf("one-way receive leg = %#v", match.YouReceive)
		}
		return
	}
	t.Fatalf("one-way lead missing from matches: %#v", result.Matches)
}

func TestPossibleSwapProjectsUnfulfillableRequest(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	receiveID := environment.manifests[1].Manifest.Items[0].ID
	requestedID := environment.manifests[0].Manifest.Items[1].ID
	for _, write := range []repository.CollectionWrite{
		{
			UserID:    "mock-devon",
			ItemID:    receiveID,
			Owned:     2,
			Available: 2,
		},
		{
			UserID: "mock-devon",
			ItemID: requestedID,
			Wanted: 3,
		},
	} {
		if _, err := environment.store.SaveCollectionEntry(
			context.Background(),
			write,
		); err != nil {
			t.Fatalf("SaveCollectionEntry unfulfillable fixture: %v", err)
		}
	}
	server := httptest.NewServer(environment.api)
	defer server.Close()
	client, _ := newAuthenticatedClient(t, server.URL, "mock-alex")

	response := doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/matches?readiness=unfulfillable&receive_catalog_id="+
			environment.manifests[1].Manifest.Catalog.ID,
		nil,
		nil,
	)
	var result struct {
		Matches []matchProjection `json:"matches"`
	}
	decodeJSONResponse(t, response, http.StatusOK, &result)
	for _, match := range result.Matches {
		if match.OtherUser.ID != "mock-devon" {
			continue
		}
		if match.TheyRequest == nil ||
			match.TheyRequest.CatalogItem.ID != requestedID ||
			match.TheyRequest.Quantity != 0 ||
			match.TheyRequest.Fulfillable {
			t.Fatalf("unfulfillable request = %#v", match.TheyRequest)
		}
		if match.YouReceive.CatalogItem.ID != receiveID ||
			match.YouReceive.Quantity != 2 {
			t.Fatalf("unfulfillable lead receive leg = %#v", match.YouReceive)
		}
		return
	}
	t.Fatalf("unfulfillable lead missing from matches: %#v", result.Matches)
}
