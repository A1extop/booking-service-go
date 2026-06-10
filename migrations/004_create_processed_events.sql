-- +goose Up
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     VARCHAR(36)  PRIMARY KEY,
    processed_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS processed_events;
