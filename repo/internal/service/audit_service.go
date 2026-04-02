package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type AuditService struct {
	auditEvents repository.AuditRepository
}

func NewAuditService(auditEvents repository.AuditRepository) *AuditService {
	return &AuditService{auditEvents: auditEvents}
}

type AuditLogInput struct {
	ActorID      *int64
	Action       string
	ResourceType string
	ResourceID   string
	Before       any
	After        any
	RequestID    string
}

func (s *AuditService) LogEvent(ctx context.Context, input AuditLogInput) error {
	return s.logEvent(ctx, nil, input)
}

func (s *AuditService) LogEventTx(ctx context.Context, tx *sql.Tx, input AuditLogInput) error {
	return s.logEvent(ctx, tx, input)
}

func (s *AuditService) logEvent(ctx context.Context, tx *sql.Tx, input AuditLogInput) error {
	now := time.Now().UTC()
	ctxMeta := AuditContext{}
	if v, ok := AuditContextFrom(ctx); ok {
		ctxMeta = v
	}

	if input.ActorID == nil && ctxMeta.OperatorID != nil {
		input.ActorID = ctxMeta.OperatorID
	}
	if strings.TrimSpace(input.RequestID) == "" {
		input.RequestID = strings.TrimSpace(ctxMeta.RequestID)
	}
	if strings.TrimSpace(input.RequestID) == "" {
		input.RequestID = "system"
	}

	metadata := map[string]any{
		"operator_id":       input.ActorID,
		"operator_username": strings.TrimSpace(ctxMeta.OperatorUsername),
		"local_ip":          strings.TrimSpace(ctxMeta.LocalIP),
		"request_id":        input.RequestID,
		"timestamp":         now.Format(time.RFC3339Nano),
	}

	beforeJSON, err := marshalAuditState(map[string]any{
		"state_before": input.Before,
		"metadata":     metadata,
	})
	if err != nil {
		return err
	}

	afterJSON, err := marshalAuditState(map[string]any{
		"state_after": input.After,
		"metadata":    metadata,
	})
	if err != nil {
		return err
	}

	var hashPrev *string
	if tx != nil {
		hashPrev, err = s.auditEvents.LastHashTx(ctx, tx)
	} else {
		hashPrev, err = s.auditEvents.LastHash(ctx)
	}
	if err != nil {
		return fmt.Errorf("get previous audit hash: %w", err)
	}

	occurredAt := now
	hashSelf := computeAuditHash(hashPrev, occurredAt, input.ActorID, input.Action, input.ResourceType, input.ResourceID, beforeJSON, afterJSON, input.RequestID)

	event := &domain.AuditEvent{
		OccurredAt:   occurredAt,
		ActorID:      input.ActorID,
		OperatorName: strings.TrimSpace(ctxMeta.OperatorUsername),
		LocalIP:      strings.TrimSpace(ctxMeta.LocalIP),
		Action:       input.Action,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		BeforeJSON:   beforeJSON,
		AfterJSON:    afterJSON,
		RequestID:    input.RequestID,
		HashPrev:     hashPrev,
		HashSelf:     hashSelf,
	}

	if tx != nil {
		err = s.auditEvents.AppendTx(ctx, tx, event)
	} else {
		err = s.auditEvents.Append(ctx, event)
	}
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}

	return nil
}

func marshalAuditState(payload map[string]any) (*string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal audit payload: %w", err)
	}
	text := string(raw)
	return &text, nil
}

func (s *AuditService) LogLoginSuccess(ctx context.Context, actorID int64, username, requestID, ip, userAgent string) error {
	return s.LogEvent(ctx, AuditLogInput{
		ActorID:      &actorID,
		Action:       "auth.login.success",
		ResourceType: "user",
		ResourceID:   username,
		After: map[string]any{
			"outcome":    "success",
			"ip":         ip,
			"user_agent": userAgent,
		},
		RequestID: requestID,
	})
}

func (s *AuditService) LogLoginFailure(ctx context.Context, actorID *int64, username, reason, requestID, ip, userAgent string) error {
	return s.LogEvent(ctx, AuditLogInput{
		ActorID:      actorID,
		Action:       "auth.login.failure",
		ResourceType: "user",
		ResourceID:   username,
		After: map[string]any{
			"outcome":    "failure",
			"reason":     reason,
			"ip":         ip,
			"user_agent": userAgent,
		},
		RequestID: requestID,
	})
}

func computeAuditHash(hashPrev *string, occurredAt time.Time, actorID *int64, action, resourceType, resourceID string, beforeJSON, afterJSON *string, requestID string) string {
	base := ""
	if hashPrev != nil {
		base = *hashPrev
	}

	actor := ""
	if actorID != nil {
		actor = strconv.FormatInt(*actorID, 10)
	}

	before := ""
	if beforeJSON != nil {
		before = *beforeJSON
	}

	after := ""
	if afterJSON != nil {
		after = *afterJSON
	}

	payload := base + "|" + occurredAt.Format(time.RFC3339Nano) + "|" + actor + "|" + action + "|" + resourceType + "|" + resourceID + "|" + before + "|" + after + "|" + requestID
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}
