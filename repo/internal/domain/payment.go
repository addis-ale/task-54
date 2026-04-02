package domain

import "time"

const (
	PaymentStatusSucceeded = "succeeded"
	PaymentStatusFailed    = "failed"
	PaymentStatusVoided    = "voided"
)

type Payment struct {
	ID              int64     `json:"id"`
	ExternalRef     *string   `json:"external_ref,omitempty"`
	Method          string    `json:"method"`
	Gateway         string    `json:"gateway"`
	AmountCents     int64     `json:"amount_cents"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	ReceivedAt      time.Time `json:"received_at"`
	ShiftID         string    `json:"shift_id"`
	IdempotencyKey  *string   `json:"idempotency_key,omitempty"`
	Version         int64     `json:"version"`
	PIIReferenceEnc *string   `json:"-"`
	PIIKeyVersion   *int      `json:"-"`
	FailureReason   *string   `json:"failure_reason,omitempty"`
}

type PaymentEvent struct {
	ID        int64     `json:"id"`
	PaymentID int64     `json:"payment_id"`
	EventType string    `json:"event_type"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
