package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
)

type CareService struct {
	db    *sql.DB
	audit *AuditService
}

func NewCareService(db *sql.DB, audit *AuditService) *CareService {
	return &CareService{db: db, audit: audit}
}

type CareCheckpointFilter struct {
	ResidentID *int64
	Status     string
	From       *time.Time
	To         *time.Time
}

type AlertEventFilter struct {
	ResidentID *int64
	Severity   string
	State      string
	From       *time.Time
	To         *time.Time
}

type CreateCheckpointInput struct {
	ResidentID     int64
	CheckpointType string
	Status         string
	Notes          string
	ActorID        *int64
	RequestID      string
}

type CreateAlertInput struct {
	ResidentID int64
	AlertType  string
	Severity   string
	State      string
	Message    string
	ActorID    *int64
	RequestID  string
}

func (s *CareService) CreateCheckpoint(ctx context.Context, input CreateCheckpointInput) (*domain.CareQualityCheckpoint, error) {
	if input.ResidentID <= 0 {
		return nil, fmt.Errorf("%w: resident_id must be positive", ErrValidation)
	}
	checkpointType := strings.TrimSpace(input.CheckpointType)
	if checkpointType == "" {
		return nil, fmt.Errorf("%w: checkpoint_type is required", ErrValidation)
	}
	status := strings.TrimSpace(strings.ToLower(input.Status))
	if status != "pass" && status != "watch" && status != "fail" {
		return nil, fmt.Errorf("%w: status must be pass, watch, or fail", ErrValidation)
	}

	// Verify resident exists
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM patients WHERE id = ?`, input.ResidentID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("%w: resident (patient) with id %d not found — create the patient first", ErrValidation, input.ResidentID)
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin checkpoint tx: %w", err)
	}
	defer tx.Rollback()

	var recordedBy sql.NullInt64
	if input.ActorID != nil {
		recordedBy = sql.NullInt64{Int64: *input.ActorID, Valid: true}
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO care_quality_checkpoints(resident_id, checkpoint_type, status, notes, recorded_by, created_at)
VALUES(?, ?, ?, ?, ?, ?)`, input.ResidentID, checkpointType, status, strings.TrimSpace(input.Notes), recordedBy, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create care checkpoint: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create care checkpoint last insert id: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.LogEventTx(ctx, tx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "care_checkpoint.create",
			ResourceType: "care_quality_checkpoint",
			ResourceID:   fmt.Sprintf("%d", id),
			After: map[string]any{
				"id":              id,
				"resident_id":     input.ResidentID,
				"checkpoint_type": checkpointType,
				"status":          status,
				"notes":           strings.TrimSpace(input.Notes),
			},
			RequestID: input.RequestID,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit care checkpoint tx: %w", err)
	}

	item := &domain.CareQualityCheckpoint{
		ID:             id,
		ResidentID:     input.ResidentID,
		CheckpointType: checkpointType,
		Status:         status,
		Notes:          strings.TrimSpace(input.Notes),
		RecordedBy:     input.ActorID,
		CreatedAt:      now,
	}
	return item, nil
}

func (s *CareService) ListCheckpoints(ctx context.Context, filter CareCheckpointFilter) ([]domain.CareQualityCheckpoint, error) {
	query := `SELECT id, resident_id, checkpoint_type, status, notes, recorded_by, created_at FROM care_quality_checkpoints WHERE 1=1`
	args := make([]any, 0, 6)

	if filter.ResidentID != nil {
		query += ` AND resident_id = ?`
		args = append(args, *filter.ResidentID)
	}
	if status := strings.TrimSpace(strings.ToLower(filter.Status)); status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if filter.From != nil {
		query += ` AND created_at >= ?`
		args = append(args, filter.From.UTC().Unix())
	}
	if filter.To != nil {
		query += ` AND created_at <= ?`
		args = append(args, filter.To.UTC().Unix())
	}
	query += ` ORDER BY created_at DESC LIMIT 200`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list care checkpoints: %w", err)
	}
	defer rows.Close()

	items := make([]domain.CareQualityCheckpoint, 0)
	for rows.Next() {
		var (
			item      domain.CareQualityCheckpoint
			recorded  sql.NullInt64
			createdAt int64
		)
		if err := rows.Scan(&item.ID, &item.ResidentID, &item.CheckpointType, &item.Status, &item.Notes, &recorded, &createdAt); err != nil {
			return nil, fmt.Errorf("scan care checkpoint row: %w", err)
		}
		if recorded.Valid {
			v := recorded.Int64
			item.RecordedBy = &v
		}
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate care checkpoint rows: %w", err)
	}
	return items, nil
}

