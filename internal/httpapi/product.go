package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ooshdaq/ooshdaq/internal/pagination"
)

func (a *API) handleCatalogs(response http.ResponseWriter, request *http.Request) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	user, ok := a.requireUser(response, request)
	if !ok {
		return
	}
	raw, issue := requestQuery(
		request,
		"brand_id",
		"q",
		"publication_status",
		"limit",
		"cursor",
	)
	if issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	query, filters, issue := a.catalogQuery(raw, user.ID)
	if issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	result, err := a.service.Catalogs(request.Context(), query)
	if err != nil {
		a.readServiceError(response, "list catalogs", err)
		return
	}
	catalogs := make([]catalogProjection, 0, len(result.Catalogs))
	for _, record := range result.Catalogs {
		catalogs = append(catalogs, projectCatalog(record))
	}
	var nextCursor *string
	if result.Next != nil {
		nextCursor, err = encodeNextCursor(a.cursors, pagination.State{
			Route:   catalogsCursorRoute,
			Subject: user.ID,
			Filters: filters,
			Limit:   result.Limit,
			Position: []string{
				strconv.Itoa(result.Next.SortOrder),
				result.Next.CatalogID,
			},
		})
		if err != nil {
			a.internalError(response, "encode catalog cursor", err)
			return
		}
	}
	a.writeJSON(response, http.StatusOK, struct {
		Catalogs []catalogProjection `json:"catalogs"`
		Page     pageProjection      `json:"page"`
	}{
		Catalogs: catalogs,
		Page: pageProjection{
			Limit:      result.Limit,
			Returned:   len(catalogs),
			NextCursor: nextCursor,
		},
	})
}

func (a *API) handleScopedCollection(
	response http.ResponseWriter,
	request *http.Request,
) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	remainder := strings.TrimPrefix(request.URL.Path, "/api/v1/catalogs/")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[1] != "collection" || !validAPISlug(parts[0]) {
		a.writeError(
			response,
			http.StatusNotFound,
			"not_found",
			"The requested endpoint was not found.",
		)
		return
	}
	user, ok := a.requireUser(response, request)
	if !ok {
		return
	}
	raw, issue := requestQuery(
		request,
		"q",
		"universe",
		"state",
		"limit",
		"cursor",
	)
	if issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	route := "/api/v1/catalogs/" + parts[0] + "/collection"
	query, filters, issue := a.collectionQuery(raw, user.ID, parts[0], route)
	if issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	result, err := a.service.Collection(request.Context(), query)
	if err != nil {
		a.readServiceError(response, "list collection", err)
		return
	}
	entries := make([]collectionEntryProjection, 0, len(result.Entries))
	for _, record := range result.Entries {
		entries = append(entries, projectCollectionEntry(record))
	}
	var nextCursor *string
	if result.Next != nil {
		nextCursor, err = encodeNextCursor(a.cursors, pagination.State{
			Route:   route,
			Subject: user.ID,
			Filters: filters,
			Limit:   result.Limit,
			Position: []string{
				strconv.Itoa(result.Next.ItemNumber),
				result.Next.ItemID,
			},
		})
		if err != nil {
			a.internalError(response, "encode collection cursor", err)
			return
		}
	}
	a.writeJSON(response, http.StatusOK, struct {
		Catalog catalogProjection           `json:"catalog"`
		Entries []collectionEntryProjection `json:"entries"`
		Page    pageProjection              `json:"page"`
	}{
		Catalog: projectCatalog(result.Catalog),
		Entries: entries,
		Page: pageProjection{
			Limit:      result.Limit,
			Returned:   len(entries),
			NextCursor: nextCursor,
		},
	})
}

