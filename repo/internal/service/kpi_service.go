package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type KPIService struct {
	db         *sql.DB
	workOrders repository.WorkOrderRepository
	rollups    repository.KPIRollupRepository
	jobRuns    *JobRunService
}

func NewKPIService(db *sql.DB, workOrders repository.WorkOrderRepository, rollups repository.KPIRollupRepository, jobRuns *JobRunService) *KPIService {
	return &KPIService{
		db:         db,
		workOrders: workOrders,
		rollups:    rollups,
		jobRuns:    jobRuns,
	}
}

type ServiceDeliveryQuery struct {
	From    time.Time
	To      time.Time
	GroupBy string
}

func (s *KPIService) QueryServiceDelivery(ctx context.Context, query ServiceDeliveryQuery) ([]domain.KPIServiceRollup, error) {
	groupBy := strings.TrimSpace(strings.ToLower(query.GroupBy))
	if groupBy == "" {
		groupBy = "hour"
	}
	if groupBy != "hour" && groupBy != "day" && groupBy != "week" && groupBy != "service_type" {
		return nil, fmt.Errorf("%w: group_by must be hour, day, week, or service_type", ErrValidation)
	}

	from := query.From.UTC()
	to := query.To.UTC()
	if !to.After(from) {
		return nil, fmt.Errorf("%w: to must be greater than from", ErrValidation)
	}

	now := time.Now().UTC()
	currentHourStart := now.Truncate(time.Hour)

	hourly := make([]domain.KPIServiceRollup, 0)

	historyTo := minTime(to, currentHourStart)
	if historyTo.After(from) {
		history, err := s.rollups.List(ctx, repository.KPIRollupFilter{
			BucketGranularity: "hour",
			From:              &from,
			To:                &historyTo,
		})
		if err != nil {
			return nil, err
		}
		hourly = append(hourly, history...)
	}

	realtimeFrom := maxTime(from, currentHourStart)
	if to.After(realtimeFrom) {
		realtime, err := s.workOrders.AggregateMetrics(ctx, realtimeFrom, to)
		if err != nil {
			return nil, err
		}
		for i := range realtime {
			realtime[i].BucketStart = realtimeFrom.Truncate(time.Hour)
			realtime[i].BucketGranularity = "hour"
		}
		hourly = append(hourly, realtime...)
	}

	return aggregateMetrics(hourly, groupBy), nil
}

func (s *KPIService) ComputeHourlyRollup(ctx context.Context, bucketStart time.Time) error {
	startRun := time.Now().UTC()
	status := "success"
	summary := map[string]any{
		"bucket_start": bucketStart.UTC().Format(time.RFC3339),
	}

	err := s.computeHourlyRollup(ctx, bucketStart)
	if err != nil {
		status = "failure"
		summary["error"] = err.Error()
	} else {
		summary["result"] = "ok"
	}

	_ = s.jobRuns.Record(ctx, JobRunInput{
		JobType:    "kpi_rollup_hourly",
		StartedAt:  startRun,
		FinishedAt: time.Now().UTC(),
		Status:     status,
		Summary:    summary,
	})

	return err
}

func (s *KPIService) computeHourlyRollup(ctx context.Context, bucketStart time.Time) error {
	bucketStart = bucketStart.UTC().Truncate(time.Hour)
	bucketEnd := bucketStart.Add(time.Hour)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin kpi rollup tx: %w", err)
	}
	defer tx.Rollback()

	metrics, err := s.workOrders.AggregateMetricsTx(ctx, tx, bucketStart, bucketEnd)
	if err != nil {
		return err
	}

	for i := range metrics {
		metrics[i].BucketStart = bucketStart
		metrics[i].BucketGranularity = "hour"
		if err := s.rollups.UpsertTx(ctx, tx, &metrics[i]); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit kpi rollup tx: %w", err)
	}

	return nil
}

func (s *KPIService) StartHourlyRollupTicker(ctx context.Context) {
	previousHour := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	_ = s.ComputeHourlyRollup(ctx, previousHour)

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			_ = s.ComputeHourlyRollup(ctx, t.UTC().Truncate(time.Hour).Add(-time.Hour))
		}
	}
}

func aggregateMetrics(hourly []domain.KPIServiceRollup, groupBy string) []domain.KPIServiceRollup {
	if groupBy == "hour" {
		return hourly
	}

	type key struct {
		bucketStart int64
		serviceType string
	}

	agg := make(map[key]*domain.KPIServiceRollup)
	for _, item := range hourly {
		bucket := item.BucketStart.UTC()
		switch groupBy {
		case "day":
			bucket = time.Date(bucket.Year(), bucket.Month(), bucket.Day(), 0, 0, 0, 0, time.UTC)
		case "week":
			bucket = weekStartUTC(bucket)
		case "service_type":
			bucket = time.Unix(0, 0).UTC()
		}

		k := key{bucketStart: bucket.Unix(), serviceType: item.ServiceType}
		existing, ok := agg[k]
		if !ok {
			existing = &domain.KPIServiceRollup{
				BucketStart:       bucket,
				BucketGranularity: groupBy,
				ServiceType:       item.ServiceType,
			}
			agg[k] = existing
		}

		existing.Total += item.Total
		existing.Completed += item.Completed
		existing.OnTime15m += item.OnTime15m
	}

	out := make([]domain.KPIServiceRollup, 0, len(agg))
	for _, item := range agg {
		item.ExecutionRate = computeExecutionRate(item.Total, item.Completed)
		item.TimelinessRate = computeTimelinessRate(item.Completed, item.OnTime15m)
		out = append(out, *item)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].BucketStart.Equal(out[j].BucketStart) {
			return out[i].ServiceType < out[j].ServiceType
		}
		return out[i].BucketStart.Before(out[j].BucketStart)
	})

	return out
}

func weekStartUTC(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := t.AddDate(0, 0, -(weekday - 1))
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func computeExecutionRate(total, completed int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) * 100 / float64(total)
}

func computeTimelinessRate(completed, onTime int64) float64 {
	if completed == 0 {
		return 0
	}
	return float64(onTime) * 100 / float64(completed)
}
