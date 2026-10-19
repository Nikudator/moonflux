package article

import (
	"context"
	"time"
)

// FeedFilter contains pagination options for an article feed.
type FeedFilter struct {
	Limit  int
	Cursor *FeedCursor
}

// FeedCursor identifies the position after which the next page starts.
type FeedCursor struct {
	PublishedAt time.Time
	UpdatedAt   time.Time
	ID          string
}

// Repository provides access to articles stored in PostgreSQL.
type Repository interface {
	Create(ctx context.Context, article Article) error

	GetPublishedByID(ctx context.Context, id string, now time.Time) (Article, error)

	ListPublic(ctx context.Context, filter FeedFilter, now time.Time) ([]Article, error)

	ListPublishedByUser(
		ctx context.Context,
		userID string,
		filter FeedFilter,
		now time.Time,
	) ([]Article, error)

	ListByAuthor(
		ctx context.Context,
		userID string,
		filter FeedFilter,
	) ([]Article, error)

	Update(ctx context.Context, article Article) (Article, error)

	SoftDelete(ctx context.Context, id, userID string, now time.Time) error
}
