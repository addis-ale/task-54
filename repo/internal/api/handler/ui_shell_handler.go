package handler

import (
	"path/filepath"
	"strings"

	"clinic-admin-suite/internal/api/uiassets"

	"github.com/gofiber/fiber/v2"
)

type UIShellHandler struct{}

func NewUIShellHandler() *UIShellHandler {
	return &UIShellHandler{}
}

func (h *UIShellHandler) IndexRedirect(c *fiber.Ctx) error {
	return c.Redirect("/app", fiber.StatusTemporaryRedirect)
}

func (h *UIShellHandler) Asset(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.Params("*"))
	if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return fiber.NewError(fiber.StatusNotFound, "Asset not found")
	}
	filePath := filepath.ToSlash(filepath.Clean("assets/" + name))
	body, err := uiassets.Files.ReadFile(filePath)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Asset not found")
	}

	switch strings.ToLower(filepath.Ext(name)) {
	case ".css":
		c.Type("css", "utf-8")
	case ".js":
		c.Type("application/javascript", "utf-8")
	default:
		c.Type("octet-stream")
	}

	return c.Send(body)
}
