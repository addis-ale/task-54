package service

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"clinic-admin-suite/internal/domain"
)

type DiagnosticsService struct {
	db                *sql.DB
	structuredLogPath string
	outputRoot        string
}

func NewDiagnosticsService(db *sql.DB, structuredLogPath, outputRoot string) *DiagnosticsService {
	return &DiagnosticsService{
		db:                db,
		structuredLogPath: structuredLogPath,
		outputRoot:        outputRoot,
	}
}

type DiagnosticsExport struct {
	ExportID   string
	BundlePath string
	FileName   string
}

func (s *DiagnosticsService) Export(ctx context.Context) (*DiagnosticsExport, error) {
	// Defense-in-depth: verify caller has diagnostics.export permission at service layer
	if err := RequireCallerPermission(ctx, domain.PermissionDiagnosticsExport); err != nil {
		return nil, fmt.Errorf("%w: diagnostics.export permission required", ErrForbidden)
	}
	if s.outputRoot == "" {
		s.outputRoot = filepath.Join(".", "data", "diagnostics")
	}

	if err := os.MkdirAll(s.outputRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create diagnostics output directory: %w", err)
	}

	exportID := fmt.Sprintf("diag_%d", time.Now().UTC().UnixNano())
	fileName := exportID + ".zip"
	bundlePath := filepath.Join(s.outputRoot, fileName)

	outFile, err := os.Create(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("create diagnostics bundle file: %w", err)
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)

	if err := s.addStructuredLogs(zipWriter); err != nil {
		zipWriter.Close()
		return nil, err
	}

	if err := s.addSchemaVersions(ctx, zipWriter); err != nil {
		zipWriter.Close()
		return nil, err
	}

	if err := s.addHealthSnapshot(ctx, zipWriter); err != nil {
		zipWriter.Close()
		return nil, err
	}

	if err := s.addRecentJobResults(ctx, zipWriter); err != nil {
		zipWriter.Close()
		return nil, err
	}

	if err := s.addConfigSnapshots(ctx, zipWriter); err != nil {
		zipWriter.Close()
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("finalize diagnostics zip: %w", err)
	}

	return &DiagnosticsExport{
		ExportID:   exportID,
		BundlePath: bundlePath,
		FileName:   fileName,
	}, nil
}

func (s *DiagnosticsService) addStructuredLogs(zipWriter *zip.Writer) error {
	entry, err := zipWriter.Create("logs/structured.log")
	if err != nil {
		return fmt.Errorf("create log entry in diagnostics bundle: %w", err)
	}

	if s.structuredLogPath == "" {
		_, err = entry.Write([]byte("{\"message\":\"structured log path not configured\"}\n"))
		return err
	}

	logFile, err := os.Open(s.structuredLogPath)
	if err != nil {
		_, writeErr := entry.Write([]byte(fmt.Sprintf("{\"message\":\"structured log unavailable\",\"error\":%q}\n", err.Error())))
		if writeErr != nil {
			return fmt.Errorf("write fallback structured log entry: %w", writeErr)
		}
		return nil
	}
	defer logFile.Close()

	if _, err := io.Copy(entry, logFile); err != nil {
		return fmt.Errorf("copy structured logs to diagnostics bundle: %w", err)
	}

	return nil
}

func (s *DiagnosticsService) addSchemaVersions(ctx context.Context, zipWriter *zip.Writer) error {
	rows, err := s.db.QueryContext(ctx, `SELECT name, applied_at FROM schema_migrations ORDER BY name ASC`)
	if err != nil {
		return fmt.Errorf("query schema migrations: %w", err)
	}
	defer rows.Close()

	type row struct {
		Name      string `json:"name"`
		AppliedAt int64  `json:"applied_at"`
	}
	items := make([]row, 0)
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.Name, &item.AppliedAt); err != nil {
			return fmt.Errorf("scan schema migration row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate schema migration rows: %w", err)
	}

	entry, err := zipWriter.Create("schema/migrations.json")
	if err != nil {
		return fmt.Errorf("create schema entry in diagnostics bundle: %w", err)
	}

	raw, err := json.MarshalIndent(map[string]any{"migrations": items}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema migration payload: %w", err)
	}

	if _, err := entry.Write(raw); err != nil {
		return fmt.Errorf("write schema payload to diagnostics bundle: %w", err)
	}

	return nil
}

func (s *DiagnosticsService) addHealthSnapshot(ctx context.Context, zipWriter *zip.Writer) error {
	health := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.db.PingContext(ctx); err != nil {
		health["db_status"] = "down"
		health["db_error"] = err.Error()
	} else {
		health["db_status"] = "up"
	}

	tableCounts := make(map[string]int64)
	for _, table := range []string{"users", "audit_events", "job_runs", "payments", "settlements", "exam_schedules"} {
		var count int64
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+table).Scan(&count); err == nil {
			tableCounts[table] = count
		}
	}

	keys := make([]string, 0, len(tableCounts))
	for k := range tableCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sortedCounts := make(map[string]int64, len(keys))
	for _, k := range keys {
		sortedCounts[k] = tableCounts[k]
	}
	health["table_counts"] = sortedCounts

	auditChain, err := s.verifyAuditChain(ctx)
	if err != nil {
		health["audit_chain"] = map[string]any{"status": "error", "error": err.Error()}
	} else {
		health["audit_chain"] = auditChain
	}

	entry, err := zipWriter.Create("health/snapshot.json")
	if err != nil {
		return fmt.Errorf("create health entry in diagnostics bundle: %w", err)
	}

	raw, err := json.MarshalIndent(health, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal health snapshot payload: %w", err)
	}

	if _, err := entry.Write(raw); err != nil {
		return fmt.Errorf("write health snapshot payload: %w", err)
	}

	return nil
}

