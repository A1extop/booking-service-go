package models

import "time"

const InitiatorSystem = "System"

const (
	CauseCreated               = "created"
	CauseCancellationRequested = "cancellation_requested"
	CauseConfirmed             = "confirmed"
	CauseDenied                = "denied"
	CauseCancellationCompleted = "cancellation_completed"
	CauseCancellationFailed    = "cancellation_failed"
)

// History — запись audit log об изменении статуса бронирования.
type History struct {
	ID             int64
	PreviousStatus string
	Status         string
	BookingID      int64
	Initiator      string
	Cause          string
	CreatedAt      time.Time
}

// NewHistory создаёт запись истории статуса.
// bookingID может быть 0 при создании бронирования — будет проставлен в репозитории.
func NewHistory(
	newStatus BookingStatus,
	previousStatus BookingStatus,
	bookingID int64,
	initiator, cause string,
) (*History, error) {
	if !newStatus.IsValid() {
		return nil, ErrInvalidStatus
	}
	if previousStatus != "" && !previousStatus.IsValid() {
		return nil, ErrInvalidStatus
	}
	if bookingID < 0 {
		return nil, ErrInvalidBookingID
	}
	if initiator == "" {
		return nil, ErrInvalidInitiator
	}
	if cause == "" {
		return nil, ErrInvalidCause
	}

	var previous string
	if previousStatus != "" {
		previous = string(previousStatus)
	}

	return &History{
		Status:         string(newStatus),
		PreviousStatus: previous,
		BookingID:      bookingID,
		Initiator:      initiator,
		Cause:          cause,
	}, nil
}
