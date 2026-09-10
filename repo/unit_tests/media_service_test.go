package unit_tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestMediaServiceIngestWithChecksumValidation(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)

	mediaRoot := filepath.Join(t.TempDir(), "media")
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	// Create a test exercise first
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	exercise := &domain.Exercise{Title: "Test Exercise", Difficulty: "beginner", SearchText: "test exercise beginner"}
	if err := exerciseRepo.CreateTx(ctx, tx, exercise); err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Ingest a media file
	content := []byte("test video content for checksum validation")
	hasher := sha256.New()
	hasher.Write(content)
	expectedChecksum := hex.EncodeToString(hasher.Sum(nil))

	asset, err := mediaSvc.Ingest(ctx, service.IngestMediaInput{
		ExerciseID: exercise.ID,
		MediaType:  "video/mp4",
		Variant:    "original",
		Filename:   "test.mp4",
		Reader:     bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("ingest media: %v", err)
	}

	if asset.ChecksumSHA256 != expectedChecksum {
		t.Fatalf("checksum mismatch: expected %s, got %s", expectedChecksum, asset.ChecksumSHA256)
	}
	if asset.Bytes != int64(len(content)) {
		t.Fatalf("byte count mismatch: expected %d, got %d", len(content), asset.Bytes)
	}
	if asset.MediaType != "video/mp4" {
		t.Fatalf("media type mismatch: expected video/mp4, got %s", asset.MediaType)
	}

	// Verify file exists on disk
	absPath := mediaSvc.ResolvePath(asset.Path)
	info, err := os.Stat(absPath)
	if err != nil {
		t.Fatalf("media file not found on disk: %v", err)
	}
	if info.Size() != int64(len(content)) {
		t.Fatalf("file size mismatch on disk")
	}

	// Verify we can read back with matching content
	fileData, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Equal(fileData, content) {
		t.Fatal("file content mismatch on disk")
	}
}

func TestMediaServiceStreamingWithRangeHeaders(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)
	mediaRoot := filepath.Join(t.TempDir(), "media")
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	exercise := &domain.Exercise{Title: "Range Test Exercise", Difficulty: "intermediate", SearchText: "range test exercise intermediate"}
	if err := exerciseRepo.CreateTx(ctx, tx, exercise); err != nil {
		t.Fatalf("create exercise: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	content := make([]byte, 1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	asset, err := mediaSvc.Ingest(ctx, service.IngestMediaInput{
		ExerciseID: exercise.ID,
		MediaType:  "application/octet-stream",
		Variant:    "original",
		Filename:   "data.bin",
		Reader:     bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("ingest media: %v", err)
	}

	// Verify we can open the file and seek to specific byte range
	absPath := mediaSvc.ResolvePath(asset.Path)
	file, err := os.Open(absPath)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer file.Close()

	// Simulate Range: bytes=100-199
	if _, err := file.Seek(100, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	buf := make([]byte, 100)
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if n != 100 {
		t.Fatalf("expected 100 bytes, got %d", n)
	}
	for i := 0; i < 100; i++ {
		if buf[i] != byte((100+i)%256) {
			t.Fatalf("byte mismatch at offset %d: expected %d, got %d", 100+i, (100+i)%256, buf[i])
		}
	}
}

func TestMediaServiceRejectsInvalidExerciseID(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)
	mediaRoot := filepath.Join(t.TempDir(), "media")
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	_, err := mediaSvc.Ingest(ctx, service.IngestMediaInput{
		ExerciseID: 0,
		MediaType:  "video/mp4",
		Variant:    "original",
		Filename:   "test.mp4",
		Reader:     strings.NewReader("content"),
	})
	if !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for exercise_id=0, got: %v", err)
	}
}

func TestMediaServiceRejectsMissingMediaType(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)
	mediaRoot := filepath.Join(t.TempDir(), "media")
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	_, err := mediaSvc.Ingest(ctx, service.IngestMediaInput{
		ExerciseID: 1,
		MediaType:  "",
		Variant:    "original",
		Filename:   "test.mp4",
		Reader:     strings.NewReader("content"),
	})
	if !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for empty media_type, got: %v", err)
	}
}

func TestMediaServiceRejectsMissingFile(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)
	mediaRoot := filepath.Join(t.TempDir(), "media")
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	_, err := mediaSvc.Ingest(ctx, service.IngestMediaInput{
		ExerciseID: 1,
		MediaType:  "video/mp4",
		Variant:    "original",
		Filename:   "test.mp4",
		Reader:     nil,
	})
	if !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for nil reader, got: %v", err)
	}
}

func TestMediaServicePathTraversalPrevention(t *testing.T) {
	mediaRoot := filepath.Join(t.TempDir(), "media")
	exerciseRepo := sqlite.NewExerciseRepository(setupTestDB(t))
	mediaRepo := sqlite.NewMediaRepository(setupTestDB(t))
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	// Test that ResolvePath cleans traversal attempts
	resolved := mediaSvc.ResolvePath("../../etc/passwd")
	if strings.Contains(resolved, "..") {
		t.Fatalf("path traversal not prevented: %s", resolved)
	}

	// A normal path should resolve within mediaRoot
	normalResolved := mediaSvc.ResolvePath("26/04/abc_original.mp4")
	absRoot, _ := filepath.Abs(mediaRoot)
	absNormalResolved, _ := filepath.Abs(normalResolved)
	if !strings.HasPrefix(absNormalResolved, absRoot) {
		t.Fatalf("normal resolved path %s should be within media root %s", absNormalResolved, absRoot)
	}
}

func TestMediaServiceRejectsNonExistentExercise(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	exerciseRepo := sqlite.NewExerciseRepository(db)
	mediaRepo := sqlite.NewMediaRepository(db)
	mediaRoot := filepath.Join(t.TempDir(), "media")
	mediaSvc := service.NewMediaService(mediaRoot, exerciseRepo, mediaRepo)

	_, err := mediaSvc.Ingest(ctx, service.IngestMediaInput{
		ExerciseID: 99999,
		MediaType:  "video/mp4",
		Variant:    "original",
		Filename:   "test.mp4",
		Reader:     strings.NewReader("content"),
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected not found error for non-existent exercise, got: %v", err)
	}
}
