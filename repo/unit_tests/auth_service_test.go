package unit_tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestPasswordPolicyRejectsShortPasswords(t *testing.T) {
	if err := service.ValidatePasswordPolicy("Short1!abc"); err == nil {
		t.Fatal("expected error for password under 12 chars")
	}
}

func TestPasswordPolicyRejectsMissingUppercase(t *testing.T) {
	if err := service.ValidatePasswordPolicy("alllowercase1!x"); err == nil {
		t.Fatal("expected error for password missing uppercase")
	}
}

func TestPasswordPolicyRejectsMissingDigit(t *testing.T) {
	if err := service.ValidatePasswordPolicy("AllLettersHere!"); err == nil {
		t.Fatal("expected error for password missing digit")
	}
}

func TestPasswordPolicyRejectsMissingSpecial(t *testing.T) {
	if err := service.ValidatePasswordPolicy("AllLetters1234X"); err == nil {
		t.Fatal("expected error for password missing special character")
	}
}

func TestPasswordPolicyAcceptsStrongPassword(t *testing.T) {
	if err := service.ValidatePasswordPolicy("StrongP@ss123!"); err != nil {
		t.Fatalf("expected valid password, got error: %v", err)
	}
}

func TestSessionExpiryAfter15MinutesInactivity(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)

	password := "StrongP@ss123!"
	if err := authService.EnsureBootstrapAdmin(ctx, "testadmin", password); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	result, err := authService.Login(ctx, service.LoginInput{
		Username:  "testadmin",
		Password:  password,
		RequestID: "req_test_session",
		IP:        "127.0.0.1",
		UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Token should be valid immediately
	user, _, err := authService.AuthenticateToken(ctx, result.Token)
	if err != nil {
		t.Fatalf("authenticate valid token: %v", err)
	}
	if user.Username != "testadmin" {
		t.Fatalf("expected testadmin, got %s", user.Username)
	}

	// Manually expire the session by manipulating DB
	_, err = db.ExecContext(ctx, `UPDATE sessions SET expires_at = ?, last_seen_at = ? WHERE user_id = ?`,
		time.Now().UTC().Add(-1*time.Minute).Unix(),
		time.Now().UTC().Add(-16*time.Minute).Unix(),
		user.ID,
	)
	if err != nil {
		t.Fatalf("expire session: %v", err)
	}

	// Token should now be invalid
	_, _, err = authService.AuthenticateToken(ctx, result.Token)
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated for expired session, got: %v", err)
	}
}

func TestAccountLockoutAfter5Failures(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)

	password := "StrongP@ss123!"
	if err := authService.EnsureBootstrapAdmin(ctx, "locktest", password); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	// 5 failed attempts
	for i := 0; i < 5; i++ {
		_, err := authService.Login(ctx, service.LoginInput{
			Username:  "locktest",
			Password:  "wrongpassword123",
			RequestID: "req_fail",
			IP:        "127.0.0.1",
			UserAgent: "test",
		})
		if !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got: %v", i+1, err)
		}
	}

	// 6th attempt should show account locked
	_, err := authService.Login(ctx, service.LoginInput{
		Username:  "locktest",
		Password:  "wrongpassword123",
		RequestID: "req_locked",
		IP:        "127.0.0.1",
		UserAgent: "test",
	})

	var lockedErr *service.AccountLockedError
	if !errors.As(err, &lockedErr) {
		t.Fatalf("expected AccountLockedError after 5+ failures, got: %v", err)
	}

	if lockedErr.Until.Before(time.Now().UTC()) {
		t.Fatalf("lockout should be in the future, got: %v", lockedErr.Until)
	}
}

func TestAccountLockoutExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)

	password := "StrongP@ss123!"
	if err := authService.EnsureBootstrapAdmin(ctx, "backofftest", password); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	// Clear lockout between attempts by resetting timestamp
	for i := 0; i < 7; i++ {
		// Clear any active lockout
		_, _ = db.ExecContext(ctx, `UPDATE users SET locked_until = NULL WHERE username = 'backofftest'`)

		_, err := authService.Login(ctx, service.LoginInput{
			Username:  "backofftest",
			Password:  "wrongpassword123",
			RequestID: "req_backoff",
			IP:        "127.0.0.1",
			UserAgent: "test",
		})
		if err == nil {
			t.Fatal("expected error for wrong password")
		}
	}

	// After 7 attempts, check lockout duration is > 30 seconds (exponential backoff)
	var lockedUntil int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(locked_until, 0) FROM users WHERE username = 'backofftest'`).Scan(&lockedUntil)
	if err != nil {
		t.Fatalf("query locked_until: %v", err)
	}
	if lockedUntil == 0 {
		t.Fatal("expected non-zero locked_until")
	}
	lockedTime := time.Unix(lockedUntil, 0)
	if lockedTime.Before(time.Now().UTC().Add(30 * time.Second)) {
		t.Fatalf("expected lockout duration > 30s for 7+ failures, got: %v", lockedTime)
	}
}

func TestTokenValidationRejectsExpiredTokens(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	// Very short TTL for testing
	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 1*time.Second)

	password := "StrongP@ss123!"
	if err := authService.EnsureBootstrapAdmin(ctx, "expiretest", password); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	result, err := authService.Login(ctx, service.LoginInput{
		Username:  "expiretest",
		Password:  password,
		RequestID: "req_expire",
		IP:        "127.0.0.1",
		UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	_, _, err = authService.AuthenticateToken(ctx, result.Token)
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated for expired token, got: %v", err)
	}
}

func TestTokenValidationRejectsEmptyToken(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)

	_, _, err := authService.AuthenticateToken(ctx, "")
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated for empty token, got: %v", err)
	}
}

func TestTokenValidationRejectsBogusToken(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)

	_, _, err := authService.AuthenticateToken(ctx, "completely-bogus-token-value")
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated for bogus token, got: %v", err)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	auditService := service.NewAuditService(auditRepo)

	authService := service.NewAuthService(userRepo, sessionRepo, auditService, 4, 15*time.Minute)

	password := "StrongP@ss123!"
	if err := authService.EnsureBootstrapAdmin(ctx, "logouttest", password); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}

	result, err := authService.Login(ctx, service.LoginInput{
		Username:  "logouttest",
		Password:  password,
		RequestID: "req_logout",
		IP:        "127.0.0.1",
		UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Token valid before logout
	_, _, err = authService.AuthenticateToken(ctx, result.Token)
	if err != nil {
		t.Fatalf("token should be valid before logout: %v", err)
	}

	// Logout
	if err := authService.Logout(ctx, result.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// Token should be invalid after logout
	_, _, err = authService.AuthenticateToken(ctx, result.Token)
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated after logout, got: %v", err)
	}
}
