package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid access token")
	ErrExpiredToken = errors.New("access token expired")
)

type TokenConfig struct {
	Issuer string
	Secret []byte
	TTL    time.Duration
	Now    func() time.Time
}

type Claims struct {
	Issuer       string `json:"iss"`
	Subject      string `json:"sub"`
	Role         int    `json:"role"`
	TokenVersion int64  `json:"token_version"`
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
}

type TokenManager struct {
	issuer string
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenManager(cfg TokenConfig) (*TokenManager, error) {
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("JWT issuer must not be empty")
	}
	if len(cfg.Secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 bytes")
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("access token TTL must be positive")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &TokenManager{
		issuer: cfg.Issuer,
		secret: append([]byte(nil), cfg.Secret...),
		ttl:    cfg.TTL,
		now:    cfg.Now,
	}, nil
}

func (m *TokenManager) Issue(subject string, role int) (string, error) {
	return m.IssueWithVersion(subject, role, 1)
}

func (m *TokenManager) IssueWithVersion(
	subject string,
	role int,
	tokenVersion int64,
) (string, error) {
	if strings.TrimSpace(subject) == "" {
		return "", errors.New("token subject must not be empty")
	}
	if role < 0 {
		return "", errors.New("token role must not be negative")
	}
	if tokenVersion < 0 {
		return "", errors.New("token version must not be negative")
	}

	now := m.now().UTC()
	claims := Claims{
		Issuer:       m.issuer,
		Subject:      subject,
		Role:         role,
		TokenVersion: tokenVersion,
		IssuedAt:     now.Unix(),
		ExpiresAt:    now.Add(m.ttl).Unix(),
	}

	header, err := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", fmt.Errorf("marshal token header: %w", err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal token claims: %w", err)
	}

	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)

	signature := m.sign(unsigned)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *TokenManager) Validate(token string) (Claims, error) {
	var claims Claims

	parts := strings.Split(token, ".")
	if len(parts) != 3 || token == "" {
		return claims, ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, ErrInvalidToken
	}

	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return claims, ErrInvalidToken
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return claims, ErrInvalidToken
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, ErrInvalidToken
	}

	expected := m.sign(parts[0] + "." + parts[1])
	if !hmac.Equal(signature, expected) {
		return claims, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrInvalidToken
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}

	if claims.Issuer != m.issuer || claims.Subject == "" ||
		claims.TokenVersion < 1 ||
		claims.IssuedAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return Claims{}, ErrInvalidToken
	}

	now := m.now().Unix()
	if claims.ExpiresAt <= now {
		return Claims{}, ErrExpiredToken
	}
	if claims.IssuedAt > now {
		return Claims{}, ErrInvalidToken
	}

	return claims, nil
}

func (m *TokenManager) sign(value string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
