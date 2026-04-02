package domain

import "time"

type Patient struct {
	ID        int64     `json:"id"`
	MRN       string    `json:"mrn"`
	Name      string    `json:"name"`
	DOB       *string   `json:"dob,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
