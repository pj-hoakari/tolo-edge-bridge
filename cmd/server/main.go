package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gcpfirestore "cloud.google.com/go/firestore"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/externaltoken"
	connectinfra "github.com/pj-hoakari/tolo-edge-bridge/internal/infra/connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/connect/authn"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/firestore"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/httpapi"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/logging"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/telemetry"
)

const (
	defaultAddr       = ":8080"
	defaultLogLevel   = "info"
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
)

const (
	envIDPIssuer       = "IDP_ISSUER"
	envIDPAudience     = "IDP_AUDIENCE"
	envIDPJWKSURL      = "IDP_JWKS_URL"
	envConsoleClientID = "CONSOLE_CLIENT_ID"
	envEdgeClientID    = "EDGE_CLIENT_ID"
)

var (
	errFirestoreProjectIDRequired = errors.New("FIRESTORE_PROJECT_ID is required")
	errEnvRequired                = errors.New("required environment variable is not set")
	errClientIDsCollide           = errors.New(envConsoleClientID + " and " + envEdgeClientID + " must differ")
)

func main() {
	if err := run(); err != nil {
		// run() installs the default logger itself, so a failure before that
		// point is reported by slog's own handler on stderr instead.
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger, err := newLogger()
	if err != nil {
		return err
	}

	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := getenv("SERVER_ADDR", defaultAddr)

	firestoreProjectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if firestoreProjectID == "" {
		return errFirestoreProjectIDRequired
	}

	authenticator, err := newAuthenticator()
	if err != nil {
		return err
	}

	shutdownTracing, err := telemetry.Setup(ctx)
	if err != nil {
		return fmt.Errorf("setup tracing: %w", err)
	}
	defer shutdownTracingWithTimeout(shutdownTracing)

	if telemetry.Enabled() {
		slog.Info("tracing enabled", "service", telemetry.ServiceName())
	}

	firestoreClient, err := gcpfirestore.NewClient(ctx, firestoreProjectID)
	if err != nil {
		return fmt.Errorf("create firestore client: %w", err)
	}
	defer closeFirestore(firestoreClient)

	slog.Info("firestore configured", "project", firestoreProjectID, "emulator_host", os.Getenv("FIRESTORE_EMULATOR_HOST"))

	signalingService := application.NewSignalingService(firestore.NewEdgeRepository(firestoreClient))

	signalingRoutes, err := connectinfra.Routes(signalingService, authenticator)
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}

	handler := httpapi.NewHandler(httpapi.HealthRoutes(), signalingRoutes)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		// net/http reports its own failures (a broken connection, a panic in a
		// handler) through this logger, so it goes to the same structured
		// stream as everything else.
		ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)

	go func() {
		slog.Info("server listening", "addr", addr)

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err

			return
		}

		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		slog.Info("server shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		return httpServer.Shutdown(shutdownCtx)
	}
}

func newAuthenticator() (*authn.Authenticator, error) {
	values := make(map[string]string)

	for _, key := range []string{envIDPIssuer, envIDPAudience, envIDPJWKSURL, envConsoleClientID, envEdgeClientID} {
		values[key] = os.Getenv(key)
		if values[key] == "" {
			return nil, fmt.Errorf("%w: %s", errEnvRequired, key)
		}
	}

	if values[envConsoleClientID] == values[envEdgeClientID] {
		return nil, errClientIDsCollide
	}

	keys, err := externaltoken.NewKeySet(externaltoken.KeySetConfig{
		URL:             values[envIDPJWKSURL],
		HTTPClient:      nil,
		CacheTTL:        0,
		RefreshCooldown: 0,
		FailureCooldown: 0,
		FetchTimeout:    0,
		RetryBackoff:    nil,
		MaxDocumentSize: 0,
		Clock:           nil,
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", envIDPJWKSURL, err)
	}

	verifier, err := externaltoken.NewVerifier(externaltoken.Config{
		Issuer:     values[envIDPIssuer],
		Audience:   values[envIDPAudience],
		Algorithms: nil,
		Keys:       keys,
		Leeway:     0,
		Clock:      nil,
	})
	if err != nil {
		return nil, fmt.Errorf("build the external token verifier: %w", err)
	}

	authenticator, err := authn.New(verifier, map[string]domain.PeerRole{
		values[envConsoleClientID]: domain.PeerRoleConsole,
		values[envEdgeClientID]:    domain.PeerRoleEdge,
	})
	if err != nil {
		return nil, fmt.Errorf("build the authenticator: %w", err)
	}

	slog.Info("external token verification configured", "issuer", values[envIDPIssuer], "audience", values[envIDPAudience], "jwks_url", values[envIDPJWKSURL])

	return authenticator, nil
}

// newLogger builds the process logger from the environment. It is the first
// thing run() does, so that everything the service reports afterwards is
// written in the structure Cloud Logging parses.
func newLogger() (*slog.Logger, error) {
	level, err := logging.ParseLevel(getenv("LOG_LEVEL", defaultLogLevel))
	if err != nil {
		return nil, fmt.Errorf("read LOG_LEVEL: %w", err)
	}

	return logging.NewLogger(os.Stdout, logging.Options{
		Level:     level,
		AddSource: false,
		// Without a project the log entries carry the bare trace ID, so Cloud
		// Logging cannot correlate them with the trace.
		ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT"),
	}), nil
}

// shutdownTracingWithTimeout flushes pending spans on a fresh context, because
// the run context is already cancelled once the process starts shutting down.
func shutdownTracingWithTimeout(shutdown telemetry.ShutdownFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := shutdown(ctx); err != nil {
		slog.Error("shutdown tracing failed", "error", err)
	}
}

func closeFirestore(client *gcpfirestore.Client) {
	if err := client.Close(); err != nil {
		slog.Error("close firestore client failed", "error", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
