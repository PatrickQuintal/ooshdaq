package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ooshdaq/ooshdaq/internal/pagination"
)

type queryIssue struct {
	code    string
	field   string
	message string
}

func (issue *queryIssue) Error() string {
	if issue == nil {
		return ""
	}
	return issue.message
}

func (a *API) writeQueryIssue(response http.ResponseWriter, issue *queryIssue) {
	a.writeFieldError(
		response,
		http.StatusBadRequest,
		issue.code,
		issue.message,
		issue.field,
	)
}

func requestQuery(
	request *http.Request,
	allowed ...string,
) (map[string]string, *queryIssue) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return nil, &queryIssue{
			code:    "invalid_query",
			field:   "query",
			message: "The query string is malformed.",
		}
	}
	return scalarQuery(values, allowed...)
}

func scalarQuery(values url.Values, allowed ...string) (map[string]string, *queryIssue) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make(map[string]string, len(names))
	for _, name := range names {
		if _, ok := allowedSet[name]; !ok {
			return nil, &queryIssue{
				code:    "unknown_query_parameter",
				field:   name,
				message: name + " is not accepted by this route.",
			}
		}
		if len(values[name]) != 1 {
			return nil, &queryIssue{
				code:    "invalid_query",
				field:   name,
				message: name + " must be supplied at most once.",
			}
		}
		result[name] = values[name][0]
	}
	if _, hasCursor := result["cursor"]; hasCursor && len(result) != 1 {
		return nil, &queryIssue{
			code:    "cursor_conflict",
			field:   "cursor",
			message: "cursor cannot be combined with other query parameters.",
		}
	}
	return result, nil
}

func parseLimit(raw string, present bool, fallback, maximum int) (int, *queryIssue) {
	if !present {
		return fallback, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximum {
		return 0, &queryIssue{
			code:    "invalid_query",
			field:   "limit",
			message: fmt.Sprintf("limit must be an integer from 1 through %d.", maximum),
		}
	}
	return limit, nil
}

func parseSearch(raw string) (string, *queryIssue) {
	search := strings.TrimSpace(raw)
	if !utf8.ValidString(search) || utf8.RuneCountInString(search) > 100 {
		return "", &queryIssue{
			code:    "invalid_query",
			field:   "q",
			message: "q must contain at most 100 valid Unicode characters.",
		}
	}
	return search, nil
}

func requireSlug(raw, field string) *queryIssue {
	if validAPISlug(raw) {
		return nil
	}
	return &queryIssue{
		code:    "invalid_query",
		field:   field,
		message: field + " must be a lowercase identifier.",
	}
}

func validAPISlug(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case character == '-' && index > 0:
		default:
			return false
		}
	}
	return value[len(value)-1] != '-'
}

func (a *API) decodeCursor(
	token string,
	route string,
	subject string,
) (pagination.State, *queryIssue) {
	state, err := a.cursors.Decode(token, route, subject)
	if err != nil {
		return pagination.State{}, invalidCursorIssue()
	}
	return state, nil
}

func invalidCursorIssue() *queryIssue {
	return &queryIssue{
		code:    "invalid_cursor",
		field:   "cursor",
		message: "The result cursor is invalid or has expired.",
	}
}

func exactFilters(filters map[string]string, names ...string) bool {
	if len(filters) != len(names) {
		return false
	}
	for _, name := range names {
		if _, exists := filters[name]; !exists {
			return false
		}
	}
	return true
}
