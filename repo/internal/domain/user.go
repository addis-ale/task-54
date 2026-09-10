package domain

import "time"

type User struct {
	ID               int64      `json:"id"`
	Username         string     `json:"username"`
	PasswordHash     string     `json:"-"`
	Role             string     `json:"role"`
	FailedLoginCount int        `json:"-"`
	LockedUntil      *time.Time `json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
