package emailverification

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"moonflux/internal/platform/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryIssueResendCooldown(t *testing.T) {
	dsn := os.Getenv("MOONFLUX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOONFLUX_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database DSN: %v", err)
	}

	if poolConfig.ConnConfig.Database != "moonflux_migration_test" {
		t.Fatalf(
			"refusing to run integration test against database %q",
			poolConfig.ConnConfig.Database,
		)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate test user ID: %v", err)
	}
	userID := uuid.String(id)
	nickname := "it_" + strings.ReplaceAll(userID, "-", "")

	email := nickname + "@example.com"

	_, err = pool.Exec(ctx, `
	INSERT INTO users (
		id, nickname, first_name, last_name, password_hash, email
	)
	VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash', $3)
`, userID, nickname, email)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := pool.Exec(
			cleanupCtx,
			`DELETE FROM email_verification_tokens WHERE user_id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test tokens: %v", cleanupErr)
		}

		_, cleanupErr = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test user: %v", cleanupErr)
		}
	})

	repo := NewPostgresRepository(pool)
	pendingUser, err := repo.FindPendingUserByEmail(
		ctx,
		strings.ToUpper(email),
	)
	if err != nil {
		t.Fatalf("find pending user by email: %v", err)
	}
	if pendingUser.ID != userID {
		t.Fatalf("found user ID = %q, want %q", pendingUser.ID, userID)
	}
	if pendingUser.Email != email {
		t.Fatalf("found email = %q, want %q", pendingUser.Email, email)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	cooldown := time.Hour

	firstHash := sha256.Sum256([]byte(fmt.Sprintf("first-%s", userID)))
	err = repo.Issue(
		ctx,
		userID,
		firstHash[:],
		now.Add(2*time.Hour),
		now,
		cooldown,
	)
	if err != nil {
		t.Fatalf("issue first token: %v", err)
	}

	secondNow := now.Add(30 * time.Minute)
	secondHash := sha256.Sum256([]byte(fmt.Sprintf("second-%s", userID)))

	err = repo.Issue(
		ctx,
		userID,
		secondHash[:],
		secondNow.Add(2*time.Hour),
		secondNow,
		cooldown,
	)
	if err != ErrResendCooldown {
		t.Fatalf("second Issue() error = %v, want ErrResendCooldown", err)
	}

	// A resend is allowed once the cooldown has elapsed.
	thirdNow := now.Add(cooldown)
	thirdHash := sha256.Sum256([]byte(fmt.Sprintf("third-%s", userID)))

	err = repo.Issue(
		ctx,
		userID,
		thirdHash[:],
		thirdNow.Add(2*time.Hour),
		thirdNow,
		cooldown,
	)
	if err != nil {
		t.Fatalf("issue token after cooldown: %v", err)
	}

	// The first token must have been revoked.
	var revokedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT revoked_at
		FROM email_verification_tokens
		WHERE token_hash = $1
	`, firstHash[:]).Scan(&revokedAt)
	if err != nil {
		t.Fatalf("load first token revocation time: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("first token was not revoked")
	}
	err = repo.Verify(ctx, firstHash[:], thirdNow)
	if err != ErrInvalidToken {
		t.Fatalf("Verify() for revoked token error = %v, want ErrInvalidToken", err)
	}
	// The newly issued token must remain active.
	var activeCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM email_verification_tokens
		WHERE token_hash = $1
		  AND used_at IS NULL
		  AND revoked_at IS NULL
	`, thirdHash[:]).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active third token: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active third token count = %d, want 1", activeCount)
	}

	// Verify the newly issued token.
	verifyNow := thirdNow.Add(time.Minute)
	err = repo.Verify(ctx, thirdHash[:], verifyNow)
	if err != nil {
		t.Fatalf("verify third token: %v", err)
	}

	// The user's email must now be verified.
	var emailVerifiedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT email_verified_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&emailVerifiedAt)
	if err != nil {
		t.Fatalf("load email verification time: %v", err)
	}
	if emailVerifiedAt == nil {
		t.Fatal("user email was not marked as verified")
	}
	if !emailVerifiedAt.Equal(verifyNow) {
		t.Fatalf(
			"email verified at = %v, want %v",
			*emailVerifiedAt,
			verifyNow,
		)
	}

	// The token must be marked as used, not revoked.
	var usedAt, verifiedRevokedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT used_at, revoked_at
		FROM email_verification_tokens
		WHERE token_hash = $1
	`, thirdHash[:]).Scan(&usedAt, &verifiedRevokedAt)
	if err != nil {
		t.Fatalf("load verified token state: %v", err)
	}
	if usedAt == nil {
		t.Fatal("verified token was not marked as used")
	}
	if !usedAt.Equal(verifyNow) {
		t.Fatalf("token used at = %v, want %v", *usedAt, verifyNow)
	}
	if verifiedRevokedAt != nil {
		t.Fatalf("verified token unexpectedly revoked at %v", *verifiedRevokedAt)
	}

	// A used token must not be accepted again.
	err = repo.Verify(ctx, thirdHash[:], verifyNow.Add(time.Second))
	if err != ErrInvalidToken {
		t.Fatalf("second Verify() error = %v, want ErrInvalidToken", err)
	}

}

