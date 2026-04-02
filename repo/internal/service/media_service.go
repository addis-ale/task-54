package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type MediaService struct {
	mediaRoot string
	exercises repository.ExerciseRepository
	media     repository.MediaRepository
}

func NewMediaService(mediaRoot string, exercises repository.ExerciseRepository, media repository.MediaRepository) *MediaService {
	return &MediaService{
		mediaRoot: strings.TrimSpace(mediaRoot),
		exercises: exercises,
		media:     media,
	}
}

type IngestMediaInput struct {
	ExerciseID int64
	MediaType  string
	Variant    string
	Filename   string
	DurationMS *int64
	Reader     io.Reader
}

func (s *MediaService) Ingest(ctx context.Context, input IngestMediaInput) (*domain.MediaAsset, error) {
	if input.ExerciseID <= 0 {
		return nil, fmt.Errorf("%w: exercise_id must be positive", ErrValidation)
	}
	if strings.TrimSpace(input.MediaType) == "" {
		return nil, fmt.Errorf("%w: media_type is required", ErrValidation)
	}
	if input.Reader == nil {
		return nil, fmt.Errorf("%w: media file is required", ErrValidation)
	}

	if _, err := s.exercises.GetByID(ctx, input.ExerciseID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: exercise not found", ErrNotFound)
		}
		return nil, err
	}

	root := s.mediaRoot
	if root == "" {
		root = filepath.Join(".", "data", "media")
	}

	now := time.Now().UTC()
	yy := now.Format("06")
	mm := now.Format("01")

	variant := sanitizeVariant(input.Variant)
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(input.Filename)))
	if ext == "" {
		ext = ".bin"
	}

	mediaToken, err := randomHexToken(10)
	if err != nil {
		return nil, fmt.Errorf("generate media token: %w", err)
	}

	relDir := filepath.ToSlash(filepath.Join(yy, mm))
	filename := mediaToken + "_" + variant + ext
	relPath := filepath.ToSlash(filepath.Join(relDir, filename))
	absDir := filepath.Join(root, yy, mm)
	absPath := filepath.Join(absDir, filename)

	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("create media directory: %w", err)
	}

	outFile, err := os.Create(absPath)
	if err != nil {
		return nil, fmt.Errorf("create media file: %w", err)
	}

	hasher := sha256.New()
	bytesWritten, copyErr := io.Copy(io.MultiWriter(outFile, hasher), input.Reader)
	closeErr := outFile.Close()
	if copyErr != nil {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("copy media file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("close media file: %w", closeErr)
	}

	asset := &domain.MediaAsset{
		ExerciseID:     input.ExerciseID,
		MediaType:      strings.TrimSpace(input.MediaType),
		Path:           relPath,
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
		DurationMS:     input.DurationMS,
		Bytes:          bytesWritten,
		Variant:        variant,
	}

	if err := s.media.Create(ctx, asset); err != nil {
		_ = os.Remove(absPath)
		return nil, err
	}

	return asset, nil
}

func (s *MediaService) GetByID(ctx context.Context, id int64) (*domain.MediaAsset, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: media_id must be positive", ErrValidation)
	}

	asset, err := s.media.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: media asset not found", ErrNotFound)
		}
		return nil, err
	}

	return asset, nil
}

func (s *MediaService) ResolvePath(storedPath string) string {
	root := s.mediaRoot
	if root == "" {
		root = filepath.Join(".", "data", "media")
	}
	clean := filepath.Clean(strings.ReplaceAll(storedPath, "/", string(filepath.Separator)))
	return filepath.Join(root, clean)
}

func sanitizeVariant(variant string) string {
	v := strings.TrimSpace(strings.ToLower(variant))
	if v == "" {
		return "original"
	}
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "-", "_")
	b := strings.Builder{}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "original"
	}
	return out
}

func randomHexToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
