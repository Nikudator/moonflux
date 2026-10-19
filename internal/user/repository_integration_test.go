package user

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"moonflux/internal/platform/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setupIntegrationPool is a shared helper for integration tests in this
// package. It mirrors the pattern from internal/emailverification and
// refuses to run against anything but moonflux_migration_test.
func setupIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

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
	t.Cleanup(func() { pool.Close() })

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	return pool
}

// TestRepositoryGetByLoginPrefersEmailMatch documents the priority rule:
// when a login matches one user's nickname and another user's email,
// the email match wins. The previous implementation used LIMIT 1 without
// ORDER BY, so the result was undefined.
func TestRepositoryGetByLoginPrefersEmailMatch(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	idA, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate user A id: %v", err)
	}
	idB, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate user B id: %v", err)
	}
	userAID := uuid.String(idA)
	userBID := uuid.String(idB)
	// nickname is varchar(40), so the shared value must be short enough
	// to fit both as a nickname and as a valid email.
	suffix := strings.ReplaceAll(userAID, "-", "")[:8]

	// The same string is user A's nickname and user B's email.
	shared := "u" + suffix + "@e.co"
	otherEmailA := "o" + suffix + "@e.co"
	nicknameB := "b" + suffix

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, nickname, email, first_name, last_name, password_hash)
		VALUES ($1, $2, $3, 'Integration', 'Test', 'test-only-hash')
	`, userAID, shared, otherEmailA)
	if err != nil {
		t.Fatalf("create user A: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, nickname, email, first_name, last_name, password_hash)
		VALUES ($1, $2, $3, 'Integration', 'Test', 'test-only-hash')
	`, userBID, nicknameB, shared)
	if err != nil {
		t.Fatalf("create user B: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id IN ($1::uuid, $2::uuid)`,
			userAID, userBID,
		)
	})

	got, err := repo.GetByLogin(ctx, shared)
	if err != nil {
		t.Fatalf("GetByLogin() error = %v", err)
	}
	if got.ID != userBID {
		t.Fatalf(
			"GetByLogin() matched %q (nickname %q), want user B %q",
			got.ID, got.Nickname, userBID,
		)
	}
}

func TestRepositoryGetByLoginEmailIsCaseInsensitive(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate user id: %v", err)
	}
	userID := uuid.String(id)
	suffix := strings.ReplaceAll(userID, "-", "")

	email := "user-" + suffix + "@example.com"
	nickname := "nick-" + suffix

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, nickname, email, first_name, last_name, password_hash)
		VALUES ($1, $2, $3, 'Integration', 'Test', 'test-only-hash')
	`, userID, nickname, email)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1::uuid`,
			userID,
		)
	})

	got, err := repo.GetByLogin(ctx, strings.ToUpper(email))
	if err != nil {
		t.Fatalf("GetByLogin() error = %v", err)
	}
	if got.ID != userID {
		t.Fatalf("GetByLogin() matched %q, want %q", got.ID, userID)
	}
}

func TestRepositoryGetByLoginNicknameIsCaseInsensitive(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate user id: %v", err)
	}
	userID := uuid.String(id)
	suffix := strings.ReplaceAll(userID, "-", "")[:8]

	nickname := "MixedCase" + suffix

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, nickname, first_name, last_name, password_hash)
		VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash')
	`, userID, nickname)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1::uuid`, userID)
	})

	for _, login := range []string{
		nickname,
		strings.ToLower(nickname),
		strings.ToUpper(nickname),
	} {
		got, err := repo.GetByLogin(ctx, login)
		if err != nil {
			t.Fatalf("GetByLogin(%q) error = %v", login, err)
		}
		if got.ID != userID {
			t.Fatalf("GetByLogin(%q) matched %q, want %q", login, got.ID, userID)
		}
	}
}

func TestRepositoryCreatesUserWithDefaultTokenVersion(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate user id: %v", err)
	}
	userID := uuid.String(id)
	suffix := strings.ReplaceAll(userID, "-", "")[:8]

	// token_version is deliberately omitted — we rely on the column default.
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, nickname, first_name, last_name, password_hash)
		VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash')
	`, userID, "n"+suffix)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1::uuid`, userID)
	})

	var version int64
	err = pool.QueryRow(ctx,
		`SELECT token_version FROM users WHERE id = $1::uuid`, userID,
	).Scan(&version)
	if err != nil {
		t.Fatalf("read token_version: %v", err)
	}
	if version != 1 {
		t.Fatalf("token_version default = %d, want 1", version)
	}
}
