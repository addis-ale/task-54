package service

import "context"

type auditContextKey struct{}

type AuditContext struct {
	OperatorID       *int64
	OperatorUsername string
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