func TestPostgresRepositoryVerifyRejectsUnknownToken(t *testing.T) {
	databaseURL := os.Getenv("MOONFLUX_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MOONFLUX_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	if config.ConnConfig.Database != "moonflux_migration_test" {
		t.Fatalf("refusing to run integration test against database %q", config.ConnConfig.Database)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("create PostgreSQL pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	repo := NewPostgresRepository(pool)

	unknownHash := sha256.Sum256([]byte("unknown-email-verification-token"))
	err = repo.Verify(ctx, unknownHash[:], time.Now().UTC())
	if err != ErrInvalidToken {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}
}

func TestPostgresRepositoryVerifyRejectsExpiredToken(t *testing.T) {
	dsn := os.Getenv("MOONFLUX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOONFLUX_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database DSN: %v", err)
	}
	if poolConfig.ConnConfig.Database != "moonflux_migration_test" {
		t.Fatalf(
			"refusing to run integration test against database %q",
			poolConfig.ConnConfig.Database,
		)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate test user ID: %v", err)
	}
	userID := uuid.String(id)
	nickname := "it_" + strings.ReplaceAll(userID, "-", "")

	_, err = pool.Exec(ctx, `
		INSERT INTO users (
			id, nickname, first_name, last_name, password_hash
		)
		VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash')
	`, userID, nickname)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := pool.Exec(
			cleanupCtx,
			`DELETE FROM email_verification_tokens WHERE user_id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test tokens: %v", cleanupErr)
		}

		_, cleanupErr = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test user: %v", cleanupErr)
		}
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	tokenHash := sha256.Sum256([]byte("expired-token-" + userID))
	expiresAt := now.Add(-time.Minute)

	tokenID, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate test token ID: %v", err)
	}

	_, err = pool.Exec(ctx, `
    INSERT INTO email_verification_tokens (
        id, user_id, token_hash, created_at, expires_at
    )
    VALUES ($1, $2, $3, $4, $5)
`, uuid.String(tokenID), userID, tokenHash[:], expiresAt.Add(-time.Hour), expiresAt)
	if err != nil {
		t.Fatalf("create expired token: %v", err)
	}

	repo := NewPostgresRepository(pool)
	err = repo.Verify(ctx, tokenHash[:], now)
	if err != ErrInvalidToken {
		t.Fatalf("Verify() error = %v, want ErrInvalidToken", err)
	}

	var emailVerifiedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT email_verified_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&emailVerifiedAt)
	if err != nil {
		t.Fatalf("load email verification time: %v", err)
	}
	if emailVerifiedAt != nil {
		t.Fatalf("email_verified_at = %v, want NULL", *emailVerifiedAt)
	}

	var usedAt, revokedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT used_at, revoked_at
		FROM email_verification_tokens
		WHERE token_hash = $1
	`, tokenHash[:]).Scan(&usedAt, &revokedAt)
	if err != nil {
		t.Fatalf("load expired token state: %v", err)
	}
	if usedAt != nil {
		t.Fatalf("expired token unexpectedly marked as used at %v", *usedAt)
	}
	if revokedAt != nil {
		t.Fatalf("expired token unexpectedly revoked at %v", *revokedAt)
	}
}

func TestPostgresRepositoryIssueRejectsVerifiedEmail(t *testing.T) {
	dsn := os.Getenv("MOONFLUX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOONFLUX_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database DSN: %v", err)
	}
	if poolConfig.ConnConfig.Database != "moonflux_migration_test" {
		t.Fatalf(
			"refusing to run integration test against database %q",
			poolConfig.ConnConfig.Database,
		)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate test user ID: %v", err)
	}
	userID := uuid.String(id)
	nickname := "it_" + strings.ReplaceAll(userID, "-", "")
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = pool.Exec(ctx, `
		INSERT INTO users (
			id, nickname, first_name, last_name, password_hash,
			email_verified_at
		)
		VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash', $3)
	`, userID, nickname, now)
	if err != nil {
		t.Fatalf("create verified test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := pool.Exec(
			cleanupCtx,
			`DELETE FROM email_verification_tokens WHERE user_id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test tokens: %v", cleanupErr)
		}

		_, cleanupErr = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test user: %v", cleanupErr)
		}
	})

	repo := NewPostgresRepository(pool)
	tokenHash := sha256.Sum256([]byte("verified-user-token-" + userID))

	err = repo.Issue(
		ctx,
		userID,
		tokenHash[:],
		now.Add(time.Hour),
		now,
		0,
	)
	if err != ErrEmailVerified {
		t.Fatalf("Issue() error = %v, want ErrEmailVerified", err)
	}

	var tokenCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM email_verification_tokens
		WHERE user_id = $1
	`, userID).Scan(&tokenCount)
	if err != nil {
		t.Fatalf("count verification tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("verification token count = %d, want 0", tokenCount)
	}
}

func TestPostgresRepositoryFindPendingUserByEmail(t *testing.T) {
	dsn := os.Getenv("MOONFLUX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MOONFLUX_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database DSN: %v", err)
	}

	if poolConfig.ConnConfig.Database != "moonflux_migration_test" {
		t.Fatalf(
			"refusing to run integration test against database %q",
			poolConfig.ConnConfig.Database,
		)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate test user ID: %v", err)
	}
	userID := uuid.String(id)
	nickname := "it_" + strings.ReplaceAll(userID, "-", "")
	email := nickname + "@example.com"

	_, err = pool.Exec(ctx, `
		INSERT INTO users (
			id, nickname, first_name, last_name, password_hash, email
		)
		VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash', $3)
	`, userID, nickname, email)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := pool.Exec(
			cleanupCtx,
			`DELETE FROM email_verification_tokens WHERE user_id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test tokens: %v", cleanupErr)
		}

		_, cleanupErr = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1`,
			userID,
		)
		if cleanupErr != nil {
			t.Errorf("clean up test user: %v", cleanupErr)
		}
	})

	repo := NewPostgresRepository(pool)

	// An active user with an unverified email must be found,
	// regardless of the email's letter case.
	found, err := repo.FindPendingUserByEmail(ctx, strings.ToUpper(email))
	if err != nil {
		t.Fatalf("find pending user: %v", err)
	}
	if found.ID != userID {
		t.Fatalf("found ID = %q, want %q", found.ID, userID)
	}
	if found.Email != email {
		t.Fatalf("found email = %q, want %q", found.Email, email)
	}

	// A verified email must not be found.
	_, err = pool.Exec(ctx, `
		UPDATE users
		SET email_verified_at = NOW()
		WHERE id = $1
	`, userID)
	if err != nil {
		t.Fatalf("verify test user email: %v", err)
	}

	_, err = repo.FindPendingUserByEmail(ctx, email)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("verified user's lookup error = %v, want ErrUserNotFound", err)
	}

	// A soft-deleted user must not be found either.
	_, err = pool.Exec(ctx, `
		UPDATE users
		SET email_verified_at = NULL,
		    deleted_at = NOW()
		WHERE id = $1
	`, userID)
	if err != nil {
		t.Fatalf("soft-delete test user: %v", err)
	}

	_, err = repo.FindPendingUserByEmail(ctx, email)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("deleted user's lookup error = %v, want ErrUserNotFound", err)
	}
}
