package domain

import "time"

const (
	SettlementStatusMatched     = "matched"
	SettlementStatusDiscrepancy = "discrepancy"
)

type Settlement struct {
	ID                 int64     `json:"id"`
	ShiftID            string    `json:"shift_id"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	Status             string    `json:"status"`
	ExpectedTotalCents int64     `json:"expected_total_cents"`
	ActualTotalCents   int64     `json:"actual_total_cents"`
}

type SettlementItem struct {
	ID                int64   `json:"id"`
	SettlementID      int64   `json:"settlement_id"`
	PaymentID         int64   `json:"payment_id"`
	Result            string  `json:"result"`
	DiscrepancyReason *string `json:"discrepancy_reason,omitempty"`
}
