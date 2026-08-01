package httpapi

import (
	"strconv"
	"unicode/utf8"

	"github.com/ooshdaq/ooshdaq/internal/pagination"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

const catalogsCursorRoute = "/api/v1/catalogs"

func (a *API) catalogQuery(
	raw map[string]string,
	userID string,
) (repository.CatalogQuery, map[string]string, *queryIssue) {
	if token, continuation := raw["cursor"]; continuation {
		state, issue := a.decodeCursor(token, catalogsCursorRoute, userID)
		if issue != nil || !exactFilters(
			state.Filters,
			"brand_id",
			"q",
			"publication_status",
		) || len(state.Position) != 2 {
			return repository.CatalogQuery{}, nil, invalidCursorIssue()
		}
		sortOrder, err := strconv.Atoi(state.Position[0])
		if err != nil || sortOrder < 1 || !validAPISlug(state.Position[1]) {
			return repository.CatalogQuery{}, nil, invalidCursorIssue()
		}
		query := repository.CatalogQuery{
			UserID:            userID,
			BrandID:           state.Filters["brand_id"],
			Search:            state.Filters["q"],
			PublicationStatus: repository.PublicationFilter(state.Filters["publication_status"]),
			Limit:             state.Limit,
			After: &repository.CatalogPosition{
				SortOrder: sortOrder,
				CatalogID: state.Position[1],
			},
		}
		if !validCatalogCursorQuery(query) {
			return repository.CatalogQuery{}, nil, invalidCursorIssue()
		}
		return query, state.Filters, nil
	}

	search, issue := parseSearch(raw["q"])
	if issue != nil {
		return repository.CatalogQuery{}, nil, issue
	}
	brandID := raw["brand_id"]
	if has(raw, "brand_id") {
		if issue := requireSlug(brandID, "brand_id"); issue != nil {
			return repository.CatalogQuery{}, nil, issue
		}
	}
	publicationStatus := repository.PublicationFilterActive
	if status, present := raw["publication_status"]; present {
		publicationStatus = repository.PublicationFilter(status)
		switch publicationStatus {
		case repository.PublicationFilterActive,
			repository.PublicationFilterRetired,
			repository.PublicationFilterAll:
		default:
			return repository.CatalogQuery{}, nil, &queryIssue{
				code:    "invalid_query",
				field:   "publication_status",
				message: "publication_status must be active, retired, or all.",
			}
		}
	}
	limit, issue := parseLimit(
		raw["limit"],
		has(raw, "limit"),
		repository.DefaultCatalogPageSize,
		repository.MaxCatalogPageSize,
	)
	if issue != nil {
		return repository.CatalogQuery{}, nil, issue
	}
	filters := map[string]string{
		"brand_id":           brandID,
		"q":                  search,
		"publication_status": string(publicationStatus),
	}
	return repository.CatalogQuery{
		UserID:            userID,
		BrandID:           brandID,
		Search:            search,
		PublicationStatus: publicationStatus,
		Limit:             limit,
	}, filters, nil
}

func validCatalogCursorQuery(query repository.CatalogQuery) bool {
	if query.BrandID != "" && !validAPISlug(query.BrandID) {
		return false
	}
	if _, issue := parseSearch(query.Search); issue != nil {
		return false
	}
	switch query.PublicationStatus {
	case repository.PublicationFilterActive,
		repository.PublicationFilterRetired,
		repository.PublicationFilterAll:
	default:
		return false
	}
	return query.Limit >= 1 && query.Limit <= repository.MaxCatalogPageSize
}

func (a *API) collectionQuery(
	raw map[string]string,
	userID string,
	catalogID string,
	route string,
) (repository.CollectionQuery, map[string]string, *queryIssue) {
	if token, continuation := raw["cursor"]; continuation {
		state, issue := a.decodeCursor(token, route, userID)
		if issue != nil ||
			!exactFilters(state.Filters, "q", "universe", "state") ||
			len(state.Position) != 2 {
			return repository.CollectionQuery{}, nil, invalidCursorIssue()
		}
		number, err := strconv.Atoi(state.Position[0])
		if err != nil || number < 1 || !validAPISlug(state.Position[1]) {
			return repository.CollectionQuery{}, nil, invalidCursorIssue()
		}
		query := repository.CollectionQuery{
			UserID:    userID,
			CatalogID: catalogID,
			Search:    state.Filters["q"],
			Universe:  state.Filters["universe"],
			State:     repository.CollectionState(state.Filters["state"]),
			Limit:     state.Limit,
			After: &repository.CollectionPosition{
				ItemNumber: number,
				ItemID:     state.Position[1],
			},
		}
		if !validCollectionCursorQuery(query) {
			return repository.CollectionQuery{}, nil, invalidCursorIssue()
		}
		return query, state.Filters, nil
	}

	search, issue := parseSearch(raw["q"])
	if issue != nil {
		return repository.CollectionQuery{}, nil, issue
	}
	state := repository.CollectionStateAll
	if value, present := raw["state"]; present {
		state = repository.CollectionState(value)
		switch state {
		case repository.CollectionStateAll,
			repository.CollectionStateOwned,
			repository.CollectionStateAvailable,
			repository.CollectionStateWanted:
		default:
			return repository.CollectionQuery{}, nil, &queryIssue{
				code:    "invalid_query",
				field:   "state",
				message: "state must be all, owned, available, or wanted.",
			}
		}
	}
	universe := raw["universe"]
	if has(raw, "universe") &&
		(universe == "" || !utf8.ValidString(universe) || utf8.RuneCountInString(universe) > 100) {
		return repository.CollectionQuery{}, nil, &queryIssue{
			code:    "invalid_query",
			field:   "universe",
			message: "universe must be a reviewed value of at most 100 characters.",
		}
	}
	limit, issue := parseLimit(
		raw["limit"],
		has(raw, "limit"),
		repository.DefaultCollectionPageSize,
		repository.MaxCollectionPageSize,
	)
	if issue != nil {
		return repository.CollectionQuery{}, nil, issue
	}
	filters := map[string]string{
		"q":        search,
		"universe": universe,
		"state":    string(state),
	}
	return repository.CollectionQuery{
		UserID:    userID,
		CatalogID: catalogID,
		Search:    search,
		Universe:  universe,
		State:     state,
		Limit:     limit,
	}, filters, nil
}

func validCollectionCursorQuery(query repository.CollectionQuery) bool {
	if _, issue := parseSearch(query.Search); issue != nil {
		return false
	}
	switch query.State {
	case repository.CollectionStateAll,
		repository.CollectionStateOwned,
		repository.CollectionStateAvailable,
		repository.CollectionStateWanted:
	default:
		return false
	}
	return query.Limit >= 1 && query.Limit <= repository.MaxCollectionPageSize
}

func encodeNextCursor(
	codecs interface {
		Encode(pagination.State) (string, error)
	},
	state pagination.State,
) (*string, error) {
	token, err := codecs.Encode(state)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func has(values map[string]string, name string) bool {
	_, exists := values[name]
	return exists
}
