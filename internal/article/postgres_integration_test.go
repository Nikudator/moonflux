package article

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"moonflux/internal/platform/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests for PostgresRepository.
//
// They require MOONFLUX_TEST_DATABASE_URL pointing at a database named
// moonflux_migration_test. Without the variable, every test is skipped.
// Each test creates its own user(s) and article(s) and cleans up after
// itself via t.Cleanup (LIFO order: articles deleted before users).

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

// newTestUser inserts a minimal user row and registers cleanup.
//
// The nickname suffix is taken from the LAST 8 hex chars of the UUID:
// they come from the random part of UUID v7. Taking the first 8 chars
// would use the timestamp prefix, which is identical for every UUID
// generated within the same ~65-second window — that collides on
// users_nickname_active_uq when a single test creates two users.
func newTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate user id: %v", err)
	}
	userID := uuid.String(id)
	cleaned := strings.ReplaceAll(userID, "-", "")
	suffix := cleaned[len(cleaned)-8:]

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, nickname, first_name, last_name, password_hash)
		VALUES ($1, $2, 'Integration', 'Test', 'test-only-hash')
	`, userID, "n"+suffix); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1::uuid`, userID)
	})
	return userID
}

// createTestArticle inserts an article row with explicit published_at.
// Title includes the article UUID, which is guaranteed unique — unlike
// a truncated UUID prefix.
func createTestArticle(
	t *testing.T,
	pool *pgxpool.Pool,
	userID string,
	draft bool,
	publishedAt *time.Time,
) string {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generate article id: %v", err)
	}
	articleID := uuid.String(id)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO articles (
			id, user_id, title, teaser, content, is_draft,
			created_at, published_at, updated_at
		) VALUES ($1, $2, $3, 'teaser', 'content', $4, NOW(), $5, NOW())
	`, articleID, userID, "Test article "+articleID, draft, publishedAt); err != nil {
		t.Fatalf("create test article: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM articles WHERE id = $1::uuid`, articleID)
	})
	return articleID
}

func ids(list []Article) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ID
	}
	return out
}

// --- Create ---

