package models

// StatisticsData — результат агрегации из хранилища.
type StatisticsData struct {
	TotalCount   int64
	ByStatus     []StatusCount
	TopResources []ResourceCount
}

// StatusCount — количество бронирований по статусу.
type StatusCount struct {
	Status BookingStatus
	Count  int64
}

// ResourceCount — количество бронирований по ресурсу.
type ResourceCount struct {
	ResourceID int64
	Count      int64
}
