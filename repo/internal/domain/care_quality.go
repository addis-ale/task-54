package domain

import "time"

type CareQualityCheckpoint struct {
	ID             int64     `json:"id"`
	ResidentID     int64     `json:"resident_id"`
	CheckpointType string    `json:"checkpoint_type"`
	Status         string    `json:"status"`
	Notes          string    `json:"notes"`
	RecordedBy     *int64    `json:"recorded_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type AlertEvent struct {
	ID         int64     `json:"id"`
	ResidentID int64     `json:"resident_id"`
	AlertType  string    `json:"alert_type"`
	Severity   string    `json:"severity"`
	State      string    `json:"state"`
	Message    string    `json:"message"`
	RecordedBy *int64    `json:"recorded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CareDashboardSummary struct {
	CheckpointCount int64 `json:"checkpoint_count"`
	AlertOpenCount  int64 `json:"alert_open_count"`
	AlertHighCount  int64 `json:"alert_high_count"`
}
