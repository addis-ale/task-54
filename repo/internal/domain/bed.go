package domain

import "time"

const (
	BedStatusAvailable   = "available"
	BedStatusOccupied    = "occupied"
	BedStatusCleaning    = "cleaning"
	BedStatusMaintenance = "maintenance"
)

type Bed struct {
	ID        int64     `json:"id"`
	WardID    int64     `json:"ward_id"`
	BedCode   string    `json:"bed_code"`
	Status    string    `json:"status"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
	WardName  string    `json:"ward_name,omitempty"`
}

type BedOccupancy struct {
	BedID       int64   `json:"bed_id"`
	WardID      int64   `json:"ward_id"`
	WardName    string  `json:"ward_name"`
	BedCode     string  `json:"bed_code"`
	Status      string  `json:"status"`
	Version     int64   `json:"version"`
	AdmissionID *int64  `json:"admission_id,omitempty"`
	PatientID   *int64  `json:"patient_id,omitempty"`
	PatientName *string `json:"patient_name,omitempty"`
}

func IsValidBedStatus(status string) bool {
	switch status {
	case BedStatusAvailable, BedStatusOccupied, BedStatusCleaning, BedStatusMaintenance:
		return true
	default:
		return false
	}
}
