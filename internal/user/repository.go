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

// UpdateProfile updates editable fields of an active user.
func (r *Repository) UpdateProfile(
	ctx context.Context,
	id string,
	in ProfileUpdateInput,
) (User, error) {
	if err := in.Validate(); err != nil {
		return User{}, err
	}

	var u User

	err := r.pool.QueryRow(ctx, `
		UPDATE users
		SET
			nickname = CASE WHEN $2 THEN $3 ELSE nickname END,
			first_name = CASE WHEN $4 THEN $5 ELSE first_name END,
			last_name = CASE WHEN $6 THEN $7 ELSE last_name END,
			patronymic = CASE WHEN $8 THEN $9 ELSE patronymic END,
			birth_date = CASE WHEN $10 THEN $11 ELSE birth_date END,
			about = CASE WHEN $12 THEN $13 ELSE about END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING
			id::text, nickname, email, first_name, last_name, patronymic,
			role, birth_date, about, password_hash, created_at, updated_at, deleted_at
	`,
		id,
		in.Nickname.Present, in.Nickname.Value,
		in.FirstName.Present, in.FirstName.Value,
		in.LastName.Present, in.LastName.Value,
		in.Patronymic.Present, optionalStringValue(in.Patronymic),
		in.BirthDate.Present, optionalBirthDateValue(in.BirthDate),
		in.About.Present, in.About.Value,
	).Scan(
		&u.ID, &u.Nickname, &u.Email, &u.FirstName, &u.LastName, &u.Patronymic,
		&u.Role, &u.BirthDate, &u.About, &u.PasswordHash,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, fmt.Errorf("%w: nickname", ErrConflict)
		}

		return User{}, fmt.Errorf("update user profile: %w", err)
	}

	return u, nil
}

func optionalStringValue(value Optional[string]) *string {
	if !value.Present || value.Null {
		return nil
	}

	return &value.Value
}

func optionalBirthDateValue(value Optional[time.Time]) *time.Time {
	if !value.Present || value.Null {
		return nil
	}

	return &value.Value
}
