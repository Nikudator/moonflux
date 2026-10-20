package article

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubArticleRepo is a minimal Repository implementation tuned for handler tests.
// Pagination semantics are simplified: it returns the first N articles from the
// internal slice, ignoring cursor. That is enough to test handler decisions
// (limit handling, cursor emission, auth) without re-implementing SQL.
type stubArticleRepo struct {
	articles      []Article
	listPublic    error
	listByUser    error
	listByAuthor  error
	getByIDErr    error
	getPubErr     error
	createErr     error
	updateErr     error
	softDeleteErr error
}

func (s *stubArticleRepo) Create(_ context.Context, a Article) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.articles = append(s.articles, a)
	return nil
}

func (s *stubArticleRepo) GetByID(_ context.Context, id string) (Article, error) {
	if s.getByIDErr != nil {
		return Article{}, s.getByIDErr
	}
	for _, a := range s.articles {
		if a.ID == id && a.DeletedAt == nil {
			return a, nil
		}
	}
	return Article{}, ErrNotFound
}

func (s *stubArticleRepo) GetPublishedByID(_ context.Context, id string, now time.Time) (Article, error) {
	if s.getPubErr != nil {
		return Article{}, s.getPubErr
	}
	for _, a := range s.articles {
		if a.ID == id && a.IsPublishedAt(now) {
			return a, nil
		}
	}
	return Article{}, ErrNotFound
}

func (s *stubArticleRepo) ListPublic(_ context.Context, filter FeedFilter, _ time.Time) ([]Article, error) {
	if s.listPublic != nil {
		return nil, s.listPublic
	}
	return truncate(s.articles, filter.Limit), nil
}

func (s *stubArticleRepo) ListPublishedByUser(_ context.Context, userID string, filter FeedFilter, _ time.Time) ([]Article, error) {
	if s.listByUser != nil {
		return nil, s.listByUser
	}
	var out []Article
	for _, a := range s.articles {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return truncate(out, filter.Limit), nil
}

func (s *stubArticleRepo) ListByAuthor(_ context.Context, userID string, filter FeedFilter) ([]Article, error) {
	if s.listByAuthor != nil {
		return nil, s.listByAuthor
	}
	var out []Article
	for _, a := range s.articles {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return truncate(out, filter.Limit), nil
}

func (s *stubArticleRepo) Update(_ context.Context, a Article) (Article, error) {
	if s.updateErr != nil {
		return Article{}, s.updateErr
	}
	for i, existing := range s.articles {
		if existing.ID == a.ID && existing.DeletedAt == nil && existing.UserID == a.UserID {
			s.articles[i] = a
			return a, nil
		}
	}
	return Article{}, ErrNotFound
}

func (s *stubArticleRepo) SoftDelete(_ context.Context, id, userID string, now time.Time) error {
	if s.softDeleteErr != nil {
		return s.softDeleteErr
	}
	for i, existing := range s.articles {
		if existing.ID == id && existing.DeletedAt == nil && existing.UserID == userID {
			s.articles[i].DeletedAt = &now
			return nil
		}
	}
	return ErrNotFound
}

func truncate(items []Article, limit int) []Article {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

// --- test helpers ---

func fixedNowHTTP() time.Time {
	return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
}

func newArticle(id, userID string, draft bool, publishedAt *time.Time) Article {
	t := fixedNowHTTP()
	return Article{
		ID:          id,
		UserID:      userID,
		Title:       "Title " + id,
		Teaser:      "Teaser " + id,
		Content:     "Content " + id,
		IsDraft:     draft,
		PublishedAt: publishedAt,
		CreatedAt:   t,
		UpdatedAt:   t,
	}
}

func newTestHandler(repo Repository, actorID string, isAdmin, authenticated bool) *HTTPHandler {
	svc := NewService(repo)
	svc.now = fixedNowHTTP
	return NewHTTPHandler(svc, func(_ *http.Request) (string, bool, bool) {
		if !authenticated {
			return "", false, false
		}
		return actorID, isAdmin, true
	})
}

// --- ListFeed ---

func TestListFeedEmpty(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles", nil)
	rec := httptest.NewRecorder()
	h.ListFeed(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp FeedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(resp.Items))
	}
	if resp.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty", resp.NextCursor)
	}
}

func TestListFeedNoMore(t *testing.T) {
	past := fixedNowHTTP().Add(-time.Hour)
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "u1", false, &past),
		newArticle("01900000-0000-7000-8000-000000000002", "u1", false, &past),
	}}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles?limit=20", nil)
	rec := httptest.NewRecorder()
	h.ListFeed(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp FeedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(resp.Items))
	}
	if resp.NextCursor != "" {
		t.Errorf("next_cursor should be empty on last page")
	}
}

