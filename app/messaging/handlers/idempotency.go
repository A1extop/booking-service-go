package handlers

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/service"
)

func skipDuplicateEvent(ctx context.Context, queries *service.BookingsQueries, logger *zap.Logger, eventID string) (bool, error) {
	processed, err := queries.IsEventProcessed(ctx, eventID)
	if err != nil {
		return false, fmt.Errorf("проверка идемпотентности: %w", err)
	}
	if processed {
		logger.Warn("дублирующееся событие, пропуск", zap.String("eventId", eventID))
		return true, nil
	}
	return false, nil
}
