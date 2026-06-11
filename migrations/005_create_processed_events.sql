-- +goose Up
CREATE TABLE IF NOT EXISTS outbox_messages (
    event_id VARCHAR(36) PRIMARY KEY,
    message JSONB NOT NULL,
    retry INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_outbox_messages_retry ON outbox_messages (retry);

-- +goose Down
DROP TABLE IF EXISTS outbox_messages;
