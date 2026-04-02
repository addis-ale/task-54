package domain

import "time"

type ExamTemplate struct {
	ID              int64                `json:"id"`
	Title           string               `json:"title"`
	Subject         string               `json:"subject"`
	DurationMinutes int                  `json:"duration_minutes"`
	RoomID          int64                `json:"room_id"`
	ProctorID       int64                `json:"proctor_id"`
	CandidateIDs    []int64              `json:"candidate_ids"`
	Windows         []ExamTemplateWindow `json:"windows"`
	CreatedBy       *int64               `json:"created_by,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

type ExamTemplateWindow struct {
	ID            int64     `json:"id"`
	TemplateID    int64     `json:"template_id"`
	Label         string    `json:"label"`
	WindowStartAt time.Time `json:"window_start_at"`
	WindowEndAt   time.Time `json:"window_end_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type ExamSessionDraft struct {
	ID                int64              `json:"id"`
	TemplateID        int64              `json:"template_id"`
	Subject           string             `json:"subject"`
	RoomID            int64              `json:"room_id"`
	ProctorID         int64              `json:"proctor_id"`
	CandidateIDs      []int64            `json:"candidate_ids"`
	StartAt           time.Time          `json:"start_at"`
	EndAt             time.Time          `json:"end_at"`
	Status            string             `json:"status"`
	Conflicts         []ScheduleConflict `json:"conflicts,omitempty"`
	PublishedSchedule *int64             `json:"published_schedule_id,omitempty"`
	CreatedBy         *int64             `json:"created_by,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}
