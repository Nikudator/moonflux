package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"moonflux/internal/auth"
	appconfig "moonflux/internal/config"
	"moonflux/internal/emailverification"
	"moonflux/internal/user"
)

// New builds the HTTP handler. /health checks process liveness; /ready also checks PostgreSQL.
func New(cfg appconfig.Config, log *slog.Logger, pool *pgxpool.Pool) (http.Handler, error) {
	mux := http.NewServeMux()
	userRepository := user.NewRepository(pool)
	userService := user.NewService(userRepository, cfg.Mail.Enabled)
	profileService := user.NewProfileService(userRepository)
	passwordChangeService := user.NewPasswordChangeService(userRepository)
	loginService := user.NewLoginService(userRepository, cfg.Mail.Enabled)
	registrationRateLimiter := NewIPRateLimiter(5, time.Minute)

	var registrationSender user.VerificationSender
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
	tokenManager, err := auth.NewTokenManager(auth.TokenConfig{
		Issuer: cfg.Security.JWTIssuer,
		Secret: []byte(cfg.Security.JWTHMACSecret),
		TTL:    cfg.Security.AccessTokenTTL,
	})
	if err != nil {
		log.Error("initialize JWT token manager", "error", err)
		return nil, err
	}

	authService, err := auth.NewLoginService(loginService, tokenManager)
	if err != nil {
		log.Error("initialize authentication service", "error", err)
		return nil, err
	}

	if cfg.Mail.Enabled {
		verificationRepository := emailverification.NewPostgresRepository(pool)

		verificationService, err := emailverification.NewService(
			verificationRepository,
			cfg.Mail.Verification.TokenTTL,
			cfg.Mail.Verification.ResendCooldown,
		)
		if err != nil {
			log.Error("initialize email verification service", "error", err)
			return nil, err
		}

		mailer, err := emailverification.NewSMTPMailer(emailverification.SMTPConfig{
			Host:     cfg.Mail.SMTP.Host,
			Port:     cfg.Mail.SMTP.Port,
			Username: cfg.Mail.SMTP.Username,
			Password: cfg.Mail.SMTP.Password,
			From:     cfg.Mail.From.Address,
			FromName: cfg.Mail.From.Name,
			BaseURL:  cfg.Mail.Verification.BaseURL,
			Timeout:  cfg.Mail.SMTP.Timeout,
		})
		if err != nil {
			log.Error("initialize SMTP mailer", "error", err)
			return nil, err
		}

		notificationService, err := emailverification.NewNotificationService(
			verificationService,
			mailer,
		)
		if err != nil {
			log.Error("initialize email notification service", "error", err)
			return nil, err
		}

		registrationSender = notificationService.Send

		verificationHandler := emailverification.NewHTTPHandlerWithResenderAndLogger(
			verificationService,
			notificationService,
			log,
		)

		verificationRateLimiter := NewIPRateLimiter(5, time.Minute)
		resendVerificationRateLimiter := NewIPRateLimiter(3, time.Minute)

		mux.Handle("POST /auth/verify-email", verificationRateLimiter.Middleware(
			http.HandlerFunc(verificationHandler.HandleVerify),
		))

		mux.Handle("POST /auth/resend-verification", resendVerificationRateLimiter.Middleware(
			http.HandlerFunc(verificationHandler.HandleResend),
		))
	}
	mux.Handle("POST /users", registrationRateLimiter.Middleware(
		user.NewHTTPHandlerWithVerification(userService, registrationSender),
	))
	authHandler := auth.NewHTTPHandler(authService)
	loginRateLimiter := NewIPRateLimiter(5, time.Minute)
	mux.Handle("POST /auth/login", loginRateLimiter.Middleware(
		http.HandlerFunc(authHandler.Login),
	))
	meHandler := auth.NewMeHandler(userRepository)
	mux.Handle("GET /auth/me", tokenManager.Middleware(
		userRepository,
		http.HandlerFunc(meHandler.Me),
	))
	profileHandler := user.NewProfileHTTPHandler(
		profileService,
		func(r *http.Request) (string, bool) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				return "", false
			}
			return claims.Subject, true
		},
	)
	mux.Handle("PATCH /users/me", tokenManager.Middleware(
		userRepository,
		http.HandlerFunc(profileHandler.UpdateMe),
	))
	passwordChangeHandler := user.NewPasswordChangeHTTPHandler(
		passwordChangeService,
		func(r *http.Request) (string, bool) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				return "", false
			}
			return claims.Subject, true
		},
	)

	mux.Handle("PATCH /users/me/password", tokenManager.Middleware(
		userRepository,
		http.HandlerFunc(passwordChangeHandler.ChangeMe),
	))

	return Middleware(log, cfg.HTTP.TrustedProxies, cfg.HTTP.RequestIDHeader, cfg.Logging.AccessLog, mux), nil

}

func NewServer(cfg appconfig.Config, log *slog.Logger, pool *pgxpool.Pool) (*http.Server, error) {
	handler, err := New(cfg, log, pool)
	if err != nil {
		return nil, err
	}

	return &http.Server{
		Addr:              cfg.App.HTTPAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}, nil
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
