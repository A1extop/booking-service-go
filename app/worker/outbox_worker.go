package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// OutboxWorker периодически публикует BookingStatusChangedEvent из transactional outbox.
type OutboxWorker struct {
	repo       models.BookingRepository
	publisher  *messaging.Publisher
	interval   time.Duration
	batchSize  int
	maxRetries int64
	logger     *zap.Logger
}

func NewOutboxWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	batchSize int,
	maxRetries int64,
	logger *zap.Logger,
) *OutboxWorker {
	return &OutboxWorker{
		repo:       repo,
		publisher:  publisher,
		interval:   interval,
		batchSize:  batchSize,
		maxRetries: maxRetries,
		logger:     logger,
	}
}

func (w *OutboxWorker) Run(ctx context.Context) {
	w.logger.Info("воркер outbox запущен",
		zap.Duration("interval", w.interval),
		zap.Int("batchSize", w.batchSize),
		zap.Int64("maxRetries", w.maxRetries),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер outbox остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	messages, err := w.repo.GetOutboxMessages(ctx, w.maxRetries, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения outbox-сообщений", zap.Error(err))
		return
	}

	if len(messages) == 0 {
		return
	}

	w.logger.Info("публикация outbox-сообщений", zap.Int("count", len(messages)))

	for i := range messages {
		w.processMessage(ctx, &messages[i])
	}
}

func (w *OutboxWorker) processMessage(ctx context.Context, outbox *models.BookingEvent) {
	logger := w.logger.With(
		zap.String("eventId", outbox.EventId),
		zap.Int64("bookingId", outbox.Message.BookingId),
		zap.Int64("retry", outbox.Retry),
	)

	event := messaging.BookingStatusChangedEvent{
		EventId:   outbox.Message.EventId,
		BookingId: outbox.Message.BookingId,
		OldStatus: outbox.Message.OldStatus,
		NewStatus: outbox.Message.NewStatus,
		ChangedAt: outbox.Message.ChangedAt,
		Reason:    outbox.Message.Reason,
	}

	if err := w.publisher.PublishBookingStatusChanged(ctx, event); err != nil {
		logger.Error("ошибка публикации BookingStatusChanged", zap.Error(err))

		if err := w.repo.IncrementOutboxRetry(ctx, outbox.EventId); err != nil {
			logger.Error("ошибка увеличения retry outbox", zap.Error(err))
			return
		}

		nextRetry := outbox.Retry + 1
		if nextRetry >= w.maxRetries {
			logger.Warn("достигнут лимит попыток публикации outbox, retry прекращён")
		}
		return
	}

	if err := w.repo.DeleteOutboxMessage(ctx, outbox.EventId); err != nil {
		logger.Error("ошибка удаления outbox-сообщения после публикации", zap.Error(err))
		return
	}

	logger.Info("BookingStatusChanged опубликован из outbox")
}
