package domain

import "time"

type MediaAsset struct {
	ID             int64     `json:"id"`
	ExerciseID     int64     `json:"exercise_id"`
	MediaType      string    `json:"media_type"`
	Path           string    `json:"path"`
	ChecksumSHA256 string    `json:"checksum_sha256"`
	DurationMS     *int64    `json:"duration_ms,omitempty"`
	Bytes          int64     `json:"bytes"`
	Variant        string    `json:"variant"`
	CreatedAt      time.Time `json:"created_at"`
}
