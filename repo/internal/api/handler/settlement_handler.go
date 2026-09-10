package handler

import (
	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type SettlementHandler struct {
	settlements *service.SettlementService
}

func NewSettlementHandler(settlements *service.SettlementService) *SettlementHandler {
	return &SettlementHandler{settlements: settlements}
}

type runSettlementRequest struct {
	ShiftID          string `json:"shift_id"`
	ActualTotalCents int64  `json:"actual_total_cents"`
}

func (h *SettlementHandler) Run(c *fiber.Ctx) error {
	var req runSettlementRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	result, err := h.settlements.RunShift(c.UserContext(), service.RunSettlementInput{
		ShiftID:          req.ShiftID,
		ActualTotalCents: req.ActualTotalCents,
		ActorID:          currentActorIDFromContext(c),
		RequestID:        httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to run settlement")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"settlement":        result.Settlement,
		"items":             result.Items,
		"discrepancy_cents": result.Discrepancy,
	})
}
