package emailverification

import "context"

// Mailer sends email verification messages.
type Mailer interface {
	SendVerification(
		ctx context.Context,
		to string,
		token string,
	) error
}
