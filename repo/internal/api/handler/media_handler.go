package handler

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"clinic-admin-suite/internal/api/httpx"
	"clinic-admin-suite/internal/service"

	"github.com/gofiber/fiber/v2"
)

type MediaHandler struct {
	media *service.MediaService
}

func NewMediaHandler(media *service.MediaService) *MediaHandler {
	return &MediaHandler{media: media}
}

func (h *MediaHandler) Upload(c *fiber.Ctx) error {
	exerciseID, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("exercise_id")), 10, 64)
	if err != nil || exerciseID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "exercise_id must be a positive integer", nil)
	}

	mediaType := strings.TrimSpace(c.FormValue("media_type"))
	variant := strings.TrimSpace(c.FormValue("variant"))

	var durationMS *int64
	if durationRaw := strings.TrimSpace(c.FormValue("duration_ms")); durationRaw != "" {
		v, err := strconv.ParseInt(durationRaw, 10, 64)
		if err != nil || v < 0 {
			return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "duration_ms must be a non-negative integer", nil)
		}
		durationMS = &v
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "file is required", nil)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to open uploaded file", nil)
	}
	defer file.Close()

	asset, err := h.media.Ingest(c.UserContext(), service.IngestMediaInput{
		ExerciseID: exerciseID,
		MediaType:  mediaType,
		Variant:    variant,
		Filename:   fileHeader.Filename,
		DurationMS: durationMS,
		Reader:     file,
	})
	if err != nil {
		return handleServiceError(c, err, "Failed to ingest media file")
	}

	return httpx.OK(c, fiber.StatusCreated, fiber.Map{"media": asset})
}

func (h *MediaHandler) UploadAndRedirect(c *fiber.Ctx) error {
	exerciseID, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("exercise_id")), 10, 64)
	if err != nil || exerciseID <= 0 {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: exercise_id must be a positive integer</div>`)
	}
	mediaType := strings.TrimSpace(c.FormValue("media_type"))
	if mediaType == "" {
		mediaType = "image"
	}
	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader == nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: file is required</div>`)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(`<div class="card">Failed to read uploaded file</div>`)
	}
	defer file.Close()
	_, err = h.media.Ingest(c.UserContext(), service.IngestMediaInput{
		ExerciseID: exerciseID,
		MediaType:  mediaType,
		Filename:   fileHeader.Filename,
		Variant:    "original",
		Reader:     file,
	})
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).SendString(`<div class="card">Failed: ` + err.Error() + `</div>`)
	}
	return c.SendString(`<div class="card" style="border-left:4px solid var(--accent)">Media uploaded for exercise #` + strconv.FormatInt(exerciseID, 10) + `</div>`)
}

func (h *MediaHandler) Get(c *fiber.Ctx) error {
	mediaID, err := strconv.ParseInt(c.Params("media_id"), 10, 64)
	if err != nil || mediaID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "media_id must be a positive integer", nil)
	}

	asset, err := h.media.GetByID(c.UserContext(), mediaID)
	if err != nil {
		return handleServiceError(c, err, "Failed to fetch media metadata")
	}

	return httpx.OK(c, fiber.StatusOK, fiber.Map{"media": asset})
}

func (h *MediaHandler) Stream(c *fiber.Ctx) error {
	mediaID, err := strconv.ParseInt(c.Params("media_id"), 10, 64)
	if err != nil || mediaID <= 0 {
		return httpx.Error(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "media_id must be a positive integer", nil)
	}

	asset, err := h.media.GetByID(c.UserContext(), mediaID)
	if err != nil {
		return handleServiceError(c, err, "Failed to fetch media asset")
	}

	absPath := h.media.ResolvePath(asset.Path)
	file, err := os.Open(absPath)
	if err != nil {
		return httpx.Error(c, fiber.StatusNotFound, "NOT_FOUND", "Media file not found on disk", nil)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to inspect media file", nil)
	}

	totalSize := info.Size()
	contentType := asset.MediaType
	if strings.TrimSpace(contentType) == "" {
		contentType = mime.TypeByExtension(filepath.Ext(absPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	c.Set("Accept-Ranges", "bytes")
	c.Set("Content-Type", contentType)

	rangeHeader := strings.TrimSpace(c.Get("Range"))
	if rangeHeader == "" {
		c.Set("Content-Length", strconv.FormatInt(totalSize, 10))
		return c.Status(fiber.StatusOK).SendStream(file, int(totalSize))
	}

	start, end, err := parseRange(rangeHeader, totalSize)
	if err != nil {
		c.Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
		return c.SendStatus(fiber.StatusRequestedRangeNotSatisfiable)
	}

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return httpx.Error(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Failed to seek media stream", nil)
	}

	length := end - start + 1
	c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
	c.Set("Content-Length", strconv.FormatInt(length, 10))

	return c.Status(fiber.StatusPartialContent).SendStream(io.LimitReader(file, length), int(length))
}

func parseRange(header string, total int64) (int64, int64, error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range unit")
	}

	raw := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(raw, ",") {
		return 0, 0, fmt.Errorf("multiple ranges not supported")
	}

	parts := strings.SplitN(raw, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	startPart := strings.TrimSpace(parts[0])
	endPart := strings.TrimSpace(parts[1])

	if startPart == "" {
		suffix, err := strconv.ParseInt(endPart, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		if suffix > total {
			suffix = total
		}
		return total - suffix, total - 1, nil
	}

	start, err := strconv.ParseInt(startPart, 10, 64)
	if err != nil || start < 0 || start >= total {
		return 0, 0, fmt.Errorf("invalid start range")
	}

	if endPart == "" {
		return start, total - 1, nil
	}

	end, err := strconv.ParseInt(endPart, 10, 64)
	if err != nil || end < start {
		return 0, 0, fmt.Errorf("invalid end range")
	}
	if end >= total {
		end = total - 1
	}

	return start, end, nil
}
