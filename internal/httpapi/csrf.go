package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
)

const csrfHeaderName = "X-CSRF-Token"

type csrfProtector struct {
	cookieName string
	secure     bool
}

func newCSRFProtector(secure bool) (*csrfProtector, error) {
	name := "ooshdaq_csrf"
	if secure {
		name = "__Host-ooshdaq_csrf"
	}
	return &csrfProtector{cookieName: name, secure: secure}, nil
}

func (p *csrfProtector) token(response http.ResponseWriter, request *http.Request) (string, error) {
	if cookie, err := request.Cookie(p.cookieName); err == nil && validCSRFToken(cookie.Value) {
		return cookie.Value, nil
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	http.SetCookie(response, &http.Cookie{
		Name:     p.cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((12 * time.Hour).Seconds()),
		Expires:  time.Now().Add(12 * time.Hour),
		HttpOnly: true,
		Secure:   p.secure,
		SameSite: http.SameSiteStrictMode,
	})
	return token, nil
}

func (p *csrfProtector) validate(request *http.Request) error {
	if strings.EqualFold(request.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return errors.New("cross-site request")
	}
	headerValues := request.Header.Values(csrfHeaderName)
	if len(headerValues) != 1 || !validCSRFToken(headerValues[0]) {
		return errors.New("missing or invalid CSRF header")
	}
	cookie, err := request.Cookie(p.cookieName)
	if err != nil || !validCSRFToken(cookie.Value) {
		return errors.New("missing or invalid CSRF cookie")
	}
	if subtle.ConstantTimeCompare([]byte(headerValues[0]), []byte(cookie.Value)) != 1 {
		return errors.New("CSRF token mismatch")
	}
	return nil
}

func validCSRFToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == 32
}
