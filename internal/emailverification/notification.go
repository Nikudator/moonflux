package emailverification

import (
	"context"
	"errors"
)

var ErrEmailAddressRequired = errors.New("email address is required")

// NotificationService issues verification tokens and sends them by email.
type NotificationService struct {
	verification *Service
	mailer       Mailer
}

// NewNotificationService creates a notification service.
func NewNotificationService(
	verification *Service,
	mailer Mailer,
) (*NotificationService, error) {
	if verification == nil {
		return nil, errors.New("email verification service is required")
	}
	if mailer == nil {
		return nil, errors.New("email verification mailer is required")
	}

	return &NotificationService{
		verification: verification,
		mailer:       mailer,
	}, nil
}

// Send issues a verification token and sends it to the specified address.
func (s *NotificationService) Send(
	ctx context.Context,
	userID string,
	email string,
) error {
	if email == "" {
		return ErrEmailAddressRequired
	}

	token, err := s.verification.Issue(ctx, userID)
	if err != nil {
		return err
	}

	if err := s.mailer.SendVerification(ctx, email, token); err != nil {
		return errors.New("send email verification message failed")
	}

	return nil
}

// Resend sends another verification email to an eligible user.
func (s *NotificationService) Resend(
	ctx context.Context,
	email string,
) error {
	user, err := s.verification.FindPendingUserByEmail(ctx, email)
	if err != nil {
		return err
	}

	return s.Send(ctx, user.ID, user.Email)
}
