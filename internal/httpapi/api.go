// Package httpapi exposes the $OOSHDAQ application service over a versioned JSON
// API and can serve the embedded browser application.
package httpapi

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/auth"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/pagination"
	"github.com/PatrickQuintal/ooshdaq/internal/service"
)

// AuthMode selects exactly one authentication boundary.
type AuthMode string

const (
	AuthModeMock  AuthMode = "mock"
	AuthModeProxy AuthMode = "proxy"

	defaultMaxBodyBytes int64 = 16 << 10
)

// Config controls HTTP behavior. Mock mode requires both AuthModeMock and
// AllowMockAuth, making accidental deployment less likely.
type Config struct {
	AuthMode      AuthMode
	AllowMockAuth bool
	MockUsers     []domain.User

	SessionSecret          []byte
	ProxyUserHeader        string
	ProxyDisplayNameHeader string

	CookieSecure bool
	MaxBodyBytes int64
	CursorKey    []byte
	CursorTTL    time.Duration
	StaticFS     fs.FS
	Logger       *slog.Logger
}

// API is an http.Handler containing the API, auth endpoints, health probes, and
// optional static browser application.
type API struct {
	service       *service.Service
	authMode      AuthMode
	authenticator auth.Authenticator
	mockAuth      *auth.MockAuthenticator
	mockUsers     []domain.User
	mockUserIDs   map[string]struct{}
	csrf          *csrfProtector
	cursors       *pagination.Codec
	maxBodyBytes  int64
	static        *staticHandler
	logger        *slog.Logger
	now           func() time.Time
}

// New validates config and constructs an HTTP handler.
func New(svc *service.Service, config Config) (*API, error) {
	if svc == nil {
		return nil, errors.New("httpapi: service is required")
	}
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = defaultMaxBodyBytes
	}
	if config.MaxBodyBytes < 256 || config.MaxBodyBytes > 1<<20 {
		return nil, errors.New("httpapi: MaxBodyBytes must be between 256 bytes and 1 MiB")
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	api := &API{
		service:      svc,
		authMode:     config.AuthMode,
		maxBodyBytes: config.MaxBodyBytes,
		logger:       config.Logger,
		now:          time.Now,
	}

	var err error
	switch config.AuthMode {
	case AuthModeMock:
		if !config.AllowMockAuth {
			return nil, errors.New("httpapi: mock authentication requires AllowMockAuth")
		}
		if len(config.MockUsers) == 0 {
			return nil, errors.New("httpapi: mock authentication requires an explicit user allowlist")
		}
		api.mockAuth, err = auth.NewMock(auth.MockConfig{
			Secure:     config.CookieSecure,
			SigningKey: config.SessionSecret,
		})
		if err != nil {
			return nil, err
		}
		api.authenticator = api.mockAuth
		api.mockUsers = append([]domain.User(nil), config.MockUsers...)
		sortUsers(api.mockUsers)
		api.mockUserIDs = make(map[string]struct{}, len(api.mockUsers))
		for _, user := range api.mockUsers {
			if err := user.Validate(); err != nil {
				return nil, fmt.Errorf("httpapi: invalid mock user %q: %w", user.ID, err)
			}
			if _, duplicate := api.mockUserIDs[user.ID]; duplicate {
				return nil, fmt.Errorf("httpapi: duplicate mock user %q", user.ID)
			}
			api.mockUserIDs[user.ID] = struct{}{}
		}
	case AuthModeProxy:
		api.authenticator, err = auth.NewProxy(auth.ProxyConfig{
			UserHeader:        config.ProxyUserHeader,
			DisplayNameHeader: config.ProxyDisplayNameHeader,
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("httpapi: unsupported authentication mode %q", config.AuthMode)
	}

	api.csrf, err = newCSRFProtector(config.CookieSecure)
	if err != nil {
		return nil, err
	}
	api.cursors, err = pagination.New(config.CursorKey, config.CursorTTL)
	if err != nil {
		return nil, err
	}
	if config.StaticFS != nil {
		api.static, err = newStaticHandler(config.StaticFS)
		if err != nil {
			return nil, err
		}
	}
	return api, nil
}

func sortUsers(users []domain.User) {
	sort.Slice(users, func(i, j int) bool {
		left := strings.ToLower(users[i].DisplayName)
		right := strings.ToLower(users[j].DisplayName)
		if left != right {
			return left < right
		}
		if users[i].DisplayName != users[j].DisplayName {
			return users[i].DisplayName < users[j].DisplayName
		}
		return users[i].ID < users[j].ID
	})
}

func (a *API) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	started := time.Now()
	recorder := &statusRecorder{ResponseWriter: response, status: http.StatusOK}
	defer func() {
		if recovered := recover(); recovered != nil {
			a.logger.Error("panic serving request", "method", request.Method, "path", request.URL.Path)
			if !recorder.wroteHeader {
				a.writeError(recorder, http.StatusInternalServerError, "internal_error", "The request could not be completed.")
			}
		}
		a.logger.Info(
			"http request",
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}()

	setSecurityHeaders(recorder, a.csrf.secure)
	a.route(recorder, request)
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func setSecurityHeaders(response http.ResponseWriter, secure bool) {
	header := response.Header()
	header.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; connect-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'self' https://cdn0.woolworths.media; object-src 'none'; script-src 'self'; style-src 'self'")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	if secure {
		header.Set("Strict-Transport-Security", "max-age=31536000")
	}
}
