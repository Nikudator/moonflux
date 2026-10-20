package article

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFullText(t *testing.T) {
	tests := []struct {
		name    string
		teaser  string
		content string
		want    string
	}{
		{
			name:    "teaser and content",
			teaser:  "Introduction",
			content: "Main text",
			want:    "Introduction\nMain text",
		},
		{
			name:    "empty teaser",
			content: "Main text",
			want:    "Main text",
		},
		{
			name:   "empty content",
			teaser: "Introduction",
			want:   "Introduction",
		},
		{
			name: "both empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Article{
				Teaser:  tt.teaser,
				Content: tt.content,
			}

			if got := a.FullText(); got != tt.want {
				t.Errorf("FullText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsPublishedAt(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	deletedAt := now

	tests := []struct {
		name    string
		article Article
		want    bool
	}{
		{
			name: "published",
			article: Article{
				PublishedAt: &past,
			},
			want: true,
		},
		{
			name: "scheduled",
			article: Article{
				PublishedAt: &future,
			},
			want: false,
		},
		{
			name: "draft",
			article: Article{
				IsDraft:     true,
				PublishedAt: &past,
			},
			want: false,
		},
		{
			name: "deleted",
			article: Article{
				PublishedAt: &past,
				DeletedAt:   &deletedAt,
			},
			want: false,
		},
		{
			name:    "missing publication date",
			article: Article{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.article.IsPublishedAt(now); got != tt.want {
				t.Errorf("IsPublishedAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArticleValidateRejectsFieldOverflow(t *testing.T) {
	base := Article{
		ID:      "01900000-0000-7000-8000-000000000001",
		UserID:  "01900000-0000-7000-8000-000000000002",
		Title:   "Title",
		IsDraft: true,
	}

	cases := []struct {
		name    string
		mutate  func(*Article)
		wantErr bool
	}{
		{
			name:    "title at limit",
			mutate:  func(a *Article) { a.Title = strings.Repeat("t", MaxTitleRunes) },
			wantErr: false,
		},
		{
			name:    "title over limit",
			mutate:  func(a *Article) { a.Title = strings.Repeat("t", MaxTitleRunes+1) },
			wantErr: true,
		},
		{
			name:    "Cyrillic title at rune limit",
			mutate:  func(a *Article) { a.Title = strings.Repeat("я", MaxTitleRunes) },
			wantErr: false,
		},
		{
			name:    "teaser over limit",
			mutate:  func(a *Article) { a.Teaser = strings.Repeat("t", MaxTeaserRunes+1) },
			wantErr: true,
		},
		{
			name:    "content over byte limit",
			mutate:  func(a *Article) { a.Content = strings.Repeat("a", MaxContentBytes+1) },
			wantErr: true,
		},
		{
			name:    "blank title",
			mutate:  func(a *Article) { a.Title = "   " },
			wantErr: true,
		},
		{
			name: "non-draft without published_at",
			mutate: func(a *Article) {
				a.IsDraft = false
				a.PublishedAt = nil
			},
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			a := base
			tt.mutate(&a)
			err := a.Validate()
			if tt.wantErr && !errors.Is(err, ErrInvalidArticle) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArticle", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}
