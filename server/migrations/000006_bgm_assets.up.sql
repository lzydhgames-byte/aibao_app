CREATE TABLE IF NOT EXISTS bgm_assets (
    id           BIGSERIAL    PRIMARY KEY,
    mood         VARCHAR(20)  NOT NULL,
    filename     VARCHAR(200) NOT NULL UNIQUE,
    object_key   VARCHAR(500) NOT NULL,
    duration_sec INT          NOT NULL DEFAULT 0,
    license      VARCHAR(100) NOT NULL DEFAULT '',
    active       BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS bgm_assets_mood_active_idx ON bgm_assets(mood, active);
