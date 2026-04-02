package handler

import (
	"strconv"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type PaymentsHandler struct {
	payments *service.PaymentService
}

func NewPaymentsHandler(payments *service.PaymentService) *PaymentsHandler {
	return &PaymentsHandler{payments: payments}
}

type createPaymentRequest struct {
	Method         string `json:"method"`
	Gateway        string `json:"gateway"`
	AmountCents    int64  `json:"amount_cents"`
	Currency       string `json:"currency"`
	ShiftID        string `json:"shift_id"`
	PIIReference   string `json:"pii_reference"`
	IdempotencyKey string `json:"idempotency_key"`
}

type refundPaymentRequest struct {
	AmountCents    int64  `json:"amount_cents"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (h *PaymentsHandler) List(c *fiber.Ctx) error {
	items, err := h.payments.List(c.UserContext(), repository.PaymentFilter{
		Status:  strings.TrimSpace(c.Query("status")),
		Method:  strings.TrimSpace(c.Query("method")),
		Gateway: strings.TrimSpace(c.Query("gateway")),
		ShiftID: strings.TrimSpace(c.Query("shift_id")),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to list payments")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"payments": items})
}

func (h *PaymentsHandler) Create(c *fiber.Ctx) error {
	var req createPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	payment, err := h.payments.Create(c.UserContext(), service.CreatePaymentInput{
		Method:         req.Method,
		Gateway:        req.Gateway,
		AmountCents:    req.AmountCents,
		Currency:       req.Currency,
		ShiftID:        req.ShiftID,
		PIIReference:   req.PIIReference,
		IdempotencyKey: req.IdempotencyKey,
		ActorID:        currentActorIDFromContext(c),
		RequestID:      httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create payment")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"payment": payment})
}

func (h *PaymentsHandler) Refund(c *fiber.Ctx) error {
	paymentID, err := strconv.ParseInt(c.Params("payment_id"), 10, 64)
	if err != nil || paymentID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "payment_id must be a positive integer", nil)
	}

	var req refundPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	refund, err := h.payments.Refund(c.UserContext(), service.RefundPaymentInput{
		PaymentID:      paymentID,
		AmountCents:    req.AmountCents,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		ActorID:        currentActorIDFromContext(c),
		RequestID:      httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to refund payment")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"refund": refund})
}