func (s *DiagnosticsService) verifyAuditChain(ctx context.Context) (map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, hash_prev, hash_self FROM audit_events ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query audit chain rows: %w", err)
	}
	defer rows.Close()

	var (
		count      int64
		previous   *string
		brokenAtID *int64
	)

	for rows.Next() {
		var (
			id       int64
			hashPrev sql.NullString
			hashSelf string
		)
		if err := rows.Scan(&id, &hashPrev, &hashSelf); err != nil {
			return nil, fmt.Errorf("scan audit chain row: %w", err)
		}
		count++

		if previous == nil {
			if hashPrev.Valid {
				brokenAtID = &id
				break
			}
		} else {
			if !hashPrev.Valid || hashPrev.String != *previous {
				brokenAtID = &id
				break
			}
		}

		prev := hashSelf
		previous = &prev
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit chain rows: %w", err)
	}

	if brokenAtID != nil {
		return map[string]any{
			"status":       "broken",
			"record_count": count,
			"broken_at_id": *brokenAtID,
		}, nil
	}

	return map[string]any{
		"status":       "ok",
		"record_count": count,
	}, nil
}

func (s *DiagnosticsService) addRecentJobResults(ctx context.Context, zipWriter *zip.Writer) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, job_type, started_at, finished_at, status, summary_json, failure_root_cause_notes FROM job_runs ORDER BY started_at DESC LIMIT 50`)
	if err != nil {
		return fmt.Errorf("query recent job runs: %w", err)
	}
	defer rows.Close()

	type jobRow struct {
		ID                    int64   `json:"id"`
		JobType               string  `json:"job_type"`
		StartedAt             int64   `json:"started_at"`
		FinishedAt            int64   `json:"finished_at"`
		Status                string  `json:"status"`
		SummaryJSON           *string `json:"summary_json,omitempty"`
		FailureRootCauseNotes *string `json:"failure_root_cause_notes,omitempty"`
	}
	items := make([]jobRow, 0)
	for rows.Next() {
		var item jobRow
		var summary sql.NullString
		var rootCause sql.NullString
		if err := rows.Scan(&item.ID, &item.JobType, &item.StartedAt, &item.FinishedAt, &item.Status, &summary, &rootCause); err != nil {
			return fmt.Errorf("scan job run row: %w", err)
		}
		if summary.Valid {
			item.SummaryJSON = &summary.String
		}
		if rootCause.Valid {
			item.FailureRootCauseNotes = &rootCause.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate job run rows: %w", err)
	}

	entry, err := zipWriter.Create("jobs/recent_results.json")
	if err != nil {
		return fmt.Errorf("create jobs entry in diagnostics bundle: %w", err)
	}

	raw, err := json.MarshalIndent(map[string]any{"job_runs": items}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job runs payload: %w", err)
	}

	if _, err := entry.Write(raw); err != nil {
		return fmt.Errorf("write job runs payload to diagnostics bundle: %w", err)
	}

	return nil
}

func (s *DiagnosticsService) addConfigSnapshots(ctx context.Context, zipWriter *zip.Writer) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, config_key, config_payload_json, created_by, created_at, is_active FROM config_versions ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		// config_versions table might not have data yet — write empty snapshot
		entry, createErr := zipWriter.Create("config/snapshots.json")
		if createErr != nil {
			return fmt.Errorf("create config entry in diagnostics bundle: %w", createErr)
		}
		_, _ = entry.Write([]byte(`{"config_versions":[],"note":"query error: ` + err.Error() + `"}`))
		return nil
	}
	defer rows.Close()

	type configRow struct {
		ID         int64   `json:"id"`
		ConfigKey  string  `json:"config_key"`
		Payload    string  `json:"payload_json"`
		CreatedBy  *int64  `json:"created_by,omitempty"`
		CreatedAt  int64   `json:"created_at"`
		IsActive   int     `json:"is_active"`
	}
	items := make([]configRow, 0)
	for rows.Next() {
		var item configRow
		var createdBy sql.NullInt64
		if err := rows.Scan(&item.ID, &item.ConfigKey, &item.Payload, &createdBy, &item.CreatedAt, &item.IsActive); err != nil {
			return fmt.Errorf("scan config version row: %w", err)
		}
		if createdBy.Valid {
			item.CreatedBy = &createdBy.Int64
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate config version rows: %w", err)
	}

	entry, err := zipWriter.Create("config/snapshots.json")
	if err != nil {
		return fmt.Errorf("create config entry in diagnostics bundle: %w", err)
	}

	raw, err := json.MarshalIndent(map[string]any{"config_versions": items}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config snapshots payload: %w", err)
	}

	if _, err := entry.Write(raw); err != nil {
		return fmt.Errorf("write config snapshots to diagnostics bundle: %w", err)
	}

	return nil
}
