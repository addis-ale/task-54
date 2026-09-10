package handler

import (
	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type WardsHandler struct {
	admissions *service.AdmissionsService
}

func NewWardsHandler(admissions *service.AdmissionsService) *WardsHandler {
	return &WardsHandler{admissions: admissions}
}

type createWardRequest struct {
	Name string `json:"name"`
}

func (h *WardsHandler) List(c *fiber.Ctx) error {
	wards, err := h.admissions.ListWards(c.UserContext())
	if err != nil {
		return handleServiceError(c, err, "Failed to list wards")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"wards": wards})
}

func (h *WardsHandler) Create(c *fiber.Ctx) error {
	var req createWardRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	ward, err := h.admissions.CreateWard(c.UserContext(), service.CreateWardInput{Name: req.Name})
	if err != nil {
		return handleServiceError(c, err, "Failed to create ward")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"ward": ward})
}
