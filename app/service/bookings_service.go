package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/patrickmn/go-cache"
	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/clients/notification"
	"booking-service/app/messaging"
	"booking-service/app/models"
)

// BookingsService обрабатывает команды (изменение состояния) для бронирований.
type BookingsService struct {
	repo         models.BookingRepository
	publisher    *messaging.Publisher
	notification *notification.Client
	statsCache   *cache.Cache
	logger       *zap.Logger
}

// NewBookingsService создаёт новый BookingsService.
func NewBookingsService(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	notificationClient *notification.Client,
	statsCache *cache.Cache,
	logger *zap.Logger,
) *BookingsService {
	return &BookingsService{
		repo:         repo,
		publisher:    publisher,
		notification: notificationClient,
		statsCache:   statsCache,
		logger:       logger,
	}
}

func (s *BookingsService) invalidateStatisticsCache() {
	if s.statsCache != nil {
		s.statsCache.Flush()
	}
}

func (s *BookingsService) notifyStatusChange(
	ctx context.Context,
	booking *models.Booking,
	previousStatus models.BookingStatus,
) {
	if s.notification == nil {
		return
	}

	s.notification.SendNotification(ctx, notification.NotificationRequest{
		BookingID:      booking.ID(),
		UserID:         booking.UserID(),
		Status:         string(booking.Status()),
		PreviousStatus: string(previousStatus),
	})
}

func (s *BookingsService) persistStatusChange(
	ctx context.Context,
	booking *models.Booking,
	previousStatus models.BookingStatus,
	initiator, cause string,
) (bool, error) {
	history, err := models.NewHistory(booking.Status(), previousStatus, booking.ID(), initiator, cause)
	if err != nil {
		return false, err
	}
	outbox := newStatusChangedOutboxEvent(booking.ID(), previousStatus, booking.Status(), cause)
	if err := s.repo.UpdateWithHistory(ctx, booking, history, outbox); err != nil {
		return false, err
	}
	s.invalidateStatisticsCache()
	return true, nil
}

func (s *BookingsService) persistStatusChangeWithEvent(
	ctx context.Context,
	booking *models.Booking,
	previousStatus models.BookingStatus,
	initiator, cause, eventID string,
) (bool, error) {
	history, err := models.NewHistory(booking.Status(), previousStatus, booking.ID(), initiator, cause)
	if err != nil {
		return false, err
	}
	outbox := newStatusChangedOutboxEvent(booking.ID(), previousStatus, booking.Status(), cause)
	if err := s.repo.UpdateWithHistoryAndEvent(ctx, booking, history, eventID, outbox); err != nil {
		if errors.Is(err, models.ErrEventAlreadyProcessed) {
			s.logger.Warn("событие уже обработано (конкурентная обработка)", zap.String("eventId", eventID))
			return false, nil
		}
		return false, err
	}
	s.invalidateStatisticsCache()
	return true, nil
}

func newStatusChangedOutboxEvent(
	bookingID int64,
	previousStatus, newStatus models.BookingStatus,
	reason string,
) *models.BookingStatusChangedEvent {
	return &models.BookingStatusChangedEvent{
		EventId:   messaging.NewMessageID(),
		BookingId: bookingID,
		OldStatus: string(previousStatus),
		NewStatus: string(newStatus),
		ChangedAt: time.Now().UTC().Format(time.RFC3339),
		Reason:    reason,
	}
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
	s.invalidateStatisticsCache()

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

	if _, err := s.persistStatusChange(ctx, booking, previousStatus, userInitiator(booking.UserID()), models.CauseCancellationRequested); err != nil {
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

// CancelWithEvent отменяет бронирование по событию с защитой идемпотентности.
func (s *BookingsService) CancelWithEvent(ctx context.Context, id int64, eventID string) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.Cancel(time.Now()); err != nil {
		return err
	}

	updated, err := s.persistStatusChangeWithEvent(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseDenied, eventID)
	if err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}
	if updated {
		s.notifyStatusChange(ctx, booking, previousStatus)
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

	if _, err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseDenied); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}
	s.notifyStatusChange(ctx, booking, previousStatus)

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

	if _, err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseCancellationCompleted); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}
	s.notifyStatusChange(ctx, booking, previousStatus)

	s.logger.Info("отмена бронирования завершена", zap.Int64("id", id))
	return nil
}

// HandleCancelErrorWithEvent выполняет rollback отмены по событию с защитой идемпотентности.
func (s *BookingsService) HandleCancelErrorWithEvent(ctx context.Context, requestID, eventID string) error {
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

	if _, err := s.persistStatusChangeWithEvent(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseCancellationFailed, eventID); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("отмена бронирования откачена", zap.Int64("id", bookingID))
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

	if _, err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseCancellationFailed); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}

	s.logger.Info("отмена бронирования откачена", zap.Int64("id", bookingID))
	return nil
}

// ConfirmWithEvent подтверждает бронирование по событию с защитой идемпотентности.
func (s *BookingsService) ConfirmWithEvent(ctx context.Context, id int64, eventID string) error {
	booking, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	previousStatus := booking.Status()
	if err := booking.Confirm(); err != nil {
		return err
	}

	updated, err := s.persistStatusChangeWithEvent(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseConfirmed, eventID)
	if err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}
	if updated {
		s.notifyStatusChange(ctx, booking, previousStatus)
	}

	s.logger.Info("бронирование подтверждено", zap.Int64("id", id))
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

	if _, err := s.persistStatusChange(ctx, booking, previousStatus, models.InitiatorSystem, models.CauseConfirmed); err != nil {
		return fmt.Errorf("обновление бронирования: %w", err)
	}
	s.notifyStatusChange(ctx, booking, previousStatus)

	s.logger.Info("бронирование подтверждено", zap.Int64("id", id))
	return nil
}
