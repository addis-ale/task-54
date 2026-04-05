package unit_tests

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository/migrations"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "unit-tests.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := migrations.Run(context.Background(), db); err != nil {
		_ = db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

// adminCtx returns a context with admin-level AuditContext for service-layer
// defense-in-depth checks in unit tests.
func adminCtx() context.Context {
	return service.WithAuditContext(context.Background(), service.AuditContext{
		OperatorUsername: "test-admin",
		OperatorRole:    string(domain.RoleAdmin),
		RequestID:       "unit-test",
	})
}
