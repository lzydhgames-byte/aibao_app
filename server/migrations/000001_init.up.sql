CREATE TABLE IF NOT EXISTS infra_check (
    id          BIGSERIAL PRIMARY KEY,
    note        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO infra_check (note) VALUES ('plan-01-bootstrap');
