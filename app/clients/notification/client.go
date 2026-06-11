package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Client — HTTP-клиент для Notification Service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	retryCfg   RetryConfig
	logger     *zap.Logger
}

// NewClient создаёт новый Notification-клиент.
func NewClient(baseURL string, timeout time.Duration, maxRetries int, retryBaseDelay time.Duration, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryCfg: RetryConfig{
			MaxRetries: maxRetries,
			BaseDelay:  retryBaseDelay,
		},
		logger: logger,
	}
}

// SendNotification отправляет уведомление о смене статуса бронирования.
func (c *Client) SendNotification(ctx context.Context, req NotificationRequest) {
	err := withRetry(ctx, c.retryCfg, "SendNotification", func() error {
		_, err := c.doJSON(ctx, http.MethodPost, "/api/notifications", req)
		return err
	})
	if err != nil {
		c.logger.Warn("не удалось отправить уведомление в Notification Service",
			zap.Error(err),
			zap.Int64("bookingId", req.BookingID),
			zap.Int64("userId", req.UserID),
			zap.String("status", req.Status),
		)
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any) ([]byte, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("сериализация тела запроса: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение запроса %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respBody, nil
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, &permanentHTTPError{
			statusCode: resp.StatusCode,
			body:       string(respBody),
		}
	}

	return nil, fmt.Errorf("HTTP %d от Notification Service (%s %s): %s",
		resp.StatusCode, method, path, string(respBody))
}
