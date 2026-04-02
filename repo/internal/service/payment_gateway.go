package service

import (
	"context"
	"fmt"
	"time"
)

type GatewayChargeRequest struct {
	AmountCents int64
	Currency    string
	Method      string
	Metadata    map[string]any
}

type GatewayChargeResult struct {
	ExternalRef   string
	Succeeded     bool
	FailureReason string
	Payload       map[string]any
}

type GatewayAdapter interface {
	Name() string
	Charge(ctx context.Context, request GatewayChargeRequest) (*GatewayChargeResult, error)
}

type CashGatewayAdapter struct{}

func (a *CashGatewayAdapter) Name() string { return "cash_local" }

func (a *CashGatewayAdapter) Charge(_ context.Context, request GatewayChargeRequest) (*GatewayChargeResult, error) {
	return &GatewayChargeResult{
		ExternalRef: fmt.Sprintf("cash_%d", time.Now().UTC().UnixNano()),
		Succeeded:   true,
		Payload: map[string]any{
			"gateway":  a.Name(),
			"method":   request.Method,
			"approved": true,
		},
	}, nil
}

type LocalCardGatewayAdapter struct{}

func (a *LocalCardGatewayAdapter) Name() string { return "card_local" }

func (a *LocalCardGatewayAdapter) Charge(_ context.Context, request GatewayChargeRequest) (*GatewayChargeResult, error) {
	approved := request.AmountCents <= 5_000_000
	reason := ""
	if !approved {
		reason = "limit_exceeded"
	}
	return &GatewayChargeResult{
		ExternalRef:   fmt.Sprintf("card_%d", time.Now().UTC().UnixNano()),
		Succeeded:     approved,
		FailureReason: reason,
		Payload: map[string]any{
			"gateway":  a.Name(),
			"method":   request.Method,
			"approved": approved,
			"reason":   reason,
		},
	}, nil
}

type CheckGatewayAdapter struct{}

func (a *CheckGatewayAdapter) Name() string { return "check_local" }

func (a *CheckGatewayAdapter) Charge(_ context.Context, request GatewayChargeRequest) (*GatewayChargeResult, error) {
	return &GatewayChargeResult{
		ExternalRef: fmt.Sprintf("check_%d", time.Now().UTC().UnixNano()),
		Succeeded:   true,
		Payload: map[string]any{
			"gateway":  a.Name(),
			"method":   request.Method,
			"approved": true,
		},
	}, nil
}

type FacilityChargeGatewayAdapter struct{}

func (a *FacilityChargeGatewayAdapter) Name() string { return "facility_charge_local" }

func (a *FacilityChargeGatewayAdapter) Charge(_ context.Context, request GatewayChargeRequest) (*GatewayChargeResult, error) {
	approved := request.AmountCents <= 20_000_000
	reason := ""
	if !approved {
		reason = "facility_charge_limit_exceeded"
	}
	return &GatewayChargeResult{
		ExternalRef:   fmt.Sprintf("fac_%d", time.Now().UTC().UnixNano()),
		Succeeded:     approved,
		FailureReason: reason,
		Payload: map[string]any{
			"gateway":  a.Name(),
			"method":   request.Method,
			"approved": approved,
			"reason":   reason,
		},
	}, nil
}

type ImportedCardBatchGatewayAdapter struct{}

func (a *ImportedCardBatchGatewayAdapter) Name() string { return "imported_card_batch_local" }

func (a *ImportedCardBatchGatewayAdapter) Charge(_ context.Context, request GatewayChargeRequest) (*GatewayChargeResult, error) {
	return &GatewayChargeResult{
		ExternalRef: fmt.Sprintf("batch_%d", time.Now().UTC().UnixNano()),
		Succeeded:   true,
		Payload: map[string]any{
			"gateway":  a.Name(),
			"method":   request.Method,
			"approved": true,
		},
	}, nil
}
