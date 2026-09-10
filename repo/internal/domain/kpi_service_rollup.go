package domain

import "time"

type KPIServiceRollup struct {
	ID                int64     `json:"id"`
	BucketStart       time.Time `json:"bucket_start"`
	BucketGranularity string    `json:"bucket_granularity"`
	ServiceType       string    `json:"service_type"`
	Total             int64     `json:"total"`
	Completed         int64     `json:"completed"`
	OnTime15m         int64     `json:"on_time_15m"`
	ExecutionRate     float64   `json:"execution_rate"`
	TimelinessRate    float64   `json:"timeliness_rate"`
}
