package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
		wantErr bool
	}{
		{
			name:    "IPv4 address",
			proxies: []string{"10.0.0.1"},
		},
		{
			name:    "IPv4 CIDR",
			proxies: []string{"10.0.0.0/8"},
		},
		{
			name:    "IPv6 address",
			proxies: []string{"2001:db8::1"},
		},
		{
			name:    "IPv6 CIDR",
			proxies: []string{"2001:db8::/32"},
		},
		{
			name:    "invalid IP",
			proxies: []string{"10.0.0.999"},
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			proxies: []string{"10.0.0.0/99"},
			wantErr: true,
		},
		{
			name:    "mixed valid and invalid entries",
			proxies: []string{"10.0.0.1", "not-an-ip"},
			wantErr: true,
		},
		{
			name:    "IPv4 address with surrounding spaces",
			proxies: []string{" 10.0.0.1 "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{}
			cfg.HTTP.TrustedProxies = tt.proxies

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatal("Validate() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), "http.trusted_proxies") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil && strings.Contains(err.Error(), "http.trusted_proxies") {
				t.Fatalf("unexpected trusted proxy validation error: %v", err)
			}
		})
	}
}
func TestValidateMailDisabled(t *testing.T) {
	cfg := validConfig()
	cfg.Mail.Enabled = false

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil when mail is disabled", err)
	}
}

func validConfig() Config {
	return Config{
		App: AppConfig{
			Name:            "Moonflux",
			Environment:     "test",
			HTTPAddress:     "127.0.0.1:8080",
			ShutdownTimeout: time.Second,
		},
		Logging: LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
		Database: DatabaseConfig{
			DSN:      "postgres://user:pass@localhost:5432/moonflux",
			MaxConns: 10,
			MinConns: 1,
		},
		Migrations: MigrationsConfig{
			Directory: "migrations",
		},
		Security: SecurityConfig{
			JWTIssuer:       "moonflux",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 720 * time.Hour,
			JWTHMACSecret:   "test-secret-that-is-long-enough-123",
		},
	}
}

func TestValidateMailEnabled(t *testing.T) {
	cfg := validConfig()
	cfg.Mail = MailConfig{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "moonflux",
			Password: "test-password",
			StartTLS: true,
			Timeout:  10 * time.Second,
		},
		From: MailFromConfig{
			Address: "noreply@example.com",
			Name:    "Moonflux",
		},
		Verification: MailVerificationConfig{
			TokenTTL:       24 * time.Hour,
			ResendCooldown: time.Minute,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for valid mail config", err)
	}
}

