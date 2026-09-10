package unit_tests

import (
	"archive/zip"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"clinic-admin-suite/internal/service"
)

// TestDiagnosticsBundleContainsExpectedEntries verifies that the diagnostics
// ZIP bundle includes logs, schema migrations, health snapshot (with audit chain
// and config), and recent job results.
func TestDiagnosticsBundleContainsExpectedEntries(t *testing.T) {
	ctx := adminCtx()
	db := setupTestDB(t)

	logPath := filepath.Join(t.TempDir(), "structured.log")
	logs := service.NewStructuredLogService(logPath)
	_ = logs.Log("info", "test.event", map[string]any{"msg": "diagnostics test"})

	diagRoot := filepath.Join(t.TempDir(), "diagnostics")
	diagService := service.NewDiagnosticsService(db, logPath, diagRoot)

	export, err := diagService.Export(ctx)
	if err != nil {
		t.Fatalf("export diagnostics: %v", err)
	}

	reader, err := zip.OpenReader(export.BundlePath)
	if err != nil {
		t.Fatalf("open diagnostics zip: %v", err)
	}
	defer reader.Close()

	expectedEntries := map[string]bool{
		"logs/structured.log":      false,
		"schema/migrations.json":   false,
		"health/snapshot.json":     false,
		"jobs/recent_results.json": false,
		"config/snapshots.json":    false,
	}

	for _, f := range reader.File {
		if _, ok := expectedEntries[f.Name]; ok {
			expectedEntries[f.Name] = true
		}
	}

	for entry, found := range expectedEntries {
		if !found {
			t.Errorf("missing expected diagnostics bundle entry: %s", entry)
		}
	}

	// Verify health snapshot contains audit_chain and table_counts
	for _, f := range reader.File {
		if f.Name != "health/snapshot.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open health snapshot: %v", err)
		}
		var health map[string]any
		if err := json.NewDecoder(rc).Decode(&health); err != nil {
			rc.Close()
			t.Fatalf("decode health snapshot: %v", err)
		}
		rc.Close()

		if _, ok := health["db_status"]; !ok {
			t.Error("health snapshot missing db_status")
		}
		if _, ok := health["audit_chain"]; !ok {
			t.Error("health snapshot missing audit_chain")
		}
		if _, ok := health["table_counts"]; !ok {
			t.Error("health snapshot missing table_counts")
		}
	}

	// Verify schema migrations contains entries
	for _, f := range reader.File {
		if f.Name != "schema/migrations.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open schema migrations: %v", err)
		}
		var schema map[string]any
		if err := json.NewDecoder(rc).Decode(&schema); err != nil {
			rc.Close()
			t.Fatalf("decode schema migrations: %v", err)
		}
		rc.Close()

		migrations, ok := schema["migrations"].([]any)
		if !ok || len(migrations) == 0 {
			t.Error("schema migrations should contain at least one migration entry")
		}
	}

	// Verify the bundle path contains a real file
	if !strings.HasSuffix(export.FileName, ".zip") {
		t.Errorf("expected .zip filename, got %s", export.FileName)
	}
}
