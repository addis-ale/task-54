package domain

import "time"

const (
	AdmissionStatusActive     = "active"
	AdmissionStatusDischarged = "discharged"
)

type Admission struct {
	ID           int64      `json:"id"`
	PatientID    int64      `json:"patient_id"`
	BedID        int64      `json:"bed_id"`
	AdmittedAt   time.Time  `json:"admitted_at"`
	DischargedAt *time.Time `json:"discharged_at,omitempty"`
	Status       string     `json:"status"`
	Version      int64      `json:"version"`
	PatientName  string     `json:"patient_name,omitempty"`
	BedCode      string     `json:"bed_code,omitempty"`
	WardName     string     `json:"ward_name,omitempty"`
}

func IsValidAdmissionStatus(status string) bool {
	switch status {
	case AdmissionStatusActive, AdmissionStatusDischarged:
		return true
	default:
		return false
	}
}
