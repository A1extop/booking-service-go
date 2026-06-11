package models

// BookingStatusChangedEvent — payload доменного события для transactional outbox.
type BookingStatusChangedEvent struct {
	EventId   string `json:"EventId"`
	BookingId int64  `json:"BookingId"`
	OldStatus string `json:"OldStatus"`
	NewStatus string `json:"NewStatus"`
	ChangedAt string `json:"ChangedAt"`
	Reason    string `json:"Reason"`
}

// BookingEvent — запись в таблице outbox_messages.
type BookingEvent struct {
	EventId string
	Message BookingStatusChangedEvent
	Retry   int64
}
