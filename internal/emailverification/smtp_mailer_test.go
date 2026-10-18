package emailverification

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func validSMTPConfig() SMTPConfig {
	return SMTPConfig{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "moonflux",
		Password: "test-password",
		From:     "noreply@example.com",
		FromName: "Moonflux",
		BaseURL:  "https://example.com/verify-email",
		Timeout:  10 * time.Second,
	}
}

func TestNewSMTPMailer(t *testing.T) {
	mailer, err := NewSMTPMailer(validSMTPConfig())
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v, want nil", err)
	}
	if mailer == nil {
		t.Fatal("NewSMTPMailer() returned nil mailer")
	}
}

func TestNewSMTPMailerRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SMTPConfig)
	}{
		{
			name: "empty host",
			mutate: func(cfg *SMTPConfig) {
				cfg.Host = ""
			},
		},
		{
			name: "invalid port",
			mutate: func(cfg *SMTPConfig) {
				cfg.Port = 70000
			},
		},
		{
			name: "zero timeout",
			mutate: func(cfg *SMTPConfig) {
				cfg.Timeout = 0
			},
		},
		{
			name: "invalid sender",
			mutate: func(cfg *SMTPConfig) {
				cfg.From = "not-an-email"
			},
		},
		{
			name: "HTTP verification URL",
			mutate: func(cfg *SMTPConfig) {
				cfg.BaseURL = "http://example.com/verify-email"
			},
		},
		{
			name: "verification URL with query",
			mutate: func(cfg *SMTPConfig) {
				cfg.BaseURL = "https://example.com/verify-email?token=secret"
			},
		},
		{
			name: "verification URL with fragment",
			mutate: func(cfg *SMTPConfig) {
				cfg.BaseURL = "https://example.com/verify-email#fragment"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validSMTPConfig()
			tt.mutate(&cfg)

			mailer, err := NewSMTPMailer(cfg)
			if err == nil {
				t.Fatal("NewSMTPMailer() error = nil, want validation error")
			}
			if mailer != nil {
				t.Fatal("NewSMTPMailer() returned non-nil mailer for invalid config")
			}
		})
	}
}

func TestSMTPMailerVerificationURL(t *testing.T) {
	mailer, err := NewSMTPMailer(validSMTPConfig())
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v", err)
	}

	got, err := mailer.verificationURL("test-token_123")
	if err != nil {
		t.Fatalf("verificationURL() error = %v", err)
	}

	want := "https://example.com/verify-email?token=test-token_123"
	if got != want {
		t.Errorf("verificationURL() = %q, want %q", got, want)
	}
}

func TestSMTPMailerVerificationURLRejectsEmptyToken(t *testing.T) {
	mailer, err := NewSMTPMailer(validSMTPConfig())
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v", err)
	}

	if _, err := mailer.verificationURL(""); err == nil {
		t.Fatal("verificationURL() error = nil, want error for empty token")
	}
}

func TestSMTPMailerBuildMessage(t *testing.T) {
	mailer, err := NewSMTPMailer(validSMTPConfig())
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v", err)
	}

	message, err := mailer.buildMessage("user@example.com", "test-token_123")
	if err != nil {
		t.Fatalf("buildMessage() error = %v", err)
	}

	got := strings.ToLower(string(message))

	for _, want := range []string{
		`from: "moonflux" <noreply@example.com>`,
		"to: <user@example.com>",
		"mime-version: 1.0",
		`content-type: text/plain; charset="utf-8"`,
		"https://example.com/verify-email?token=test-token_123",
		"если вы не регистрировались в moonflux",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildMessage() result does not contain %q", want)
		}
	}
}

func TestSMTPMailerBuildMessageRejectsInvalidInput(t *testing.T) {
	mailer, err := NewSMTPMailer(validSMTPConfig())
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v", err)
	}

	tests := []struct {
		name  string
		to    string
		token string
	}{
		{
			name:  "empty recipient",
			to:    "",
			token: "test-token",
		},
		{
			name:  "invalid recipient",
			to:    "not-an-email",
			token: "test-token",
		},
		{
			name:  "empty token",
			to:    "user@example.com",
			token: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mailer.buildMessage(tt.to, tt.token); err == nil {
				t.Fatal("buildMessage() error = nil, want error")
			}
		})
	}
}

func startSMTPServerWithoutSTARTTLS(t *testing.T) (string, int) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort() error = %v", err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("strconv.Atoi() error = %v", err)
	}

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		time.Sleep(300 * time.Millisecond)
		_, _ = fmt.Fprint(conn, "220 localhost ESMTP\r\n")

		reader := bufio.NewReader(conn)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			command := strings.ToUpper(strings.TrimSpace(line))

			switch {
			case strings.HasPrefix(command, "EHLO "):
				_, _ = fmt.Fprint(conn,
					"250-localhost\r\n"+
						"250 SIZE 1000000\r\n",
				)
			case strings.HasPrefix(command, "HELO "):
				_, _ = fmt.Fprint(conn, "250 localhost\r\n")
			case command == "QUIT":
				_, _ = fmt.Fprint(conn, "221 Bye\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "500 Unsupported command\r\n")
			}
		}
	}()

	return host, port
}

func TestSMTPMailerSendVerificationRejectsCanceledContext(t *testing.T) {
	mailer, err := NewSMTPMailer(validSMTPConfig())
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = mailer.SendVerification(ctx, "user@example.com", "test-token")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SendVerification() error = %v, want %v", err, context.Canceled)
	}
}

func TestSMTPMailerSendVerificationRespectsContextDeadline(t *testing.T) {
	host, port := startSMTPServerWithoutSTARTTLS(t)

	cfg := validSMTPConfig()
	cfg.Host = host
	cfg.Port = port
	cfg.Timeout = 5 * time.Second

	mailer, err := NewSMTPMailer(cfg)
	if err != nil {
		t.Fatalf("NewSMTPMailer() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = mailer.SendVerification(ctx, "user@example.com", "test-token")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("SendVerification() error = %v, want %v", err, context.DeadlineExceeded)
	}
}
