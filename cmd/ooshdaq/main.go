// Command ooshdaq runs the API and embedded browser application, or one
// explicitly selected offline maintenance operation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/demo"
	"github.com/ooshdaq/ooshdaq/internal/httpapi"
	"github.com/ooshdaq/ooshdaq/internal/maintenance"
	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
	"github.com/ooshdaq/ooshdaq/internal/service"
	"github.com/ooshdaq/ooshdaq/web"
)

type runtimeConfig struct {
	address                string
	dataPath               string
	backupDirectory        string
	backupInterval         time.Duration
	backupRetention        int
	authMode               httpapi.AuthMode
	sessionSecret          []byte
	proxyUserHeader        string
	proxyDisplayNameHeader string
	cookieSecure           bool
	allowMockAuth          bool
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("$OOSHDAQ stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, arguments []string) error {
	if len(arguments) > 0 && arguments[0] == "maintenance" {
		return runMaintenance(arguments[1:])
	}
	if len(arguments) > 1 || (len(arguments) == 1 && arguments[0] != "serve") {
		return errors.New("usage: ooshdaq [serve] | ooshdaq maintenance restore [options]")
	}
	return runServer(logger)
}

func runServer(logger *slog.Logger) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	store, err := sqliterepo.Open(context.Background(), sqliterepo.Config{
		Path: config.dataPath,
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("close SQLite repository", "error_type", fmt.Sprintf("%T", err))
		}
	}()

	manifests, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		return err
	}
	if err := store.ImportManifests(context.Background(), manifests); err != nil {
		return err
	}
	mockUsers := demo.Users()
	if config.authMode == httpapi.AuthModeMock {
		for _, user := range mockUsers {
			if err := store.UpsertUser(context.Background(), user); err != nil {
				return fmt.Errorf("initialize mock user: %w", err)
			}
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
		return err
	}
	handler, err := httpapi.New(svc, httpapi.Config{
		AuthMode:               config.authMode,
		AllowMockAuth:          config.allowMockAuth,
		MockUsers:              mockUsers,
		SessionSecret:          config.sessionSecret,
		ProxyUserHeader:        config.proxyUserHeader,
		ProxyDisplayNameHeader: config.proxyDisplayNameHeader,
		CookieSecure:           config.cookieSecure,
		StaticFS:               web.Files(),
		Logger:                 logger,
	})
	if err != nil {
		return err
	}
	backups, err := maintenance.NewScheduler(store, maintenance.SchedulerConfig{
		Directory:      config.backupDirectory,
		Interval:       config.backupInterval,
		RetentionCount: config.backupRetention,
		Logger:         logger,
	})
	if err != nil {
		return err
	}
	backups.Start(context.Background())
	var stopMaintenanceOnce sync.Once
	var stopMaintenanceError error
	stopMaintenance := func(ctx context.Context) error {
		stopMaintenanceOnce.Do(func() {
			if err := backups.Stop(ctx); err != nil {
				stopMaintenanceError = fmt.Errorf("stop SQLite backup scheduler: %w", err)
				return
			}
			checkpoint, err := store.Checkpoint(ctx, true)
			if err != nil {
				stopMaintenanceError = fmt.Errorf("checkpoint SQLite during shutdown: %w", err)
				return
			}
			logger.Info(
				"SQLite shutdown checkpoint complete",
				"log_frames",
				checkpoint.LogFrames,
				"checkpointed_frames",
				checkpoint.CheckpointedFrames,
				"duration_ms",
				checkpoint.Duration.Milliseconds(),
			)
		})
		return stopMaintenanceError
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := stopMaintenance(ctx); err != nil {
			logger.Error("stop SQLite maintenance", "error_type", fmt.Sprintf("%T", err))
		}
	}()

	listener, err := net.Listen("tcp", config.address)
	if err != nil {
		return fmt.Errorf("listen on configured address: %w", err)
	}
	defer listener.Close()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	shutdownContext, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()
	serveErrors := make(chan error, 1)
	go func() {
		logger.Info(
			"$OOSHDAQ listening",
			"address",
			listener.Addr().String(),
			"auth_mode",
			config.authMode,
		)
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErrors <- err
	}()

	select {
	case err := <-serveErrors:
		if err != nil {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-shutdownContext.Done():
		stopSignals()
		timeout, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(timeout); err != nil {
			return fmt.Errorf("graceful HTTP shutdown: %w", err)
		}
		if err := stopMaintenance(timeout); err != nil {
			return err
		}
		if err := <-serveErrors; err != nil {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		return nil
	}
}

func runMaintenance(arguments []string) error {
	if len(arguments) == 0 || arguments[0] != "restore" {
		return errors.New("usage: ooshdaq maintenance restore [options]")
	}
	flags := flag.NewFlagSet("maintenance restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	backup := flags.String("backup", "", "verified SQLite backup")
	database := flags.String("database", "", "offline live SQLite database")
	retained := flags.String("retain-replaced", "", "new path for retained live database")
	if err := flags.Parse(arguments[1:]); err != nil {
		return fmt.Errorf("maintenance restore flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("maintenance restore accepts no positional arguments")
	}
	return maintenance.RestoreOffline(context.Background(), maintenance.RestoreConfig{
		Backup:         *backup,
		Database:       *database,
		RetainReplaced: *retained,
	})
}

func loadConfig() (runtimeConfig, error) {
	config := runtimeConfig{
		address:         envOrDefault("OOSHDAQ_ADDR", "127.0.0.1:8080"),
		dataPath:        envOrDefault("OOSHDAQ_DATA_PATH", "./data/ooshdaq.sqlite"),
		backupDirectory: envOrDefault("OOSHDAQ_BACKUP_DIR", "./backups"),
		authMode: httpapi.AuthMode(
			envOrDefault("OOSHDAQ_AUTH_MODE", string(httpapi.AuthModeMock)),
		),
		proxyUserHeader: envOrDefault(
			"OOSHDAQ_PROXY_USER_ID_HEADER",
			"X-Forwarded-User-ID",
		),
		proxyDisplayNameHeader: envOrDefault(
			"OOSHDAQ_PROXY_USER_NAME_HEADER",
			"X-Forwarded-User-Name",
		),
	}
	if strings.TrimSpace(config.address) == "" {
		return runtimeConfig{}, errors.New("OOSHDAQ_ADDR must not be empty")
	}
	if strings.TrimSpace(config.dataPath) == "" {
		return runtimeConfig{}, errors.New("OOSHDAQ_DATA_PATH must not be empty")
	}
	if strings.TrimSpace(config.backupDirectory) == "" {
		return runtimeConfig{}, errors.New("OOSHDAQ_BACKUP_DIR must not be empty")
	}
	interval, err := time.ParseDuration(envOrDefault("OOSHDAQ_BACKUP_INTERVAL", "24h"))
	if err != nil || interval < time.Minute || interval > 31*24*time.Hour {
		return runtimeConfig{}, errors.New(
			"OOSHDAQ_BACKUP_INTERVAL must be between 1m and 744h",
		)
	}
	config.backupInterval = interval
	retention, err := strconv.Atoi(envOrDefault("OOSHDAQ_BACKUP_RETENTION_COUNT", "14"))
	if err != nil || retention < 1 || retention > 365 {
		return runtimeConfig{}, errors.New(
			"OOSHDAQ_BACKUP_RETENTION_COUNT must be an integer from 1 through 365",
		)
	}
	config.backupRetention = retention
	dataDirectory, err := filepath.Abs(filepath.Dir(config.dataPath))
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("resolve OOSHDAQ_DATA_PATH: %w", err)
	}
	backupDirectory, err := filepath.Abs(config.backupDirectory)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("resolve OOSHDAQ_BACKUP_DIR: %w", err)
	}
	if dataDirectory == backupDirectory {
		return runtimeConfig{}, errors.New(
			"OOSHDAQ_BACKUP_DIR must differ from the live database directory",
		)
	}

	switch config.authMode {
	case httpapi.AuthModeMock:
		allowPublicMock, err := optionalBool("OOSHDAQ_ALLOW_MOCK_AUTH", false)
		if err != nil {
			return runtimeConfig{}, err
		}
		config.allowMockAuth = isLoopbackAddress(config.address) || allowPublicMock
		if !config.allowMockAuth {
			return runtimeConfig{}, errors.New(
				"mock authentication on a non-loopback address requires OOSHDAQ_ALLOW_MOCK_AUTH=true",
			)
		}
		if secret, configured := os.LookupEnv("OOSHDAQ_SESSION_SECRET"); configured {
			if len(secret) < 32 {
				return runtimeConfig{}, errors.New(
					"OOSHDAQ_SESSION_SECRET must contain at least 32 bytes",
				)
			}
			config.sessionSecret = []byte(secret)
		}
	case httpapi.AuthModeProxy:
		if strings.TrimSpace(config.proxyUserHeader) == "" {
			return runtimeConfig{}, errors.New(
				"OOSHDAQ_PROXY_USER_ID_HEADER must not be empty in proxy mode",
			)
		}
	default:
		return runtimeConfig{}, fmt.Errorf(
			"OOSHDAQ_AUTH_MODE must be %q or %q",
			httpapi.AuthModeMock,
			httpapi.AuthModeProxy,
		)
	}

	defaultCookieSecure := config.authMode == httpapi.AuthModeProxy
	cookieSecure, err := optionalBool("OOSHDAQ_COOKIE_SECURE", defaultCookieSecure)
	if err != nil {
		return runtimeConfig{}, err
	}
	config.cookieSecure = cookieSecure
	return config, nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func envOrDefault(name, fallback string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}
	return fallback
}

func optionalBool(name string, fallback bool) (bool, error) {
	value, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}
