package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

// CancellationRetryWorker периодически переотправляет CancelBookingJob
// для бронирований, застрявших в cancellation_pending.
type CancellationRetryWorker struct {
	repo      models.BookingRepository
	publisher *messaging.Publisher
	interval  time.Duration
	timeout   time.Duration
	batchSize int
	logger    *zap.Logger
}

func NewCancellationRetryWorker(
	repo models.BookingRepository,
	publisher *messaging.Publisher,
	interval time.Duration,
	timeout time.Duration,
	batchSize int,
	logger *zap.Logger,
) *CancellationRetryWorker {
	return &CancellationRetryWorker{
		repo:      repo,
		publisher: publisher,
		interval:  interval,
		timeout:   timeout,
		batchSize: batchSize,
		logger:    logger,
	}
}

func (w *CancellationRetryWorker) Run(ctx context.Context) {
	w.logger.Info("воркер повторной отмены запущен",
		zap.Duration("interval", w.interval),
		zap.Duration("timeout", w.timeout),
		zap.Int("batchSize", w.batchSize),
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("воркер повторной отмены остановлен")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *CancellationRetryWorker) processBatch(ctx context.Context) {
	cutoff := time.Now().Add(-w.timeout)

	bookingsPtr, err := w.repo.GetStuckCancellation(ctx, cutoff, w.batchSize)
	if err != nil {
		w.logger.Error("ошибка получения зависших отмен", zap.Error(err))
		return
	}

	if bookingsPtr == nil || len(*bookingsPtr) == 0 {
		return
	}

	bookings := *bookingsPtr
	w.logger.Info("повторная отправка отмены", zap.Int("count", len(bookings)))

	for i := range bookings {
		w.processBooking(ctx, &bookings[i])
	}
}

func (w *CancellationRetryWorker) processBooking(ctx context.Context, booking *models.Booking) {
	bookingID := booking.ID()
	logger := w.logger.With(zap.Int64("bookingId", bookingID))

	if err := w.publisher.PublishCancelBookingJob(ctx, messaging.CancelBookingJobCommand{
		EventId:   messaging.NewMessageID(),
		RequestId: messaging.BookingIDToRequestID(bookingID),
	}); err != nil {
		logger.Error("ошибка повторной публикации CancelBookingJob", zap.Error(err))
		return
	}

	logger.Info("команда отмены переотправлена")
}
