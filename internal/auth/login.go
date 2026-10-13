package auth

import (
	"context"
	"errors"
	"fmt"

	"moonflux/internal/user"
)

type UserAuthenticator interface {
	Authenticate(ctx context.Context, input user.LoginInput) (user.User, error)
}

type LoginResult struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type LoginService struct {
	authenticator UserAuthenticator
	tokens        *TokenManager
}

func NewLoginService(
	authenticator UserAuthenticator,
	tokens *TokenManager,
) (*LoginService, error) {
	if authenticator == nil {
		return nil, errors.New("user authenticator must not be nil")
	}
	if tokens == nil {
		return nil, errors.New("token manager must not be nil")
	}

	return &LoginService{
		authenticator: authenticator,
		tokens:        tokens,
	}, nil
}

func (s *LoginService) Login(
	ctx context.Context,
	input user.LoginInput,
) (LoginResult, error) {
	u, err := s.authenticator.Authenticate(ctx, input)
	if err != nil {
		return LoginResult{}, err
	}

	token, err := s.tokens.Issue(u.ID, int(u.Role))
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue access token: %w", err)
	}

	return LoginResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.tokens.ttl.Seconds()),
	}, nil
}
