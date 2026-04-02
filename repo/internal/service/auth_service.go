package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrUnauthenticated    = errors.New("auth: unauthenticated")
)

type AccountLockedError struct {
	Until time.Time
}

func (e *AccountLockedError) Error() string {
	return fmt.Sprintf("auth: account locked until %s", e.Until.Format(time.RFC3339))
}

type AuthService struct {
	users      repository.UserRepository
	sessions   repository.SessionRepository
	audit      *AuditService
	bcryptCost int
	sessionTTL time.Duration
}

func NewAuthService(users repository.UserRepository, sessions repository.SessionRepository, audit *AuditService, bcryptCost int, sessionTTL time.Duration) *AuthService {
	if bcryptCost < bcrypt.MinCost {
		bcryptCost = bcrypt.DefaultCost
	}
	if sessionTTL <= 0 {
		sessionTTL = 15 * time.Minute
	}

	return &AuthService{
		users:      users,
		sessions:   sessions,
		audit:      audit,
		bcryptCost: bcryptCost,
		sessionTTL: sessionTTL,
	}
}

type LoginInput struct {
	Username  string
	Password  string
	RequestID string
	IP        string
	UserAgent string
}

type LoginResult struct {
	User       *domain.User
	Token      string
	ExpiresAt  time.Time
	Permission []domain.Permission
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	now := time.Now().UTC()
	username := strings.TrimSpace(input.Username)

	if username == "" || input.Password == "" {
		if err := s.audit.LogLoginFailure(ctx, nil, username, "missing_credentials", input.RequestID, input.IP, input.UserAgent); err != nil {
			return nil, err
		}
		return nil, ErrInvalidCredentials
	}

	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			if logErr := s.audit.LogLoginFailure(ctx, nil, username, "invalid_credentials", input.RequestID, input.IP, input.UserAgent); logErr != nil {
				return nil, logErr
			}
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		if err := s.audit.LogLoginFailure(ctx, &user.ID, username, "account_locked", input.RequestID, input.IP, input.UserAgent); err != nil {
			return nil, err
		}
		return nil, &AccountLockedError{Until: *user.LockedUntil}
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		failedCount := user.FailedLoginCount + 1
		lockedUntil := calculateLockout(now, failedCount)

		if updateErr := s.users.RecordFailedLogin(ctx, user.ID, failedCount, lockedUntil); updateErr != nil {
			return nil, fmt.Errorf("record failed login: %w", updateErr)
		}

		reason := "invalid_credentials"
		if lockedUntil != nil {
			reason = "lockout_backoff"
		}

		if auditErr := s.audit.LogLoginFailure(ctx, &user.ID, username, reason, input.RequestID, input.IP, input.UserAgent); auditErr != nil {
			return nil, auditErr
		}

		return nil, ErrInvalidCredentials
	}

	if err := s.users.ResetFailedLogins(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("reset failed login state: %w", err)
	}

	rawToken, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}

	expiresAt := now.Add(s.sessionTTL)
	session := &domain.Session{
		UserID:     user.ID,
		TokenHash:  hashToken(rawToken),
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
		LastSeenAt: now,
	}

	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	if err := s.audit.LogLoginSuccess(ctx, user.ID, user.Username, input.RequestID, input.IP, input.UserAgent); err != nil {
		return nil, err
	}

	return &LoginResult{
		User:       user,
		Token:      rawToken,
		ExpiresAt:  expiresAt,
		Permission: domain.PermissionsForRole(user.Role),
	}, nil
}

func (s *AuthService) AuthenticateToken(ctx context.Context, rawToken string) (*domain.User, *domain.Session, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, nil, ErrUnauthenticated
	}

	now := time.Now().UTC()
	session, user, err := s.sessions.GetActiveByTokenHash(ctx, hashToken(rawToken), now)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, ErrUnauthenticated
		}
		return nil, nil, fmt.Errorf("fetch active session: %w", err)
	}

	newExpiresAt := now.Add(s.sessionTTL)
	if err := s.sessions.TouchActivity(ctx, session.ID, now, newExpiresAt); err != nil {
		return nil, nil, fmt.Errorf("touch session activity: %w", err)
	}
	session.LastSeenAt = now
	session.ExpiresAt = newExpiresAt

	return user, session, nil
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil
	}
	return s.sessions.DeleteByTokenHash(ctx, hashToken(rawToken))
}

func (s *AuthService) EnsureBootstrapAdmin(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil
	}

	if err := ValidatePasswordPolicy(password); err != nil {
		return fmt.Errorf("bootstrap admin password policy: %w", err)
	}

	_, err := s.users.GetByUsername(ctx, username)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("check bootstrap admin user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("hash bootstrap admin password: %w", err)
	}

	user := &domain.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         string(domain.RoleAdmin),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return fmt.Errorf("create bootstrap admin user: %w", err)
	}

	return nil
}

func calculateLockout(now time.Time, failedCount int) *time.Time {
	if failedCount < 5 {
		return nil
	}

	backoffSeconds := 30 * (1 << (failedCount - 5))
	if backoffSeconds > 1800 {
		backoffSeconds = 1800
	}

	lockedUntil := now.Add(time.Duration(backoffSeconds) * time.Second)
	return &lockedUntil
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
