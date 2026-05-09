ALTER TABLE stories
    ADD COLUMN IF NOT EXISTS audio_status     VARCHAR(16) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS audio_failed_at  TIMESTAMPTZ NULL;

-- Partial index: only 'pending' rows are interesting for ops dashboards.
-- Worker itself joins via outbox_events, not via this index.
CREATE INDEX IF NOT EXISTS stories_audio_status_idx
    ON stories(audio_status)
    WHERE audio_status = 'pending';