func TestListFeedWithCursor(t *testing.T) {
	past := fixedNowHTTP().Add(-time.Hour)
	var arts []Article
	for i := 0; i < 30; i++ {
		id := "01900000-0000-7000-8000-0000000000"
		id += string(rune('a'+i%26)) + string(rune('a'+i/26))
		arts = append(arts, newArticle(id, "u1", false, &past))
	}
	repo := &stubArticleRepo{articles: arts}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles?limit=20", nil)
	rec := httptest.NewRecorder()
	h.ListFeed(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp FeedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 20 {
		t.Fatalf("items = %d, want 20", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Fatal("next_cursor must be present when there are more items")
	}
}

func TestListFeedInvalidLimit(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	for _, q := range []string{"?limit=0", "?limit=-5", "?limit=abc"} {
		req := httptest.NewRequest(http.MethodGet, "/articles"+q, nil)
		rec := httptest.NewRecorder()
		h.ListFeed(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestListFeedInvalidCursor(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	for _, q := range []string{"?cursor=bad", "?cursor=" + base64Of(`{"k":"wrong","t":1,"i":"01900000-0000-7000-8000-000000000001"}`)} {
		req := httptest.NewRequest(http.MethodGet, "/articles"+q, nil)
		rec := httptest.NewRecorder()
		h.ListFeed(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestListFeedRepoError(t *testing.T) {
	repo := &stubArticleRepo{listPublic: errors.New("db down")}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles", nil)
	rec := httptest.NewRecorder()
	h.ListFeed(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db down") {
		t.Fatal("response leaks internal error")
	}
}

// --- GetByID ---

func TestGetByIDPublished(t *testing.T) {
	past := fixedNowHTTP().Add(-time.Hour)
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "u1", false, &past),
	}}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var full FullArticle
	_ = json.Unmarshal(rec.Body.Bytes(), &full)
	if full.ID != "01900000-0000-7000-8000-000000000001" {
		t.Errorf("id = %q", full.ID)
	}
	if full.Content == "" {
		t.Error("content should be present in full article")
	}
}

func TestGetByIDDraftHidden(t *testing.T) {
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "u1", true, nil),
	}}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for draft", rec.Code)
	}
}

func TestGetByIDScheduledHidden(t *testing.T) {
	future := fixedNowHTTP().Add(time.Hour)
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "u1", false, &future),
	}}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for scheduled", rec.Code)
	}
}

// --- Preview ---

func TestPreviewRequiresAuth(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001/preview", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Preview(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPreviewAuthorOK(t *testing.T) {
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "author-1", true, nil),
	}}
	h := newTestHandler(repo, "author-1", false, true)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001/preview", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreviewAdminOK(t *testing.T) {
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "author-1", true, nil),
	}}
	h := newTestHandler(repo, "admin-1", true, true)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001/preview", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Preview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreviewStrangerForbidden(t *testing.T) {
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "author-1", true, nil),
	}}
	h := newTestHandler(repo, "stranger", false, true)

	req := httptest.NewRequest(http.MethodGet, "/articles/01900000-0000-7000-8000-000000000001/preview", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Preview(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// --- ListByUser ---

func TestListByUserReturnsItems(t *testing.T) {
	past := fixedNowHTTP().Add(-time.Hour)
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "author-1", false, &past),
		newArticle("01900000-0000-7000-8000-000000000002", "author-1", false, &past),
		newArticle("01900000-0000-7000-8000-000000000003", "author-2", false, &past),
	}}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/users/author-1/articles", nil)
	req.SetPathValue("id", "author-1")
	rec := httptest.NewRecorder()
	h.ListByUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp FeedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d, want 2 (author-1 only)", len(resp.Items))
	}
	for _, it := range resp.Items {
		if it.UserID != "author-1" {
			t.Errorf("unexpected user_id %q in feed", it.UserID)
		}
	}
}

