-- +goose Up
CREATE INDEX IF NOT EXISTS idx_bookings_created_at ON bookings (created_at);

CREATE INDEX IF NOT EXISTS idx_bookings_status_created_at ON bookings (status, created_at);

CREATE INDEX IF NOT EXISTS idx_bookings_status_cancellation_requested_at
    ON bookings (status, cancellation_requested_at);

CREATE INDEX IF NOT EXISTS idx_booking_status_history_booking_created
    ON booking_status_history (booking_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_outbox_messages_retry_event_id ON outbox_messages (retry, event_id);

-- +goose Down
DROP INDEX IF EXISTS idx_outbox_messages_retry_event_id;
DROP INDEX IF EXISTS idx_booking_status_history_booking_created;
DROP INDEX IF EXISTS idx_bookings_status_cancellation_requested_at;
DROP INDEX IF EXISTS idx_bookings_status_created_at;
DROP INDEX IF EXISTS idx_bookings_created_at;
