CREATE TABLE outline_events (
    id                     BIGSERIAL PRIMARY KEY,
    occurred_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    outline_id             VARCHAR(64) NOT NULL,
    outline_group_id       VARCHAR(64) NOT NULL,
    user_id                BIGINT NOT NULL,
    child_id_hash          VARCHAR(32) NOT NULL,
    outcome                VARCHAR(16) NOT NULL CHECK (outcome IN ('pending','accepted','refreshed','expired')),
    outline_prompt_version VARCHAR(32),
    duration_min           INTEGER,
    trace_id               VARCHAR(64)
);

CREATE INDEX idx_outline_events_outline_id ON outline_events(outline_id);
CREATE INDEX idx_outline_events_group ON outline_events(outline_group_id);
CREATE INDEX idx_outline_events_user_day ON outline_events(user_id, occurred_at);

-- Plan 11A §5.5: append-only event stream. "最新生命周期" via:
--   SELECT DISTINCT ON (outline_id) * FROM outline_events
--   ORDER BY outline_id, occurred_at DESC, id DESC;
-- expired 也是追加新行，不 UPDATE。
COMMENT ON TABLE outline_events IS 'Plan 11A append-only event stream for outline lifecycle';
