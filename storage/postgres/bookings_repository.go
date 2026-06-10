package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// BookingsRepository реализует models.BookingRepository.
type BookingsRepository struct {
	pool *pgxpool.Pool
}

// NewBookingsRepository создаёт новый экземпляр BookingsRepository.
func NewBookingsRepository(pool *pgxpool.Pool) *BookingsRepository {
	return &BookingsRepository{pool: pool}
}

// CreateWithHistory сохраняет бронирование и запись истории в одной транзакции.
func (r *BookingsRepository) CreateWithHistory(ctx context.Context, booking *models.Booking, history *models.History) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("начало транзакции: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, queryInsertBooking,
		string(booking.Status()),
		booking.UserID(),
		booking.ResourceID(),
		booking.StartDate(),
		booking.EndDate(),
		booking.CreatedAt(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("создание бронирования: %w", err)
	}

	history.BookingID = id
	if err := insertHistory(ctx, tx, history); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("фиксация транзакции: %w", err)
	}

	return id, nil
}

// GetByID возвращает бронирование по ID.
func (r *BookingsRepository) GetByID(ctx context.Context, id int64) (*models.Booking, error) {
	booking, err := r.scanBooking(r.pool.QueryRow(ctx, queryGetBookingByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrBookingNotFound
		}
		return nil, fmt.Errorf("получение бронирования id=%d: %w", id, err)
	}
	return booking, nil
}

// UpdateWithHistory обновляет бронирование и сохраняет запись истории в одной транзакции.
func (r *BookingsRepository) UpdateWithHistory(ctx context.Context, booking *models.Booking, history *models.History) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := updateBooking(ctx, tx, booking); err != nil {
		return err
	}

	history.BookingID = booking.ID()
	if err := insertHistory(ctx, tx, history); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("фиксация транзакции: %w", err)
	}

	return nil
}

// GetByFilter возвращает бронирования с фильтрацией и пагинацией.
func (r *BookingsRepository) GetByFilter(ctx context.Context, filter models.BookingFilter) ([]models.Booking, int64, error) {
	offset := (filter.Page - 1) * filter.Size

	var userID, resourceID *int64
	var status *string
	if filter.UserID != nil {
		userID = filter.UserID
	}
	if filter.ResourceID != nil {
		resourceID = filter.ResourceID
	}
	if filter.Status != nil {
		s := string(*filter.Status)
		status = &s
	}

	var totalCount int64
	err := r.pool.QueryRow(ctx, queryCountBookingsByFilter, userID, resourceID, status).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт бронирований: %w", err)
	}

	rows, err := r.pool.Query(ctx, queryGetBookingsByFilter, userID, resourceID, status, filter.Size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение бронирований по фильтру: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("итерация по строкам: %w", err)
	}

	return bookings, totalCount, nil
}

// GetAwaitingConfirmation возвращает бронирования, ожидающие подтверждения,
// с пессимистичной блокировкой FOR UPDATE SKIP LOCKED.
func (r *BookingsRepository) GetAwaitingConfirmation(ctx context.Context, limit int) ([]models.Booking, error) {
	rows, err := r.pool.Query(ctx, queryGetAwaitingConfirmation, limit)
	if err != nil {
		return nil, fmt.Errorf("получение бронирований для подтверждения: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	return bookings, rows.Err()
}

func (r *BookingsRepository) GetStatistics(ctx context.Context, dateFrom, dateToExclusive time.Time) (*models.StatisticsData, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, queryCountBooking, dateFrom, dateToExclusive).Scan(&count); err != nil {
		return nil, fmt.Errorf("получение количества бронирований за период: %w", err)
	}

	rowsByStatus, err := r.pool.Query(ctx, queryByStatus, dateFrom, dateToExclusive)
	if err != nil {
		return nil, fmt.Errorf("получение разбивки по статусам: %w", err)
	}
	defer rowsByStatus.Close()

	var byStatus []models.StatusCount
	for rowsByStatus.Next() {
		item, err := scanStatusCount(rowsByStatus.Scan)
		if err != nil {
			return nil, fmt.Errorf("сканирование статусов: %w", err)
		}
		byStatus = append(byStatus, *item)
	}
	if err := rowsByStatus.Err(); err != nil {
		return nil, fmt.Errorf("итерация по статусам: %w", err)
	}

	rowsTopResources, err := r.pool.Query(ctx, queryTop5Resources, dateFrom, dateToExclusive)
	if err != nil {
		return nil, fmt.Errorf("получение топ ресурсов: %w", err)
	}
	defer rowsTopResources.Close()

	var topResources []models.ResourceCount
	for rowsTopResources.Next() {
		item, err := scanResourceCount(rowsTopResources.Scan)
		if err != nil {
			return nil, fmt.Errorf("сканирование топ ресурсов: %w", err)
		}
		topResources = append(topResources, *item)
	}
	if err := rowsTopResources.Err(); err != nil {
		return nil, fmt.Errorf("итерация по топ ресурсам: %w", err)
	}

	return &models.StatisticsData{
		TotalCount:   count,
		ByStatus:     byStatus,
		TopResources: topResources,
	}, nil
}

func (r *BookingsRepository) GetStuckCancellation(ctx context.Context, cutoff time.Time, limit int) (*[]models.Booking, error) {
	rows, err := r.pool.Query(ctx, queryGetStuckCancellation, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения бронирований %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("сканирование бронирований: %w", err)
		}
		bookings = append(bookings, *booking)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &bookings, nil
}

func (r *BookingsRepository) GetHistoryByBookingID(ctx context.Context, bookingID int64, page, size int) ([]models.History, int64, error) {
	var totalCount int64
	if err := r.pool.QueryRow(ctx, queryCountHistoryByBookingID, bookingID).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("подсчёт истории бронирования: %w", err)
	}

	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 25
	}
	offset := (page - 1) * size

	rows, err := r.pool.Query(ctx, queryGetHistoryByBookingID, bookingID, size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение истории бронирования: %w", err)
	}
	defer rows.Close()

	var histories []models.History
	for rows.Next() {
		history, err := scanHistory(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("сканирование истории бронирования: %w", err)
		}
		histories = append(histories, *history)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return histories, totalCount, nil
}

