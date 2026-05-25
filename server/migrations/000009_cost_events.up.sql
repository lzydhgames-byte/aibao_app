CREATE TABLE cost_events (
    id                     BIGSERIAL PRIMARY KEY,
    event_id               VARCHAR(96) NOT NULL UNIQUE,
    occurred_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id                BIGINT,
    child_id_hash          VARCHAR(32),
    purpose                VARCHAR(32) NOT NULL,
    provider               VARCHAR(32) NOT NULL,
    model                  VARCHAR(64),
    billing_mode           VARCHAR(32) NOT NULL DEFAULT 'standard',
    tokens_in              INTEGER,
    tokens_out             INTEGER,
    tokens_cached          INTEGER,
    chars                  INTEGER,
    bytes                  BIGINT,
    audio_seconds          NUMERIC(8, 2),
    cost_yuan              NUMERIC(12, 6) NOT NULL,
    currency               VARCHAR(8) NOT NULL DEFAULT 'CNY',
    price_version          VARCHAR(32) NOT NULL,
    unit_price_snapshot    JSONB,
    outcome                VARCHAR(16) NOT NULL CHECK (outcome IN ('ok','fallback','fail')),
    duration_ms            INTEGER,
    story_id               BIGINT,
    outline_id             VARCHAR(64),
    outline_group_id       VARCHAR(64),
    outline_prompt_version VARCHAR(32),
    trace_id               VARCHAR(64)
);

CREATE INDEX idx_cost_events_occurred ON cost_events(occurred_at);
CREATE INDEX idx_cost_events_user_day ON cost_events(user_id, occurred_at);
CREATE INDEX idx_cost_events_purpose ON cost_events(purpose, occurred_at);
CREATE INDEX idx_cost_events_outline ON cost_events(outline_id) WHERE outline_id IS NOT NULL;
CREATE INDEX idx_cost_events_outline_group ON cost_events(outline_group_id) WHERE outline_group_id IS NOT NULL;

COMMENT ON TABLE cost_events IS 'Plan 11B fact-source for cost; Prometheus is approximate observation';
COMMENT ON COLUMN cost_events.event_id IS 'Idempotency key: {trace_id}:{purpose}:{stage}:{attempt}';
COMMENT ON COLUMN cost_events.outcome IS 'Call result (ok/fallback/fail); NOT lifecycle — see outline_events.outcome for lifecycle';
