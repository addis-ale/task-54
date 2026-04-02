package domain

import "time"

type AuditTrailRecord struct {
	ID           int64     `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ActorID      *int64    `json:"actor_id,omitempty"`
	OperatorName string    `json:"operator_name,omitempty"`
	LocalIP      string    `json:"local_ip,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	RequestID    string    `json:"request_id"`
	BeforeJSON   *string   `json:"before_json,omitempty"`
	AfterJSON    *string   `json:"after_json,omitempty"`
}

type ReportSchedule struct {
	ID               int64     `json:"id"`
	ReportType       string    `json:"report_type"`
	Format           string    `json:"format"`
	SharedFolderPath string    `json:"shared_folder_path"`
	FiltersJSON      string    `json:"filters_json,omitempty"`
	IntervalMinutes  int       `json:"interval_minutes"`
	NextRunAt        time.Time `json:"next_run_at"`
	Enabled          bool      `json:"enabled"`
	CreatedBy        *int64    `json:"created_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ConfigVersion struct {
	ID                int64     `json:"id"`
	ConfigKey         string    `json:"config_key"`
	ConfigPayloadJSON string    `json:"config_payload_json"`
	CreatedBy         *int64    `json:"created_by,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	IsActive          bool      `json:"is_active"`
}
