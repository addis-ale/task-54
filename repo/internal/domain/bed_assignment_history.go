package domain

import "time"

type BedAssignmentHistory struct {
	ID          int64     `json:"id"`
	AdmissionID int64     `json:"admission_id"`
	FromBedID   *int64    `json:"from_bed_id,omitempty"`
	ToBedID     *int64    `json:"to_bed_id,omitempty"`
	ChangedAt   time.Time `json:"changed_at"`
	ActorID     *int64    `json:"actor_id,omitempty"`
}
