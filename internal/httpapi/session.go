package httpapi

import (
	"errors"
	"net/http"

	"github.com/PatrickQuintal/ooshdaq/internal/auth"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

type sessionResponse struct {
	Authenticated bool          `json:"authenticated"`
	User          *domain.User  `json:"user,omitempty"`
	AuthMode      AuthMode      `json:"auth_mode"`
	CSRFToken     string        `json:"csrf_token"`
	MockUsers     []domain.User `json:"mock_users,omitempty"`
}

func (a *API) handleSession(response http.ResponseWriter, request *http.Request) {
	if !a.requireMethod(response, request, http.MethodGet) {
		return
	}
	token, err := a.csrf.token(response, request)
	if err != nil {
		a.internalError(response, "create CSRF token", err)
		return
	}
	payload := sessionResponse{AuthMode: a.authMode, CSRFToken: token}
	if a.authMode == AuthModeMock {
		payload.MockUsers = append([]domain.User(nil), a.mockUsers...)
	}
	user, err := a.currentUser(request)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) ||
			errors.Is(err, auth.ErrInvalidIdentity) {
			a.writeJSON(response, http.StatusOK, payload)
			return
		}
		a.internalError(response, "resolve session", err)
		return
	}
	payload.Authenticated = true
	payload.User = &user
	a.writeJSON(response, http.StatusOK, payload)
}

func (a *API) handleMockLogin(response http.ResponseWriter, request *http.Request) {
	if a.authMode != AuthModeMock {
		a.writeError(
			response,
			http.StatusNotFound,
			"not_found",
			"The requested endpoint was not found.",
		)
		return
	}
	if !a.requireMethod(response, request, http.MethodPost) ||
		!a.requireCSRF(response, request) {
		return
	}
	var input *struct {
		UserID string `json:"user_id"`
	}
	if apiErr := a.decodeJSON(response, request, &input); apiErr != nil {
		a.writeRequestError(response, apiErr)
		return
	}
	if input == nil {
		a.writeError(
			response,
			http.StatusBadRequest,
			"invalid_request",
			"The request body must contain one valid JSON object.",
		)
		return
	}
	if _, allowed := a.mockUserIDs[input.UserID]; !allowed {
		a.writeError(
			response,
			http.StatusUnauthorized,
			"invalid_mock_user",
			"That local demo profile is not available.",
		)
		return
	}
	user, err := a.service.User(request.Context(), input.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			a.writeError(
				response,
				http.StatusUnauthorized,
				"invalid_mock_user",
				"That local demo profile is not available.",
			)
			return
		}
		a.internalError(response, "load mock user", err)
		return
	}
	if err := a.mockAuth.Login(response, user.ID); err != nil {
		a.internalError(response, "create mock session", err)
		return
	}
	token, err := request.Cookie(a.csrf.cookieName)
	if err != nil {
		a.internalError(response, "read CSRF token", err)
		return
	}
	a.writeJSON(response, http.StatusOK, sessionResponse{
		Authenticated: true,
		User:          &user,
		AuthMode:      a.authMode,
		CSRFToken:     token.Value,
		MockUsers:     append([]domain.User(nil), a.mockUsers...),
	})
}

func (a *API) handleLogout(response http.ResponseWriter, request *http.Request) {
	if a.authMode != AuthModeMock {
		a.writeError(
			response,
			http.StatusNotImplemented,
			"logout_not_supported",
			"Sign out is managed by the configured identity provider.",
		)
		return
	}
	if !a.requireMethod(response, request, http.MethodPost) {
		return
	}
	if _, ok := a.requireUser(response, request); !ok {
		return
	}
	if !a.requireCSRF(response, request) {
		return
	}
	if apiErr := a.requireEmptyBody(response, request); apiErr != nil {
		a.writeRequestError(response, apiErr)
		return
	}
	a.mockAuth.Logout(response)
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusNoContent)
}

func (a *API) currentUser(request *http.Request) (domain.User, error) {
	identity, err := a.authenticator.Authenticate(request)
	if err != nil {
		return domain.User{}, err
	}
	if a.authMode == AuthModeProxy {
		user, err := a.service.EnsureUser(
			request.Context(),
			identity.UserID,
			identity.DisplayName,
		)
		if err != nil {
			var validationError domain.ValidationError
			if errors.As(err, &validationError) {
				return domain.User{}, auth.ErrInvalidIdentity
			}
			return domain.User{}, err
		}
		return user, nil
	}
	user, err := a.service.User(request.Context(), identity.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.User{}, auth.ErrUnauthenticated
	}
	return user, err
}

func (a *API) requireUser(
	response http.ResponseWriter,
	request *http.Request,
) (domain.User, bool) {
	user, err := a.currentUser(request)
	if err == nil {
		return user, true
	}
	if errors.Is(err, auth.ErrUnauthenticated) ||
		errors.Is(err, auth.ErrInvalidIdentity) {
		a.writeError(
			response,
			http.StatusUnauthorized,
			"authentication_required",
			"Authentication is required.",
		)
		return domain.User{}, false
	}
	a.internalError(response, "resolve authenticated user", err)
	return domain.User{}, false
}

func (a *API) requireCSRF(response http.ResponseWriter, request *http.Request) bool {
	if err := a.csrf.validate(request); err != nil {
		a.writeError(
			response,
			http.StatusForbidden,
			"csrf_failed",
			"The request could not be verified.",
		)
		return false
	}
	return true
}
