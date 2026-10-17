package config

import (
	"errors"
	"fmt"
	netmail "net/mail"
	"net/netip"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Logging    LoggingConfig    `yaml:"logging"`
	HTTP       HTTPConfig       `yaml:"http"`
	Audit      AuditConfig      `yaml:"audit"`
	Database   DatabaseConfig   `yaml:"database"`
	Migrations MigrationsConfig `yaml:"migrations"`
	Security   SecurityConfig   `yaml:"security"`
	Mail       MailConfig       `yaml:"mail"`
}

type AppConfig struct {
	Name            string        `yaml:"name"`
	Environment     string        `yaml:"environment"`
	HTTPAddress     string        `yaml:"http_address"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type LoggingConfig struct {
	Level       string `yaml:"level"`
	Format      string `yaml:"format"`
	AddSource   bool   `yaml:"add_source"`
	AccessLog   bool   `yaml:"access_log"`
	SecurityLog bool   `yaml:"security_log"`
}

type HTTPConfig struct {
	TrustedProxies  []string `yaml:"trusted_proxies"`
	RequestIDHeader string   `yaml:"request_id_header"`
}

type AuditConfig struct {
	Enabled          bool          `yaml:"enabled"`
	Storage          string        `yaml:"storage"`
	RetentionEnabled bool          `yaml:"retention_enabled"`
	RetentionPeriod  time.Duration `yaml:"retention_period"`
}

type DatabaseConfig struct {
	DSN               string        `yaml:"dsn"`
	MaxConns          int32         `yaml:"max_conns"`
	MinConns          int32         `yaml:"min_conns"`
	MaxConnLifetime   time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime   time.Duration `yaml:"max_conn_idle_time"`
	HealthCheckPeriod time.Duration `yaml:"health_check_period"`
}

type MigrationsConfig struct {
	Directory string `yaml:"directory"`
}

type SecurityConfig struct {
	JWTIssuer       string        `yaml:"jwt_issuer"`
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl"`
	JWTHMACSecret   string        `yaml:"jwt_hmac_secret"`
}
type MailConfig struct {
	Enabled      bool                   `yaml:"enabled"`
	SMTP         SMTPConfig             `yaml:"smtp"`
	From         MailFromConfig         `yaml:"from"`
	Verification MailVerificationConfig `yaml:"verification"`
}

type SMTPConfig struct {
	Host     string        `yaml:"host"`
	Port     int           `yaml:"port"`
	Username string        `yaml:"username"`
	Password string        `yaml:"password"`
	StartTLS bool          `yaml:"start_tls"`
	Timeout  time.Duration `yaml:"timeout"`
}

type MailFromConfig struct {
	Address string `yaml:"address"`
	Name    string `yaml:"name"`
}

type MailVerificationConfig struct {
	TokenTTL       time.Duration `yaml:"token_ttl"`
	ResendCooldown time.Duration `yaml:"resend_cooldown"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.App.Name) == "" {
		problems = append(problems, "app.name is required")
	}
	if strings.TrimSpace(c.App.Environment) == "" {
		problems = append(problems, "app.environment is required")
	}
	if strings.TrimSpace(c.App.HTTPAddress) == "" {
		problems = append(problems, "app.http_address is required")
	}
	if c.HTTP.RequestIDHeader == "" {
		c.HTTP.RequestIDHeader = "X-Request-ID"
	}
	if strings.TrimSpace(c.Logging.Level) == "" {
		problems = append(problems, "logging.level is required")
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		problems = append(problems, "logging.format must be json or text")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		problems = append(problems, "database.dsn is required")
	}
	if c.Database.MaxConns < 1 {
		problems = append(problems, "database.max_conns must be at least 1")
	}
	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		problems = append(problems, "database.min_conns must be between 0 and database.max_conns")
	}
	if strings.TrimSpace(c.Migrations.Directory) == "" {
		problems = append(problems, "migrations.directory is required")
	}
	if strings.TrimSpace(c.Security.JWTIssuer) == "" {
		problems = append(problems, "security.jwt_issuer is required")
	}
	if c.Security.AccessTokenTTL <= 0 {
		problems = append(problems, "security.access_token_ttl must be positive")
	}
	if c.Security.RefreshTokenTTL <= 0 {
		problems = append(problems, "security.refresh_token_ttl must be positive")
	}
	if len(c.Security.JWTHMACSecret) < 32 || strings.Contains(c.Security.JWTHMACSecret, "CHANGE-ME") {
		problems = append(problems, "security.jwt_hmac_secret must be a non-placeholder secret of at least 32 bytes")
	}
	if c.App.ShutdownTimeout <= 0 {
		problems = append(problems, "app.shutdown_timeout must be positive")
	}
	if c.Audit.Enabled && c.Audit.Storage != "postgres_and_logs" {
		problems = append(problems, "audit.storage must be postgres_and_logs when audit.enabled is true")
	}
	if c.Audit.RetentionEnabled && c.Audit.RetentionPeriod <= 0 {
		problems = append(problems, "audit.retention_period must be positive when retention is enabled")
	}
	for _, raw := range c.HTTP.TrustedProxies {
		raw = strings.TrimSpace(raw)
		if _, err := netip.ParsePrefix(raw); err != nil {
			if _, addrErr := netip.ParseAddr(raw); addrErr != nil {
				problems = append(problems, fmt.Sprintf("http.trusted_proxies entry %q must be an IP or CIDR", raw))
			}
		}
	}
	if c.Mail.SMTP.Timeout < 0 {
		problems = append(problems, "mail.smtp.timeout must not be negative")
	}
	if c.Mail.Verification.TokenTTL < 0 {
		problems = append(problems, "mail.verification.token_ttl must not be negative")
	}
	if c.Mail.Verification.ResendCooldown < 0 {
		problems = append(problems, "mail.verification.resend_cooldown must not be negative")
	}

	if c.Mail.Enabled {
		if strings.TrimSpace(c.Mail.SMTP.Host) == "" {
			problems = append(problems, "mail.smtp.host is required when mail is enabled")
		}
		if c.Mail.SMTP.Port < 1 || c.Mail.SMTP.Port > 65535 {
			problems = append(problems, "mail.smtp.port must be between 1 and 65535")
		}
		if c.Mail.SMTP.Timeout <= 0 {
			problems = append(problems, "mail.smtp.timeout must be positive when mail is enabled")
		}
		if (c.Mail.SMTP.Username == "") != (c.Mail.SMTP.Password == "") {
			problems = append(problems, "mail.smtp.username and mail.smtp.password must both be set or both be empty")
		}

		address := strings.TrimSpace(c.Mail.From.Address)
		parsedAddress, err := netmail.ParseAddress(address)
		if err != nil || parsedAddress.Address != address {
			problems = append(problems, "mail.from.address must be a valid email address")
		}
		if strings.TrimSpace(c.Mail.From.Name) == "" {
			problems = append(problems, "mail.from.name is required when mail is enabled")
		}
		if c.Mail.Verification.TokenTTL <= 0 {
			problems = append(problems, "mail.verification.token_ttl must be positive when mail is enabled")
		}
		if c.Mail.Verification.ResendCooldown <= 0 {
			problems = append(problems, "mail.verification.resend_cooldown must be positive when mail is enabled")
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
