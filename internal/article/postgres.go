package article

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxFeedLimit = 100

// PostgresRepository provides access to articles stored in PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

func (r *PostgresRepository) Create(ctx context.Context, a Article) error {
	if err := a.Validate(); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO articles (
			id, user_id, title, teaser, content, is_draft,
			created_at, published_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, NOW())
	`,
		a.ID, a.UserID, a.Title, a.Teaser, a.Content,
		a.IsDraft, a.PublishedAt,
	)
	if err != nil {
		return fmt.Errorf("create article: %w", err)
	}

	return nil
}

func (r *PostgresRepository) GetPublishedByID(
	ctx context.Context,
	id string,
	now time.Time,
) (Article, error) {
	a, err := scanArticle(r.pool.QueryRow(ctx, `
		SELECT
			id::text, user_id::text, title, teaser, content, is_draft,
			created_at, published_at, updated_at, deleted_at
		FROM articles
		WHERE id = $1
		  AND is_draft = FALSE
		  AND published_at <= $2
		  AND deleted_at IS NULL
	`, id, now))
	if err != nil {
		return Article{}, fmt.Errorf("get published article: %w", err)
	}

	return a, nil
}

func (r *PostgresRepository) ListPublic(
	ctx context.Context,
	filter FeedFilter,
	now time.Time,
) ([]Article, error) {
	if err := validateFeedFilter(filter); err != nil {
		return nil, err
	}

	var rows pgx.Rows
	var err error

	if filter.Cursor == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT
				id::text, user_id::text, title, teaser, content, is_draft,
				created_at, published_at, updated_at, deleted_at
			FROM articles
			WHERE is_draft = FALSE
			  AND published_at <= $1
			  AND deleted_at IS NULL
			ORDER BY published_at DESC, id DESC
			LIMIT $2
		`, now, filter.Limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT
				id::text, user_id::text, title, teaser, content, is_draft,
				created_at, published_at, updated_at, deleted_at
			FROM articles
			WHERE is_draft = FALSE
			  AND published_at <= $1
			  AND deleted_at IS NULL
			  AND (published_at, id) < ($2, $3::uuid)
			ORDER BY published_at DESC, id DESC
			LIMIT $4
		`, now, filter.Cursor.PublishedAt, filter.Cursor.ID, filter.Limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list public articles: %w", err)
	}

	return collectArticles(rows)
}

func (r *PostgresRepository) ListPublishedByUser(
	ctx context.Context,
	userID string,
	filter FeedFilter,
	now time.Time,
) ([]Article, error) {
	if err := validateFeedFilter(filter); err != nil {
		return nil, err
	}

	var rows pgx.Rows
	var err error

	if filter.Cursor == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT
				id::text, user_id::text, title, teaser, content, is_draft,
				created_at, published_at, updated_at, deleted_at
			FROM articles
			WHERE user_id = $1
			  AND is_draft = FALSE
			  AND published_at <= $2
			  AND deleted_at IS NULL
			ORDER BY published_at DESC, id DESC
			LIMIT $3
		`, userID, now, filter.Limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT
				id::text, user_id::text, title, teaser, content, is_draft,
				created_at, published_at, updated_at, deleted_at
			FROM articles
			WHERE user_id = $1
			  AND is_draft = FALSE
			  AND published_at <= $2
			  AND deleted_at IS NULL
			  AND (published_at, id) < ($3, $4::uuid)
			ORDER BY published_at DESC, id DESC
			LIMIT $5
		`, userID, now, filter.Cursor.PublishedAt, filter.Cursor.ID, filter.Limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list published articles by user: %w", err)
	}

	return collectArticles(rows)
}

func (r *PostgresRepository) ListByAuthor(
	ctx context.Context,
	userID string,
	filter FeedFilter,
) ([]Article, error) {
	if err := validateFeedFilter(filter); err != nil {
		return nil, err
	}

	var rows pgx.Rows
	var err error

	if filter.Cursor == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT
				id::text, user_id::text, title, teaser, content, is_draft,
				created_at, published_at, updated_at, deleted_at
			FROM articles
			WHERE user_id = $1
			  AND deleted_at IS NULL
			ORDER BY updated_at DESC, id DESC
			LIMIT $2
		`, userID, filter.Limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT
				id::text, user_id::text, title, teaser, content, is_draft,
				created_at, published_at, updated_at, deleted_at
			FROM articles
			WHERE user_id = $1
			  AND deleted_at IS NULL
			  AND (updated_at, id) < ($2, $3::uuid)
			ORDER BY updated_at DESC, id DESC
			LIMIT $4
		`, userID, filter.Cursor.UpdatedAt, filter.Cursor.ID, filter.Limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list author's articles: %w", err)
	}

	return collectArticles(rows)
}

func (r *PostgresRepository) Update(
	ctx context.Context,
	a Article,
) (Article, error) {
	if err := a.Validate(); err != nil {
		return Article{}, err
	}

	updated, err := scanArticle(r.pool.QueryRow(ctx, `
		UPDATE articles
		SET title = $3,
		    teaser = $4,
		    content = $5,
		    is_draft = $6,
		    published_at = $7,
		    updated_at = NOW()
		WHERE id = $1
		  AND user_id = $2
		  AND deleted_at IS NULL
		RETURNING
			id::text, user_id::text, title, teaser, content, is_draft,
			created_at, published_at, updated_at, deleted_at
	`,
		a.ID, a.UserID, a.Title, a.Teaser, a.Content,
		a.IsDraft, a.PublishedAt,
	))
	if err != nil {
		return Article{}, fmt.Errorf("update article: %w", err)
	}

	return updated, nil
}

func (r *PostgresRepository) SoftDelete(
	ctx context.Context,
	id string,
	userID string,
	now time.Time,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE articles
		SET deleted_at = $3,
		    updated_at = $3
		WHERE id = $1
		  AND user_id = $2
		  AND deleted_at IS NULL
	`, id, userID, now)
	if err != nil {
		return fmt.Errorf("soft-delete article: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

type articleRow interface {
	Scan(dest ...any) error
}

func scanArticle(row articleRow) (Article, error) {
	var a Article

	err := row.Scan(
		&a.ID,
		&a.UserID,
		&a.Title,
		&a.Teaser,
		&a.Content,
		&a.IsDraft,
		&a.CreatedAt,
		&a.PublishedAt,
		&a.UpdatedAt,
		&a.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Article{}, ErrNotFound
		}
		return Article{}, err
	}

	return a, nil
}

func collectArticles(rows pgx.Rows) ([]Article, error) {
	defer rows.Close()

	articles := make([]Article, 0)

	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan article: %w", err)
		}

		articles = append(articles, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate articles: %w", err)
	}

	return articles, nil
}

func validateFeedFilter(filter FeedFilter) error {
	if filter.Limit < 1 || filter.Limit > maxFeedLimit {
		return fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidArticle, maxFeedLimit)
	}

	return nil
}
