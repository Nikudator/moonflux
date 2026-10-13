package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	appconfig "moonflux/internal/config"
	"moonflux/internal/user"
)

// New builds the HTTP handler. /health checks process liveness; /ready also checks PostgreSQL.
func New(cfg appconfig.Config, log *slog.Logger, pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	userRepository := user.NewRepository(pool)
	userService := user.NewService(userRepository)
	mux.Handle("POST /users", user.NewHTTPHandler(userService))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	return Middleware(log, cfg.HTTP.TrustedProxies, cfg.HTTP.RequestIDHeader, cfg.Logging.AccessLog, mux)
}

func NewServer(cfg appconfig.Config, log *slog.Logger, pool *pgxpool.Pool) *http.Server {
	return &http.Server{
		Addr:              cfg.App.HTTPAddress,
		Handler:           New(cfg, log, pool),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func Serve(ctx context.Context, server *http.Server, shutdownTimeout time.Duration, log *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() {
		log.Info("HTTP server starting", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		log.Info("HTTP server stopped gracefully")
		return nil
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
