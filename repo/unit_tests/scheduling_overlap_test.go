package unit_tests

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/sqlite"
	"clinic-admin-suite/internal/service"
)

func TestSchedulingOverlapPredicateForRoomProctorCandidate(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	repo := sqlite.NewExamScheduleRepository(db)

	existingID := insertSchedule(t, db, domain.ExamSchedule{
		ExamID:       "exam-1",
		RoomID:       101,
		ProctorID:    501,
		CandidateIDs: []int64{1001, 1002},
		StartAt:      mustTime("2026-06-01T10:00:00Z"),
		EndAt:        mustTime("2026-06-01T11:00:00Z"),
		Status:       domain.ExamScheduleStatusScheduled,
	})

	roomConflicts, err := repo.DetectConflicts(ctx, nil, 101, 999, []int64{9999}, mustTime("2026-06-01T10:30:00Z"), mustTime("2026-06-01T10:45:00Z"))
	if err != nil {
		t.Fatalf("detect room conflicts: %v", err)
	}
	if len(roomConflicts) == 0 || roomConflicts[0].ConflictType != "room" {
		t.Fatalf("expected room conflict, got %+v", roomConflicts)
	}

	boundaryNoConflict, err := repo.DetectConflicts(ctx, nil, 101, 501, []int64{1001}, mustTime("2026-06-01T11:00:00Z"), mustTime("2026-06-01T12:00:00Z"))
	if err != nil {
		t.Fatalf("detect boundary conflicts: %v", err)
	}
	if len(boundaryNoConflict) != 0 {
		t.Fatalf("expected no conflict when start == existing.end, got %+v", boundaryNoConflict)
	}

	candidateConflicts, err := repo.DetectConflicts(ctx, nil, 999, 999, []int64{1002}, mustTime("2026-06-01T10:40:00Z"), mustTime("2026-06-01T11:20:00Z"))
	if err != nil {
		t.Fatalf("detect candidate conflicts: %v", err)
	}
	foundCandidateConflict := false
	for _, c := range candidateConflicts {
		if c.ConflictType == "candidate" && c.EntityID == 1002 && c.ConflictingScheduleID == existingID {
			foundCandidateConflict = true
		}
	}
	if !foundCandidateConflict {
		t.Fatalf("expected candidate conflict for candidate 1002, got %+v", candidateConflicts)
	}
}

func TestSchedulingServiceValidateWithRepositoryMock(t *testing.T) {
	mockRepo := &mockScheduleRepo{
		getByIDFn: func(_ context.Context, id int64) (*domain.ExamSchedule, error) {
			return &domain.ExamSchedule{
				ID:           id,
				RoomID:       55,
				ProctorID:    77,
				CandidateIDs: []int64{9001},
				StartAt:      mustTime("2026-07-01T09:00:00Z"),
				EndAt:        mustTime("2026-07-01T10:00:00Z"),
			}, nil
		},
		detectFn: func(_ context.Context, _ *int64, _, _ int64, _ []int64, _, _ time.Time) ([]domain.ScheduleConflict, error) {
			return []domain.ScheduleConflict{{
				ConflictType:          "room",
				EntityID:              55,
				ConflictingScheduleID: 123,
				ExistingStartAt:       mustTime("2026-07-01T09:30:00Z"),
				ExistingEndAt:         mustTime("2026-07-01T10:30:00Z"),
				Message:               "Room is already booked for overlapping interval",
			}}, nil
		},
	}

	svc := service.NewSchedulingService(nil, mockRepo, nil, nil)
	result, err := svc.Validate(context.Background(), 1)
	if err != nil {
		t.Fatalf("validate schedule with mock repo: %v", err)
	}
	if !result.HasConflicts || len(result.Conflicts) != 1 {
		t.Fatalf("expected single conflict from mocked repository, got %+v", result)
	}
}

func TestSchedulingServiceIllegalInputs(t *testing.T) {
	mockRepo := &mockScheduleRepo{}
	svc := service.NewSchedulingService(nil, mockRepo, nil, nil)

	if _, err := svc.Validate(context.Background(), 0); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected validation error for zero schedule id, got: %v", err)
	}
}

func insertSchedule(t *testing.T, db *sql.DB, schedule domain.ExamSchedule) int64 {
	t.Helper()

	repo := sqlite.NewExamScheduleRepository(db)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin schedule tx: %v", err)
	}
	defer tx.Rollback()

	if err := repo.CreateTx(context.Background(), tx, &schedule); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if err := repo.ReplaceCandidatesTx(context.Background(), tx, schedule.ID, schedule.CandidateIDs); err != nil {
		t.Fatalf("replace candidates: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit schedule tx: %v", err)
	}

	return schedule.ID
}

func mustTime(raw string) time.Time {
	t, _ := time.Parse(time.RFC3339, raw)
	return t.UTC()
}

type mockScheduleRepo struct {
	getByIDFn func(ctx context.Context, id int64) (*domain.ExamSchedule, error)
	detectFn  func(ctx context.Context, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error)
}

func (m *mockScheduleRepo) List(_ context.Context, _ repository.ExamScheduleFilter) ([]domain.ExamSchedule, error) {
	return nil, nil
}
func (m *mockScheduleRepo) GetByID(ctx context.Context, id int64) (*domain.ExamSchedule, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (m *mockScheduleRepo) GetByIDTx(_ context.Context, _ *sql.Tx, _ int64) (*domain.ExamSchedule, error) {
	return nil, repository.ErrNotFound
}
func (m *mockScheduleRepo) CreateTx(_ context.Context, _ *sql.Tx, _ *domain.ExamSchedule) error {
	return nil
}
func (m *mockScheduleRepo) ReplaceCandidatesTx(_ context.Context, _ *sql.Tx, _ int64, _ []int64) error {
	return nil
}
func (m *mockScheduleRepo) ListCandidatesTx(_ context.Context, _ *sql.Tx, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *mockScheduleRepo) ListCandidates(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *mockScheduleRepo) DetectConflictsTx(_ context.Context, _ *sql.Tx, _ *int64, _ int64, _ int64, _ []int64, _ time.Time, _ time.Time) ([]domain.ScheduleConflict, error) {
	return nil, nil
}
func (m *mockScheduleRepo) DetectConflicts(ctx context.Context, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error) {
	if m.detectFn != nil {
		return m.detectFn(ctx, excludeScheduleID, roomID, proctorID, candidateIDs, startAt, endAt)
	}
	return nil, nil
}
