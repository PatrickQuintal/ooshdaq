package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/httpapi"
)

func TestLoadConfigDefaultsToLocalMockMode(t *testing.T) {
	clearConfigEnvironment(t)

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if config.address != "127.0.0.1:8080" ||
		config.dataPath != "./data/ooshdaq.sqlite" ||
		config.backupDirectory != "./backups" ||
		config.backupInterval != 24*time.Hour ||
		config.backupRetention != 14 ||
		config.authMode != httpapi.AuthModeMock ||
		config.cookieSecure ||
		!config.allowMockAuth {
		t.Fatalf("unexpected default config: %#v", config)
	}
	if config.sessionSecret != nil {
		t.Fatal("unset session secret should use a process-random key")
	}
}

func TestLoadConfigProxyModeIsSecureByDefault(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("OOSHDAQ_AUTH_MODE", "proxy")
	t.Setenv("OOSHDAQ_PROXY_USER_ID_HEADER", "X-Verified-Subject")
	t.Setenv("OOSHDAQ_PROXY_USER_NAME_HEADER", "X-Verified-Name")

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if config.authMode != httpapi.AuthModeProxy || !config.cookieSecure {
		t.Fatalf("unexpected proxy config: %#v", config)
	}
	if config.proxyUserHeader != "X-Verified-Subject" ||
		config.proxyDisplayNameHeader != "X-Verified-Name" {
		t.Fatalf("unexpected proxy headers: %#v", config)
	}
}

func TestLoadConfigRejectsInvalidSecurityValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		message string
	}{
		{
			name: "invalid backup interval",
			env: map[string]string{
				"OOSHDAQ_BACKUP_INTERVAL": "instant",
			},
			message: "OOSHDAQ_BACKUP_INTERVAL",
		},
		{
			name: "invalid backup retention",
			env: map[string]string{
				"OOSHDAQ_BACKUP_RETENTION_COUNT": "0",
			},
			message: "OOSHDAQ_BACKUP_RETENTION_COUNT",
		},
		{
			name: "backup beside database",
			env: map[string]string{
				"OOSHDAQ_DATA_PATH":  "./same/ooshdaq.sqlite",
				"OOSHDAQ_BACKUP_DIR": "./same",
			},
			message: "OOSHDAQ_BACKUP_DIR",
		},
		{
			name:    "unknown auth mode",
			env:     map[string]string{"OOSHDAQ_AUTH_MODE": "automatic"},
			message: "OOSHDAQ_AUTH_MODE",
		},
		{
			name: "short mock secret",
			env: map[string]string{
				"OOSHDAQ_AUTH_MODE":      "mock",
				"OOSHDAQ_SESSION_SECRET": "short",
			},
			message: "OOSHDAQ_SESSION_SECRET",
		},
		{
			name: "invalid cookie boolean",
			env: map[string]string{
				"OOSHDAQ_COOKIE_SECURE": "sometimes",
			},
			message: "OOSHDAQ_COOKIE_SECURE",
		},
		{
			name: "public mock without explicit opt in",
			env: map[string]string{
				"OOSHDAQ_ADDR": ":8080",
			},
			message: "OOSHDAQ_ALLOW_MOCK_AUTH",
		},
		{
			name: "missing proxy subject header",
			env: map[string]string{
				"OOSHDAQ_AUTH_MODE":            "proxy",
				"OOSHDAQ_PROXY_USER_ID_HEADER": "",
			},
			message: "OOSHDAQ_PROXY_USER_ID_HEADER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			if _, err := loadConfig(); err == nil {
				t.Fatal("loadConfig succeeded")
			} else if !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("loadConfig error = %q, want substring %q", err, tt.message)
			}
		})
	}
}

func TestLoadConfigAllowsExplicitContainerMockMode(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("OOSHDAQ_ADDR", ":8080")
	t.Setenv("OOSHDAQ_ALLOW_MOCK_AUTH", "true")

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !config.allowMockAuth {
		t.Fatal("explicit mock opt-in was not retained")
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OOSHDAQ_ADDR",
		"OOSHDAQ_DATA_PATH",
		"OOSHDAQ_AUTH_MODE",
		"OOSHDAQ_BACKUP_DIR",
		"OOSHDAQ_BACKUP_INTERVAL",
		"OOSHDAQ_BACKUP_RETENTION_COUNT",
		"OOSHDAQ_SESSION_SECRET",
		"OOSHDAQ_PROXY_USER_ID_HEADER",
		"OOSHDAQ_PROXY_USER_NAME_HEADER",
		"OOSHDAQ_COOKIE_SECURE",
		"OOSHDAQ_ALLOW_MOCK_AUTH",
	} {
		oldValue, existed := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("Unsetenv(%q): %v", name, err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, oldValue)
				return
			}
			_ = os.Unsetenv(name)
		})
	}
}
