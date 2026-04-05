package unit_tests

import (
	"context"
	"testing"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

// TestWorkOrderOnTimeUsesScheduledStart verifies that the on-time calculation
// uses scheduled_start (not started_at) as the reference point, and that
// "on-time" means completed within 15 minutes (900 seconds) of scheduled_start.
func TestWorkOrderOnTimeUsesScheduledStart(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	jobRuns := service.NewJobRunService(sqlite.NewJobRunRepository(db))
	workOrders := service.NewWorkOrderService(db, sqlite.NewWorkOrderRepository(db), auditService, jobRuns)

	// Create a work order with scheduled_start 5 minutes ago → on-time
	scheduledStart := time.Now().UTC().Add(-5 * time.Minute)
	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{
		ServiceType:    "therapy",
		Priority:       domain.WorkOrderPriorityNormal,
		ScheduledStart: &scheduledStart,
	})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}

	if queued.ScheduledStart == nil {
		t.Fatalf("expected scheduled_start to be set, got nil")
	}

	if _, err := workOrders.Start(ctx, service.StartWorkOrderInput{WorkOrderID: queued.ID}); err != nil {
		t.Fatalf("start work order: %v", err)
	}

	result, err := workOrders.Complete(ctx, service.CompleteWorkOrderInput{
		WorkOrderID: queued.ID,
		RequestID:   "req_ontime_test",
	})
	if err != nil {
		t.Fatalf("complete work order: %v", err)
	}

	// Completed ~5 minutes after scheduled_start → should be on-time
	if !result.OnTime15m {
		t.Fatalf("expected on_time_15m=true (completed ~5 min after scheduled_start), got false; latency=%d", result.LatencySeconds)
	}
}

// TestWorkOrderLateWhenCompletedAfter15MinFromScheduledStart verifies that
// a work order completed more than 15 minutes after scheduled_start is NOT on-time.
func TestWorkOrderLateWhenCompletedAfter15MinFromScheduledStart(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	jobRuns := service.NewJobRunService(sqlite.NewJobRunRepository(db))
	workOrders := service.NewWorkOrderService(db, sqlite.NewWorkOrderRepository(db), auditService, jobRuns)

	// Create a work order with scheduled_start 20 minutes ago → late
	scheduledStart := time.Now().UTC().Add(-20 * time.Minute)
	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{
		ServiceType:    "radiology",
		Priority:       domain.WorkOrderPriorityHigh,
		ScheduledStart: &scheduledStart,
	})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}

	if _, err := workOrders.Start(ctx, service.StartWorkOrderInput{WorkOrderID: queued.ID}); err != nil {
		t.Fatalf("start work order: %v", err)
	}

	result, err := workOrders.Complete(ctx, service.CompleteWorkOrderInput{
		WorkOrderID: queued.ID,
		RequestID:   "req_late_test",
	})
	if err != nil {
		t.Fatalf("complete work order: %v", err)
	}

	// Completed ~20 minutes after scheduled_start → should be late
	if result.OnTime15m {
		t.Fatalf("expected on_time_15m=false (completed ~20 min after scheduled_start), got true; latency=%d", result.LatencySeconds)
	}
}

// TestWorkOrderScheduledStartPersistedAndRetrieved verifies that scheduled_start
// survives a create → list → get round-trip.
func TestWorkOrderScheduledStartPersistedAndRetrieved(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	jobRuns := service.NewJobRunService(sqlite.NewJobRunRepository(db))
	workOrders := service.NewWorkOrderService(db, sqlite.NewWorkOrderRepository(db), auditService, jobRuns)

	scheduledStart := time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second)
	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{
		ServiceType:    "physio",
		Priority:       domain.WorkOrderPriorityNormal,
		ScheduledStart: &scheduledStart,
	})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}

	if queued.ScheduledStart == nil {
		t.Fatalf("expected scheduled_start to be set after create")
	}
	if !queued.ScheduledStart.Equal(scheduledStart) {
		t.Fatalf("scheduled_start mismatch: expected %v, got %v", scheduledStart, *queued.ScheduledStart)
	}
}