// --- ListOwn ---

func TestListOwnRequiresAuth(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodGet, "/users/me/articles", nil)
	rec := httptest.NewRecorder()
	h.ListOwn(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestListOwnIncludesDrafts(t *testing.T) {
	past := fixedNowHTTP().Add(-time.Hour)
	repo := &stubArticleRepo{articles: []Article{
		newArticle("01900000-0000-7000-8000-000000000001", "me", true, nil),    // draft
		newArticle("01900000-0000-7000-8000-000000000002", "me", false, &past), // published
		newArticle("01900000-0000-7000-8000-000000000003", "other", true, nil), // not mine
	}}
	h := newTestHandler(repo, "me", false, true)

	req := httptest.NewRequest(http.MethodGet, "/users/me/articles", nil)
	rec := httptest.NewRecorder()
	h.ListOwn(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp FeedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(resp.Items))
	}
	for _, it := range resp.Items {
		if it.UserID != "me" {
			t.Errorf("unexpected user_id %q", it.UserID)
		}
	}
}

// --- Create ---

func TestCreateRequiresAuth(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(`{"title":"x","is_draft":true}`))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCreateSuccess(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "author-1", false, true)

	body := `{"title":"  Hello  ","teaser":"  Teaser  ","content":"Body","is_draft":true}`
	req := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/articles/") {
		t.Errorf("Location = %q", loc)
	}
	var full FullArticle
	if err := json.Unmarshal(rec.Body.Bytes(), &full); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if full.Title != "Hello" {
		t.Errorf("title not trimmed: %q", full.Title)
	}
	if full.UserID != "author-1" {
		t.Errorf("user_id = %q", full.UserID)
	}
	if !full.IsDraft {
		t.Error("is_draft should be true")
	}
	if len(repo.articles) != 1 {
		t.Fatalf("repository got %d articles, want 1", len(repo.articles))
	}
}

func TestCreateRejectsInvalidBody(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "author-1", false, true)

	for _, body := range []string{
		`{`,
		`{"title":"x","is_draft":true,"extra":"field"}`,
		`{"title":"x","is_draft":true} garbage`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.Create(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%q: status = %d, want 400", body, rec.Code)
		}
	}
	if len(repo.articles) != 0 {
		t.Fatalf("repository received %d articles despite bad requests", len(repo.articles))
	}
}

func TestCreateValidationFails(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "author-1", false, true)

	req := httptest.NewRequest(http.MethodPost, "/articles", strings.NewReader(`{"title":"   ","is_draft":true}`))
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- Update ---

func TestUpdateRequiresAuth(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodPatch, "/articles/01900000-0000-7000-8000-000000000001", strings.NewReader(`{"title":"x","is_draft":true}`))
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestUpdateAsAuthor(t *testing.T) {
	tm := fixedNowHTTP()
	repo := &stubArticleRepo{articles: []Article{{
		ID:        "01900000-0000-7000-8000-000000000001",
		UserID:    "author-1",
		Title:     "Original",
		IsDraft:   true,
		CreatedAt: tm,
		UpdatedAt: tm,
	}}}
	h := newTestHandler(repo, "author-1", false, true)

	body := `{"title":"Updated","teaser":"t","content":"c","is_draft":true}`
	req := httptest.NewRequest(http.MethodPatch, "/articles/01900000-0000-7000-8000-000000000001", strings.NewReader(body))
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var full FullArticle
	_ = json.Unmarshal(rec.Body.Bytes(), &full)
	if full.Title != "Updated" {
		t.Errorf("title = %q", full.Title)
	}
}

