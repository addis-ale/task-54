package handler

import (
	"strings"
	"time"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type KPIHandler struct {
	kpis *service.KPIService
}

func NewKPIHandler(kpis *service.KPIService) *KPIHandler {
	return &KPIHandler{kpis: kpis}
}

func (h *KPIHandler) ServiceDelivery(c *fiber.Ctx) error {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if fromRaw := strings.TrimSpace(c.Query("from")); fromRaw != "" {
		parsed, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "from must be RFC3339 timestamp", nil)
		}
		from = parsed.UTC()
	}

	if toRaw := strings.TrimSpace(c.Query("to")); toRaw != "" {
		parsed, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "to must be RFC3339 timestamp", nil)
		}
		to = parsed.UTC()
	}

	groupBy := strings.TrimSpace(c.Query("group_by"))
	items, err := h.kpis.QueryServiceDelivery(c.UserContext(), service.ServiceDeliveryQuery{
		From:    from,
		To:      to,
		GroupBy: groupBy,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to query service delivery KPIs")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"group_by": groupByOrDefault(groupBy),
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"items":    items,
	})
}

func groupByOrDefault(groupBy string) string {
	v := strings.TrimSpace(groupBy)
	if v == "" {
		return "hour"
	}
	return strings.ToLower(v)
}
