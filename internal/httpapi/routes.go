package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

func (a *API) route(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/health/live":
		a.handleLive(response, request)
	case "/health/ready":
		a.handleReady(response, request)
	case "/api/v1/session":
		a.handleSession(response, request)
	case "/api/v1/activity":
		a.handleActivity(response, request)
	case "/api/v1/catalogs":
		a.handleCatalogs(response, request)
	case "/api/v1/collection/summary":
		a.handleCollectionSummary(response, request)
	case "/api/v1/matches":
		a.handleMatches(response, request)
	case "/auth/mock-login":
		a.handleMockLogin(response, request)
	case "/auth/logout":
		a.handleLogout(response, request)
	default:
		switch {
		case strings.HasPrefix(request.URL.Path, "/api/v1/catalogs/"):
			a.handleScopedCollection(response, request)
		case strings.HasPrefix(request.URL.Path, "/api/v1/collection/"):
			a.handleCollectionItem(response, request)
		default:
			if isReservedPath(request.URL.Path) {
				a.writeError(
					response,
					http.StatusNotFound,
					"not_found",
					"The requested endpoint was not found.",
				)
				return
			}
			if a.static != nil && a.static.serve(response, request) {
				return
			}
			http.NotFound(response, request)
		}
	}
}

func (a *API) handleLive(response http.ResponseWriter, request *http.Request) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	a.writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleReady(response http.ResponseWriter, request *http.Request) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := a.service.Ready(ctx); err != nil {
		a.logger.Error("readiness check failed", "error_type", fmt.Sprintf("%T", err))
		a.writeError(
			response,
			http.StatusServiceUnavailable,
			"service_unavailable",
			"The service is not ready.",
		)
		return
	}
	a.writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) requireMethod(
	response http.ResponseWriter,
	request *http.Request,
	method string,
) bool {
	if request.Method == method {
		return true
	}
	response.Header().Set("Allow", method)
	a.writeError(
		response,
		http.StatusMethodNotAllowed,
		"method_not_allowed",
		"The request method is not allowed.",
	)
	return false
}

func (a *API) readServiceError(
	response http.ResponseWriter,
	operation string,
	err error,
) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		a.writeError(
			response,
			http.StatusNotFound,
			"not_found",
			"The requested resource was not found.",
		)
	case errors.Is(err, repository.ErrInvalidArgument):
		var validationError domain.ValidationError
		if errors.As(err, &validationError) && validationError.Field != "" {
			a.writeFieldError(
				response,
				http.StatusBadRequest,
				"invalid_query",
				"The query is invalid.",
				validationError.Field,
			)
		} else {
			a.writeError(
				response,
				http.StatusBadRequest,
				"invalid_query",
				"The query is invalid.",
			)
		}
	case storageUnavailable(err):
		a.logger.Error(operation, "failure_kind", repository.Classify(err))
		a.writeError(
			response,
			http.StatusServiceUnavailable,
			"service_unavailable",
			"The service cannot safely complete the request.",
		)
	default:
		a.internalError(response, operation, err)
	}
}

func (a *API) writeServiceError(
	response http.ResponseWriter,
	operation string,
	err error,
) {
	var validationError domain.ValidationError
	switch {
	case errors.As(err, &validationError),
		errors.Is(err, repository.ErrInvalidArgument):
		field := validationError.Field
		if field == "" {
			field = "entry"
		}
		a.writeFieldError(
			response,
			http.StatusUnprocessableEntity,
			"validation_failed",
			"The collection quantities are invalid.",
			field,
		)
	case errors.Is(err, repository.ErrNotFound):
		a.writeError(
			response,
			http.StatusNotFound,
			"not_found",
			"The requested resource was not found.",
		)
	case storageUnavailable(err):
		a.logger.Error(operation, "failure_kind", repository.Classify(err))
		a.writeError(
			response,
			http.StatusServiceUnavailable,
			"service_unavailable",
			"The service cannot safely complete the request.",
		)
	default:
		a.internalError(response, operation, err)
	}
}

func storageUnavailable(err error) bool {
	for _, target := range []error{
		repository.ErrBusy,
		repository.ErrUnavailable,
		repository.ErrReadOnly,
		repository.ErrFull,
		repository.ErrCorrupt,
		repository.ErrIncompatible,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (a *API) internalError(response http.ResponseWriter, operation string, err error) {
	a.logger.Error(operation, "error_type", fmt.Sprintf("%T", err))
	a.writeError(
		response,
		http.StatusInternalServerError,
		"internal_error",
		"The request could not be completed.",
	)
}

func isReservedPath(requestPath string) bool {
	for _, prefix := range []string{"/api", "/auth", "/health"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}
