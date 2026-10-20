package article

import (
	"context"
	"fmt"
	"strings"
	"time"

	"moonflux/internal/platform/uuid"
)

// ArticleInput is the client-provided data for creating or updating an article.
// The service trims Title and Teaser and fills PublishedAt when the article
// is published without an explicit timestamp.
type ArticleInput struct {
	Title       string
	Teaser      string
	Content     string
	IsDraft     bool
	PublishedAt *time.Time
}

// Service implements article business rules.
type Service struct {
	repo  Repository
	now   func() time.Time
	newID func() (string, error)
}

// NewService creates an article service.
func NewService(repo Repository) *Service {
	return &Service{
		repo:  repo,
		now:   func() time.Time { return time.Now().UTC() },
		newID: defaultNewID,
	}
}

func defaultNewID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate article id: %w", err)
	}
	return uuid.String(id), nil
}

// Create creates a new article owned by actorID.
func (s *Service) Create(
	ctx context.Context,
	actorID string,
	input ArticleInput,
) (Article, error) {
	if strings.TrimSpace(actorID) == "" {
		return Article{}, ErrInvalidArticle
	}

	now := s.now()
	input = normalizeInput(input, now)

	id, err := s.newID()
	if err != nil {
		return Article{}, err
	}

	a := Article{
		ID:          id,
		UserID:      actorID,
		Title:       input.Title,
		Teaser:      input.Teaser,
		Content:     input.Content,
		IsDraft:     input.IsDraft,
		PublishedAt: input.PublishedAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := a.Validate(); err != nil {
		return Article{}, err
	}

	if err := s.repo.Create(ctx, a); err != nil {
		return Article{}, err
	}

	return a, nil
}

// Update replaces an existing article. Only the author or an admin may update.
func (s *Service) Update(
	ctx context.Context,
	actorID string,
	isAdmin bool,
	id string,
	input ArticleInput,
) (Article, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(id) == "" {
		return Article{}, ErrInvalidArticle
	}

	now := s.now()
	input = normalizeInput(input, now)

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Article{}, err
	}

	if !isAdmin && existing.UserID != actorID {
		return Article{}, ErrForbidden
	}

	existing.Title = input.Title
	existing.Teaser = input.Teaser
	existing.Content = input.Content
	existing.IsDraft = input.IsDraft
	existing.PublishedAt = input.PublishedAt

	if err := existing.Validate(); err != nil {
		return Article{}, err
	}

	return s.repo.Update(ctx, existing)
}

// Delete soft-deletes an article. Only the author or an admin may delete.
func (s *Service) Delete(
	ctx context.Context,
	actorID string,
	isAdmin bool,
	id string,
) error {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(id) == "" {
		return ErrInvalidArticle
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !isAdmin && existing.UserID != actorID {
		return ErrForbidden
	}

	// Pass existing.UserID, not actorID: the repository uses the owner ID
	// as a concurrency guard. For admin deletion of another user's article
	// we still need to match the actual owner row.
	return s.repo.SoftDelete(ctx, id, existing.UserID, s.now())
}

// GetPublished returns a published article by ID.
func (s *Service) GetPublished(ctx context.Context, id string) (Article, error) {
	if strings.TrimSpace(id) == "" {
		return Article{}, ErrInvalidArticle
	}
	return s.repo.GetPublishedByID(ctx, id, s.now())
}

// GetForAuthor returns an article for editing. Only the author or an admin may access.
func (s *Service) GetForAuthor(
	ctx context.Context,
	actorID string,
	isAdmin bool,
	id string,
) (Article, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(id) == "" {
		return Article{}, ErrInvalidArticle
	}

	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Article{}, err
	}

	if !isAdmin && a.UserID != actorID {
		return Article{}, ErrForbidden
	}

	return a, nil
}

// ListPublic returns the public feed at the current time.
func (s *Service) ListPublic(ctx context.Context, filter FeedFilter) ([]Article, error) {
	return s.repo.ListPublic(ctx, filter, s.now())
}

// ListByAuthorPublic returns the published articles of a specific author.
func (s *Service) ListByAuthorPublic(
	ctx context.Context,
	userID string,
	filter FeedFilter,
) ([]Article, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidArticle
	}
	return s.repo.ListPublishedByUser(ctx, userID, filter, s.now())
}

// ListOwn returns the actor's own articles, including drafts.
func (s *Service) ListOwn(
	ctx context.Context,
	actorID string,
	filter FeedFilter,
) ([]Article, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, ErrInvalidArticle
	}
	return s.repo.ListByAuthor(ctx, actorID, filter)
}

// normalizeInput trims text fields and resolves PublishedAt:
//   - drafts never carry a PublishedAt,
//   - a non-draft with no PublishedAt is published "now",
//   - a non-draft with an explicit PublishedAt keeps it (for scheduling or backdating).
//
// Content is intentionally NOT trimmed — whitespace is meaningful in long-form text.
func normalizeInput(input ArticleInput, now time.Time) ArticleInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Teaser = strings.TrimSpace(input.Teaser)
	if input.IsDraft {
		input.PublishedAt = nil
	} else if input.PublishedAt == nil {
		publishedAt := now
		input.PublishedAt = &publishedAt
	}
	return input
}
