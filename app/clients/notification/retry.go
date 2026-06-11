package notification

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
)

// RetryConfig — конфигурация retry-политики.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
}

// permanentHTTPError — клиентская ошибка (4xx), повторять бессмысленно.
type permanentHTTPError struct {
	statusCode int
	body       string
}

func (e *permanentHTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.statusCode, e.body)
}

func isPermanent(err error) bool {
	var perm *permanentHTTPError
	return errors.As(err, &perm)
}

// withRetry выполняет функцию с экспоненциальным backoff.

func withRetry(ctx context.Context, cfg RetryConfig, operation string, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := cfg.BaseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			zap.L().Warn("retry попытка",
				zap.String("operation", operation),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay),
				zap.Error(lastErr),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("%s: контекст отменён: %w", operation, ctx.Err())
			case <-time.After(delay):
			}
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if isPermanent(lastErr) {
			return lastErr
		}
	}

	return fmt.Errorf("%s: все %d попыток исчерпаны: %w", operation, cfg.MaxRetries+1, lastErr)
}
