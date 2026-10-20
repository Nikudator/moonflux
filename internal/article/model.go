package article

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidArticle = errors.New("invalid article")
	ErrNotFound       = errors.New("article not found")
	ErrForbidden      = errors.New("article access forbidden")
)

const (
	// MaxTitleRunes bounds the article title in Unicode code points.
	MaxTitleRunes = 300
	// MaxTeaserRunes bounds the article teaser in Unicode code points.
	MaxTeaserRunes = 1000
	// MaxContentBytes bounds the article body in bytes.
	MaxContentBytes = 1 << 20 // 1 MiB
)

// Article represents an article stored in PostgreSQL.
type Article struct {
	ID          string
	UserID      string
	Title       string
	Teaser      string
	Content     string
	IsDraft     bool
	CreatedAt   time.Time
	PublishedAt *time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// Validate checks required fields and length limits.
func (a Article) Validate() error {
	if strings.TrimSpace(a.UserID) == "" {
		return ErrInvalidArticle
	}

	title := strings.TrimSpace(a.Title)
	if title == "" || len([]rune(title)) > MaxTitleRunes {
		return ErrInvalidArticle
	}

	if len([]rune(a.Teaser)) > MaxTeaserRunes {
		return ErrInvalidArticle
	}

	if len(a.Content) > MaxContentBytes {
		return ErrInvalidArticle
	}

	if !a.IsDraft && a.PublishedAt == nil {
		return ErrInvalidArticle
	}

	return nil
}

// FullText returns the teaser followed by the article content.
func (a Article) FullText() string {
	if a.Teaser == "" {
		return a.Content
	}
	if a.Content == "" {
		return a.Teaser
	}

	return a.Teaser + "\n" + a.Content
}

// IsPublishedAt reports whether the article is publicly available
// at the specified time. Repository queries must also exclude soft-deleted
// articles and enforce the same visibility rules.
func (a Article) IsPublishedAt(now time.Time) bool {
	return !a.IsDraft &&
		a.DeletedAt == nil &&
		a.PublishedAt != nil &&
		!a.PublishedAt.After(now)
}
