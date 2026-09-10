package handler

import (
	"strconv"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type ExerciseFavoritesHandler struct {
	favorites *service.ExerciseFavoriteService
}

func NewExerciseFavoritesHandler(favorites *service.ExerciseFavoriteService) *ExerciseFavoritesHandler {
	return &ExerciseFavoritesHandler{favorites: favorites}
}

func (h *ExerciseFavoritesHandler) Toggle(c *fiber.Ctx) error {
	exerciseID, err := strconv.ParseInt(c.Params("exercise_id"), 10, 64)
	if err != nil || exerciseID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "exercise_id must be a positive integer", nil)
	}
	actorID := currentActorIDFromContext(c)
	if actorID == nil {
		return httpx.Error(c, fiber.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Authentication required", nil)
	}

	active, err := h.favorites.Toggle(c.UserContext(), *actorID, exerciseID)
	if err != nil {
		return handleServiceError(c, err, "Failed to toggle favorite")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"exercise_id": exerciseID, "favorite": active})
}
