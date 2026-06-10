-- +goose Up
CREATE TABLE IF NOT EXISTS booking_status_history (
    id              BIGSERIAL    PRIMARY KEY,
    status          VARCHAR(30)  NOT NULL,
    previous_status VARCHAR(30),
    booking_id      BIGINT       NOT NULL REFERENCES bookings(id),
    initiator       VARCHAR(30)  NOT NULL CHECK (initiator = 'System' OR initiator ~ '^[0-9]+$'),
    cause           VARCHAR(64)  NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS booking_status_history;
