package emailverification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"strconv"
	"time"

	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strings"
)

// SMTPConfig contains the settings required to send email.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	BaseURL  string
	Timeout  time.Duration
}

// SMTPMailer sends email verification messages over SMTP with STARTTLS.
type SMTPMailer struct {
	config SMTPConfig
}

// NewSMTPMailer creates an SMTP mailer after validating its configuration.
func NewSMTPMailer(cfg SMTPConfig) (*SMTPMailer, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("SMTP host is required")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("SMTP port must be between 1 and 65535")
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("SMTP timeout must be positive")
	}
	if cfg.From == "" {
		return nil, errors.New("sender email address is required")
	}
	if cfg.FromName == "" {
		return nil, errors.New("sender name is required")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("verification base URL is required")
	}

	parsedURL, err := url.Parse(cfg.BaseURL)
	if err != nil ||
		parsedURL == nil ||
		parsedURL.Scheme != "https" ||
		parsedURL.Host == "" ||
		parsedURL.User != nil ||
		parsedURL.RawQuery != "" ||
		parsedURL.ForceQuery ||
		parsedURL.Fragment != "" ||
		parsedURL.Opaque != "" {
		return nil, errors.New("verification base URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}

	parsedAddress, err := netmail.ParseAddress(cfg.From)
	if err != nil || parsedAddress.Address != cfg.From {
		return nil, errors.New("sender email address is invalid")
	}

	return &SMTPMailer{config: cfg}, nil
}

// verificationURL builds a URL containing the verification token.
func (m *SMTPMailer) verificationURL(token string) (string, error) {
	if token == "" {
		return "", errors.New("verification token is required")
	}

	u, err := url.Parse(m.config.BaseURL)
	if err != nil {
		return "", errors.New("invalid verification base URL")
	}

	query := u.Query()
	query.Set("token", token)
	u.RawQuery = query.Encode()

	return u.String(), nil
}

// buildMessage creates a plain-text email verification message.
func (m *SMTPMailer) buildMessage(to string, token string) ([]byte, error) {
	if strings.TrimSpace(to) == "" {
		return nil, errors.New("recipient email address is required")
	}

	recipient, err := netmail.ParseAddress(to)
	if err != nil || recipient.Address != to {
		return nil, errors.New("recipient email address is invalid")
	}

	verificationURL, err := m.verificationURL(token)
	if err != nil {
		return nil, err
	}

	headers := textproto.MIMEHeader{}
	headers.Set("From", (&netmail.Address{
		Name:    m.config.FromName,
		Address: m.config.From,
	}).String())
	headers.Set("To", recipient.String())
	headers.Set("Subject", mime.QEncoding.Encode("UTF-8", "Подтверждение адреса электронной почты"))
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", `text/plain; charset="UTF-8"`)
	headers.Set("Content-Transfer-Encoding", "8bit")

	body := fmt.Sprintf(
		"Здравствуйте!\r\n\r\n"+
			"Для подтверждения адреса электронной почты перейдите по ссылке:\r\n"+
			"%s\r\n\r\n"+
			"Если вы не регистрировались в Moonflux, просто проигнорируйте это письмо.\r\n",
		verificationURL,
	)

	var message strings.Builder

	for key, values := range headers {
		for _, value := range values {
			message.WriteString(key)
			message.WriteString(": ")
			message.WriteString(value)
			message.WriteString("\r\n")
		}
	}

	message.WriteString("\r\n")
	message.WriteString(body)

	return []byte(message.String()), nil
}

// SendVerification sends an email verification message over SMTP with STARTTLS.
func (m *SMTPMailer) SendVerification(ctx context.Context, to string, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	message, err := m.buildMessage(to, token)
	if err != nil {
		return err
	}

	address := net.JoinHostPort(m.config.Host, strconv.Itoa(m.config.Port))

	dialer := net.Dialer{
		Timeout: m.config.Timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return errors.New("SMTP connection failed")
	}
	defer conn.Close()

	deadline := time.Now().Add(m.config.Timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	if err := conn.SetDeadline(deadline); err != nil {
		return errors.New("SMTP connection setup failed")
	}

	stopCancellation := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	defer stopCancellation()

	client, err := smtp.NewClient(conn, m.config.Host)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return errors.New("SMTP handshake failed")
	}
	defer client.Close()

	if err := ctx.Err(); err != nil {
		return err
	}

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not support STARTTLS")
	}

	tlsConfig := &tls.Config{
		ServerName: m.config.Host,
		MinVersion: tls.VersionTLS12,
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return errors.New("SMTP STARTTLS negotiation failed")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if m.config.Username != "" {
		auth := smtp.PlainAuth(
			"",
			m.config.Username,
			m.config.Password,
			m.config.Host,
		)

		if err := client.Auth(auth); err != nil {
			return errors.New("SMTP authentication failed")
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := client.Mail(m.config.From); err != nil {
		return errors.New("SMTP sender rejected")
	}

	recipient, err := netmail.ParseAddress(to)
	if err != nil {
		return errors.New("SMTP recipient address is invalid")
	}

	if err := client.Rcpt(recipient.Address); err != nil {
		return errors.New("SMTP recipient rejected")
	}

	writer, err := client.Data()
	if err != nil {
		return errors.New("SMTP message submission failed")
	}

	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return errors.New("SMTP message transmission failed")
	}

	if err := writer.Close(); err != nil {
		return errors.New("SMTP message submission failed")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := client.Quit(); err != nil {
		return errors.New("SMTP session close failed")
	}

	return nil
}