func (a *API) handleCollectionSummary(
	response http.ResponseWriter,
	request *http.Request,
) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	user, ok := a.requireUser(response, request)
	if !ok {
		return
	}
	if raw, issue := requestQuery(request); issue != nil || len(raw) != 0 {
		a.writeQueryIssue(response, issue)
		return
	}
	result, err := a.service.CollectionSummary(request.Context(), user.ID)
	if err != nil {
		a.readServiceError(response, "get collection summary", err)
		return
	}
	a.writeJSON(response, http.StatusOK, struct {
		Summary overallSummaryProjection `json:"summary"`
	}{
		Summary: overallSummaryProjection{
			ActiveCatalogCount: result.ActiveCatalogCount,
			ActiveItemCount:    result.ActiveItemCount,
			CatalogsStarted:    result.CatalogsStarted,
			PossibleSwapCount:  result.PossibleSwapCount,
			OwnedQuantity:      result.MemberSummary.OwnedQuantity,
			OwnedDesigns:       result.MemberSummary.OwnedDesigns,
			AvailableQuantity:  result.MemberSummary.AvailableQuantity,
			AvailableDesigns:   result.MemberSummary.AvailableDesigns,
			WantedQuantity:     result.MemberSummary.WantedQuantity,
			WantedDesigns:      result.MemberSummary.WantedDesigns,
		},
	})
}

func (a *API) handleCollectionItem(
	response http.ResponseWriter,
	request *http.Request,
) {
	if !a.requireMethod(response, request, http.MethodPut) {
		return
	}
	itemID := strings.TrimPrefix(request.URL.Path, "/api/v1/collection/")
	if !validAPISlug(itemID) || strings.Contains(itemID, "/") {
		a.writeError(
			response,
			http.StatusNotFound,
			"not_found",
			"The requested catalog item was not found.",
		)
		return
	}
	if !a.requireCSRF(response, request) {
		return
	}
	user, ok := a.requireUser(response, request)
	if !ok {
		return
	}
	var input *struct {
		Owned     *int `json:"owned"`
		Available *int `json:"available"`
		Wanted    *int `json:"wanted"`
	}
	if apiErr := a.decodeJSON(response, request, &input); apiErr != nil {
		a.writeRequestError(response, apiErr)
		return
	}
	if input == nil || input.Owned == nil || input.Available == nil || input.Wanted == nil {
		a.writeError(
			response,
			http.StatusBadRequest,
			"invalid_request",
			"owned, available, and wanted are required.",
		)
		return
	}
	result, err := a.service.UpdateCollection(
		request.Context(),
		user.ID,
		itemID,
		*input.Owned,
		*input.Available,
		*input.Wanted,
	)
	if err != nil {
		a.writeServiceError(response, "update collection", err)
		return
	}
	a.writeJSON(response, http.StatusOK, struct {
		Entry collectionEntryProjection `json:"entry"`
	}{Entry: projectCollectionEntry(result)})
}

func (a *API) handleMatches(response http.ResponseWriter, request *http.Request) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	user, ok := a.requireUser(response, request)
	if !ok {
		return
	}
	raw, issue := requestQuery(
		request,
		"give_catalog_id",
		"receive_catalog_id",
		"scope",
		"readiness",
		"limit",
		"cursor",
	)
	if issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	query, filters, issue := a.matchQuery(raw, user.ID)
	if issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	result, err := a.service.Matches(request.Context(), query)
	if err != nil {
		a.readServiceError(response, "find matches", err)
		return
	}
	matches := make([]matchProjection, 0, len(result.Matches))
	for _, record := range result.Matches {
		matches = append(matches, projectMatch(record))
	}
	var nextCursor *string
	if result.Next != nil {
		nextCursor, err = encodeNextCursor(a.cursors, pagination.State{
			Route:   matchesCursorRoute,
			Subject: user.ID,
			Filters: filters,
			Limit:   result.Limit,
			Position: []string{
				strconv.Itoa(result.Next.RequestClass),
				strconv.FormatInt(result.Next.CappedSince, 10),
				strconv.Itoa(boolInt(result.Next.Stale)),
				result.Next.RequestItemID,
				result.Next.CounterpartID,
				result.Next.ReceiveItemID,
			},
		})
		if err != nil {
			a.internalError(response, "encode match cursor", err)
			return
		}
	}
	a.writeJSON(response, http.StatusOK, struct {
		Matches []matchProjection `json:"matches"`
		Page    pageProjection    `json:"page"`
	}{
		Matches: matches,
		Page: pageProjection{
			Limit:      result.Limit,
			Returned:   len(matches),
			NextCursor: nextCursor,
		},
	})
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