func updateBooking(ctx context.Context, tx pgx.Tx, booking *models.Booking) error {
	var previousStatus *string
	if ps := booking.PreviousStatus(); ps != "" {
		s := string(ps)
		previousStatus = &s
	}

	var cancellationRequestedAt *time.Time
	if t := booking.CancellationRequestedAt(); !t.IsZero() {
		cancellationRequestedAt = &t
	}

	tag, err := tx.Exec(ctx, queryUpdateBookingStatus,
		string(booking.Status()),
		previousStatus,
		cancellationRequestedAt,
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования id=%d: %w", booking.ID(), err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrBookingNotFound
	}
	return nil
}

func insertHistory(ctx context.Context, tx pgx.Tx, history *models.History) error {
	var previousStatus *string
	if history.PreviousStatus != "" {
		s := history.PreviousStatus
		previousStatus = &s
	}

	_, err := tx.Exec(ctx, queryInsertBookingStatusHistory,
		history.Status,
		previousStatus,
		history.BookingID,
		history.Initiator,
		history.Cause,
	)
	if err != nil {
		return fmt.Errorf("добавление истории: %w", err)
	}
	return nil
}

func (r *BookingsRepository) scanBooking(row pgx.Row) (*models.Booking, error) {
	return scanBookingFields(row.Scan)
}

func (r *BookingsRepository) scanBookingFromRows(rows pgx.Rows) (*models.Booking, error) {
	return scanBookingFields(rows.Scan)
}

func scanHistory(scan func(dest ...any) error) (*models.History, error) {
	var (
		id         int64
		status     string
		prevStatus *string
		bookingID  int64
		initiator  string
		cause      string
		createdAt  time.Time
	)
	if err := scan(&id, &status, &prevStatus, &bookingID, &initiator, &cause, &createdAt); err != nil {
		return nil, err
	}

	var previous string
	if prevStatus != nil {
		previous = *prevStatus
	}

	return &models.History{
		ID:             id,
		Status:         status,
		PreviousStatus: previous,
		BookingID:      bookingID,
		Initiator:      initiator,
		Cause:          cause,
		CreatedAt:      createdAt,
	}, nil
}

func scanStatusCount(scan func(dest ...any) error) (*models.StatusCount, error) {
	var (
		status string
		count  int64
	)

	if err := scan(&status, &count); err != nil {
		return nil, err
	}

	return &models.StatusCount{
		Status: models.BookingStatus(status),
		Count:  count,
	}, nil
}

func scanResourceCount(scan func(dest ...any) error) (*models.ResourceCount, error) {
	var (
		resourceID int64
		count      int64
	)

	if err := scan(&resourceID, &count); err != nil {
		return nil, err
	}

	return &models.ResourceCount{
		ResourceID: resourceID,
		Count:      count,
	}, nil
}

func scanBookingFields(scan func(dest ...any) error) (*models.Booking, error) {
	var (
		id                      int64
		status                  string
		userID                  int64
		resourceID              int64
		previousStatus          *string
		cancellationRequestedAt *time.Time
		startDate               time.Time
		endDate                 time.Time
		createdAt               time.Time
	)

	if err := scan(
		&id, &status, &userID, &resourceID,
		&previousStatus, &cancellationRequestedAt,
		&startDate, &endDate, &createdAt,
	); err != nil {
		return nil, err
	}

	var prev models.BookingStatus
	if previousStatus != nil {
		prev = models.BookingStatus(*previousStatus)
	}

	var cancelledAt time.Time
	if cancellationRequestedAt != nil {
		cancelledAt = *cancellationRequestedAt
	}

	return models.RestoreBooking(
		id, models.BookingStatus(status), userID, resourceID,
		prev, cancelledAt,
		startDate, endDate, createdAt,
	), nil
}
