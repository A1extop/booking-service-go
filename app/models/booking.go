package models

import "time"

// BookingStatus представляет статус бронирования.
type BookingStatus string

const (
	BookingStatusAwaitsConfirmation  BookingStatus = "awaits_confirmation"
	BookingStatusConfirmed           BookingStatus = "confirmed"
	BookingStatusCancelled           BookingStatus = "cancelled"
	BookingStatusCancellationPending BookingStatus = "cancellation_pending"
)

// IsValid проверяет, что статус принадлежит допустимому множеству.
func (s BookingStatus) IsValid() bool {
	switch s {
	case BookingStatusAwaitsConfirmation, BookingStatusConfirmed, BookingStatusCancelled, BookingStatusCancellationPending:
		return true
	default:
		return false
	}
}

// Booking -- доменная сущность бронирования.
// Поля неэкспортируемые для обеспечения инкапсуляции.
type Booking struct {
	id                      int64
	status                  BookingStatus
	userID                  int64
	resourceID              int64
	previousStatus          BookingStatus
	cancellationRequestedAt time.Time
	startDate               time.Time
	endDate                 time.Time
	createdAt               time.Time
}

func (b *Booking) ID() int64                          { return b.id }
func (b *Booking) Status() BookingStatus              { return b.status }
func (b *Booking) UserID() int64                      { return b.userID }
func (b *Booking) ResourceID() int64                  { return b.resourceID }
func (b *Booking) StartDate() time.Time               { return b.startDate }
func (b *Booking) EndDate() time.Time                 { return b.endDate }
func (b *Booking) CreatedAt() time.Time               { return b.createdAt }
func (b *Booking) PreviousStatus() BookingStatus      { return b.previousStatus }
func (b *Booking) CancellationRequestedAt() time.Time { return b.cancellationRequestedAt }

// NewBooking создаёт новое бронирование в статусе AwaitsConfirmation.
func NewBooking(userID, resourceID int64, startDate, endDate time.Time) (*Booking, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if resourceID <= 0 {
		return nil, ErrInvalidResourceID
	}
	if startDate.IsZero() || endDate.IsZero() {
		return nil, ErrInvalidDateRange
	}
	if !endDate.After(startDate) {
		return nil, ErrEndDateBeforeStartDate
	}

	return &Booking{
		status:     BookingStatusAwaitsConfirmation,
		userID:     userID,
		resourceID: resourceID,
		startDate:  startDate,
		endDate:    endDate,
		createdAt:  time.Now(),
	}, nil
}

// Confirm подтверждает бронирование.
// Допустимый переход: AwaitsConfirmation -> Confirmed.
func (b *Booking) Confirm() error {
	if b.status != BookingStatusAwaitsConfirmation {
		return ErrInvalidStatusTransition
	}
	b.status = BookingStatusConfirmed
	return nil
}

// Cancel отменяет бронирование.
// Допустимые переходы:
//   - AwaitsConfirmation -> Cancelled
//   - Confirmed -> Cancelled (только если StartDate > today)
func (b *Booking) Cancel(today time.Time) error {
	switch b.status {
	case BookingStatusAwaitsConfirmation:
		b.status = BookingStatusCancelled
		return nil
	case BookingStatusConfirmed:
		if !b.startDate.After(today) {
			return ErrCannotCancelPastBooking
		}
		b.status = BookingStatusCancelled
		return nil
	case BookingStatusCancelled:
		return ErrInvalidStatusTransition
	default:
		return ErrInvalidStatusTransition
	}
}

func (b *Booking) BeginCancel(today time.Time) error {
	switch b.status {
	case BookingStatusAwaitsConfirmation:
		b.previousStatus = b.status
		b.status = BookingStatusCancellationPending
		b.cancellationRequestedAt = time.Now()
		return nil
	case BookingStatusConfirmed:
		if !b.startDate.After(today) {
			return ErrCannotCancelPastBooking
		}
		b.previousStatus = b.status
		b.status = BookingStatusCancellationPending
		b.cancellationRequestedAt = time.Now()
		return nil
	default:
		return ErrInvalidStatusTransition
	}
}

func (b *Booking) CompleteCancel() error {
	if b.status != BookingStatusCancellationPending {
		return ErrInvalidStatusTransition
	}
	b.status = BookingStatusCancelled
	b.previousStatus = ""
	b.cancellationRequestedAt = time.Time{}
	return nil
}

func (b *Booking) RollbackCancel() error {
	if b.status != BookingStatusCancellationPending {
		return ErrInvalidStatusTransition
	}
	if b.previousStatus != BookingStatusAwaitsConfirmation && b.previousStatus != BookingStatusConfirmed {
		return ErrInvalidStatusTransition
	}
	b.status = b.previousStatus
	b.previousStatus = ""
	b.cancellationRequestedAt = time.Time{}
	return nil
}

// RestoreBooking восстанавливает Booking из данных хранилища.
// Используется только в слое storage для маппинга строк БД на доменный объект.
func RestoreBooking(
	id int64,
	status BookingStatus,
	userID, resourceID int64,
	previousStatus BookingStatus,
	cancellationRequestedAt time.Time,
	startDate, endDate, createdAt time.Time,
) *Booking {
	return &Booking{
		id:                      id,
		status:                  status,
		userID:                  userID,
		resourceID:              resourceID,
		previousStatus:          previousStatus,
		cancellationRequestedAt: cancellationRequestedAt,
		startDate:               startDate,
		endDate:                 endDate,
		createdAt:               createdAt,
	}
}
