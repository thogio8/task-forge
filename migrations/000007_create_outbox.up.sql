CREATE TABLE outbox (
    id            BIGSERIAL    PRIMARY KEY,
    event_type    VARCHAR(50)  NOT NULL,
    payload       JSONB        NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    published_at  TIMESTAMPTZ  NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (created_at)
    WHERE published_at IS NULL;