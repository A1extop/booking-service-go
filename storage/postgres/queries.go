package postgres

const (
	queryInsertBooking = `
		INSERT INTO bookings (status, user_id, resource_id, start_date, end_date, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	queryGetBookingByID = `
		SELECT id, status, user_id, resource_id, previous_status, cancellation_requested_at,
		       start_date, end_date, created_at
		FROM bookings
		WHERE id = $1`

	queryUpdateBookingStatus = `
		UPDATE bookings
		SET status = $1,
		    previous_status = $2,
		    cancellation_requested_at = $3
		WHERE id = $4`

	queryGetBookingsByFilter = `
		SELECT id, status, user_id, resource_id, previous_status, cancellation_requested_at,
		       start_date, end_date, created_at
		FROM bookings
		WHERE ($1::BIGINT IS NULL OR user_id = $1)
		  AND ($2::BIGINT IS NULL OR resource_id = $2)
		  AND ($3::VARCHAR IS NULL OR status = $3)
		ORDER BY id DESC
		LIMIT $4 OFFSET $5`

	queryCountBookingsByFilter = `
		SELECT COUNT(*)
		FROM bookings
		WHERE ($1::BIGINT IS NULL OR user_id = $1)
		  AND ($2::BIGINT IS NULL OR resource_id = $2)
		  AND ($3::VARCHAR IS NULL OR status = $3)`

	queryGetAwaitingConfirmation = `
		SELECT id, status, user_id, resource_id, previous_status, cancellation_requested_at,
		       start_date, end_date, created_at
		FROM bookings
		WHERE status = 'awaits_confirmation'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	queryCountBooking = `
	SELECT COUNT(*)
	FROM bookings
	WHERE created_at >= $1
  	AND created_at < $2`

	queryByStatus = `
	SELECT status, COUNT(*) AS count
	FROM bookings
	WHERE created_at >= $1
	  AND created_at < $2
	GROUP BY status
	ORDER BY status`

	queryTop5Resources = `
	SELECT resource_id, COUNT(*) AS count
	FROM bookings
	WHERE created_at >= $1
	  AND created_at < $2
	GROUP BY resource_id
	ORDER BY count DESC, resource_id ASC
	LIMIT 5`

	queryGetStuckCancellation = `
	SELECT id, status, user_id, resource_id, previous_status, cancellation_requested_at,
	       start_date, end_date, created_at
	FROM bookings
	WHERE status = 'cancellation_pending'
	  AND cancellation_requested_at IS NOT NULL
	  AND cancellation_requested_at < $1
	ORDER BY cancellation_requested_at ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED`

	queryCountHistoryByBookingID = `
	SELECT COUNT(*)
	FROM booking_status_history
	WHERE booking_id = $1`

	queryGetHistoryByBookingID = `
	SELECT id, status, previous_status, booking_id, initiator, cause, created_at
	FROM booking_status_history
	WHERE booking_id = $1
	ORDER BY created_at DESC, id DESC
	LIMIT $2 OFFSET $3`

	queryInsertBookingStatusHistory = `
	INSERT INTO booking_status_history (status, previous_status, booking_id, initiator, cause)
	VALUES ($1, $2, $3, $4, $5)`
)
