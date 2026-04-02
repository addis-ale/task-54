package domain

import "time"

const (
	WorkOrderStatusQueued     = "queued"
	WorkOrderStatusInProgress = "in_progress"
	WorkOrderStatusCompleted  = "completed"
)

const (
	WorkOrderPriorityLow    = "low"
	WorkOrderPriorityNormal = "normal"
	WorkOrderPriorityHigh   = "high"
	WorkOrderPriorityUrgent = "urgent"
)

type WorkOrder struct {
	ID          int64      `json:"id"`
	ServiceType string     `json:"service_type"`
	Priority    string     `json:"priority"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Status      string     `json:"status"`
	AssigneeID  *int64     `json:"assignee_id,omitempty"`
	Version     int64      `json:"version"`
}

func IsValidWorkOrderStatus(status string) bool {
	switch status {
	case WorkOrderStatusQueued, WorkOrderStatusInProgress, WorkOrderStatusCompleted:
		return true
	default:
		return false
	}
}

func IsValidWorkOrderPriority(priority string) bool {
	switch priority {
	case WorkOrderPriorityLow, WorkOrderPriorityNormal, WorkOrderPriorityHigh, WorkOrderPriorityUrgent:
		return true
	default:
		return false
	}
}
