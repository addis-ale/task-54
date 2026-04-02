package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type PaymentService struct {
	db            *sql.DB
	payments      repository.PaymentRepository
	paymentEvents repository.PaymentEventRepository
	audit         *AuditService
	fieldCipher   *FieldCipher
	keyVersion    int
	adapters      map[string]GatewayAdapter
	logs          *StructuredLogService
}

func NewPaymentService(
	db *sql.DB,
	payments repository.PaymentRepository,
	paymentEvents repository.PaymentEventRepository,
	audit *AuditService,
	fieldCipher *FieldCipher,
	keyVersion int,
	adapters []GatewayAdapter,
	logs *StructuredLogService,
) *PaymentService {
	adapterMap := make(map[string]GatewayAdapter)
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		adapterMap[adapter.Name()] = adapter
	}
	return &PaymentService{
		db:            db,
		payments:      payments,
		paymentEvents: paymentEvents,
		audit:         audit,
		fieldCipher:   fieldCipher,
		keyVersion:    keyVersion,
		adapters:      adapterMap,
		logs:          logs,
	}
}

type CreatePaymentInput struct {
	Method         string
	Gateway        string
	AmountCents    int64
	Currency       string
	ShiftID        string
	PIIReference   string
	IdempotencyKey string
	ActorID        *int64
	RequestID      string
}

type RefundPaymentInput struct {
	PaymentID      int64
	AmountCents    int64
	Reason         string
	IdempotencyKey string
	ActorID        *int64
	RequestID      string
}

