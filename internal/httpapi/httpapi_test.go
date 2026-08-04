package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"testing"

	catalogdata "github.com/PatrickQuintal/ooshdaq/internal/catalog"
	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	"github.com/PatrickQuintal/ooshdaq/internal/repository/sqliterepo"
	"github.com/PatrickQuintal/ooshdaq/internal/service"
)

var testUsers = []domain.User{
	{ID: "mock-alex", DisplayName: "Alex Example"},
	{ID: "mock-bailey", DisplayName: "Bailey Example"},
	{ID: "mock-casey", DisplayName: "Casey Example"},
	{ID: "mock-devon", DisplayName: "Devon Example"},
}

type testSession struct {
	Authenticated bool          `json:"authenticated"`
	User          *domain.User  `json:"user"`
	AuthMode      string        `json:"auth_mode"`
	CSRFToken     string        `json:"csrf_token"`
	MockUsers     []domain.User `json:"mock_users"`
}

type testEnvironment struct {
	api       *API
	store     *sqliterepo.Store
	manifests []catalogdata.LoadedManifest
}

func newTestEnvironment(t *testing.T, staticFS fs.FS) testEnvironment {
	t.Helper()
	store, err := sqliterepo.Open(context.Background(), sqliterepo.Config{
		Path: filepath.Join(t.TempDir(), "httpapi.sqlite"),
	})
	if err != nil {
		t.Fatalf("sqliterepo.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close SQLite store: %v", err)
		}
	})
	loaded, err := catalogdata.LoadEmbeddedDemoManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedDemoManifests: %v", err)
	}
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("ImportManifests: %v", err)
	}
	for _, user := range testUsers {
		if err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatalf("UpsertUser: %v", err)
		}
	}
	give := loaded[0].Manifest.Items[0].ID
	receive := loaded[1].Manifest.Items[0].ID
	for _, write := range []repository.CollectionWrite{
		{UserID: "mock-alex", ItemID: give, Owned: 3, Available: 2},
		{UserID: "mock-alex", ItemID: receive, Wanted: 2},
		{UserID: "mock-bailey", ItemID: give, Wanted: 1},
		{UserID: "mock-bailey", ItemID: receive, Owned: 1, Available: 1},
		{UserID: "mock-casey", ItemID: give, Wanted: 1},
		{UserID: "mock-casey", ItemID: receive, Owned: 1, Available: 1},
	} {
		if _, err := store.SaveCollectionEntry(context.Background(), write); err != nil {
			t.Fatalf("SaveCollectionEntry: %v", err)
		}
	}
	svc, err := service.New(service.Dependencies{
		Health:      store,
		Users:       store,
		Activity:    store,
		Catalogs:    store,
		Collections: store,
		Writes:      store,
		Matches:     store,
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	api, err := New(svc, Config{
		AuthMode:      AuthModeMock,
		AllowMockAuth: true,
		MockUsers:     testUsers,
		SessionSecret: []byte("test-session-key-32-bytes-long!!"),
		CursorKey:     []byte("test-cursor-key-32-bytes-long!!!!"),
		StaticFS:      staticFS,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return testEnvironment{api: api, store: store, manifests: loaded}
}

func newAuthenticatedClient(
	t *testing.T,
	serverURL string,
	userID string,
) (*http.Client, testSession) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}
	session := getSession(t, client, serverURL, nil)
	response := doRequest(
		t,
		client,
		http.MethodPost,
		serverURL+"/auth/mock-login",
		map[string]any{"user_id": userID},
		map[string]string{"X-CSRF-Token": session.CSRFToken},
	)
	assertJSONStatus(t, response, http.StatusOK, "")
	return client, getSession(t, client, serverURL, nil)
}

func getSession(
	t *testing.T,
	client *http.Client,
	serverURL string,
	headers map[string]string,
) testSession {
	t.Helper()
	response := doRequest(t, client, http.MethodGet, serverURL+"/api/v1/session", nil, headers)
	var session testSession
	decodeJSONResponse(t, response, http.StatusOK, &session)
	return session
}

func doRequest(
	t *testing.T,
	client *http.Client,
	method string,
	target string,
	body any,
	headers map[string]string,
) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	return response
}

func decodeJSONResponse(t *testing.T, response *http.Response, status int, destination any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, status, body)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertJSONStatus(t *testing.T, response *http.Response, status int, code string) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, status, body)
	}
	if code == "" {
		return
	}
	var envelope errorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if envelope.Error.Code != code {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, code)
	}
}

func queryURL(base, path string, values url.Values) string {
	return base + path + "?" + values.Encode()
}
