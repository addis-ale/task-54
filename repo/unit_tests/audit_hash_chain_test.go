package unit_tests

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"

	"strconv"
)

func TestAuditHashChainIntegrity(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	// Create a user for the FK constraint
	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	authSvc := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)
	if err := authSvc.EnsureBootstrapAdmin(ctx, "audituser", "StrongP@ss123!"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, _ := userRepo.GetByUsername(ctx, "audituser")

	// Create multiple audit events
	for i := 0; i < 5; i++ {
		actorID := user.ID
		err := auditService.LogEvent(ctx, service.AuditLogInput{
			ActorID:      &actorID,
			Action:       "test.action." + strconv.Itoa(i),
			ResourceType: "test_resource",
			ResourceID:   strconv.Itoa(i),
			After:        map[string]any{"value": i},
			RequestID:    "req_hash_chain_" + strconv.Itoa(i),
		})
		if err != nil {
			t.Fatalf("log event %d: %v", i, err)
		}
	}

	// Read all audit events and verify the hash chain linkage
	rows, err := db.QueryContext(ctx, `SELECT id, hash_prev, hash_self FROM audit_events ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	defer rows.Close()

	type chainEntry struct {
		id       int64
		hashPrev sql.NullString
		hashSelf string
	}

	var entries []chainEntry
	for rows.Next() {
		var e chainEntry
		if err := rows.Scan(&e.id, &e.hashPrev, &e.hashSelf); err != nil {
			t.Fatalf("scan: %v", err)
		}
		entries = append(entries, e)
	}

	if len(entries) < 5 {
		t.Fatalf("expected at least 5 audit events, got %d", len(entries))
	}

	// Verify chain: each event's hash_prev should equal the previous event's hash_self
	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		curr := entries[i]

		if !curr.hashPrev.Valid {
			t.Fatalf("event %d (id=%d): hash_prev should not be null", i, curr.id)
		}
		if curr.hashPrev.String != prev.hashSelf {
			t.Fatalf("event %d (id=%d): hash_prev (%s) does not match previous event's hash_self (%s)",
				i, curr.id, curr.hashPrev.String, prev.hashSelf)
		}
	}

	// Verify all hash_self values are non-empty and unique
	seen := map[string]bool{}
	for _, e := range entries {
		if e.hashSelf == "" {
			t.Fatalf("event id=%d: hash_self is empty", e.id)
		}
		if seen[e.hashSelf] {
			t.Fatalf("event id=%d: duplicate hash_self %s", e.id, e.hashSelf)
		}
		seen[e.hashSelf] = true
	}
}

func TestAuditHashChainAppendOnlyEnforced(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	// Create a user for the FK constraint
	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	authSvc := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)
	if err := authSvc.EnsureBootstrapAdmin(ctx, "appenduser", "StrongP@ss123!"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, _ := userRepo.GetByUsername(ctx, "appenduser")

	// Create audit events
	for i := 0; i < 3; i++ {
		actorID := user.ID
		err := auditService.LogEvent(ctx, service.AuditLogInput{
			ActorID:      &actorID,
			Action:       "append.test." + strconv.Itoa(i),
			ResourceType: "test",
			ResourceID:   strconv.Itoa(i),
			After:        map[string]any{"value": i},
			RequestID:    "req_append_" + strconv.Itoa(i),
		})
		if err != nil {
			t.Fatalf("log event %d: %v", i, err)
		}
	}

	// Attempt to tamper with an audit record - should fail due to append-only trigger
	_, err := db.ExecContext(ctx, `UPDATE audit_events SET after_json = '{"tampered":true}' WHERE id = 1`)
	if err == nil {
		t.Fatal("expected error when attempting to update append-only audit table")
	}

	// Attempt to delete an audit record - should also fail
	_, err = db.ExecContext(ctx, `DELETE FROM audit_events WHERE id = 1`)
	if err == nil {
		t.Fatal("expected error when attempting to delete from append-only audit table")
	}
}

func TestAuditHashChainFirstEventHasNilOrEmptyPrev(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	authSvc := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)
	if err := authSvc.EnsureBootstrapAdmin(ctx, "firstuser", "StrongP@ss123!"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, _ := userRepo.GetByUsername(ctx, "firstuser")

	// Log a first event explicitly
	actorID := user.ID
	if err := auditService.LogEvent(ctx, service.AuditLogInput{
		ActorID:      &actorID,
		Action:       "first.event",
		ResourceType: "test",
		ResourceID:   "1",
		After:        map[string]any{"test": true},
		RequestID:    "req_first",
	}); err != nil {
		t.Fatalf("log first event: %v", err)
	}

	// The very first event in the chain should have hash_prev = NULL
	var hashPrev sql.NullString
	err := db.QueryRowContext(ctx, `SELECT hash_prev FROM audit_events ORDER BY id ASC LIMIT 1`).Scan(&hashPrev)
	if err != nil {
		t.Fatalf("query first event hash_prev: %v", err)
	}
	// First event's hash_prev should either be NULL or empty
	if hashPrev.Valid && hashPrev.String != "" {
		t.Fatalf("first event hash_prev should be null/empty, got: %s", hashPrev.String)
	}
}
