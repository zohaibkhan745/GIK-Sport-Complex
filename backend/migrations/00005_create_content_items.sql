-- +goose Up
-- Step 10A: Create announcements and sports achievements content items table (FR-07).
-- Curated editorial lifecycle (draft-to-published) for campus sports notices and match achievements.
-- Interpretation timezone for timestamps: Asia/Karachi (PKT, UTC+5).

CREATE TABLE IF NOT EXISTS giki_sports.content_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind VARCHAR(50) NOT NULL,
    title VARCHAR(255),
    content TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    creator_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    CONSTRAINT fk_content_items_creator FOREIGN KEY (creator_id) REFERENCES giki_sports.users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_content_items_kind_valid CHECK (kind IN ('announcement', 'achievement')),
    CONSTRAINT chk_content_items_status_valid CHECK (status IN ('draft', 'published')),
    CONSTRAINT chk_content_items_published_title CHECK (status != 'published' OR (title IS NOT NULL AND length(trim(title)) > 0)),
    CONSTRAINT chk_content_items_published_content CHECK (status != 'published' OR (content IS NOT NULL AND length(trim(content)) > 0)),
    CONSTRAINT chk_content_items_published_at CHECK (status != 'published' OR (published_at IS NOT NULL)),
    CONSTRAINT chk_content_items_updated_after_created CHECK (updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS idx_content_items_creator_id ON giki_sports.content_items(creator_id);
CREATE INDEX IF NOT EXISTS idx_content_items_kind_status ON giki_sports.content_items(kind, status);
CREATE INDEX IF NOT EXISTS idx_content_items_published_at ON giki_sports.content_items(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_items_created_at ON giki_sports.content_items(created_at DESC);

-- +goose Down
-- Reverse migration for Step 10A: drop content_items table using RESTRICT.
DROP TABLE IF EXISTS giki_sports.content_items RESTRICT;

