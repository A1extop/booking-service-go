package dto

import "time"

// CreateBookingRequest -- запрос на создание бронирования.
type CreateBookingRequest struct {
	UserID     int64  `json:"userId"`
	ResourceID int64  `json:"resourceId"`
	StartDate  string `json:"startDate"` // формат: "2006-01-02"
	EndDate    string `json:"endDate"`   // формат: "2006-01-02"
}

// CreateBookingResponse -- ответ при создании бронирования.
type CreateBookingResponse struct {
	ID int64 `json:"id"`
}

// BookingResponse -- полные данные бронирования.
type BookingResponse struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	UserID     int64  `json:"userId"`
	ResourceID int64  `json:"resourceId"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	CreatedAt  string `json:"createdAt"` // формат: RFC3339
}

// BookingStatusResponse -- статус бронирования.
type BookingStatusResponse struct {
	Status string `json:"status"`
}

// GetBookingsByFilterRequest -- запрос с фильтром и пагинацией.
type GetBookingsByFilterRequest struct {
	UserID     *int64  `json:"userId,omitempty"`
	ResourceID *int64  `json:"resourceId,omitempty"`
	Status     *string `json:"status,omitempty"`
	Page       int     `json:"page"`
	Size       int     `json:"size"`
}

// PagedResponse -- ответ с пагинацией.
type PagedResponse[T any] struct {
	Items      []T   `json:"items"`
	TotalCount int64 `json:"totalCount"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
}

// ProblemDetails -- стандартный формат ошибки RFC 7807.
type ProblemDetails struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}
// BookingStatisticsResponse -- агрегированная статистика по бронированиям.
type BookingStatisticsResponse struct {
	TotalCount   int64               `json:"totalCount"`
	ByStatus     []StatusCountItem   `json:"byStatus"`
	TopResources []ResourceCountItem `json:"topResources"`
}

// StatusCountItem -- количество бронирований по статусу.
type StatusCountItem struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// ResourceCountItem -- количество бронирований по ресурсу.
type ResourceCountItem struct {
	ResourceID int64 `json:"resourceId"`
	Count      int64 `json:"count"`
}

// HistoryItem -- запись audit log изменения статуса.
type HistoryItem struct {
	ID             int64     `json:"id"`
	PreviousStatus string    `json:"previousStatus"`
	Status         string    `json:"status"`
	Initiator      string    `json:"initiator"`
	Reason         string    `json:"reason"`
	BookingID      int64     `json:"bookingId"`
	ChangedAt      time.Time `json:"changedAt"`
}

// DateFormat -- формат даты для JSON-сериализации.
const DateFormat = "2006-01-02"
