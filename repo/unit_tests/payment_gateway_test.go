package unit_tests

import (
	"context"
	"testing"

	"clinic-admin-suite/internal/service"
)

func TestCashGatewayAdapter(t *testing.T) {
	adapter := &service.CashGatewayAdapter{}
	if adapter.Name() != "cash_local" {
		t.Fatalf("expected name cash_local, got %s", adapter.Name())
	}

	result, err := adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 5000,
		Currency:    "USD",
		Method:      "cash",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if !result.Succeeded {
		t.Fatal("expected cash charge to succeed")
	}
	if result.ExternalRef == "" {
		t.Fatal("expected non-empty external ref")
	}
	if result.Payload == nil {
		t.Fatal("expected non-nil payload")
	}
	if result.Payload["gateway"] != "cash_local" {
		t.Fatalf("expected gateway=cash_local in payload, got %v", result.Payload["gateway"])
	}
}

func TestCheckGatewayAdapter(t *testing.T) {
	adapter := &service.CheckGatewayAdapter{}
	if adapter.Name() != "check_local" {
		t.Fatalf("expected name check_local, got %s", adapter.Name())
	}

	result, err := adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 15000,
		Currency:    "USD",
		Method:      "check",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if !result.Succeeded {
		t.Fatal("expected check charge to succeed")
	}
	if result.ExternalRef == "" {
		t.Fatal("expected non-empty external ref")
	}
}

func TestFacilityChargeGatewayAdapter(t *testing.T) {
	adapter := &service.FacilityChargeGatewayAdapter{}
	if adapter.Name() != "facility_charge_local" {
		t.Fatalf("expected name facility_charge_local, got %s", adapter.Name())
	}

	// Within limit
	result, err := adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 10_000_000,
		Currency:    "USD",
		Method:      "facility_charge",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if !result.Succeeded {
		t.Fatal("expected facility charge within limit to succeed")
	}

	// Over limit
	result, err = adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 25_000_000,
		Currency:    "USD",
		Method:      "facility_charge",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if result.Succeeded {
		t.Fatal("expected facility charge over limit to fail")
	}
	if result.FailureReason == "" {
		t.Fatal("expected non-empty failure reason for over-limit charge")
	}
}

func TestLocalCardGatewayAdapter(t *testing.T) {
	adapter := &service.LocalCardGatewayAdapter{}
	if adapter.Name() != "card_local" {
		t.Fatalf("expected name card_local, got %s", adapter.Name())
	}

	// Within limit
	result, err := adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 1000,
		Currency:    "USD",
		Method:      "card",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if !result.Succeeded {
		t.Fatal("expected card charge within limit to succeed")
	}

	// Over limit
	result, err = adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 6_000_000,
		Currency:    "USD",
		Method:      "card",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if result.Succeeded {
		t.Fatal("expected card charge over limit to fail")
	}
	if result.FailureReason != "limit_exceeded" {
		t.Fatalf("expected limit_exceeded failure reason, got: %s", result.FailureReason)
	}
}

func TestImportedCardBatchGatewayAdapter(t *testing.T) {
	adapter := &service.ImportedCardBatchGatewayAdapter{}
	if adapter.Name() != "imported_card_batch_local" {
		t.Fatalf("expected name imported_card_batch_local, got %s", adapter.Name())
	}

	result, err := adapter.Charge(context.Background(), service.GatewayChargeRequest{
		AmountCents: 99999,
		Currency:    "USD",
		Method:      "imported_card_batch",
	})
	if err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if !result.Succeeded {
		t.Fatal("expected imported card batch charge to succeed")
	}
	if result.ExternalRef == "" {
		t.Fatal("expected non-empty external ref")
	}
	if result.Payload["gateway"] != "imported_card_batch_local" {
		t.Fatalf("expected gateway=imported_card_batch_local in payload, got %v", result.Payload["gateway"])
	}
}

func TestGatewayAdaptersHandleZeroAmount(t *testing.T) {
	adapters := []service.GatewayAdapter{
		&service.CashGatewayAdapter{},
		&service.CheckGatewayAdapter{},
		&service.FacilityChargeGatewayAdapter{},
		&service.LocalCardGatewayAdapter{},
		&service.ImportedCardBatchGatewayAdapter{},
	}

	for _, adapter := range adapters {
		result, err := adapter.Charge(context.Background(), service.GatewayChargeRequest{
			AmountCents: 0,
			Currency:    "USD",
			Method:      "test",
		})
		if err != nil {
			t.Fatalf("adapter %s: unexpected error for zero amount: %v", adapter.Name(), err)
		}
		// Zero amount should still return a result
		if result == nil {
			t.Fatalf("adapter %s: expected non-nil result for zero amount", adapter.Name())
		}
	}
}
