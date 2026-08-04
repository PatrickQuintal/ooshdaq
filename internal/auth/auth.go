// Package auth translates an explicitly selected authentication mechanism into
// a small identity value. Authorization and user persistence remain in the
// service and HTTP layers.
package auth

import (
	"errors"
	"net/http"
)

var (
	// ErrUnauthenticated means the request has no valid identity.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrInvalidIdentity means authentication material was present but malformed
	// or ambiguous. It is deliberately presented to clients as unauthenticated.
	ErrInvalidIdentity = errors.New("invalid authentication identity")
)

// Identity contains only the stable values $OOSHDAQ needs from authentication.
type Identity struct {
	UserID      string
	DisplayName string
}

// Authenticator resolves the current request identity.
type Authenticator interface {
	Authenticate(request *http.Request) (Identity, error)
}
