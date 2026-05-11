ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS story_id BIGINT NULL REFERENCES stories(id) ON DELETE SET NULL;

-- Partial index for the hot read path: latest story_summary per child.
CREATE INDEX IF NOT EXISTS memories_child_story_summary_idx
    ON memories(child_id, created_at DESC)
    WHERE memory_type = 'story_summary';
