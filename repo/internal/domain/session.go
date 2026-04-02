package domain

import "time"

type Session struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	TokenHash  string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}
