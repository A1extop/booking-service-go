package notification

// NotificationRequest — запрос на отправку уведомления пользователю.
type NotificationRequest struct {
	BookingID      int64  `json:"bookingId"`
	UserID         int64  `json:"userId"`
	Status         string `json:"status"`
	PreviousStatus string `json:"previousStatus"`
}
