package httpapi

import (
	"strconv"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/repository"
)

const matchesCursorRoute = "/api/v1/matches"

func (a *API) matchQuery(
	raw map[string]string,
	userID string,
) (repository.MatchQuery, map[string]string, *queryIssue) {
	if token, continuation := raw["cursor"]; continuation {
		state, issue := a.decodeCursor(token, matchesCursorRoute, userID)
		if issue != nil || !exactFilters(
			state.Filters,
			"give_catalog_id",
			"receive_catalog_id",
			"scope",
			"readiness",
			"ranking_at",
		) || len(state.Position) != 6 {
			return repository.MatchQuery{}, nil, invalidCursorIssue()
		}
		rankingAt, err := time.Parse(time.RFC3339Nano, state.Filters["ranking_at"])
		requestClass, classErr := strconv.Atoi(state.Position[0])
		cappedSince, sinceErr := strconv.ParseInt(state.Position[1], 10, 64)
		stale, staleOK := parseCursorBoolean(state.Position[2])
		if err != nil || classErr != nil || sinceErr != nil || !staleOK {
			return repository.MatchQuery{}, nil, invalidCursorIssue()
		}
		query := repository.MatchQuery{
			UserID:           userID,
			GiveCatalogID:    state.Filters["give_catalog_id"],
			ReceiveCatalogID: state.Filters["receive_catalog_id"],
			Scope:            repository.MatchScope(state.Filters["scope"]),
			Readiness:        repository.MatchReadiness(state.Filters["readiness"]),
			RankingAt:        rankingAt,
			Limit:            state.Limit,
			After: &repository.MatchPosition{
				RequestClass:  requestClass,
				CappedSince:   cappedSince,
				Stale:         stale,
				RequestItemID: state.Position[3],
				CounterpartID: state.Position[4],
				ReceiveItemID: state.Position[5],
			},
		}
		if !validMatchCursorQuery(query) {
			return repository.MatchQuery{}, nil, invalidCursorIssue()
		}
		return query, state.Filters, nil
	}

	for _, field := range []string{"give_catalog_id", "receive_catalog_id"} {
		if value, present := raw[field]; present {
			if issue := requireSlug(value, field); issue != nil {
				return repository.MatchQuery{}, nil, issue
			}
		}
	}
	scope := repository.MatchScopeAll
	if value, present := raw["scope"]; present {
		scope = repository.MatchScope(value)
		switch scope {
		case repository.MatchScopeAll,
			repository.MatchScopeSameCatalog,
			repository.MatchScopeCrossCatalog:
		default:
			return repository.MatchQuery{}, nil, &queryIssue{
				code:    "invalid_query",
				field:   "scope",
				message: "scope must be all, same_catalog, or cross_catalog.",
			}
		}
	}
	readiness := repository.MatchReadinessActionable
	if value, present := raw["readiness"]; present {
		readiness = repository.MatchReadiness(value)
		switch readiness {
		case repository.MatchReadinessActionable,
			repository.MatchReadinessUnfulfillable:
		default:
			return repository.MatchQuery{}, nil, &queryIssue{
				code:    "invalid_query",
				field:   "readiness",
				message: "readiness must be actionable or unfulfillable.",
			}
		}
	}
	if readiness == repository.MatchReadinessUnfulfillable &&
		(raw["give_catalog_id"] != "" ||
			scope != repository.MatchScopeAll) {
		return repository.MatchQuery{}, nil, &queryIssue{
			code:    "invalid_query",
			field:   "readiness",
			message: "unfulfillable cannot be combined with give_catalog_id or a reciprocal scope.",
		}
	}
	limit, issue := parseLimit(
		raw["limit"],
		has(raw, "limit"),
		repository.DefaultMatchPageSize,
		repository.MaxMatchPageSize,
	)
	if issue != nil {
		return repository.MatchQuery{}, nil, issue
	}
	rankingAt := a.now().UTC().Truncate(time.Second)
	filters := map[string]string{
		"give_catalog_id":    raw["give_catalog_id"],
		"receive_catalog_id": raw["receive_catalog_id"],
		"scope":              string(scope),
		"readiness":          string(readiness),
		"ranking_at":         rankingAt.Format(time.RFC3339Nano),
	}
	return repository.MatchQuery{
		UserID:           userID,
		GiveCatalogID:    raw["give_catalog_id"],
		ReceiveCatalogID: raw["receive_catalog_id"],
		Scope:            scope,
		Readiness:        readiness,
		RankingAt:        rankingAt,
		Limit:            limit,
	}, filters, nil
}

func validMatchCursorQuery(query repository.MatchQuery) bool {
	if query.GiveCatalogID != "" && !validAPISlug(query.GiveCatalogID) {
		return false
	}
	if query.ReceiveCatalogID != "" && !validAPISlug(query.ReceiveCatalogID) {
		return false
	}
	switch query.Scope {
	case repository.MatchScopeAll,
		repository.MatchScopeSameCatalog,
		repository.MatchScopeCrossCatalog:
	default:
		return false
	}
	switch query.Readiness {
	case repository.MatchReadinessActionable:
	case repository.MatchReadinessUnfulfillable:
		if query.GiveCatalogID != "" || query.Scope != repository.MatchScopeAll {
			return false
		}
		if query.After != nil && query.After.RequestClass != 0 {
			return false
		}
	default:
		return false
	}
	return query.Limit >= 1 &&
		query.Limit <= repository.MaxMatchPageSize &&
		!query.RankingAt.IsZero() &&
		query.RankingAt.Unix() > 0 &&
		query.After != nil &&
		query.After.RequestClass >= 0 &&
		query.After.RequestClass <= 1 &&
		query.After.CappedSince > 0 &&
		((query.After.RequestClass == 0 &&
			validAPISlug(query.After.RequestItemID)) ||
			(query.After.RequestClass == 1 &&
				query.After.RequestItemID == "")) &&
		query.After.CounterpartID != "" &&
		validAPISlug(query.After.ReceiveItemID)
}

func parseCursorBoolean(value string) (bool, bool) {
	switch value {
	case "0":
		return false, true
	case "1":
		return true, true
	default:
		return false, false
	}
}