func (s *CareService) CreateAlert(ctx context.Context, input CreateAlertInput) (*domain.AlertEvent, error) {
	if input.ResidentID <= 0 {
		return nil, fmt.Errorf("%w: resident_id must be positive", ErrValidation)
	}
	alertType := strings.TrimSpace(input.AlertType)
	if alertType == "" {
		return nil, fmt.Errorf("%w: alert_type is required", ErrValidation)
	}
	severity := strings.TrimSpace(strings.ToLower(input.Severity))
	if severity != "low" && severity != "medium" && severity != "high" && severity != "critical" {
		return nil, fmt.Errorf("%w: severity must be low, medium, high, or critical", ErrValidation)
	}
	state := strings.TrimSpace(strings.ToLower(input.State))
	if state == "" {
		state = "open"
	}
	if state != "open" && state != "acknowledged" && state != "resolved" {
		return nil, fmt.Errorf("%w: state must be open, acknowledged, or resolved", ErrValidation)
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, fmt.Errorf("%w: message is required", ErrValidation)
	}

	// Verify resident exists
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM patients WHERE id = ?`, input.ResidentID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("%w: resident (patient) with id %d not found — create the patient first", ErrValidation, input.ResidentID)
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin alert tx: %w", err)
	}
	defer tx.Rollback()

	var recordedBy sql.NullInt64
	if input.ActorID != nil {
		recordedBy = sql.NullInt64{Int64: *input.ActorID, Valid: true}
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO alert_events(resident_id, alert_type, severity, state, message, recorded_by, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?)`, input.ResidentID, alertType, severity, state, message, recordedBy, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create alert event: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create alert event last insert id: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.LogEventTx(ctx, tx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "alert_event.create",
			ResourceType: "alert_event",
			ResourceID:   fmt.Sprintf("%d", id),
			After: map[string]any{
				"id":          id,
				"resident_id": input.ResidentID,
				"alert_type":  alertType,
				"severity":    severity,
				"state":       state,
				"message":     message,
			},
			RequestID: input.RequestID,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit alert tx: %w", err)
	}

	item := &domain.AlertEvent{
		ID:         id,
		ResidentID: input.ResidentID,
		AlertType:  alertType,
		Severity:   severity,
		State:      state,
		Message:    message,
		RecordedBy: input.ActorID,
		CreatedAt:  now,
	}
	return item, nil
}

func (s *CareService) ListAlerts(ctx context.Context, filter AlertEventFilter) ([]domain.AlertEvent, error) {
	query := `SELECT id, resident_id, alert_type, severity, state, message, recorded_by, created_at FROM alert_events WHERE 1=1`
	args := make([]any, 0, 6)

	if filter.ResidentID != nil {
		query += ` AND resident_id = ?`
		args = append(args, *filter.ResidentID)
	}
	if sev := strings.TrimSpace(strings.ToLower(filter.Severity)); sev != "" {
		query += ` AND severity = ?`
		args = append(args, sev)
	}
	if st := strings.TrimSpace(strings.ToLower(filter.State)); st != "" {
		query += ` AND state = ?`
		args = append(args, st)
	}
	if filter.From != nil {
		query += ` AND created_at >= ?`
		args = append(args, filter.From.UTC().Unix())
	}
	if filter.To != nil {
		query += ` AND created_at <= ?`
		args = append(args, filter.To.UTC().Unix())
	}
	query += ` ORDER BY created_at DESC LIMIT 200`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list alert events: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AlertEvent, 0)
	for rows.Next() {
		var (
			item      domain.AlertEvent
			recorded  sql.NullInt64
			createdAt int64
		)
		if err := rows.Scan(&item.ID, &item.ResidentID, &item.AlertType, &item.Severity, &item.State, &item.Message, &recorded, &createdAt); err != nil {
			return nil, fmt.Errorf("scan alert row: %w", err)
		}
		if recorded.Valid {
			v := recorded.Int64
			item.RecordedBy = &v
		}
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert rows: %w", err)
	}
	return items, nil
}

func (s *CareService) Dashboard(ctx context.Context) (*domain.CareDashboardSummary, error) {
	var summary domain.CareDashboardSummary
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM care_quality_checkpoints`).Scan(&summary.CheckpointCount); err != nil {
		return nil, fmt.Errorf("count care checkpoints: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM alert_events WHERE state = 'open'`).Scan(&summary.AlertOpenCount); err != nil {
		return nil, fmt.Errorf("count open alerts: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM alert_events WHERE severity IN ('high', 'critical') AND state = 'open'`).Scan(&summary.AlertHighCount); err != nil {
		return nil, fmt.Errorf("count high alerts: %w", err)
	}
	return &summary, nil
}