func (s *PaymentService) Create(ctx context.Context, input CreatePaymentInput) (*domain.Payment, error) {
	method := strings.TrimSpace(strings.ToLower(input.Method))
	if method == "" {
		return nil, fmt.Errorf("%w: payment method is required", ErrValidation)
	}
	gatewayName := strings.TrimSpace(strings.ToLower(input.Gateway))
	adapter, ok := s.adapters[gatewayName]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported payment gateway", ErrValidation)
	}
	if input.AmountCents <= 0 {
		return nil, fmt.Errorf("%w: amount_cents must be positive", ErrValidation)
	}
	currency := strings.TrimSpace(strings.ToUpper(input.Currency))
	if currency == "" {
		currency = "USD"
	}
	shiftID, err := normalizeShiftID(input.ShiftID)
	if err != nil {
		return nil, err
	}

	var piiEnc *string
	var piiKeyVersion *int
	if strings.TrimSpace(input.PIIReference) != "" {
		if s.fieldCipher == nil {
			return nil, fmt.Errorf("%w: field encryption key not configured", ErrValidation)
		}
		encrypted, err := s.fieldCipher.Encrypt([]byte(strings.TrimSpace(input.PIIReference)))
		if err != nil {
			return nil, err
		}
		piiEnc = &encrypted
		version := s.keyVersion
		if version <= 0 {
			version = 1
		}
		piiKeyVersion = &version
	}

	chargeResult, err := adapter.Charge(ctx, GatewayChargeRequest{
		AmountCents: input.AmountCents,
		Currency:    currency,
		Method:      method,
		Metadata: map[string]any{
			"shift_id": shiftID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gateway charge failed: %w", err)
	}

	now := time.Now().UTC()
	status := domain.PaymentStatusSucceeded
	var failureReason *string
	if !chargeResult.Succeeded {
		status = domain.PaymentStatusFailed
		reason := strings.TrimSpace(chargeResult.FailureReason)
		if reason == "" {
			reason = "gateway_rejected"
		}
		failureReason = &reason
	}

	externalRef := strings.TrimSpace(chargeResult.ExternalRef)
	var externalRefPtr *string
	if externalRef != "" {
		externalRefPtr = &externalRef
	}

	var idemPtr *string
	if strings.TrimSpace(input.IdempotencyKey) != "" {
		v := strings.TrimSpace(input.IdempotencyKey)
		idemPtr = &v
	}

	payment := &domain.Payment{
		ExternalRef:     externalRefPtr,
		Method:          method,
		Gateway:         gatewayName,
		AmountCents:     input.AmountCents,
		Currency:        currency,
		Status:          status,
		ReceivedAt:      now,
		ShiftID:         shiftID,
		IdempotencyKey:  idemPtr,
		PIIReferenceEnc: piiEnc,
		PIIKeyVersion:   piiKeyVersion,
		FailureReason:   failureReason,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin payment tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.payments.CreateTx(ctx, tx, payment); err != nil {
		return nil, err
	}

	eventPayload, err := json.Marshal(map[string]any{
		"gateway_result": chargeResult.Payload,
		"status":         status,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal payment event payload: %w", err)
	}

	eventType := "payment.succeeded"
	if status == domain.PaymentStatusFailed {
		eventType = "payment.failed"
	}

	if err := s.paymentEvents.CreateTx(ctx, tx, &domain.PaymentEvent{
		PaymentID: payment.ID,
		EventType: eventType,
		Payload:   string(eventPayload),
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	resourceID := fmt.Sprintf("%d", payment.ID)
	if err := s.audit.LogEventTx(ctx, tx, AuditLogInput{
		ActorID:      input.ActorID,
		Action:       "payment.create",
		ResourceType: "payment",
		ResourceID:   resourceID,
		After: map[string]any{
			"gateway":      payment.Gateway,
			"method":       payment.Method,
			"status":       payment.Status,
			"amount_cents": payment.AmountCents,
			"currency":     payment.Currency,
			"shift_id":     payment.ShiftID,
		},
		RequestID: fallbackRequestID(input.RequestID),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit payment tx: %w", err)
	}

	if s.logs != nil {
		_ = s.logs.Log("info", "payment.create", map[string]any{
			"payment_id":   payment.ID,
			"gateway":      payment.Gateway,
			"method":       payment.Method,
			"status":       payment.Status,
			"amount_cents": payment.AmountCents,
			"currency":     payment.Currency,
			"shift_id":     payment.ShiftID,
			"request_id":   fallbackRequestID(input.RequestID),
		})
	}

	return s.payments.GetByID(ctx, payment.ID)
}

func (s *PaymentService) List(ctx context.Context, filter repository.PaymentFilter) ([]domain.Payment, error) {
	return s.payments.List(ctx, filter)
}

func (s *PaymentService) Refund(ctx context.Context, input RefundPaymentInput) (*domain.Payment, error) {
	if input.PaymentID <= 0 {
		return nil, fmt.Errorf("%w: payment_id must be a positive integer", ErrValidation)
	}
	if input.AmountCents <= 0 {
		return nil, fmt.Errorf("%w: amount_cents must be positive", ErrValidation)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrValidation)
	}

	original, err := s.payments.GetByID(ctx, input.PaymentID)
	if err != nil {
		return nil, err
	}
	if original.Status != domain.PaymentStatusSucceeded {
		return nil, fmt.Errorf("%w: only succeeded payments can be refunded", ErrConflict)
	}
	if original.AmountCents <= 0 {
		return nil, fmt.Errorf("%w: refund source payment amount is invalid", ErrConflict)
	}
	if input.AmountCents > original.AmountCents {
		return nil, fmt.Errorf("%w: refund amount exceeds original payment amount", ErrValidation)
	}

	now := time.Now().UTC()
	externalRef := fmt.Sprintf("refund_%d_%d", original.ID, now.UnixNano())

	var idemPtr *string
	if strings.TrimSpace(input.IdempotencyKey) != "" {
		v := strings.TrimSpace(input.IdempotencyKey)
		idemPtr = &v
	}

	refundPayment := &domain.Payment{
		ExternalRef:    &externalRef,
		Method:         "refund",
		Gateway:        original.Gateway,
		AmountCents:    -input.AmountCents,
		Currency:       original.Currency,
		Status:         domain.PaymentStatusSucceeded,
		ReceivedAt:     now,
		ShiftID:        original.ShiftID,
		IdempotencyKey: idemPtr,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin refund tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.payments.CreateTx(ctx, tx, refundPayment); err != nil {
		return nil, err
	}

	eventPayload, err := json.Marshal(map[string]any{
		"original_payment_id": original.ID,
		"refund_payment_id":   refundPayment.ID,
		"amount_cents":        input.AmountCents,
		"reason":              reason,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal refund event payload: %w", err)
	}

	if err := s.paymentEvents.CreateTx(ctx, tx, &domain.PaymentEvent{
		PaymentID: original.ID,
		EventType: "payment.refund_issued",
		Payload:   string(eventPayload),
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	if err := s.paymentEvents.CreateTx(ctx, tx, &domain.PaymentEvent{
		PaymentID: refundPayment.ID,
		EventType: "payment.refund_created",
		Payload:   string(eventPayload),
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}

	if err := s.audit.LogEventTx(ctx, tx, AuditLogInput{
		ActorID:      input.ActorID,
		Action:       "payment.refund",
		ResourceType: "payment",
		ResourceID:   fmt.Sprintf("%d", original.ID),
		After: map[string]any{
			"original_payment_id": original.ID,
			"refund_payment_id":   refundPayment.ID,
			"amount_cents":        input.AmountCents,
			"reason":              reason,
		},
		RequestID: fallbackRequestID(input.RequestID),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit refund tx: %w", err)
	}

	if s.logs != nil {
		_ = s.logs.Log("info", "payment.refund", map[string]any{
			"original_payment_id": original.ID,
			"refund_payment_id":   refundPayment.ID,
			"amount_cents":        input.AmountCents,
			"reason":              reason,
			"gateway":             original.Gateway,
			"request_id":          fallbackRequestID(input.RequestID),
		})
	}

	return s.payments.GetByID(ctx, refundPayment.ID)
}

func fallbackRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "system"
	}
	return requestID
}
