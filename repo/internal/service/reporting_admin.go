package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
)

type AuditSearchFilter struct {
	ResidentID *int64
	RecordType string
	From       *time.Time
	To         *time.Time
	Limit      int
}

func (s *ReportService) SearchAudit(ctx context.Context, filter AuditSearchFilter) ([]domain.AuditTrailRecord, error) {
	if s.db == nil {
		return nil, fmt.Errorf("%w: reporting database is not configured", ErrValidation)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	query := `
SELECT id, occurred_at, actor_id, operator_username, local_ip, action, resource_type, resource_id, before_json, after_json, request_id
FROM audit_events
WHERE 1=1`
	args := make([]any, 0, 8)

	if v := strings.TrimSpace(filter.RecordType); v != "" {
		query += ` AND resource_type = ?`
		args = append(args, v)
	}
	if filter.From != nil {
		query += ` AND occurred_at >= ?`
		args = append(args, filter.From.UTC().Unix())
	}
	if filter.To != nil {
		query += ` AND occurred_at <= ?`
		args = append(args, filter.To.UTC().Unix())
	}
	if filter.ResidentID != nil {
		query += ` AND (
resource_id = ?
OR json_extract(before_json, '$.state_before.resident_id') = ?
OR json_extract(after_json, '$.state_after.resident_id') = ?
OR json_extract(before_json, '$.state_before.patient_id') = ?
OR json_extract(after_json, '$.state_after.patient_id') = ?
)`
		args = append(args, *filter.ResidentID, *filter.ResidentID, *filter.ResidentID, *filter.ResidentID, *filter.ResidentID)
	}

	query += ` ORDER BY occurred_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search audit events: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AuditTrailRecord, 0)
	for rows.Next() {
		var (
			item     domain.AuditTrailRecord
			occurred int64
			actorID  sql.NullInt64
			before   sql.NullString
			after    sql.NullString
			operator sql.NullString
			localIP  sql.NullString
			resource sql.NullString
		)
		if err := rows.Scan(&item.ID, &occurred, &actorID, &operator, &localIP, &item.Action, &item.ResourceType, &resource, &before, &after, &item.RequestID); err != nil {
			return nil, fmt.Errorf("scan audit search row: %w", err)
		}
		item.OccurredAt = time.Unix(occurred, 0).UTC()
		if actorID.Valid {
			v := actorID.Int64
			item.ActorID = &v
		}
		if before.Valid {
			v := before.String
			item.BeforeJSON = &v
		}
		if after.Valid {
			v := after.String
			item.AfterJSON = &v
		}
		if operator.Valid {
			item.OperatorName = operator.String
		}
		if localIP.Valid {
			item.LocalIP = localIP.String
		}
		if resource.Valid {
			item.ResourceID = resource.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit search rows: %w", err)
	}

	return items, nil
}

func (s *ReportService) ExportAudit(ctx context.Context, filter AuditSearchFilter, format string) (*ReportFile, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "xlsx" {
		return nil, fmt.Errorf("%w: format must be csv or xlsx", ErrValidation)
	}

	rows, err := s.SearchAudit(ctx, filter)
	if err != nil {
		return nil, err
	}

	headers := []string{"id", "occurred_at", "actor_id", "operator_name", "local_ip", "action", "record_type", "record_id", "request_id"}
	body := make([][]string, 0, len(rows))
	for _, item := range rows {
		actor := ""
		if item.ActorID != nil {
			actor = strconv.FormatInt(*item.ActorID, 10)
		}
		body = append(body, []string{
			strconv.FormatInt(item.ID, 10),
			item.OccurredAt.Format(time.RFC3339),
			actor,
			item.OperatorName,
			item.LocalIP,
			item.Action,
			item.ResourceType,
			item.ResourceID,
			item.RequestID,
		})
	}

	timestamp := time.Now().UTC().Format("20060102_150405")
	if format == "csv" {
		var raw strings.Builder
		raw.WriteString(strings.Join(headers, ",") + "\n")
		for _, line := range body {
			raw.WriteString(strings.Join(escapeCSVLine(line), ",") + "\n")
		}
		return &ReportFile{FileName: fmt.Sprintf("audit_report_%s.csv", timestamp), ContentType: "text/csv", Body: []byte(raw.String())}, nil
	}

	xlsx, err := buildXLSX(headers, body)
	if err != nil {
		return nil, err
	}
	return &ReportFile{FileName: fmt.Sprintf("audit_report_%s.xlsx", timestamp), ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx}, nil
}

func escapeCSVLine(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		if strings.ContainsAny(v, ",\n\"") {
			out[i] = `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
		} else {
			out[i] = v
		}
	}
	return out
}

