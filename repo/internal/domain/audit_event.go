package domain

import "time"

type AuditEvent struct {
	ID           int64     `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ActorID      *int64    `json:"actor_id"`
	OperatorName string    `json:"operator_name,omitempty"`
	LocalIP      string    `json:"local_ip,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	BeforeJSON   *string   `json:"before_json"`
	AfterJSON    *string   `json:"after_json"`
	RequestID    string    `json:"request_id"`
	HashPrev     *string   `json:"hash_prev"`
	HashSelf     string    `json:"hash_self"`
}
