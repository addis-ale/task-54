package domain

import "time"

const (
	ExamScheduleStatusScheduled = "scheduled"
	ExamScheduleStatusCancelled = "cancelled"
)

type ExamSchedule struct {
	ID           int64     `json:"id"`
	ExamID       string    `json:"exam_id"`
	RoomID       int64     `json:"room_id"`
	ProctorID    int64     `json:"proctor_id"`
	CandidateIDs []int64   `json:"candidate_ids"`
	StartAt      time.Time `json:"start_at"`
	EndAt        time.Time `json:"end_at"`
	Status       string    `json:"status"`
	Version      int64     `json:"version"`
	ActorID      *int64    `json:"actor_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ScheduleConflict struct {
	ConflictType          string    `json:"conflict_type"`
	EntityID              int64     `json:"entity_id"`
	ConflictingScheduleID int64     `json:"conflicting_schedule_id"`
	ExistingStartAt       time.Time `json:"existing_start_at"`
	ExistingEndAt         time.Time `json:"existing_end_at"`
	Message               string    `json:"message"`
}
