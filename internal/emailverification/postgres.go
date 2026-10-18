package emailverification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"moonflux/internal/platform/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository stores email verification tokens in PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository creates an email verification repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Issue(
	ctx context.Context,
	userID string,
	tokenHash []byte,
	expiresAt time.Time,
	now time.Time,
	resendCooldown time.Duration,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email verification token issue: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Lock the active user row to serialize token issuance for this user.
	var emailVerifiedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT email_verified_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&emailVerifiedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("lock user for email verification: %w", err)
	}
	if emailVerifiedAt != nil {
		return ErrEmailVerified
	}
	// Enforce the resend cooldown while holding the user row lock.
	if resendCooldown > 0 {
		var lastCreatedAt time.Time
		err = tx.QueryRow(ctx, `
        SELECT created_at
        FROM email_verification_tokens
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT 1
    `, userID).Scan(&lastCreatedAt)

		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check email verification resend cooldown: %w", err)
		}

		if err == nil && now.Before(lastCreatedAt.Add(resendCooldown)) {
			return ErrResendCooldown
		}
	}
	_, err = tx.Exec(ctx, `
    UPDATE email_verification_tokens
    SET revoked_at = $2
    WHERE user_id = $1 AND used_at IS NULL AND revoked_at IS NULL
`, userID, now)
	if err != nil {
		return fmt.Errorf("revoke previous email verification tokens: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate email verification token ID: %w", err)
	}

	_, err = tx.Exec(ctx, `
    INSERT INTO email_verification_tokens (
        id, user_id, token_hash, created_at, expires_at
    )
    VALUES ($1, $2, $3, $4, $5)
    `, uuid.String(id), userID, tokenHash, now, expiresAt)
	if err != nil {
		return fmt.Errorf("insert email verification token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email verification token issue: %w", err)
	}

	return nil
}

func (r *PostgresRepository) Verify(
	ctx context.Context,
	tokenHash []byte,
	now time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email verification: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Find the user without locking the token yet.
	var userID string
	err = tx.QueryRow(ctx, `
		SELECT user_id::text
		FROM email_verification_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		return fmt.Errorf("find email verification token: %w", err)
	}

	// Lock the active user first, matching Issue's lock order.
	var emailVerifiedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT email_verified_at
		FROM users
		WHERE id = $1
		  AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&emailVerifiedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		return fmt.Errorf("lock user for email verification: %w", err)
	}
	if emailVerifiedAt != nil {
		return ErrInvalidToken
	}

	// Lock and re-check the token after locking the user.
	var tokenID string
	var expiresAt time.Time
	var usedAt, revokedAt *time.Time

	err = tx.QueryRow(ctx, `
		SELECT id::text, expires_at, used_at, revoked_at
		FROM email_verification_tokens
		WHERE token_hash = $1
		  AND user_id = $2
		FOR UPDATE
	`, tokenHash, userID).Scan(
		&tokenID,
		&expiresAt,
		&usedAt,
		&revokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		return fmt.Errorf("lock email verification token: %w", err)
	}

	if usedAt != nil || revokedAt != nil || !expiresAt.After(now) {
		return ErrInvalidToken
	}

	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET email_verified_at = $2,
		    updated_at = $2
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND email_verified_at IS NULL
	`, userID, now)
	if err != nil {
		return fmt.Errorf("verify user email: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidToken
	}

	tag, err = tx.Exec(ctx, `
		UPDATE email_verification_tokens
		SET used_at = $3
		WHERE id = $1
		  AND user_id = $2
		  AND used_at IS NULL
		  AND revoked_at IS NULL
		  AND expires_at > $3
	`, tokenID, userID, now)
	if err != nil {
		return fmt.Errorf("mark email verification token as used: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidToken
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email verification: %w", err)
	}

	return nil
}

// FindPendingUserByEmail finds an active user whose email is not verified.
func (r *PostgresRepository) FindPendingUserByEmail(
	ctx context.Context,
	email string,
) (PendingUser, error) {
	var user PendingUser

	err := r.pool.QueryRow(ctx, `
		SELECT id::text, email
		FROM users
		WHERE deleted_at IS NULL
		  AND email_verified_at IS NULL
		  AND email IS NOT NULL
		  AND lower(email) = lower($1)
	`, email).Scan(&user.ID, &user.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PendingUser{}, ErrUserNotFound
		}

		return PendingUser{}, errors.New("find pending user by email failed")
	}

	return user, nil
}
