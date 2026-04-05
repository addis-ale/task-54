package unit_tests

import (
	"context"
	"strings"
	"testing"

	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

// TestAuditRedactsSensitiveFieldsInState verifies that sensitive fields such as
// password, token, payer_reference, and card_number are redacted (masked) in the
// before/after JSON stored in audit_events.
func TestAuditRedactsSensitiveFieldsInState(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Create a user so the foreign key constraint is satisfied
	_, err := db.ExecContext(ctx, `INSERT INTO users(username, password_hash, role, created_at, updated_at) VALUES('testuser', '$2a$12$dummy', 'admin', strftime('%s','now'), strftime('%s','now'))`)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	var actorID int64
	if err = db.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'testuser'`).Scan(&actorID); err != nil {
		t.Fatalf("get user id: %v", err)
	}

	auditService := service.NewAuditService(sqlite.NewAuditRepository(db))
	err = auditService.LogEvent(ctx, service.AuditLogInput{
		ActorID:      &actorID,
		Action:       "user.update",
		ResourceType: "user",
		ResourceID:   "42",
		Before: map[string]any{
			"username":     "oldname",
			"password":     "secret-old-password",
			"role":         "admin",
			"session_token": "abc123token",
		},
		After: map[string]any{
			"username":        "newname",
			"password":        "secret-new-password",
			"payer_reference": "CC-4242-1234-5678",
			"card_number":     "4111111111111111",
			"role":            "admin",
		},
		RequestID: "test-redaction",
	})
	if err != nil {
		t.Fatalf("log audit event: %v", err)
	}

	// Read back the audit event
	var beforeJSON, afterJSON *string
	err = db.QueryRowContext(ctx, `SELECT before_json, after_json FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&beforeJSON, &afterJSON)
	if err != nil {
		t.Fatalf("query audit event: %v", err)
	}

	if beforeJSON == nil || afterJSON == nil {
		t.Fatalf("expected non-nil before/after JSON")
	}

	// Before JSON should NOT contain the actual password or token
	if strings.Contains(*beforeJSON, "secret-old-password") {
		t.Fatalf("before_json contains unredacted password: %s", *beforeJSON)
	}
	if strings.Contains(*beforeJSON, "abc123token") {
		t.Fatalf("before_json contains unredacted session_token: %s", *beforeJSON)
	}

	// After JSON should NOT contain actual sensitive values
	if strings.Contains(*afterJSON, "secret-new-password") {
		t.Fatalf("after_json contains unredacted password: %s", *afterJSON)
	}
	if strings.Contains(*afterJSON, "CC-4242-1234-5678") {
		t.Fatalf("after_json contains unredacted payer_reference: %s", *afterJSON)
	}
	if strings.Contains(*afterJSON, "4111111111111111") {
		t.Fatalf("after_json contains unredacted card_number: %s", *afterJSON)
	}

	// Verify the redaction marker is present
	if !strings.Contains(*beforeJSON, "***REDACTED***") {
		t.Fatalf("expected ***REDACTED*** marker in before_json: %s", *beforeJSON)
	}
	if !strings.Contains(*afterJSON, "***REDACTED***") {
		t.Fatalf("expected ***REDACTED*** marker in after_json: %s", *afterJSON)
	}
}
