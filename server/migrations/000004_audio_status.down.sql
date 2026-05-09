DROP INDEX IF EXISTS stories_audio_status_idx;
ALTER TABLE stories
    DROP COLUMN IF EXISTS audio_failed_at,
    DROP COLUMN IF EXISTS audio_status;