func TestValidateMailEnabledRequiresSMTPHost(t *testing.T) {
	cfg := validConfig()
	cfg.Mail = MailConfig{
		Enabled: true,
		SMTP: SMTPConfig{
			Port:    587,
			Timeout: 10 * time.Second,
		},
		From: MailFromConfig{
			Address: "noreply@example.com",
			Name:    "Moonflux",
		},
		Verification: MailVerificationConfig{
			TokenTTL:       24 * time.Hour,
			ResendCooldown: time.Minute,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for empty SMTP host")
	}

	if !strings.Contains(err.Error(), "mail.smtp.host") {
		t.Fatalf("Validate() error = %v, want mail.smtp.host validation error", err)
	}
}

func TestValidateMailEnabledRequiresValidSMTPPort(t *testing.T) {
	cfg := validConfig()
	cfg.Mail = MailConfig{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:    "smtp.example.com",
			Port:    70000,
			Timeout: 10 * time.Second,
		},
		From: MailFromConfig{
			Address: "noreply@example.com",
			Name:    "Moonflux",
		},
		Verification: MailVerificationConfig{
			TokenTTL:       24 * time.Hour,
			ResendCooldown: time.Minute,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for invalid SMTP port")
	}

	if !strings.Contains(err.Error(), "mail.smtp.port") {
		t.Fatalf("Validate() error = %v, want mail.smtp.port validation error", err)
	}
}
func TestValidateMailSMTPCredentialsMustBePaired(t *testing.T) {
	cfg := validConfig()
	cfg.Mail = MailConfig{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "moonflux",
			Timeout:  10 * time.Second,
		},
		From: MailFromConfig{
			Address: "noreply@example.com",
			Name:    "Moonflux",
		},
		Verification: MailVerificationConfig{
			TokenTTL:       24 * time.Hour,
			ResendCooldown: time.Minute,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want an error when SMTP username is set without password")
	}

	if !strings.Contains(err.Error(), "mail.smtp.username and mail.smtp.password") {
		t.Fatalf("Validate() error = %v, want SMTP credentials validation error", err)
	}
}

func TestValidateMailEnabledRequiresValidFromAddress(t *testing.T) {
	cfg := validConfig()
	cfg.Mail = MailConfig{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:    "smtp.example.com",
			Port:    587,
			Timeout: 10 * time.Second,
		},
		From: MailFromConfig{
			Address: "not-an-email",
			Name:    "Moonflux",
		},
		Verification: MailVerificationConfig{
			TokenTTL:       24 * time.Hour,
			ResendCooldown: time.Minute,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for invalid sender address")
	}

	if !strings.Contains(err.Error(), "mail.from.address") {
		t.Fatalf("Validate() error = %v, want mail.from.address validation error", err)
	}
}

func TestValidateMailRejectsNegativeDurations(t *testing.T) {
	tests := []struct {
		name string
		set  func(*MailConfig)
		want string
	}{
		{
			name: "negative SMTP timeout",
			set: func(mail *MailConfig) {
				mail.SMTP.Timeout = -time.Second
			},
			want: "mail.smtp.timeout",
		},
		{
			name: "negative verification token TTL",
			set: func(mail *MailConfig) {
				mail.Verification.TokenTTL = -time.Second
			},
			want: "mail.verification.token_ttl",
		},
		{
			name: "negative resend cooldown",
			set: func(mail *MailConfig) {
				mail.Verification.ResendCooldown = -time.Second
			},
			want: "mail.verification.resend_cooldown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Mail = MailConfig{
				Enabled: true,
				SMTP: SMTPConfig{
					Host:    "smtp.example.com",
					Port:    587,
					Timeout: 10 * time.Second,
				},
				From: MailFromConfig{
					Address: "noreply@example.com",
					Name:    "Moonflux",
				},
				Verification: MailVerificationConfig{
					TokenTTL:       24 * time.Hour,
					ResendCooldown: time.Minute,
				},
			}

			tt.set(&cfg.Mail)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadMailConfig(t *testing.T) {
	content := `
app:
  name: Moonflux
  environment: test
  http_address: "127.0.0.1:8080"
  shutdown_timeout: 1s

logging:
  level: debug
  format: json

database:
  dsn: "postgres://user:pass@localhost:5432/moonflux"
  max_conns: 10
  min_conns: 1

migrations:
  directory: migrations

security:
  jwt_issuer: moonflux
  access_token_ttl: 15m
  refresh_token_ttl: 720h
  jwt_hmac_secret: "test-secret-that-is-long-enough-123"

mail:
  enabled: true
  smtp:
    host: smtp.example.com
    port: 587
    username: moonflux
    password: test-password
    start_tls: true
    timeout: 10s
  from:
    address: noreply@example.com
    name: Moonflux
  verification:
    token_ttl: 24h
    resend_cooldown: 1m
`

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Mail.Enabled {
		t.Fatal("Mail.Enabled = false, want true")
	}
	if cfg.Mail.SMTP.Host != "smtp.example.com" {
		t.Errorf("Mail.SMTP.Host = %q, want %q", cfg.Mail.SMTP.Host, "smtp.example.com")
	}
	if cfg.Mail.SMTP.Port != 587 {
		t.Errorf("Mail.SMTP.Port = %d, want 587", cfg.Mail.SMTP.Port)
	}
	if cfg.Mail.SMTP.Timeout != 10*time.Second {
		t.Errorf("Mail.SMTP.Timeout = %v, want %v", cfg.Mail.SMTP.Timeout, 10*time.Second)
	}
	if cfg.Mail.From.Address != "noreply@example.com" {
		t.Errorf("Mail.From.Address = %q, want %q", cfg.Mail.From.Address, "noreply@example.com")
	}
	if cfg.Mail.Verification.TokenTTL != 24*time.Hour {
		t.Errorf("Mail.Verification.TokenTTL = %v, want %v", cfg.Mail.Verification.TokenTTL, 24*time.Hour)
	}
	if cfg.Mail.Verification.ResendCooldown != time.Minute {
		t.Errorf("Mail.Verification.ResendCooldown = %v, want %v", cfg.Mail.Verification.ResendCooldown, time.Minute)
	}
}
func TestLoadRejectsInvalidMailConfig(t *testing.T) {
	content := `
app:
  name: Moonflux
  environment: test
  http_address: "127.0.0.1:8080"
  shutdown_timeout: 1s
logging:
  level: debug
  format: json
database:
  dsn: "postgres://user:pass@localhost:5432/moonflux"
  max_conns: 10
  min_conns: 1
migrations:
  directory: migrations
security:
  jwt_issuer: moonflux
  access_token_ttl: 15m
  refresh_token_ttl: 720h
  jwt_hmac_secret: "test-secret-that-is-long-enough-123"
mail:
  enabled: true
  smtp:
    host: ""
    port: 587
    timeout: 10s
  from:
    address: noreply@example.com
    name: Moonflux
  verification:
    token_ttl: 24h
    resend_cooldown: 1m
`

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error for empty SMTP host")
	}
	if !strings.Contains(err.Error(), "mail.smtp.host") {
		t.Fatalf("Load() error = %v, want mail.smtp.host validation error", err)
	}
}
