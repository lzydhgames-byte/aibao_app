CREATE TABLE IF NOT EXISTS storylines (
    id                  BIGSERIAL PRIMARY KEY,
    child_id            BIGINT NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    title               VARCHAR(200) NOT NULL DEFAULT '',
    status              VARCHAR(20)  NOT NULL DEFAULT 'active',
    next_episode_hint   VARCHAR(200) NOT NULL DEFAULT '',
    episode_count       INT          NOT NULL DEFAULT 0,
    last_episode_at     TIMESTAMPTZ  NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS storylines_child_status_idx ON storylines(child_id, status, last_episode_at DESC);
ALTER TABLE stories ADD CONSTRAINT stories_storyline_fk FOREIGN KEY (storyline_id) REFERENCES storylines(id) ON DELETE SET NULL;