func TestPostgresRepositoryCreateAndGetByID(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	userID := newTestUser(t, pool)

	publishedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	id, _ := uuid.NewV7()
	articleID := uuid.String(id)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM articles WHERE id = $1::uuid`, articleID)
	})

	a := Article{
		ID:          articleID,
		UserID:      userID,
		Title:       "Title",
		Teaser:      "Teaser",
		Content:     "Content",
		IsDraft:     false,
		PublishedAt: &publishedAt,
	}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(ctx, articleID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Title != "Title" || got.Teaser != "Teaser" || got.Content != "Content" {
		t.Errorf("unexpected content: %+v", got)
	}
	if got.IsDraft {
		t.Error("IsDraft = true, want false")
	}
	if got.PublishedAt == nil || !got.PublishedAt.Equal(publishedAt) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, publishedAt)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps must not be zero")
	}
}

func TestPostgresRepositoryCreateRejectsInvalidArticle(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	userID := newTestUser(t, pool)

	id, _ := uuid.NewV7()
	err := repo.Create(ctx, Article{
		ID:      uuid.String(id),
		UserID:  userID,
		Title:   "   ", // invalid: blank after trim
		IsDraft: true,
	})
	if !errors.Is(err, ErrInvalidArticle) {
		t.Fatalf("Create() error = %v, want ErrInvalidArticle", err)
	}
}

// --- GetPublishedByID visibility ---

func TestPostgresRepositoryGetPublishedByIDVisibility(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	userID := newTestUser(t, pool)

	now := time.Now().UTC()
	past := now.Add(-time.Hour).Truncate(time.Microsecond)
	future := now.Add(time.Hour).Truncate(time.Microsecond)

	publishedID := createTestArticle(t, pool, userID, false, &past)
	scheduledID := createTestArticle(t, pool, userID, false, &future)
	draftID := createTestArticle(t, pool, userID, true, nil)

	// Published: visible.
	if _, err := repo.GetPublishedByID(ctx, publishedID, now); err != nil {
		t.Fatalf("published: error = %v", err)
	}

	// Scheduled (future): hidden.
	if _, err := repo.GetPublishedByID(ctx, scheduledID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scheduled: error = %v, want ErrNotFound", err)
	}

	// Draft: hidden.
	if _, err := repo.GetPublishedByID(ctx, draftID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft: error = %v, want ErrNotFound", err)
	}

	// Unknown ID: hidden.
	if _, err := repo.GetPublishedByID(ctx, "01900000-0000-7000-8000-000000000000", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: error = %v, want ErrNotFound", err)
	}
}

// GetByID must return drafts (unlike GetPublishedByID); Service enforces
// visibility rules using the owner / admin check.
func TestPostgresRepositoryGetByIDReturnsDrafts(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	userID := newTestUser(t, pool)

	draftID := createTestArticle(t, pool, userID, true, nil)

	got, err := repo.GetByID(ctx, draftID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !got.IsDraft {
		t.Error("IsDraft = false, want true")
	}
}

// --- ListPublic + cursor pagination ---

func TestPostgresRepositoryListPublicPagination(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	userID := newTestUser(t, pool)
	now := time.Now().UTC()

	// Three published articles with distinct timestamps, one second apart.
	base := now.Add(-time.Hour).Truncate(time.Microsecond)
	t1 := base.Add(3 * time.Second) // newest
	t2 := base.Add(2 * time.Second)
	t3 := base.Add(1 * time.Second) // oldest

	// Insert out of order to prove ORDER BY published_at DESC works.
	id1 := createTestArticle(t, pool, userID, false, &t1)
	id2 := createTestArticle(t, pool, userID, false, &t2)
	id3 := createTestArticle(t, pool, userID, false, &t3)

	// A draft and a scheduled article must NOT appear in the public feed.
	createTestArticle(t, pool, userID, true, nil)
	createTestArticle(t, pool, userID, false, ptrTime(now.Add(time.Hour).Truncate(time.Microsecond)))

	// Sanity: without a limit issue, the public feed returns only 3.
	all, err := repo.ListPublic(ctx, FeedFilter{Limit: 100}, now)
	if err != nil {
		t.Fatalf("ListPublic(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListPublic(all) = %d articles, want 3; got %v", len(all), ids(all))
	}

	// Page 1: limit=2 → newest two.
	page1, err := repo.ListPublic(ctx, FeedFilter{Limit: 2}, now)
	if err != nil {
		t.Fatalf("ListPublic page1: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != id1 || page1[1].ID != id2 {
		t.Fatalf("page1 = %v, want [%s %s]", ids(page1), id1, id2)
	}

	// Page 2: cursor from the last item of page 1.
	cursor := &FeedCursor{
		PublishedAt: *page1[1].PublishedAt,
		ID:          page1[1].ID,
	}
	page2, err := repo.ListPublic(ctx, FeedFilter{Limit: 2, Cursor: cursor}, now)
	if err != nil {
		t.Fatalf("ListPublic page2: %v", err)
	}
	if len(page2) != 1 || page2[0].ID != id3 {
		t.Fatalf("page2 = %v, want [%s]", ids(page2), id3)
	}
}

// --- ListPublishedByUser ---

func TestPostgresRepositoryListPublishedByUser(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	authorID := newTestUser(t, pool)
	otherAuthorID := newTestUser(t, pool)
	now := time.Now().UTC()

	past := now.Add(-time.Hour).Truncate(time.Microsecond)
	future := now.Add(time.Hour).Truncate(time.Microsecond)

	publishedID := createTestArticle(t, pool, authorID, false, &past)
	createTestArticle(t, pool, authorID, false, &future)    // scheduled, excluded
	createTestArticle(t, pool, authorID, true, nil)         // draft, excluded
	createTestArticle(t, pool, otherAuthorID, false, &past) // other author, excluded

	list, err := repo.ListPublishedByUser(ctx, authorID, FeedFilter{Limit: 10}, now)
	if err != nil {
		t.Fatalf("ListPublishedByUser: %v", err)
	}
	if len(list) != 1 || list[0].ID != publishedID {
		t.Fatalf("result = %v, want only %s", ids(list), publishedID)
	}
}

// --- ListByAuthor (own feed, includes drafts) ---

func TestPostgresRepositoryListByAuthorIncludesDrafts(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	authorID := newTestUser(t, pool)
	now := time.Now().UTC()

	past := now.Add(-time.Hour).Truncate(time.Microsecond)
	future := now.Add(time.Hour).Truncate(time.Microsecond)

	publishedID := createTestArticle(t, pool, authorID, false, &past)
	draftID := createTestArticle(t, pool, authorID, true, nil)
	scheduledID := createTestArticle(t, pool, authorID, false, &future)

	list, err := repo.ListByAuthor(ctx, authorID, FeedFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListByAuthor: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListByAuthor returned %d, want 3", len(list))
	}
	got := make(map[string]bool, len(list))
	for _, a := range list {
		got[a.ID] = true
	}
	for _, want := range []string{publishedID, draftID, scheduledID} {
		if !got[want] {
			t.Errorf("missing article %q", want)
		}
	}
}

// --- Update owner guard ---

func TestPostgresRepositoryUpdateOwnerGuard(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	authorID := newTestUser(t, pool)
	otherUserID := newTestUser(t, pool)
	now := time.Now().UTC()

	past := now.Add(-time.Hour).Truncate(time.Microsecond)
	articleID := createTestArticle(t, pool, authorID, false, &past)

	// Wrong owner: WHERE id = $1 AND user_id = $2 → 0 rows affected → ErrNotFound.
	_, err := repo.Update(ctx, Article{
		ID:          articleID,
		UserID:      otherUserID,
		Title:       "Hacked",
		IsDraft:     false,
		PublishedAt: &past,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update as stranger: error = %v, want ErrNotFound", err)
	}

	// Correct owner: fields update, published_at can be cleared.
	updated, err := repo.Update(ctx, Article{
		ID:          articleID,
		UserID:      authorID,
		Title:       "Updated",
		Teaser:      "new teaser",
		Content:     "new content",
		IsDraft:     true,
		PublishedAt: nil,
	})
	if err != nil {
		t.Fatalf("Update as author: %v", err)
	}
	if updated.Title != "Updated" || updated.Teaser != "new teaser" || updated.Content != "new content" {
		t.Errorf("update did not apply: %+v", updated)
	}
	if !updated.IsDraft {
		t.Error("IsDraft = false, want true")
	}
	if updated.PublishedAt != nil {
		t.Errorf("PublishedAt = %v, want nil", updated.PublishedAt)
	}
}

// --- SoftDelete owner guard + idempotency ---

func TestPostgresRepositorySoftDelete(t *testing.T) {
	pool := setupIntegrationPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()
	authorID := newTestUser(t, pool)
	otherUserID := newTestUser(t, pool)
	now := time.Now().UTC()

	past := now.Add(-time.Hour).Truncate(time.Microsecond)
	articleID := createTestArticle(t, pool, authorID, false, &past)

	// Wrong owner.
	if err := repo.SoftDelete(ctx, articleID, otherUserID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SoftDelete as stranger: error = %v, want ErrNotFound", err)
	}

	// After failed delete the article is still visible.
	if _, err := repo.GetByID(ctx, articleID); err != nil {
		t.Fatalf("article should still exist after failed delete: %v", err)
	}

	// Correct owner.
	if err := repo.SoftDelete(ctx, articleID, authorID, now); err != nil {
		t.Fatalf("SoftDelete as author: %v", err)
	}

	// After delete: GetByID → ErrNotFound.
	if _, err := repo.GetByID(ctx, articleID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID after delete: %v, want ErrNotFound", err)
	}

	// Second SoftDelete → ErrNotFound (idempotent 404).
	if err := repo.SoftDelete(ctx, articleID, authorID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second SoftDelete: %v, want ErrNotFound", err)
	}
}

// --- helpers ---

func ptrTime(t time.Time) *time.Time { return &t }
