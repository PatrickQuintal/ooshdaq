package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const serverHelperEnvironment = "OOSHDAQ_SERVER_PROCESS_HELPER"

func TestServerSurvivesGracefulRestartAndAbruptKill(t *testing.T) {
	if os.Getenv(serverHelperEnvironment) == "1" {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
		if err := runServer(logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	runDirectory := t.TempDir()
	database := filepath.Join(runDirectory, "data", "ooshdaq.sqlite")
	backups := filepath.Join(runDirectory, "backups")
	address := unusedLoopbackAddress(t)

	process := startServerProcess(t, address, database, backups)
	client := authenticatedProcessClient(t, "http://"+address)
	catalogID, itemID := firstCatalogItem(t, client, "http://"+address)
	saveProcessEntry(t, client, "http://"+address, itemID, 3)
	stopServerProcess(t, process, syscall.SIGTERM)

	if matches, err := filepath.Glob(filepath.Join(backups, "ooshdaq-*.sqlite")); err != nil || len(matches) == 0 {
		t.Fatalf("online backup files = %v, error = %v", matches, err)
	}

	process = startServerProcess(t, address, database, backups)
	client = authenticatedProcessClient(t, "http://"+address)
	assertProcessEntry(t, client, "http://"+address, catalogID, itemID, 3)
	saveProcessEntry(t, client, "http://"+address, itemID, 4)
	stopServerProcess(t, process, os.Kill)

	process = startServerProcess(t, address, database, backups)
	client = authenticatedProcessClient(t, "http://"+address)
	assertProcessEntry(t, client, "http://"+address, catalogID, itemID, 4)
	stopServerProcess(t, process, syscall.SIGTERM)
}

type runningProcess struct {
	command *exec.Cmd
	logPath string
}

func startServerProcess(t *testing.T, address, database, backups string) *runningProcess {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create server log: %v", err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestServerSurvivesGracefulRestartAndAbruptKill$")
	command.Stdout = logFile
	command.Stderr = logFile
	command.Env = append(
		os.Environ(),
		serverHelperEnvironment+"=1",
		"OOSHDAQ_ADDR="+address,
		"OOSHDAQ_AUTH_MODE=mock",
		"OOSHDAQ_ALLOW_MOCK_AUTH=true",
		"OOSHDAQ_DATA_PATH="+database,
		"OOSHDAQ_BACKUP_DIR="+backups,
		"OOSHDAQ_BACKUP_INTERVAL=744h",
		"OOSHDAQ_BACKUP_RETENTION_COUNT=5",
		"OOSHDAQ_COOKIE_SECURE=false",
	)
	if err := command.Start(); err != nil {
		logFile.Close()
		t.Fatalf("start server process: %v", err)
	}
	if err := logFile.Close(); err != nil {
		t.Fatalf("close parent server log: %v", err)
	}
	process := &runningProcess{command: command, logPath: logPath}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	waitForProcessReady(t, process, "http://"+address)
	return process
}

func waitForProcessReady(t *testing.T, process *runningProcess, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/health/ready")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if process.command.ProcessState != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = process.command.Process.Kill()
	_ = process.command.Wait()
	output, _ := os.ReadFile(process.logPath)
	t.Fatalf("server did not become ready:\n%s", output)
}

func stopServerProcess(t *testing.T, process *runningProcess, signal os.Signal) {
	t.Helper()
	if err := process.command.Process.Signal(signal); err != nil {
		t.Fatalf("signal server with %v: %v", signal, err)
	}
	done := make(chan error, 1)
	go func() {
		done <- process.command.Wait()
	}()
	select {
	case err := <-done:
		if signal != os.Kill && err != nil {
			output, _ := os.ReadFile(process.logPath)
			t.Fatalf("graceful server exit: %v\n%s", err, output)
		}
	case <-time.After(10 * time.Second):
		_ = process.command.Process.Kill()
		t.Fatal("server did not stop within ten seconds")
	}
}

func authenticatedProcessClient(t *testing.T, baseURL string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	var session struct {
		CSRFToken string `json:"csrf_token"`
	}
	processJSON(t, client, http.MethodGet, baseURL+"/api/v1/session", nil, "", &session)
	processJSON(
		t,
		client,
		http.MethodPost,
		baseURL+"/auth/mock-login",
		map[string]string{"user_id": "mock-alex"},
		session.CSRFToken,
		&struct{}{},
	)
	return client
}

func firstCatalogItem(t *testing.T, client *http.Client, baseURL string) (string, string) {
	t.Helper()
	var catalogs struct {
		Catalogs []struct {
			ID string `json:"id"`
		} `json:"catalogs"`
	}
	processJSON(t, client, http.MethodGet, baseURL+"/api/v1/catalogs?limit=1", nil, "", &catalogs)
	if len(catalogs.Catalogs) != 1 {
		t.Fatalf("catalog response = %#v", catalogs)
	}
	catalogID := catalogs.Catalogs[0].ID
	var collection struct {
		Entries []struct {
			CatalogItem struct {
				ID string `json:"id"`
			} `json:"catalog_item"`
		} `json:"entries"`
	}
	processJSON(
		t,
		client,
		http.MethodGet,
		baseURL+"/api/v1/catalogs/"+catalogID+"/collection?limit=1",
		nil,
		"",
		&collection,
	)
	if len(collection.Entries) != 1 {
		t.Fatalf("collection response = %#v", collection)
	}
	return catalogID, collection.Entries[0].CatalogItem.ID
}

func saveProcessEntry(
	t *testing.T,
	client *http.Client,
	baseURL string,
	itemID string,
	owned int,
) {
	t.Helper()
	var session struct {
		CSRFToken string `json:"csrf_token"`
	}
	processJSON(t, client, http.MethodGet, baseURL+"/api/v1/session", nil, "", &session)
	processJSON(
		t,
		client,
		http.MethodPut,
		baseURL+"/api/v1/collection/"+itemID,
		map[string]int{"owned": owned, "available": 1, "wanted": 0},
		session.CSRFToken,
		&struct{}{},
	)
}

func assertProcessEntry(
	t *testing.T,
	client *http.Client,
	baseURL string,
	catalogID string,
	itemID string,
	owned int,
) {
	t.Helper()
	var collection struct {
		Entries []struct {
			CatalogItem struct {
				ID string `json:"id"`
			} `json:"catalog_item"`
			Owned int `json:"owned"`
		} `json:"entries"`
	}
	processJSON(
		t,
		client,
		http.MethodGet,
		baseURL+"/api/v1/catalogs/"+catalogID+"/collection?state=owned&limit=100",
		nil,
		"",
		&collection,
	)
	for _, entry := range collection.Entries {
		if entry.CatalogItem.ID == itemID {
			if entry.Owned != owned {
				t.Fatalf("persisted owned = %d, want %d", entry.Owned, owned)
			}
			return
		}
	}
	t.Fatalf("persisted item %q missing from %#v", itemID, collection)
}

func processJSON(
	t *testing.T,
	client *http.Client,
	method string,
	target string,
	body any,
	csrf string,
	destination any,
) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, target, reader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s = %d: %s", method, target, response.StatusCode, payload)
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode %s %s: %v", method, target, err)
	}
}

func unusedLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate test address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release test address: %v", err)
	}
	return address
}
