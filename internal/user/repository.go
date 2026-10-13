package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides access to users stored in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create stores a new user.
// UUID generation and password hashing are the caller's responsibility.
func (r *Repository) Create(ctx context.Context, u User) error {
	if err := u.Validate(); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (
			id, nickname, email, first_name, last_name, patronymic,
			role, birth_date, about, password_hash, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW()
		)`,
		u.ID, u.Nickname, u.Email, u.FirstName, u.LastName, u.Patronymic,
		u.Role, u.BirthDate, u.About, u.PasswordHash,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: nickname or email", ErrConflict)
		}
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

// GetByID returns an active user by UUID.
func (r *Repository) GetByID(ctx context.Context, id string) (User, error) {
	var u User

	err := r.pool.QueryRow(ctx, `
		SELECT
			id::text, nickname, email, first_name, last_name, patronymic,
			role, birth_date, about, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&u.ID, &u.Nickname, &u.Email, &u.FirstName, &u.LastName, &u.Patronymic,
		&u.Role, &u.BirthDate, &u.About, &u.PasswordHash,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by id: %w", err)
	}

	return u, nil
}

// SoftDelete marks an active user as deleted.
func (r *Repository) SoftDelete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users
		SET deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`, id, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("soft-delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetByLogin returns an active user by nickname or email.
// The login value is matched exactly.
func (r *Repository) GetByLogin(ctx context.Context, login string) (User, error) {
	var u User

	err := r.pool.QueryRow(ctx, `
		SELECT
			id::text, nickname, email, first_name, last_name, patronymic,
			role, birth_date, about, password_hash, created_at, updated_at, deleted_at
		FROM users
		WHERE deleted_at IS NULL
		  AND (nickname = $1 OR email = $1)
		LIMIT 1
	`, login).Scan(
		&u.ID, &u.Nickname, &u.Email, &u.FirstName, &u.LastName, &u.Patronymic,
		&u.Role, &u.BirthDate, &u.About, &u.PasswordHash,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by login: %w", err)
	}

	return u, nil
}