type CreateReportScheduleInput struct {
	ReportType      string
	Format          string
	SharedFolder    string
	FiltersJSON     string
	IntervalMinutes int
	FirstRunAt      time.Time
	ActorID         *int64
	RequestID       string
}

func (s *ReportService) CreateSchedule(ctx context.Context, input CreateReportScheduleInput) (*domain.ReportSchedule, error) {
	reportType := strings.TrimSpace(strings.ToLower(input.ReportType))
	if reportType != "finance" && reportType != "audit" {
		return nil, fmt.Errorf("%w: report_type must be finance or audit", ErrValidation)
	}
	format := strings.TrimSpace(strings.ToLower(input.Format))
	if format != "csv" && format != "xlsx" {
		return nil, fmt.Errorf("%w: format must be csv or xlsx", ErrValidation)
	}
	if input.IntervalMinutes < 5 {
		return nil, fmt.Errorf("%w: interval_minutes must be >= 5", ErrValidation)
	}
	if input.FirstRunAt.IsZero() {
		input.FirstRunAt = time.Now().UTC().Add(5 * time.Minute)
	}
	shared := strings.TrimSpace(input.SharedFolder)
	if shared == "" {
		shared = s.sharedRoot
	}
	now := time.Now().UTC()

	result, err := s.db.ExecContext(ctx, `
INSERT INTO report_schedules(report_type, format, shared_folder_path, filters_json, interval_minutes, next_run_at, enabled, created_by, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, reportType, format, shared, strings.TrimSpace(input.FiltersJSON), input.IntervalMinutes, input.FirstRunAt.UTC().Unix(), nullableInt64(input.ActorID), now.Unix(), now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create report schedule: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create report schedule last insert id: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.LogEvent(ctx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "report_schedule.create",
			ResourceType: "report_schedule",
			ResourceID:   fmt.Sprintf("%d", id),
			After: map[string]any{
				"report_type":      reportType,
				"format":           format,
				"shared_folder":    shared,
				"interval_minutes": input.IntervalMinutes,
				"next_run_at":      input.FirstRunAt.UTC().Format(time.RFC3339),
			},
			RequestID: input.RequestID,
		})
	}

	return &domain.ReportSchedule{
		ID:               id,
		ReportType:       reportType,
		Format:           format,
		SharedFolderPath: shared,
		FiltersJSON:      strings.TrimSpace(input.FiltersJSON),
		IntervalMinutes:  input.IntervalMinutes,
		NextRunAt:        input.FirstRunAt.UTC(),
		Enabled:          true,
		CreatedBy:        input.ActorID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *ReportService) ListSchedules(ctx context.Context) ([]domain.ReportSchedule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, report_type, format, shared_folder_path, filters_json, interval_minutes, next_run_at, enabled, created_by, created_at, updated_at
FROM report_schedules
ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list report schedules: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ReportSchedule, 0)
	for rows.Next() {
		var (
			item      domain.ReportSchedule
			nextRun   int64
			createdAt int64
			updatedAt int64
			enabled   int64
			createdBy sql.NullInt64
			filters   sql.NullString
		)
		if err := rows.Scan(&item.ID, &item.ReportType, &item.Format, &item.SharedFolderPath, &filters, &item.IntervalMinutes, &nextRun, &enabled, &createdBy, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan report schedule row: %w", err)
		}
		item.NextRunAt = time.Unix(nextRun, 0).UTC()
		item.Enabled = enabled == 1
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		if filters.Valid {
			item.FiltersJSON = filters.String
		}
		if createdBy.Valid {
			v := createdBy.Int64
			item.CreatedBy = &v
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report schedule rows: %w", err)
	}

	return items, nil
}

func (s *ReportService) RunDueSchedules(ctx context.Context, now time.Time) error {
	schedules, err := s.ListSchedules(ctx)
	if err != nil {
		return err
	}

	for _, schedule := range schedules {
		if !schedule.Enabled || schedule.NextRunAt.After(now.UTC()) {
			continue
		}
		start := time.Now().UTC()
		err := s.runSchedule(ctx, schedule)
		finish := time.Now().UTC()

		status := "completed"
		summary := map[string]any{"schedule_id": schedule.ID, "report_type": schedule.ReportType}
		var rootCause string
		if err != nil {
			status = "failed"
			summary["error"] = err.Error()
			rootCause = err.Error()
		}
		if s.jobRuns != nil {
			_ = s.jobRuns.Record(ctx, JobRunInput{JobType: "report_schedule", StartedAt: start, FinishedAt: finish, Status: status, Summary: summary, FailureRootCauseNotes: rootCause})
		}

		nextRun := now.UTC().Add(time.Duration(schedule.IntervalMinutes) * time.Minute)
		if _, updateErr := s.db.ExecContext(ctx, `UPDATE report_schedules SET next_run_at = ?, updated_at = ? WHERE id = ?`, nextRun.Unix(), time.Now().UTC().Unix(), schedule.ID); updateErr != nil {
			return fmt.Errorf("update next run for schedule %d: %w", schedule.ID, updateErr)
		}
	}

	return nil
}

func (s *ReportService) runSchedule(ctx context.Context, schedule domain.ReportSchedule) error {
	filters := map[string]any{}
	if strings.TrimSpace(schedule.FiltersJSON) != "" {
		_ = json.Unmarshal([]byte(schedule.FiltersJSON), &filters)
	}

	var file *ReportFile
	var err error
	if schedule.ReportType == "audit" {
		filter := AuditSearchFilter{}
		if v, ok := toInt64(filters["resident_id"]); ok {
			filter.ResidentID = &v
		}
		if v, ok := filters["record_type"].(string); ok {
			filter.RecordType = v
		}
		file, err = s.ExportAudit(ctx, filter, schedule.Format)
	} else {
		file, err = s.ExportFinance(ctx, ExportFinanceInput{
			Format:  schedule.Format,
			Status:  toString(filters["status"]),
			Method:  toString(filters["method"]),
			Gateway: toString(filters["gateway"]),
			ShiftID: toString(filters["shift_id"]),
		})
	}
	if err != nil {
		return err
	}

	targetRoot := strings.TrimSpace(schedule.SharedFolderPath)
	if targetRoot == "" {
		targetRoot = s.sharedRoot
	}
	if targetRoot == "" {
		targetRoot = "./data/shared_reports"
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("create shared reports path: %w", err)
	}

	filePath := filepath.Join(targetRoot, file.FileName)
	if err := os.WriteFile(filePath, file.Body, 0o644); err != nil {
		return fmt.Errorf("write scheduled report: %w", err)
	}

	if s.logs != nil {
		_ = s.logs.Log("info", "report.schedule.run", map[string]any{"schedule_id": schedule.ID, "report_type": schedule.ReportType, "file": filePath})
	}

	return nil
}

type CreateConfigVersionInput struct {
	ConfigKey   string
	PayloadJSON string
	ActorID     *int64
	RequestID   string
}

func (s *ReportService) CreateConfigVersion(ctx context.Context, input CreateConfigVersionInput) (*domain.ConfigVersion, error) {
	key := strings.TrimSpace(input.ConfigKey)
	if key == "" {
		return nil, fmt.Errorf("%w: config_key is required", ErrValidation)
	}
	payload := strings.TrimSpace(input.PayloadJSON)
	if payload == "" {
		return nil, fmt.Errorf("%w: payload_json is required", ErrValidation)
	}
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin config version tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE config_versions SET is_active = 0 WHERE config_key = ?`, key); err != nil {
		return nil, fmt.Errorf("deactivate existing config versions: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO config_versions(config_key, config_payload_json, created_by, created_at, is_active)
VALUES(?, ?, ?, ?, 1)`, key, payload, nullableInt64(input.ActorID), now.Unix())
	if err != nil {
		return nil, fmt.Errorf("create config version: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create config version last insert id: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.LogEventTx(ctx, tx, AuditLogInput{
			ActorID:      input.ActorID,
			Action:       "config.version.create",
			ResourceType: "config_version",
			ResourceID:   fmt.Sprintf("%d", id),
			After: map[string]any{
				"config_key": key,
				"is_active":  true,
			},
			RequestID: input.RequestID,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit config version tx: %w", err)
	}

	return &domain.ConfigVersion{ID: id, ConfigKey: key, ConfigPayloadJSON: payload, CreatedBy: input.ActorID, CreatedAt: now, IsActive: true}, nil
}

func (s *ReportService) ListConfigVersions(ctx context.Context, key string) ([]domain.ConfigVersion, error) {
	query := `SELECT id, config_key, config_payload_json, created_by, created_at, is_active FROM config_versions`
	args := make([]any, 0, 1)
	if strings.TrimSpace(key) != "" {
		query += ` WHERE config_key = ?`
		args = append(args, strings.TrimSpace(key))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list config versions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ConfigVersion, 0)
	for rows.Next() {
		var (
			item      domain.ConfigVersion
			createdBy sql.NullInt64
			createdAt int64
			active    int64
		)
		if err := rows.Scan(&item.ID, &item.ConfigKey, &item.ConfigPayloadJSON, &createdBy, &createdAt, &active); err != nil {
			return nil, fmt.Errorf("scan config version row: %w", err)
		}
		item.CreatedAt = time.Unix(createdAt, 0).UTC()
		item.IsActive = active == 1
		if createdBy.Valid {
			v := createdBy.Int64
			item.CreatedBy = &v
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate config version rows: %w", err)
	}
	return items, nil
}

func (s *ReportService) RollbackConfigVersion(ctx context.Context, versionID int64, actorID *int64, requestID string) (*domain.ConfigVersion, error) {
	if versionID <= 0 {
		return nil, fmt.Errorf("%w: version_id must be positive", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin rollback tx: %w", err)
	}
	defer tx.Rollback()

	var (
		item      domain.ConfigVersion
		createdBy sql.NullInt64
		createdAt int64
	)
	err = tx.QueryRowContext(ctx, `SELECT id, config_key, config_payload_json, created_by, created_at FROM config_versions WHERE id = ?`, versionID).Scan(&item.ID, &item.ConfigKey, &item.ConfigPayloadJSON, &createdBy, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: config version not found", ErrNotFound)
		}
		return nil, fmt.Errorf("get config version: %w", err)
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	if createdBy.Valid {
		v := createdBy.Int64
		item.CreatedBy = &v
	}

	if _, err := tx.ExecContext(ctx, `UPDATE config_versions SET is_active = 0 WHERE config_key = ?`, item.ConfigKey); err != nil {
		return nil, fmt.Errorf("deactivate config versions by key: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE config_versions SET is_active = 1 WHERE id = ?`, versionID); err != nil {
		return nil, fmt.Errorf("activate rollback config version: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.LogEventTx(ctx, tx, AuditLogInput{
			ActorID:      actorID,
			Action:       "config.version.rollback",
			ResourceType: "config_version",
			ResourceID:   fmt.Sprintf("%d", versionID),
			After: map[string]any{
				"config_key":                item.ConfigKey,
				"rolled_back_to_version_id": versionID,
			},
			RequestID: requestID,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rollback tx: %w", err)
	}

	item.IsActive = true
	return &item, nil
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func nullableInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
