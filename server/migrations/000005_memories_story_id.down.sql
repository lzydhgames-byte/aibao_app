DROP INDEX IF EXISTS memories_child_story_summary_idx;
ALTER TABLE memories
    DROP COLUMN IF EXISTS story_id;
