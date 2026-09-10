package domain

import "time"

type JobRun struct {
	ID          int64     `json:"id"`
	JobType     string    `json:"job_type"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Status      string    `json:"status"`
	SummaryJSON           *string `json:"summary_json,omitempty"`
	FailureRootCauseNotes *string `json:"failure_root_cause_notes,omitempty"`
}
