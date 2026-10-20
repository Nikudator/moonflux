package article

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"moonflux/internal/platform/uuid"
)

const (
	defaultLimit = 20
	// maxLimit is maxFeedLimit - 1: the handler always fetches one extra item
	// to detect whether another page exists, so a full page requires
	// limit + 1 <= maxFeedLimit.
	maxLimit = maxFeedLimit - 1
)

// Authenticator resolves the caller from the request context.
// ok reports whether the request carries valid claims.
type Authenticator func(r *http.Request) (userID string, isAdmin bool, ok bool)

// HTTPHandler serves article routes.
type HTTPHandler struct {
	service      *Service
	authenticate Authenticator
}

func NewHTTPHandler(service *Service, auth Authenticator) *HTTPHandler {
	return &HTTPHandler{service: service, authenticate: auth}
}

// --- DTOs ---

// FeedItem is the lightweight representation used inside a feed.
type FeedItem struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Title       string     `json:"title"`
	Teaser      string     `json:"teaser"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (a Article) ToFeedItem() FeedItem {
	return FeedItem{
		ID:          a.ID,
		UserID:      a.UserID,
		Title:       a.Title,
		Teaser:      a.Teaser,
		PublishedAt: a.PublishedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

// FullArticle is the complete representation with content.
type FullArticle struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Title       string     `json:"title"`
	Teaser      string     `json:"teaser"`
	Content     string     `json:"content"`
	IsDraft     bool       `json:"is_draft"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (a Article) ToFull() FullArticle {
	return FullArticle{
		ID:          a.ID,
		UserID:      a.UserID,
		Title:       a.Title,
		Teaser:      a.Teaser,
		Content:     a.Content,
		IsDraft:     a.IsDraft,
		PublishedAt: a.PublishedAt,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

// FeedResponse is the envelope for paginated feeds.
type FeedResponse struct {
	Items      []FeedItem `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// --- Cursor codec ---

type cursorKind string

const (
	cursorByPublished cursorKind = "p"
	cursorByUpdated   cursorKind = "u"
)

type cursorPayload struct {
	Kind cursorKind `json:"k"`
	Time int64      `json:"t"` // UnixNano
	ID   string     `json:"i"`
}

func encodeCursor(kind cursorKind, t time.Time, id string) string {
	b, _ := json.Marshal(cursorPayload{Kind: kind, Time: t.UnixNano(), ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw string, expected cursorKind) (*FeedCursor, error) {
	if raw == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("invalid cursor encoding")
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, errors.New("invalid cursor payload")
	}
	if p.Kind != expected {
		return nil, errors.New("cursor kind mismatch")
	}
	if p.Time <= 0 || p.ID == "" {
		return nil, errors.New("cursor is incomplete")
	}
	if _, err := uuid.Parse(p.ID); err != nil {
		return nil, errors.New("cursor id is not a uuid")
	}
	t := time.Unix(0, p.Time).UTC()
	c := &FeedCursor{ID: p.ID}
	switch expected {
	case cursorByPublished:
		c.PublishedAt = t
	case cursorByUpdated:
		c.UpdatedAt = t
	}
	return c, nil
}

// --- Request parsing ---

func parseLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errors.New("invalid limit")
	}
	if n > maxLimit {
		n = maxLimit
	}
	return n, nil
}

// --- Handlers ---

// ListFeed handles GET /articles.
func (h *HTTPHandler) ListFeed(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		http.Error(w, "invalid limit", http.StatusBadRequest)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"), cursorByPublished)
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}

	items, err := h.service.ListPublic(r.Context(), FeedFilter{Limit: limit + 1, Cursor: cursor})
	if err != nil {
		http.Error(w, "feed unavailable", http.StatusInternalServerError)
		return
	}
	respondWithFeed(w, items, limit, cursorByPublished)
}

// GetByID handles GET /articles/{id}.
// Returns only publicly available articles; drafts and scheduled items are 404.
func (h *HTTPHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "invalid article id", http.StatusBadRequest)
		return
	}

	a, err := h.service.GetPublished(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, "article not found", http.StatusNotFound)
		case errors.Is(err, ErrInvalidArticle):
			http.Error(w, "invalid article id", http.StatusBadRequest)
		default:
			http.Error(w, "article unavailable", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, a.ToFull())
}

// Preview handles GET /articles/{id}/preview.
// Requires authentication: author or admin only.
func (h *HTTPHandler) Preview(w http.ResponseWriter, r *http.Request) {
	actorID, isAdmin, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "invalid article id", http.StatusBadRequest)
		return
	}

	a, err := h.service.GetForAuthor(r.Context(), actorID, isAdmin, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, "article not found", http.StatusNotFound)
		case errors.Is(err, ErrForbidden):
			http.Error(w, "forbidden", http.StatusForbidden)
		case errors.Is(err, ErrInvalidArticle):
			http.Error(w, "invalid article id", http.StatusBadRequest)
		default:
			http.Error(w, "article unavailable", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, a.ToFull())
}

// ListByUser handles GET /users/{id}/articles.
// Returns only published articles of that user; drafts are hidden.
func (h *HTTPHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if strings.TrimSpace(userID) == "" {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		http.Error(w, "invalid limit", http.StatusBadRequest)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"), cursorByPublished)
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}

	items, err := h.service.ListByAuthorPublic(r.Context(), userID, FeedFilter{Limit: limit + 1, Cursor: cursor})
	if err != nil {
		http.Error(w, "feed unavailable", http.StatusInternalServerError)
		return
	}
	respondWithFeed(w, items, limit, cursorByPublished)
}

// ListOwn handles GET /users/me/articles.
// Requires authentication. Returns all own articles, including drafts.
func (h *HTTPHandler) ListOwn(w http.ResponseWriter, r *http.Request) {
	actorID, _, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		http.Error(w, "invalid limit", http.StatusBadRequest)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"), cursorByUpdated)
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}

	items, err := h.service.ListOwn(r.Context(), actorID, FeedFilter{Limit: limit + 1, Cursor: cursor})
	if err != nil {
		http.Error(w, "feed unavailable", http.StatusInternalServerError)
		return
	}
	respondWithFeed(w, items, limit, cursorByUpdated)
}

// --- Create / Update / Delete (authenticated) ---

// maxArticleBodyBytes bounds the request body for POST/PATCH /articles.
// MaxContentBytes (1 MiB) is the size limit for the content field itself;
// the extra 1 MiB covers JSON escaping overhead (e.g. newlines become \\n).
const maxArticleBodyBytes = 2 * MaxContentBytes

type articleRequest struct {
	Title       string     `json:"title"`
	Teaser      string     `json:"teaser"`
	Content     string     `json:"content"`
	IsDraft     bool       `json:"is_draft"`
	PublishedAt *time.Time `json:"published_at"`
}

// parseArticleInput reads and validates the JSON envelope for POST and PATCH.
// Unknown fields and trailing JSON are rejected to keep the contract strict.
func parseArticleInput(w http.ResponseWriter, r *http.Request) (ArticleInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxArticleBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req articleRequest
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return ArticleInput{}, false
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "request must contain a single JSON object", http.StatusBadRequest)
		return ArticleInput{}, false
	}

	return ArticleInput{
		Title:       req.Title,
		Teaser:      req.Teaser,
		Content:     req.Content,
		IsDraft:     req.IsDraft,
		PublishedAt: req.PublishedAt,
	}, true
}

// Create handles POST /articles.
func (h *HTTPHandler) Create(w http.ResponseWriter, r *http.Request) {
	actorID, _, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	input, ok := parseArticleInput(w, r)
	if !ok {
		return
	}

	a, err := h.service.Create(r.Context(), actorID, input)
	if err != nil {
		if errors.Is(err, ErrInvalidArticle) {
			http.Error(w, "invalid article", http.StatusBadRequest)
			return
		}
		http.Error(w, "article creation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", "/articles/"+a.ID)
	writeJSON(w, http.StatusCreated, a.ToFull())
}

// Update handles PATCH /articles/{id}.
//
// Semantics: full replacement of editable fields (not JSON merge-patch).
// Fields omitted from the body reset to zero values:
//
//	title="", teaser="", content="", is_draft=false, published_at=null.
//
// A non-draft with published_at=null is published at the service's "now".
func (h *HTTPHandler) Update(w http.ResponseWriter, r *http.Request) {
	actorID, isAdmin, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "invalid article id", http.StatusBadRequest)
		return
	}

	input, ok := parseArticleInput(w, r)
	if !ok {
		return
	}

	a, err := h.service.Update(r.Context(), actorID, isAdmin, id, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, "article not found", http.StatusNotFound)
		case errors.Is(err, ErrForbidden):
			http.Error(w, "forbidden", http.StatusForbidden)
		case errors.Is(err, ErrInvalidArticle):
			http.Error(w, "invalid article", http.StatusBadRequest)
		default:
			http.Error(w, "article update failed", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusOK, a.ToFull())
}

// Delete handles DELETE /articles/{id}. Soft delete.
func (h *HTTPHandler) Delete(w http.ResponseWriter, r *http.Request) {
	actorID, isAdmin, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "invalid article id", http.StatusBadRequest)
		return
	}

	err := h.service.Delete(r.Context(), actorID, isAdmin, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, "article not found", http.StatusNotFound)
		case errors.Is(err, ErrForbidden):
			http.Error(w, "forbidden", http.StatusForbidden)
		case errors.Is(err, ErrInvalidArticle):
			http.Error(w, "invalid article id", http.StatusBadRequest)
		default:
			http.Error(w, "article deletion failed", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func respondWithFeed(w http.ResponseWriter, items []Article, limit int, kind cursorKind) {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	resp := FeedResponse{Items: make([]FeedItem, 0, len(items))}
	for _, a := range items {
		resp.Items = append(resp.Items, a.ToFeedItem())
	}

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		switch kind {
		case cursorByPublished:
			if last.PublishedAt != nil {
				resp.NextCursor = encodeCursor(kind, *last.PublishedAt, last.ID)
			}
		case cursorByUpdated:
			resp.NextCursor = encodeCursor(kind, last.UpdatedAt, last.ID)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
