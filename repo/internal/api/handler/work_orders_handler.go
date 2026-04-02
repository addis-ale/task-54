package handler

import (
	"strconv"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/api/middleware"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type WorkOrdersHandler struct {
	workOrders *service.WorkOrderService
}

func NewWorkOrdersHandler(workOrders *service.WorkOrderService) *WorkOrdersHandler {
	return &WorkOrdersHandler{workOrders: workOrders}
}

type createWorkOrderRequest struct {
	ServiceType string `json:"service_type"`
	Priority    string `json:"priority"`
	AssigneeID  *int64 `json:"assignee_id"`
}

func (h *WorkOrdersHandler) List(c *fiber.Ctx) error {
	filter := repository.WorkOrderFilter{
		Status:      strings.TrimSpace(c.Query("status")),
		Priority:    strings.TrimSpace(c.Query("priority")),
		ServiceType: strings.TrimSpace(c.Query("service_type")),
	}

	if assignedToRaw := strings.TrimSpace(c.Query("assigned_to")); assignedToRaw != "" {
		assignedTo, err := strconv.ParseInt(assignedToRaw, 10, 64)
		if err != nil || assignedTo <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "assigned_to must be a positive integer", nil)
		}
		filter.AssignedTo = &assignedTo
	}

	items, err := h.workOrders.List(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to list work orders")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"work_orders": items})
}

func (h *WorkOrdersHandler) Create(c *fiber.Ctx) error {
	var req createWorkOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	item, err := h.workOrders.Queue(c.UserContext(), service.QueueWorkOrderInput{
		ServiceType: req.ServiceType,
		Priority:    req.Priority,
		AssigneeID:  req.AssigneeID,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to queue work order")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"work_order": item})
}

func (h *WorkOrdersHandler) Start(c *fiber.Ctx) error {
	workOrderID, err := strconv.ParseInt(c.Params("work_order_id"), 10, 64)
	if err != nil || workOrderID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "work_order_id must be a positive integer", nil)
	}

	item, err := h.workOrders.Start(c.UserContext(), service.StartWorkOrderInput{WorkOrderID: workOrderID})
	if err != nil {
		return handleServiceError(c, err, "Failed to start work order")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"work_order": item})
}

func (h *WorkOrdersHandler) Complete(c *fiber.Ctx) error {
	workOrderID, err := strconv.ParseInt(c.Params("work_order_id"), 10, 64)
	if err != nil || workOrderID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "work_order_id must be a positive integer", nil)
	}

	result, err := h.workOrders.Complete(c.UserContext(), service.CompleteWorkOrderInput{
		WorkOrderID: workOrderID,
		ActorID:     currentActorIDFromContext(c),
		RequestID:   httpx.RequestID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to complete work order")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{
		"work_order":      result.WorkOrder,
		"latency_seconds": result.LatencySeconds,
		"on_time_15m":     result.OnTime15m,
	})
}

func currentActorIDFromContext(c *fiber.Ctx) *int64 {
	authContext, ok := middleware.CurrentAuth(c)
	if !ok || authContext.User == nil {
		return nil
	}
	id := authContext.User.ID
	return &id
}
