package httpapi

import (
	"net/http"
)

func (a *API) handleActivity(response http.ResponseWriter, request *http.Request) {
	if !a.requireMethod(response, request, http.MethodPost) {
		return
	}
	if !a.requireCSRF(response, request) {
		return
	}
	user, ok := a.requireUser(response, request)
	if !ok {
		return
	}
	if _, issue := requestQuery(request); issue != nil {
		a.writeQueryIssue(response, issue)
		return
	}
	if apiErr := a.requireEmptyBody(response, request); apiErr != nil {
		a.writeRequestError(response, apiErr)
		return
	}
	if err := a.service.TouchUserActivity(request.Context(), user.ID); err != nil {
		a.writeServiceError(response, "touch user activity", err)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusNoContent)
}
