package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

// CancelBookingErrorHandler обрабатывает события CancelBookingJobError (ошибка отмены в Catalog / DLQ).
type CancelBookingErrorHandler struct {
	service *service.BookingsService
	queries *service.BookingsQueries
	logger  *zap.Logger
}

// NewCancelBookingErrorHandler создаёт новый обработчик.
func NewCancelBookingErrorHandler(svc *service.BookingsService, queries *service.BookingsQueries, logger *zap.Logger) *CancelBookingErrorHandler {
	return &CancelBookingErrorHandler{
		service: svc,
		queries: queries,
		logger:  logger,
	}
}

// Handle обрабатывает событие ошибки отмены бронирования.
func (h *CancelBookingErrorHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.CancelBookingJobError
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация CancelBookingJobError: %w", err)
	}

	if skip, err := skipDuplicateEvent(ctx, h.queries, h.logger, event.EventId); err != nil {
		return err
	} else if skip {
		return nil
	}

	h.logger.Info("получено событие CancelBookingJobError",
		zap.String("requestId", event.RequestId),
		zap.String("reason", event.Reason),
	)

	if err := h.service.HandleCancelErrorWithEvent(ctx, event.RequestId, event.EventId); err != nil {
		return fmt.Errorf("rollback отмены бронирования (requestId=%s): %w", event.RequestId, err)
	}

	h.logger.Info("отмена бронирования откачена через событие",
		zap.String("requestId", event.RequestId),
		zap.String("reason", event.Reason),
	)
	return nil
}
