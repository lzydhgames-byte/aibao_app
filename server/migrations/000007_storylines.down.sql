ALTER TABLE stories DROP CONSTRAINT IF EXISTS stories_storyline_fk;
DROP INDEX IF EXISTS storylines_child_status_idx;
DROP TABLE IF EXISTS storylines;
