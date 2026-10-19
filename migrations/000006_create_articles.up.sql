CREATE TABLE articles (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),

    title TEXT NOT NULL,
    teaser TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',

    is_draft BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT articles_title_not_empty
        CHECK (BTRIM(title) <> ''),

    CONSTRAINT articles_publication_date_required
        CHECK (is_draft OR published_at IS NOT NULL)
);

-- Public feed: newest published articles first.
CREATE INDEX idx_articles_public_feed
    ON articles (published_at DESC, id DESC)
    WHERE is_draft = FALSE
      AND deleted_at IS NULL;

-- Published articles by author.
CREATE INDEX idx_articles_user_public_feed
    ON articles (user_id, published_at DESC, id DESC)
    WHERE is_draft = FALSE
      AND deleted_at IS NULL;

-- Author's personal feed, including drafts and scheduled articles.
CREATE INDEX idx_articles_user_personal_feed
    ON articles (user_id, updated_at DESC, id DESC)
    WHERE deleted_at IS NULL;
