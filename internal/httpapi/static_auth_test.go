package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ooshdaq/ooshdaq/internal/repository"
	"github.com/ooshdaq/ooshdaq/internal/service"
)

func TestStaticFilesSPAFallbackAndSecurityHeaders(t *testing.T) {
	t.Parallel()

	staticFS := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>$OOSHDAQ</title>")},
		"app.js":          {Data: []byte(`import "./js/module.js";`)},
		"js/module.js":    {Data: []byte(`export const ready = true;`)},
		"styles/main.css": {Data: []byte(`body { color: black; }`)},
	}
	server := httptest.NewServer(newTestEnvironment(t, staticFS).api)
	defer server.Close()

	for _, test := range []struct {
		path, accept, contentType, body string
		status                          int
	}{
		{path: "/", accept: "text/html", contentType: "text/html", body: "$OOSHDAQ", status: 200},
		{path: "/app.js", contentType: "javascript", body: "import", status: 200},
		{path: "/catalog/disney-ooshies-2026", accept: "text/html", contentType: "text/html", body: "$OOSHDAQ", status: 200},
		{path: "/missing.js", accept: "text/html", contentType: "text/plain", body: "404", status: 404},
		{path: "/api/v1/legacy", accept: "text/html", contentType: "application/json", body: "not_found", status: 404},
	} {
		request, err := http.NewRequest(http.MethodGet, server.URL+test.path, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if test.accept != "" {
			request.Header.Set("Accept", test.accept)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if response.StatusCode != test.status ||
			!strings.Contains(response.Header.Get("Content-Type"), test.contentType) ||
			!strings.Contains(string(body), test.body) {
			t.Errorf("%s: status=%d type=%q body=%q", test.path, response.StatusCode, response.Header.Get("Content-Type"), body)
		}
		csp := response.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self'") ||
			!strings.Contains(csp, "https://cdn0.woolworths.media") {
			t.Errorf("%s CSP = %q", test.path, csp)
		}
	}
}

func TestProxyModeProvisionsIdentityAndRejectsAmbiguity(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	svc, err := service.New(service.Dependencies{
		Health:      environment.store,
		Users:       environment.store,
		Activity:    environment.store,
		Catalogs:    environment.store,
		Collections: environment.store,
		Writes:      environment.store,
		Matches:     environment.store,
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	api, err := New(svc, Config{
		AuthMode:               AuthModeProxy,
		ProxyUserHeader:        "X-Verified-Subject",
		ProxyDisplayNameHeader: "X-Verified-Display-Name",
		CursorKey:              []byte("proxy-cursor-key-32-bytes-long!!!"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server := httptest.NewServer(api)
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	headers := map[string]string{
		"X-Verified-Subject":      "idp:fictional-subject",
		"X-Verified-Display-Name": "Proxy Example",
	}
	session := getSession(t, client, server.URL, headers)
	if !session.Authenticated || session.User == nil ||
		session.User.DisplayName != "Proxy Example" || len(session.MockUsers) != 0 {
		t.Fatalf("proxy session = %#v", session)
	}
	response := doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/catalogs",
		nil,
		headers,
	)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("proxy catalogs = %d: %s", response.StatusCode, body)
	}
	response.Body.Close()
	response = doRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/api/v1/catalogs",
		nil,
		map[string]string{"X-Verified-Subject": "first,second"},
	)
	assertJSONStatus(t, response, http.StatusUnauthorized, "authentication_required")
}

func TestHealthUsesRepositoryReadiness(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment(t, nil)
	for _, path := range []string{"/health/live", "/health/ready"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		environment.api.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", path, recorder.Code)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := environment.store.Ping(cancelled); err == nil {
		t.Fatal("cancelled memory readiness succeeded")
	}
	_, _ = environment.store.GetUser(context.Background(), "missing")
	_ = repository.Classify(nil)
}
