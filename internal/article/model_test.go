package article

import (
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
