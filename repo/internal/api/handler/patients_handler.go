package handler

import (
	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type PatientsHandler struct {
	admissions *service.AdmissionsService
}

func NewPatientsHandler(admissions *service.AdmissionsService) *PatientsHandler {
	return &PatientsHandler{admissions: admissions}
}

type createPatientRequest struct {
	MRN  string  `json:"mrn"`
	Name string  `json:"name"`
	DOB  *string `json:"dob"`
}

func (h *PatientsHandler) List(c *fiber.Ctx) error {
	patients, err := h.admissions.ListPatients(c.UserContext())
	if err != nil {
		return handleServiceError(c, err, "Failed to list patients")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"patients": patients})
}

func (h *PatientsHandler) Create(c *fiber.Ctx) error {
	var req createPatientRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	patient, err := h.admissions.CreatePatient(c.UserContext(), service.CreatePatientInput{
		MRN:  req.MRN,
		Name: req.Name,
		DOB:  req.DOB,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create patient")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"patient": patient})
}
