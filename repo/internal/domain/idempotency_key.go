package domain

import "time"

type IdempotencyKeyRecord struct {
	ID           int64     `json:"id"`
	ActorID      int64     `json:"actor_id"`
	RouteKey     string    `json:"route_key"`
	Key          string    `json:"key"`
	RequestHash  string    `json:"request_hash"`
	ResponseCode int       `json:"response_code"`
	ResponseBody string    `json:"response_body"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}
