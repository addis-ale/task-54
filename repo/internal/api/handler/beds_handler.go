package handler

import (
	"strconv"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type BedsHandler struct {
	admissions *service.AdmissionsService
}

func NewBedsHandler(admissions *service.AdmissionsService) *BedsHandler {
	return &BedsHandler{admissions: admissions}
}

type createBedRequest struct {
	WardID  int64  `json:"ward_id"`
	BedCode string `json:"bed_code"`
	Status  string `json:"status"`
}

type updateBedStatusRequest struct {
	Status string `json:"status"`
}

func (h *BedsHandler) List(c *fiber.Ctx) error {
	filter := repository.BedFilter{}

	if wardIDRaw := strings.TrimSpace(c.Query("ward_id")); wardIDRaw != "" {
		wardID, err := strconv.ParseInt(wardIDRaw, 10, 64)
		if err != nil || wardID <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "ward_id must be a positive integer", nil)
		}
		filter.WardID = &wardID
	}

	filter.Status = strings.TrimSpace(c.Query("status"))

	beds, err := h.admissions.ListBeds(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to list beds")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"beds": beds})
}

func (h *BedsHandler) Create(c *fiber.Ctx) error {
	var req createBedRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	bed, err := h.admissions.CreateBed(c.UserContext(), service.CreateBedInput{
		WardID:  req.WardID,
		BedCode: req.BedCode,
		Status:  req.Status,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create bed")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"bed": bed})
}

func (h *BedsHandler) PatchStatus(c *fiber.Ctx) error {
	bedID, err := strconv.ParseInt(c.Params("bed_id"), 10, 64)
	if err != nil || bedID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "bed_id must be a positive integer", nil)
	}

	versionRaw := strings.TrimSpace(c.Get("If-Match-Version"))
	if versionRaw == "" {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "If-Match-Version header is required", nil)
	}

	expectedVersion, err := strconv.ParseInt(versionRaw, 10, 64)
	if err != nil || expectedVersion <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "If-Match-Version must be a positive integer", nil)
	}

	var req updateBedStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	bed, err := h.admissions.UpdateBedStatus(c.UserContext(), service.UpdateBedStatusInput{
		BedID:           bedID,
		Status:          req.Status,
		ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to update bed status")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"bed": bed})
}
