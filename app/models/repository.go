package models

import (
	"context"
	"time"
)

// BookingRepository -- интерфейс репозитория бронирований.
type BookingRepository interface {
	// CreateWithHistory сохраняет бронирование и запись истории в одной транзакции.
	CreateWithHistory(ctx context.Context, booking *Booking, history *History) (int64, error)

	// GetByID возвращает бронирование по ID.
	GetByID(ctx context.Context, id int64) (*Booking, error)

	// UpdateWithHistory обновляет бронирование, историю и (опционально) outbox-сообщение в одной транзакции.
	UpdateWithHistory(ctx context.Context, booking *Booking, history *History, outbox *BookingStatusChangedEvent) error

	// UpdateWithHistoryAndEvent то же, что UpdateWithHistory, плюс запись event_id в processed_events.
	UpdateWithHistoryAndEvent(ctx context.Context, booking *Booking, history *History, eventID string, outbox *BookingStatusChangedEvent) error

	// IsEventProcessed проверяет, было ли событие уже обработано.
	IsEventProcessed(ctx context.Context, eventID string) (bool, error)

	// GetByFilter возвращает список бронирований с пагинацией.
	GetByFilter(ctx context.Context, filter BookingFilter) ([]Booking, int64, error)

	// GetAwaitingConfirmation возвращает бронирования в статусе AwaitsConfirmation
	// с пессимистичной блокировкой (SELECT ... FOR UPDATE SKIP LOCKED).
	GetAwaitingConfirmation(ctx context.Context, limit int) ([]Booking, error)

	// GetStatistics возвращает агрегированную статистику за период [dateFrom, dateToExclusive).
	GetStatistics(ctx context.Context, dateFrom, dateToExclusive time.Time) (*StatisticsData, error)

	// GetStuckCancellation возвращает бронирования в cancellation_pending
	// у которых cancellation_requested_at старше cutoff.
	GetStuckCancellation(ctx context.Context, cutoff time.Time, limit int) (*[]Booking, error)

	// GetHistoryByBookingID возвращает историю статусов бронирования с пагинацией.
	GetHistoryByBookingID(ctx context.Context, bookingID int64, page, size int) ([]History, int64, error)

	GetOutboxMessages(ctx context.Context, maxRetry int64, limit int) ([]BookingEvent, error)

	IncrementOutboxRetry(ctx context.Context, eventID string) error

	DeleteOutboxMessage(ctx context.Context, eventID string) error
}

// BookingFilter содержит параметры фильтрации и пагинации.
type BookingFilter struct {
	UserID     *int64
	ResourceID *int64
	Status     *BookingStatus
	Page       int
	Size       int
}

// NewDefaultFilter создаёт фильтр с пагинацией по умолчанию.
func NewDefaultFilter() BookingFilter {
	return BookingFilter{
		Page: 1,
		Size: 25,
	}
}
