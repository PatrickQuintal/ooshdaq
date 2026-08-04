package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMockSessionRoundTripAndTamperRejection(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	mock, err := NewMock(MockConfig{
		SigningKey: []byte("01234567890123456789012345678901"),
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewMock: %v", err)
	}
	recorder := httptest.NewRecorder()
	if err := mock.Login(recorder, "mock-alex"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie lacks security attributes: %#v", cookies[0])
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookies[0])
	identity, err := mock.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if identity.UserID != "mock-alex" {
		t.Fatalf("identity = %#v", identity)
	}

	cookies[0].Value += "tampered"
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookies[0])
	if _, err := mock.Authenticate(request); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("tampered cookie error = %v, want ErrInvalidIdentity", err)
	}
}

func TestMockSessionExpires(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	mock, err := NewMock(MockConfig{
		SigningKey: []byte("01234567890123456789012345678901"),
		TTL:        time.Minute,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewMock: %v", err)
	}
	recorder := httptest.NewRecorder()
	if err := mock.Login(recorder, "mock-alex"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(recorder.Result().Cookies()[0])

	now = now.Add(time.Minute)
	if _, err := mock.Authenticate(request); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session error = %v, want ErrUnauthenticated", err)
	}
}

func TestProxyAuthenticatorRejectsMissingAndAmbiguousHeaders(t *testing.T) {
	t.Parallel()

	proxy, err := NewProxy(ProxyConfig{
		UserHeader:        "X-Verified-User",
		DisplayNameHeader: "X-Verified-Name",
	})
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := proxy.Authenticate(request); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing header error = %v, want ErrUnauthenticated", err)
	}

	request.Header.Add("X-Verified-User", "subject-one")
	request.Header.Add("X-Verified-User", "subject-two")
	if _, err := proxy.Authenticate(request); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("ambiguous header error = %v, want ErrInvalidIdentity", err)
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Verified-User", "subject-one,subject-two")
	if _, err := proxy.Authenticate(request); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("comma-joined header error = %v, want ErrInvalidIdentity", err)
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Verified-User", "subject-one")
	request.Header.Set("X-Verified-Name", "Example Person")
	identity, err := proxy.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if identity.UserID != "subject-one" || identity.DisplayName != "Example Person" {
		t.Fatalf("identity = %#v", identity)
	}
}
