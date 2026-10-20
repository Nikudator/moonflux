package article

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRepository is a minimal in-memory implementation of Repository.
// It does not model cursor pagination or ordering: tests target service
// decisions, not SQL semantics.
type fakeRepository struct {
	articles map[string]Article

	createErr     error
	updateErr     error
	softDeleteErr error
	getByIDErr    error

	createCalls     int
	updateCalls     int
	softDeleteCalls int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{articles: make(map[string]Article)}
}

func (f *fakeRepository) Create(_ context.Context, a Article) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	f.articles[a.ID] = a
	return nil
}

func (f *fakeRepository) GetByID(_ context.Context, id string) (Article, error) {
	if f.getByIDErr != nil {
		return Article{}, f.getByIDErr
	}
	a, ok := f.articles[id]
	if !ok || a.DeletedAt != nil {
		return Article{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeRepository) GetPublishedByID(_ context.Context, id string, now time.Time) (Article, error) {
	a, ok := f.articles[id]
	if !ok || !a.IsPublishedAt(now) {
		return Article{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeRepository) ListPublic(_ context.Context, _ FeedFilter, now time.Time) ([]Article, error) {
	var out []Article
	for _, a := range f.articles {
		if a.IsPublishedAt(now) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListPublishedByUser(_ context.Context, userID string, _ FeedFilter, now time.Time) ([]Article, error) {
	var out []Article
	for _, a := range f.articles {
		if a.UserID == userID && a.IsPublishedAt(now) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListByAuthor(_ context.Context, userID string, _ FeedFilter) ([]Article, error) {
	var out []Article
	for _, a := range f.articles {
		if a.UserID == userID && a.DeletedAt == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepository) Update(_ context.Context, a Article) (Article, error) {
	f.updateCalls++
	if f.updateErr != nil {
		return Article{}, f.updateErr
	}
	existing, ok := f.articles[a.ID]
	if !ok || existing.DeletedAt != nil || existing.UserID != a.UserID {
		return Article{}, ErrNotFound
	}
	f.articles[a.ID] = a
	return a, nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, id, userID string, now time.Time) error {
	f.softDeleteCalls++
	if f.softDeleteErr != nil {
		return f.softDeleteErr
	}
	a, ok := f.articles[id]
	if !ok || a.DeletedAt != nil || a.UserID != userID {
		return ErrNotFound
	}
	a.DeletedAt = &now
	f.articles[id] = a
	return nil
}

const (
	testAuthorID = "01900000-0000-7000-8000-0000000000aa"
	testOtherID  = "01900000-0000-7000-8000-0000000000bb"
)

func fixedNow() time.Time {
	return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
}

func newTestService(repo Repository) *Service {
	svc := NewService(repo)
	svc.now = fixedNow
	svc.newID = func() (string, error) { return "01900000-0000-7000-8000-000000000001", nil }
	return svc
}

// --- Create ---

func TestServiceCreate(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	a, err := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "  Hello  ",
		Teaser:  "  short teaser  ",
		Content: "  body  ",
		IsDraft: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if a.ID != "01900000-0000-7000-8000-000000000001" {
		t.Errorf("ID = %q", a.ID)
	}
	if a.UserID != testAuthorID {
		t.Errorf("UserID = %q", a.UserID)
	}
	if a.Title != "Hello" {
		t.Errorf("Title was not trimmed: %q", a.Title)
	}
	if a.Teaser != "short teaser" {
		t.Errorf("Teaser was not trimmed: %q", a.Teaser)
	}
	if a.Content != "  body  " {
		t.Errorf("Content should not be trimmed: %q", a.Content)
	}
	if a.PublishedAt != nil {
		t.Errorf("draft must not carry PublishedAt: %v", a.PublishedAt)
	}
	if !a.CreatedAt.Equal(fixedNow()) || !a.UpdatedAt.Equal(fixedNow()) {
		t.Errorf("timestamps not set from service.now(): %+v", a)
	}
	if repo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1", repo.createCalls)
	}
}

func TestServiceCreateFillsPublishedAt(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	a, err := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Hello",
		IsDraft: false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if a.PublishedAt == nil {
		t.Fatal("PublishedAt must be filled for non-draft")
	}
	if !a.PublishedAt.Equal(fixedNow()) {
		t.Errorf("PublishedAt = %v, want %v", a.PublishedAt, fixedNow())
	}
}

func TestServiceCreateKeepsScheduledPublishedAt(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	future := fixedNow().Add(24 * time.Hour)
	a, err := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:       "Hello",
		IsDraft:     false,
		PublishedAt: &future,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if a.PublishedAt == nil || !a.PublishedAt.Equal(future) {
		t.Errorf("PublishedAt = %v, want %v", a.PublishedAt, future)
	}
}

func TestServiceCreateRejectsEmptyActor(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	_, err := svc.Create(context.Background(), " ", ArticleInput{Title: "x"})
	if !errors.Is(err, ErrInvalidArticle) {
		t.Fatalf("Create() error = %v, want ErrInvalidArticle", err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("repository should not be called, got %d calls", repo.createCalls)
	}
}

func TestServiceCreateRejectsInvalidInput(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	cases := []ArticleInput{
		{Title: ""},
		{Title: "   "},
		{Title: strings.Repeat("t", MaxTitleRunes+1)},
		{Title: "ok", Teaser: strings.Repeat("t", MaxTeaserRunes+1)},
	}
	for _, input := range cases {
		if _, err := svc.Create(context.Background(), testAuthorID, input); !errors.Is(err, ErrInvalidArticle) {
			t.Errorf("Create(%+v) error = %v, want ErrInvalidArticle", input, err)
		}
	}
	if repo.createCalls != 0 {
		t.Fatalf("repository calls = %d, want 0", repo.createCalls)
	}
}

// --- Update ---

func TestServiceUpdateAsAuthor(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Original",
		IsDraft: true,
	})

	updated, err := svc.Update(context.Background(), testAuthorID, false, created.ID, ArticleInput{
		Title:   "Updated",
		Content: "new body",
		IsDraft: true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Title != "Updated" || updated.Content != "new body" {
		t.Errorf("Update() did not apply fields: %+v", updated)
	}
	if repo.updateCalls != 1 {
		t.Errorf("Update calls = %d, want 1", repo.updateCalls)
	}
}

func TestServiceUpdateAsAdmin(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Original",
		IsDraft: true,
	})

	_, err := svc.Update(context.Background(), "admin-id", true, created.ID, ArticleInput{
		Title:   "Admin edit",
		IsDraft: true,
	})
	if err != nil {
		t.Fatalf("Update() as admin error = %v", err)
	}
}

func TestServiceUpdateAsStrangerForbidden(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Original",
		IsDraft: true,
	})

	_, err := svc.Update(context.Background(), testOtherID, false, created.ID, ArticleInput{
		Title:   "Hacked",
		IsDraft: true,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Update() error = %v, want ErrForbidden", err)
	}
	if repo.updateCalls != 0 {
		t.Errorf("repository.Update called despite forbidden: %d", repo.updateCalls)
	}
}

func TestServiceUpdateNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	_, err := svc.Update(context.Background(), testAuthorID, false, "missing-id", ArticleInput{
		Title:   "Whatever",
		IsDraft: true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

// --- Delete ---

func TestServiceDeleteAsAuthor(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Original",
		IsDraft: true,
	})

	if err := svc.Delete(context.Background(), testAuthorID, false, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if repo.articles[created.ID].DeletedAt == nil {
		t.Fatal("article was not marked deleted in repository")
	}
}

func TestServiceDeleteAsAdmin(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Original",
		IsDraft: true,
	})

	if err := svc.Delete(context.Background(), "admin-id", true, created.ID); err != nil {
		t.Fatalf("Delete() as admin error = %v", err)
	}
}

func TestServiceDeleteAsStrangerForbidden(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Original",
		IsDraft: true,
	})

	err := svc.Delete(context.Background(), testOtherID, false, created.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Delete() error = %v, want ErrForbidden", err)
	}
	if repo.softDeleteCalls != 0 {
		t.Errorf("SoftDelete called despite forbidden: %d", repo.softDeleteCalls)
	}
}

// --- Read ---

func TestServiceGetPublishedHidesDraft(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Draft",
		IsDraft: true,
	})

	_, err := svc.GetPublished(context.Background(), created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPublished() error = %v, want ErrNotFound", err)
	}
}

func TestServiceGetPublishedReturnsScheduledHidden(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	future := fixedNow().Add(time.Hour)
	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:       "Scheduled",
		IsDraft:     false,
		PublishedAt: &future,
	})

	_, err := svc.GetPublished(context.Background(), created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPublished() error = %v, want ErrNotFound for scheduled article", err)
	}
}

func TestServiceGetForAuthor(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	created, _ := svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Draft",
		IsDraft: true,
	})

	if _, err := svc.GetForAuthor(context.Background(), testAuthorID, false, created.ID); err != nil {
		t.Fatalf("GetForAuthor(author) error = %v", err)
	}
	if _, err := svc.GetForAuthor(context.Background(), "admin-id", true, created.ID); err != nil {
		t.Fatalf("GetForAuthor(admin) error = %v", err)
	}
	_, err := svc.GetForAuthor(context.Background(), testOtherID, false, created.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetForAuthor(stranger) error = %v, want ErrForbidden", err)
	}
}

func TestServiceListOwnIncludesDrafts(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)

	_, _ = svc.Create(context.Background(), testAuthorID, ArticleInput{
		Title:   "Draft",
		IsDraft: true,
	})

	list, err := svc.ListOwn(context.Background(), testAuthorID, FeedFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListOwn() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListOwn() returned %d articles, want 1", len(list))
	}
}
