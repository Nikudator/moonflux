package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appconfig "moonflux/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthEndpointAndRequestID(t *testing.T) {
	cfg := appconfig.Config{}
	cfg.HTTP.RequestIDHeader = "X-Request-ID"
	cfg.Logging.AccessLog = true
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(cfg, log, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("expected X-Request-ID response header")
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}
func TestReadyReturns503WhenPoolIsClosed(t *testing.T) {
	pool, err := pgxpool.New(
		context.Background(),
		"postgres://user:password@localhost:5432/moonflux",
	)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	pool.Close()

	cfg := appconfig.Config{}
	cfg.HTTP.RequestIDHeader = "X-Request-ID"

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := New(cfg, log, pool)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"status = %d, want %d; body = %s",
			rec.Code,
			http.StatusServiceUnavailable,
			rec.Body.String(),
		)
	}

	if !strings.Contains(rec.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}
