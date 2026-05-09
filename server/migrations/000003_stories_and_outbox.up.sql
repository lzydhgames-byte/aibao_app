CREATE TABLE IF NOT EXISTS stories (
    id                     BIGSERIAL PRIMARY KEY,
    child_id               BIGINT NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    title                  VARCHAR(200) NOT NULL DEFAULT '',
    text_content           TEXT NOT NULL,
    audio_object_key       VARCHAR(500) NOT NULL DEFAULT '',
    audio_format           VARCHAR(10) NOT NULL DEFAULT '',
    audio_size_bytes       BIGINT NOT NULL DEFAULT 0,
    audio_duration_seconds INT NOT NULL DEFAULT 0,
    duration_minutes       INT NOT NULL,
    style                  VARCHAR(20) NOT NULL,
    topic                  VARCHAR(50) NOT NULL DEFAULT '',
    storyline_id           BIGINT,
    episode_no             INT,
    has_bgm                BOOLEAN NOT NULL DEFAULT TRUE,
    prompt_version         VARCHAR(10) NOT NULL DEFAULT 'v1',
    llm_model              VARCHAR(50) NOT NULL DEFAULT '',
    llm_input_tokens       INT NOT NULL DEFAULT 0,
    llm_output_tokens      INT NOT NULL DEFAULT 0,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS stories_child_created_idx ON stories(child_id, created_at DESC);

CREATE TABLE IF NOT EXISTS story_elements (
    id            BIGSERIAL PRIMARY KEY,
    story_id      BIGINT NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    element_type  VARCHAR(20) NOT NULL,
    name          VARCHAR(100) NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    recall_weight DOUBLE PRECISION NOT NULL DEFAULT 1.0
);
CREATE INDEX IF NOT EXISTS story_elements_story_type_idx ON story_elements(story_id, element_type);

CREATE TABLE IF NOT EXISTS memories (
    id          BIGSERIAL PRIMARY KEY,
    child_id    BIGINT NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    memory_type VARCHAR(30) NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}'::JSONB,
    weight      DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS memories_child_type_created_idx ON memories(child_id, memory_type, created_at DESC);

CREATE TABLE IF NOT EXISTS outbox_events (
    id              BIGSERIAL PRIMARY KEY,
    event_type      VARCHAR(50) NOT NULL,
    aggregate_id    BIGINT,
    payload         JSONB NOT NULL DEFAULT '{}'::JSONB,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts        INT NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS outbox_status_next_idx ON outbox_events(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS outbox_type_status_idx ON outbox_events(event_type, status);
CREATE INDEX IF NOT EXISTS outbox_aggregate_idx ON outbox_events(aggregate_id);
