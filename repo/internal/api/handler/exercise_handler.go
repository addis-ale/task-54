package handler

import (
	"strconv"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ExerciseHandler struct {
	exercises *service.ExerciseService
}

func NewExerciseHandler(exercises *service.ExerciseService) *ExerciseHandler {
	return &ExerciseHandler{exercises: exercises}
}

type createExerciseRequest struct {
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	CoachingPoints    string   `json:"coaching_points"`
	Difficulty        string   `json:"difficulty"`
	Tags              []string `json:"tags"`
	Equipment         []string `json:"equipment"`
	Contraindications []string `json:"contraindications"`
	BodyRegions       []string `json:"body_regions"`
}

type updateExerciseRequest struct {
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	CoachingPoints    string   `json:"coaching_points"`
	Difficulty        string   `json:"difficulty"`
	Tags              []string `json:"tags"`
	Equipment         []string `json:"equipment"`
	Contraindications []string `json:"contraindications"`
	BodyRegions       []string `json:"body_regions"`
}

type updateTagsRequest struct {
	TagType string   `json:"tag_type"`
	Attach  []string `json:"attach"`
	Detach  []string `json:"detach"`
}

func (h *ExerciseHandler) List(c *fiber.Ctx) error {
	items, err := h.exercises.List(c.UserContext(), repository.ExerciseFilter{
		Query:             strings.TrimSpace(c.Query("q")),
		Difficulty:        strings.TrimSpace(c.Query("difficulty")),
		Tags:              splitCSV(c.Query("tags")),
		Equipment:         splitCSV(c.Query("equipment")),
		Contraindications: splitCSV(c.Query("contraindications")),
		BodyRegions:       splitCSV(c.Query("body_region")),
		CoachingPoints:    splitCSV(c.Query("coaching_points")),
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to list exercises")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exercises": items})
}

func (h *ExerciseHandler) Create(c *fiber.Ctx) error {
	var req createExerciseRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	created, err := h.exercises.Create(c.UserContext(), service.CreateExerciseInput{
		Title:             req.Title,
		Description:       req.Description,
		CoachingPoints:    req.CoachingPoints,
		Difficulty:        req.Difficulty,
		Tags:              req.Tags,
		Equipment:         req.Equipment,
		Contraindications: req.Contraindications,
		BodyRegions:       req.BodyRegions,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to create exercise")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"exercise": created})
}

func (h *ExerciseHandler) Patch(c *fiber.Ctx) error {
	exerciseID, err := strconv.ParseInt(c.Params("exercise_id"), 10, 64)
	if err != nil || exerciseID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "exercise_id must be a positive integer", nil)
	}

	versionRaw := strings.TrimSpace(c.Get("If-Match-Version"))
	if versionRaw == "" {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "If-Match-Version header is required", nil)
	}

	expectedVersion, err := strconv.ParseInt(versionRaw, 10, 64)
	if err != nil || expectedVersion <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "If-Match-Version must be a positive integer", nil)
	}

	var req updateExerciseRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	updated, err := h.exercises.Update(c.UserContext(), service.UpdateExerciseInput{
		ExerciseID:        exerciseID,
		ExpectedVersion:   expectedVersion,
		Title:             req.Title,
		Description:       req.Description,
		CoachingPoints:    req.CoachingPoints,
		Difficulty:        req.Difficulty,
		Tags:              req.Tags,
		Equipment:         req.Equipment,
		Contraindications: req.Contraindications,
		BodyRegions:       req.BodyRegions,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to update exercise")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exercise": updated})
}

func (h *ExerciseHandler) Get(c *fiber.Ctx) error {
	exerciseID, err := strconv.ParseInt(c.Params("exercise_id"), 10, 64)
	if err != nil || exerciseID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "exercise_id must be a positive integer", nil)
	}

	exercise, err := h.exercises.GetByID(c.UserContext(), exerciseID)
	if err != nil {
		return handleServiceError(c, err, "Failed to fetch exercise")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exercise": exercise})
}

func (h *ExerciseHandler) ListTags(c *fiber.Ctx) error {
	items, err := h.exercises.ListTags(c.UserContext(), strings.TrimSpace(c.Query("type")))
	if err != nil {
		return handleServiceError(c, err, "Failed to list tags")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"tags": items})
}

func (h *ExerciseHandler) UpdateTags(c *fiber.Ctx) error {
	exerciseID, err := strconv.ParseInt(c.Params("exercise_id"), 10, 64)
	if err != nil || exerciseID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "exercise_id must be a positive integer", nil)
	}

	var req updateTagsRequest
	if err := c.BodyParser(&req); err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid request payload", nil)
	}

	updated, err := h.exercises.UpdateTags(c.UserContext(), service.UpdateExerciseTagsInput{
		ExerciseID: exerciseID,
		TagType:    req.TagType,
		Attach:     req.Attach,
		Detach:     req.Detach,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to update exercise tags")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exercise": updated})
}

func splitCSV(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		v := strings.TrimSpace(part)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
