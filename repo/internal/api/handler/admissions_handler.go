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

type AdmissionsHandler struct {
	admissions *service.AdmissionsService
}

func NewAdmissionsHandler(admissions *service.AdmissionsService) *AdmissionsHandler {
	return &AdmissionsHandler{admissions: admissions}
}

type createAdmissionRequest struct {
	PatientID int64 `json:"patient_id"`
	BedID     int64 `json:"bed_id"`
}

type transferAdmissionRequest struct {
	ToBedID int64 `json:"to_bed_id"`
}

func (h *AdmissionsHandler) List(c *fiber.Ctx) error {
	filter := repository.AdmissionFilter{}
	filter.Status = strings.TrimSpace(c.Query("status"))

	if patientIDRaw := strings.TrimSpace(c.Query("patient_id")); patientIDRaw != "" {
		patientID, err := strconv.ParseInt(patientIDRaw, 10, 64)
		if err != nil || patientID <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "patient_id must be a positive integer", nil)
		}
		filter.PatientID = &patientID
	}

	if bedIDRaw := strings.TrimSpace(c.Query("bed_id")); bedIDRaw != "" {
		bedID, err := strconv.ParseInt(bedIDRaw, 10, 64)
		if err != nil || bedID <= 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "bed_id must be a positive integer", nil)
		}
		filter.BedID = &bedID
	}

	admissions, err := h.admissions.ListAdmissions(c.UserContext(), filter)
	if err != nil {
		return handleServiceError(c, err, "Failed to list admissions")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"admissions": admissions})
}

func (h *AdmissionsHandler) Create(c *fiber.Ctx) error {
	var req createAdmissionRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	admission, err := h.admissions.AssignAdmission(c.UserContext(), service.AssignAdmissionInput{
		PatientID: req.PatientID,
		BedID:     req.BedID,
		ActorID:   currentActorID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create admission")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"admission": admission})
}

func (h *AdmissionsHandler) Transfer(c *fiber.Ctx) error {
	admissionID, err := strconv.ParseInt(c.Params("admission_id"), 10, 64)
	if err != nil || admissionID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "admission_id must be a positive integer", nil)
	}

	var req transferAdmissionRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	admission, err := h.admissions.TransferAdmission(c.UserContext(), service.TransferAdmissionInput{
		AdmissionID: admissionID,
		ToBedID:     req.ToBedID,
		ActorID:     currentActorID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to transfer admission")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"admission": admission})
}

func (h *AdmissionsHandler) Discharge(c *fiber.Ctx) error {
	admissionID, err := strconv.ParseInt(c.Params("admission_id"), 10, 64)
	if err != nil || admissionID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "admission_id must be a positive integer", nil)
	}

	admission, err := h.admissions.DischargeAdmission(c.UserContext(), service.DischargeAdmissionInput{
		AdmissionID: admissionID,
		ActorID:     currentActorID(c),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to discharge admission")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"admission": admission})
}

func currentActorID(c *fiber.Ctx) *int64 {
	authContext, ok := middleware.CurrentAuth(c)
	if !ok || authContext.User == nil {
		return nil
	}
	id := authContext.User.ID
	return &id
}