func TestUpdateAsAdmin(t *testing.T) {
	tm := fixedNowHTTP()
	repo := &stubArticleRepo{articles: []Article{{
		ID:        "01900000-0000-7000-8000-000000000001",
		UserID:    "author-1",
		Title:     "Original",
		IsDraft:   true,
		CreatedAt: tm,
		UpdatedAt: tm,
	}}}
	h := newTestHandler(repo, "admin-1", true, true)

	body := `{"title":"Admin edit","is_draft":true}`
	req := httptest.NewRequest(http.MethodPatch, "/articles/01900000-0000-7000-8000-000000000001", strings.NewReader(body))
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateAsStrangerForbidden(t *testing.T) {
	tm := fixedNowHTTP()
	repo := &stubArticleRepo{articles: []Article{{
		ID:        "01900000-0000-7000-8000-000000000001",
		UserID:    "author-1",
		Title:     "Original",
		IsDraft:   true,
		CreatedAt: tm,
		UpdatedAt: tm,
	}}}
	h := newTestHandler(repo, "stranger", false, true)

	req := httptest.NewRequest(http.MethodPatch, "/articles/01900000-0000-7000-8000-000000000001", strings.NewReader(`{"title":"x","is_draft":true}`))
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestUpdateNotFound(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "author-1", false, true)

	req := httptest.NewRequest(http.MethodPatch, "/articles/missing", strings.NewReader(`{"title":"x","is_draft":true}`))
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Delete ---

func TestDeleteRequiresAuth(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "", false, false)

	req := httptest.NewRequest(http.MethodDelete, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Delete(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDeleteAsAuthor(t *testing.T) {
	tm := fixedNowHTTP()
	repo := &stubArticleRepo{articles: []Article{{
		ID:        "01900000-0000-7000-8000-000000000001",
		UserID:    "author-1",
		Title:     "Original",
		IsDraft:   true,
		CreatedAt: tm,
		UpdatedAt: tm,
	}}}
	h := newTestHandler(repo, "author-1", false, true)

	req := httptest.NewRequest(http.MethodDelete, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if repo.articles[0].DeletedAt == nil {
		t.Fatal("article not marked deleted")
	}
}

func TestDeleteAsAdmin(t *testing.T) {
	tm := fixedNowHTTP()
	repo := &stubArticleRepo{articles: []Article{{
		ID:        "01900000-0000-7000-8000-000000000001",
		UserID:    "author-1",
		Title:     "Original",
		IsDraft:   true,
		CreatedAt: tm,
		UpdatedAt: tm,
	}}}
	h := newTestHandler(repo, "admin-1", true, true)

	req := httptest.NewRequest(http.MethodDelete, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestDeleteAsStrangerForbidden(t *testing.T) {
	tm := fixedNowHTTP()
	repo := &stubArticleRepo{articles: []Article{{
		ID:        "01900000-0000-7000-8000-000000000001",
		UserID:    "author-1",
		Title:     "Original",
		IsDraft:   true,
		CreatedAt: tm,
		UpdatedAt: tm,
	}}}
	h := newTestHandler(repo, "stranger", false, true)

	req := httptest.NewRequest(http.MethodDelete, "/articles/01900000-0000-7000-8000-000000000001", nil)
	req.SetPathValue("id", "01900000-0000-7000-8000-000000000001")
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if repo.articles[0].DeletedAt != nil {
		t.Fatal("article must not be deleted on forbidden")
	}
}

func TestDeleteNotFound(t *testing.T) {
	repo := &stubArticleRepo{}
	h := newTestHandler(repo, "author-1", false, true)

	req := httptest.NewRequest(http.MethodDelete, "/articles/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.Delete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- helpers ---

func base64Of(s string) string {
	// base64url without padding, matching the handler's encoding.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var out []byte
	data := []byte(s)
	for i := 0; i < len(data); i += 3 {
		var chunk [3]byte
		n := copy(chunk[:], data[i:])
		out = append(out, alphabet[chunk[0]>>2])
		out = append(out, alphabet[(chunk[0]&0x03)<<4|chunk[1]>>4])
		if n > 1 {
			out = append(out, alphabet[(chunk[1]&0x0F)<<2|chunk[2]>>6])
		}
		if n > 2 {
			out = append(out, alphabet[chunk[2]&0x3F])
		}
	}
	return string(out)
}
