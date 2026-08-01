package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultMockCookieName = "ooshdaq_mock_session"
	defaultMockSessionTTL = 12 * time.Hour
)

// MockConfig configures local-development session cookies.
type MockConfig struct {
	CookieName string
	Secure     bool
	TTL        time.Duration
	SigningKey []byte
	Now        func() time.Time
}

// MockAuthenticator issues signed, short-lived cookies for fictional demo
// users. It is not an identity provider and must never be enabled in deployment.
type MockAuthenticator struct {
	cookieName string
	secure     bool
	ttl        time.Duration
	signingKey []byte
	now        func() time.Time
}

type mockClaims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
	Nonce   string `json:"nonce"`
}

// NewMock constructs a local mock authenticator. When SigningKey is omitted, a
// process-local 256-bit key is generated, invalidating sessions on restart.
func NewMock(config MockConfig) (*MockAuthenticator, error) {
	if config.CookieName == "" {
		config.CookieName = DefaultMockCookieName
	}
	if !validCookieName(config.CookieName) {
		return nil, errors.New("auth: invalid mock cookie name")
	}
	if config.TTL == 0 {
		config.TTL = defaultMockSessionTTL
	}
	if config.TTL < time.Minute || config.TTL > 7*24*time.Hour {
		return nil, errors.New("auth: mock session TTL must be between one minute and seven days")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	key := append([]byte(nil), config.SigningKey...)
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("auth: generate mock signing key: %w", err)
		}
	}
	if len(key) < 32 {
		return nil, errors.New("auth: mock signing key must contain at least 32 bytes")
	}
	return &MockAuthenticator{
		cookieName: config.CookieName,
		secure:     config.Secure,
		ttl:        config.TTL,
		signingKey: key,
		now:        config.Now,
	}, nil
}

func (a *MockAuthenticator) Authenticate(request *http.Request) (Identity, error) {
	cookie, err := request.Cookie(a.cookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return Identity{}, ErrUnauthenticated
		}
		return Identity{}, ErrInvalidIdentity
	}
	claims, err := a.parse(cookie.Value)
	if err != nil {
		return Identity{}, ErrInvalidIdentity
	}
	if !a.now().Before(time.Unix(claims.Expires, 0)) {
		return Identity{}, ErrUnauthenticated
	}
	return Identity{UserID: claims.Subject}, nil
}

// Login writes a new signed session for userID.
func (a *MockAuthenticator) Login(response http.ResponseWriter, userID string) error {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("auth: generate session nonce: %w", err)
	}
	expires := a.now().Add(a.ttl)
	claims := mockClaims{
		Subject: userID,
		Expires: expires.Unix(),
		Nonce:   base64.RawURLEncoding.EncodeToString(nonce),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("auth: encode session: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	value := encodedPayload + "." + a.signature(encodedPayload)

	http.SetCookie(response, &http.Cookie{
		Name:     a.cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(a.ttl.Seconds()),
		Expires:  expires,
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

// Logout expires the browser's mock session cookie.
func (a *MockAuthenticator) Logout(response http.ResponseWriter) {
	http.SetCookie(response, &http.Cookie{
		Name:     a.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *MockAuthenticator) parse(value string) (mockClaims, error) {
	payload, signature, ok := strings.Cut(value, ".")
	if !ok || payload == "" || signature == "" {
		return mockClaims{}, ErrInvalidIdentity
	}
	expectedSignature := a.signature(payload)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return mockClaims{}, ErrInvalidIdentity
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return mockClaims{}, ErrInvalidIdentity
	}
	var claims mockClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return mockClaims{}, ErrInvalidIdentity
	}
	if claims.Subject == "" || claims.Expires <= 0 || claims.Nonce == "" {
		return mockClaims{}, ErrInvalidIdentity
	}
	return claims, nil
}

func (a *MockAuthenticator) signature(payload string) string {
	mac := hmac.New(sha256.New, a.signingKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validCookieName(name string) bool {
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
