package auth

import (
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ProxyConfig names the headers set by an authenticating reverse proxy.
// Deployments must ensure untrusted clients cannot reach $OOSHDAQ directly and
// that the proxy strips client-supplied copies of these headers.
type ProxyConfig struct {
	UserHeader        string
	DisplayNameHeader string
}

// ProxyAuthenticator trusts identity headers only because this mode must be
// selected explicitly by deployment configuration.
type ProxyAuthenticator struct {
	userHeader        string
	displayNameHeader string
}

// NewProxy constructs an authenticator for a trusted reverse proxy.
func NewProxy(config ProxyConfig) (*ProxyAuthenticator, error) {
	if !validHeaderName(config.UserHeader) {
		return nil, errors.New("auth: a valid proxy user header is required")
	}
	if config.DisplayNameHeader != "" && !validHeaderName(config.DisplayNameHeader) {
		return nil, errors.New("auth: invalid proxy display-name header")
	}
	if http.CanonicalHeaderKey(config.UserHeader) == http.CanonicalHeaderKey(config.DisplayNameHeader) {
		return nil, errors.New("auth: proxy identity headers must be distinct")
	}
	return &ProxyAuthenticator{
		userHeader:        http.CanonicalHeaderKey(config.UserHeader),
		displayNameHeader: http.CanonicalHeaderKey(config.DisplayNameHeader),
	}, nil
}

func (a *ProxyAuthenticator) Authenticate(request *http.Request) (Identity, error) {
	userValues := request.Header.Values(a.userHeader)
	if len(userValues) == 0 || strings.TrimSpace(userValues[0]) == "" {
		return Identity{}, ErrUnauthenticated
	}
	if len(userValues) != 1 {
		return Identity{}, ErrInvalidIdentity
	}
	userID := strings.TrimSpace(userValues[0])
	// A comma commonly indicates that an intermediary joined duplicate header
	// values. Reject it rather than guess which asserted identity should win.
	if strings.ContainsAny(userID, ",\r\n") || len(userID) > 200 || !utf8.ValidString(userID) {
		return Identity{}, ErrInvalidIdentity
	}

	displayName := ""
	if a.displayNameHeader != "" {
		displayValues := request.Header.Values(a.displayNameHeader)
		if len(displayValues) > 1 {
			return Identity{}, ErrInvalidIdentity
		}
		if len(displayValues) == 1 {
			displayName = strings.TrimSpace(displayValues[0])
			if len(displayName) > 100 || !utf8.ValidString(displayName) {
				return Identity{}, ErrInvalidIdentity
			}
			for _, r := range displayName {
				if unicode.IsControl(r) {
					return Identity{}, ErrInvalidIdentity
				}
			}
		}
	}
	return Identity{UserID: userID, DisplayName: displayName}, nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}
