package service_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestWorkOrderCompletionCreatesAuditAndJobRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, workOrders, _, _, cleanup := setupWorkOrderKPITest(t)
	defer cleanup()

	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{ServiceType: "housekeeping"})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}

	if _, err := workOrders.Start(ctx, service.StartWorkOrderInput{WorkOrderID: queued.ID}); err != nil {
		t.Fatalf("start work order: %v", err)
	}

	if _, err := workOrders.Complete(ctx, service.CompleteWorkOrderInput{WorkOrderID: queued.ID, RequestID: "req_test_work_order"}); err != nil {
		t.Fatalf("complete work order: %v", err)
	}

	var auditCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM audit_events WHERE action = 'work_order.complete' AND resource_id = ?`, intToString(queued.ID)).Scan(&auditCount); err != nil {
		t.Fatalf("query audit count: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 audit event, got %d", auditCount)
	}

	var jobRunCount int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM job_runs WHERE job_type = 'work_order_completion'`).Scan(&jobRunCount); err != nil {
		t.Fatalf("query job run count: %v", err)
	}
	if jobRunCount != 1 {
		t.Fatalf("expected 1 completion job run, got %d", jobRunCount)
	}

}

func TestKPIHybridQueryCombinesHistoryAndRealtime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, workOrders, kpis, rollups, cleanup := setupWorkOrderKPITest(t)
	defer cleanup()

	queued, err := workOrders.Queue(ctx, service.QueueWorkOrderInput{ServiceType: "radiology"})
	if err != nil {
		t.Fatalf("queue work order: %v", err)
	}
	if _, err := workOrders.Start(ctx, service.StartWorkOrderInput{WorkOrderID: queued.ID}); err != nil {
		t.Fatalf("start work order: %v", err)
	}
	if _, err := workOrders.Complete(ctx, service.CompleteWorkOrderInput{WorkOrderID: queued.ID, RequestID: "req_test_kpi"}); err != nil {
		t.Fatalf("complete work order: %v", err)
	}

	now := time.Now().UTC()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	if err := rollups.Upsert(ctx, &domain.KPIServiceRollup{
		BucketStart:       previousHour,
		BucketGranularity: "hour",
		ServiceType:       "lab",
		Total:             12,
		Completed:         9,
		OnTime15m:         7,
		ExecutionRate:     75,
	}); err != nil {
		t.Fatalf("seed historical rollup: %v", err)
	}

	items, err := kpis.QueryServiceDelivery(ctx, service.ServiceDeliveryQuery{
		From:    previousHour,
		To:      now.Add(5 * time.Minute),
		GroupBy: "hour",
	})
	if err != nil {
		t.Fatalf("query kpi service delivery: %v", err)
	}

	if len(items) < 2 {
		t.Fatalf("expected at least 2 KPI rows (historical + realtime), got %d", len(items))
	}

	foundHistorical := false
	foundRealtime := false
	for _, item := range items {
		if item.ServiceType == "lab" && item.BucketStart.Equal(previousHour) {
			foundHistorical = true
		}
		if item.ServiceType == "radiology" && item.BucketStart.Equal(now.Truncate(time.Hour)) {
			foundRealtime = true
		}
	}

	if !foundHistorical {
		t.Fatalf("did not find historical rollup row in KPI response")
	}
	if !foundRealtime {
		t.Fatalf("did not find realtime row in KPI response")
	}
}

func setupWorkOrderKPITest(t *testing.T) (*sql.DB, *service.WorkOrderService, *service.KPIService, repository.KPIRollupRepository, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "work_order_kpi.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := migrations.Run(context.Background(), db); err != nil {
		_ = db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	auditRepo := sqlite.NewAuditRepository(db)
	jobRunRepo := sqlite.NewJobRunRepository(db)
	workOrderRepo := sqlite.NewWorkOrderRepository(db)
	rollupRepo := sqlite.NewKPIRollupRepository(db)

	auditService := service.NewAuditService(auditRepo)
	jobRuns := service.NewJobRunService(jobRunRepo)
	workOrders := service.NewWorkOrderService(db, workOrderRepo, auditService, jobRuns)
	kpis := service.NewKPIService(db, workOrderRepo, rollupRepo, jobRuns)

	return db, workOrders, kpis, rollupRepo, func() {
		_ = db.Close()
	}
}

func intToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
