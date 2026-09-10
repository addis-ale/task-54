package domain

import "time"

const (
	ExerciseDifficultyBeginner     = "beginner"
	ExerciseDifficultyIntermediate = "intermediate"
	ExerciseDifficultyAdvanced     = "advanced"
)

type Exercise struct {
	ID                int64              `json:"id"`
	Title             string             `json:"title"`
	Description       string             `json:"description"`
	CoachingPoints    string             `json:"coaching_points"`
	Difficulty        string             `json:"difficulty"`
	SearchText        string             `json:"search_text"`
	Version           int64              `json:"version"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	Tags              []Tag              `json:"tags"`
	Contraindications []Contraindication `json:"contraindications"`
	BodyRegions       []BodyRegion       `json:"body_regions"`
	MediaAssets       []MediaAsset       `json:"media_assets"`
}

func IsValidExerciseDifficulty(difficulty string) bool {
	switch difficulty {
	case ExerciseDifficultyBeginner, ExerciseDifficultyIntermediate, ExerciseDifficultyAdvanced:
		return true
	default:
		return false
	}
}
