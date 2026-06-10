package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/messaging"
	"booking-service/app/models"
)

// BookingsService обрабатывает команды (изменение состояния) для бронирований.
type BookingsService struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	logger    *zap.Logger
}

// NewBookingsService создаёт новый BookingsService.
func NewBookingsService(repo models.BookingRepository, publisher *messaging.Publisher, logger *zap.Logger) *BookingsService {
	return &BookingsService{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
	}
}

func (s *BookingsService) persistStatusChange(
	ctx context.Context,
	booking *models.Booking,
	previousStatus models.BookingStatus,
	initiator, cause string,
) error {
	history, err := models.NewHistory(booking.Status(), previousStatus, booking.ID(), initiator, cause)
	if err != nil {
		return err
	}
	return s.repo.UpdateWithHistory(ctx, booking, history)
}

func userInitiator(userID int64) string {
	return strconv.FormatInt(userID, 10)
}

// Create создаёт новое бронирование.
func (s *BookingsService) Create(ctx context.Context, req dto.CreateBookingRequest) (int64, error) {
	startDate, err := time.Parse(dto.DateFormat, req.StartDate)
	if err != nil {
		return 0, fmt.Errorf("некорректный формат startDate: %w", err)
	}

	endDate, err := time.Parse(dto.DateFormat, req.EndDate)
	if err != nil {
		return 0, fmt.Errorf("некорректный формат endDate: %w", err)
	}

	booking, err := models.NewBooking(req.UserID, req.ResourceID, startDate, endDate)
	if err != nil {
		return 0, err
	}

	history, err := models.NewHistory(
		booking.Status(),
		"",
		0,
		userInitiator(req.UserID),
		models.CauseCreated,
	)
	if err != nil {
		return 0, err
	}

	id, err := s.repo.CreateWithHistory(ctx, booking, history)
	if err != nil {
		return 0, fmt.Errorf("сохранение бронирования: %w", err)
	}

	s.logger.Info("бронирование создано",
		zap.Int64("id", id),
		zap.Int64("userId", req.UserID),
		zap.Int64("resourceId", req.ResourceID),
	)

	if err := s.publisher.PublishCreateBookingJob(ctx, messaging.CreateBookingJobCommand{
		EventId:    messaging.NewMessageID(),
		RequestId:  messaging.BookingIDToRequestID(id),
		ResourceId: req.ResourceID,
		StartDate:  req.StartDate,
		EndDate:    req.EndDate,
	}); err != nil {
		s.logger.Error("ошибка публикации CreateBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return id, nil
}

// RequestCancel инициирует отмену через compensating transaction (HTTP PUT /cancel).
func (s *BookingsService) RequestCancel(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.BeginCancel(time.Now()); err != nil {
		return err
	}

	if err := s.persistStatusChange(ctx, booking, previousStatus, userInitiator(booking.UserID()), models.CauseCancellationRequested); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("отмена бронирования инициирована", zap.Int64("id", id))

	if err := s.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(id),
	}); err != nil {
		s.logger.Error("ошибка публикации CancelBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return nil
}

// Cancel немедленно отменяет бронирование без compensating transaction.
func (s *BookingsService) Cancel(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.Cancel(time.Now()); err != nil {
		return err
	}

	if err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseDenied); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("бронирование отменено", zap.Int64("id", id))

	if err := s.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(id),
	}); err != nil {
		s.logger.Error("ошибка публикации CancelBookingJob", zap.Error(err), zap.Int64("bookingId", id))
	}

	return nil
}

// CompleteCancel завершает отмену после успешной обработки командой Catalog.
func (s *BookingsService) CompleteCancel(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.CompleteCancel(); err != nil {
		return err
	}

	if err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseCancellationCompleted); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("отмена бронирования завершена", zap.Int64("id", id))
	return nil
}

// HandleCancelError выполняет rollback при ошибке обработки команды отмены в Catalog (DLQ).
func (s *BookingsService) HandleCancelError(ctx context.Context, requestID string) error {
	bookingID, err := messaging.RequestIDToBookingID(requestID)
	if err != nil {
		return err
	}

	booking, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.RollbackCancel(); err != nil {
		return err
	}

	if err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseCancellationFailed); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("отмена бронирования откачена", zap.Int64("id", bookingID))
	return nil
}

// Confirm подтверждает бронирование по ID.
func (s *BookingsService) Confirm(ctx context.Context, id int64) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.Confirm(); err != nil {
		return err
	}

	if err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseConfirmed); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("бронирование подтверждено", zap.Int64("id", id))
	return nil
}
