package service

import (
	"context"
	"fmt"

	"clinic-admin-suite/internal/domain"
)

type auditContextKey struct{}

type AuditContext struct {
	OperatorID       *int64
	OperatorUsername string
	OperatorRole     string
	LocalIP          string
	RequestID        string
}

func WithAuditContext(ctx context.Context, values AuditContext) context.Context {
	return context.WithValue(ctx, auditContextKey{}, values)
}

func AuditContextFrom(ctx context.Context) (AuditContext, bool) {
	v := ctx.Value(auditContextKey{})
	item, ok := v.(AuditContext)
	if !ok {
		return AuditContext{}, false
	}
	return item, true
}

// RequireCallerPermission is a defense-in-depth check usable at the service
// layer. It extracts the caller's role from the AuditContext and verifies the
// required permission is granted. Fails closed: if the context or role is
// missing, access is denied rather than silently allowed.
func RequireCallerPermission(ctx context.Context, required domain.Permission) error {
	ac, ok := AuditContextFrom(ctx)
	if !ok || ac.OperatorRole == "" {
		// Fail closed: if no caller context is available, deny access to
		// privileged operations. Internal/system callers should set a
		// system-level AuditContext with an appropriate role.
		return fmt.Errorf("%w: caller context missing for privileged operation", ErrForbidden)
	}
	if !domain.HasPermissions(ac.OperatorRole, required) {
		return ErrForbidden
	}
	return nil
}

