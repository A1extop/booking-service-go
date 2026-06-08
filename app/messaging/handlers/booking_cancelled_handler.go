package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/service"
)

type BookingCancelledHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

func NewBookingCancelledHandler(svc *service.BookingsService, logger *zap.Logger) *BookingCancelledHandler {
	return &BookingCancelledHandler{
		service: svc,
		logger:  logger,
	}
}

func (h *BookingCancelledHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.BookingJobCancelled
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация BookingJobCancelled: %w", err)
	}

	bookingID, err := messaging.RequestIDToBookingID(event.RequestId)
	if err != nil {
		return fmt.Errorf("извлечение bookingId из RequestId: %w", err)
	}

	h.logger.Info("получено событие BookingJobCancelled",
		zap.Int64("bookingId", bookingID),
		zap.Int64("catalogJobId", event.Id),
	)

	if err := h.service.CompleteCancel(ctx, bookingID); err != nil {
		return fmt.Errorf("завершение отмены бронирования %d: %w", bookingID, err)
	}

	h.logger.Info("отмена бронирования завершена через событие", zap.Int64("bookingId", bookingID))
	return nil
}
