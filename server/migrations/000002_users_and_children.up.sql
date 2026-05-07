CREATE TABLE IF NOT EXISTS users (
    id                 BIGSERIAL PRIMARY KEY,
    phone_hash         VARCHAR(64)  NOT NULL UNIQUE,
    phone_encrypted    BYTEA        NOT NULL,
    nickname           VARCHAR(50)  NOT NULL,
    subscription_tier  VARCHAR(20)  NOT NULL DEFAULT 'free',
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS children (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname    VARCHAR(50)  NOT NULL,
    gender      VARCHAR(10)  NOT NULL,
    birthday    DATE         NOT NULL,
    profile     JSONB        NOT NULL DEFAULT '{}'::JSONB,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT children_user_unique UNIQUE (user_id)
);
