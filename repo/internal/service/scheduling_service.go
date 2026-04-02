package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type SchedulingService struct {
	db          *sql.DB
	schedules   repository.ExamScheduleRepository
	idempotency repository.IdempotencyRepository
	audit       *AuditService
}

func NewSchedulingService(db *sql.DB, schedules repository.ExamScheduleRepository, idempotency repository.IdempotencyRepository, audit *AuditService) *SchedulingService {
	return &SchedulingService{
		db:          db,
		schedules:   schedules,
		idempotency: idempotency,
		audit:       audit,
	}
}

type CreateExamScheduleInput struct {
	ExamID         string
	RoomID         int64
	ProctorID      int64
	CandidateIDs   []int64
	StartAt        time.Time
	EndAt          time.Time
	ActorID        int64
	IdempotencyKey string
	RouteKey       string
	RequestID      string
}

type IdempotentCreateScheduleResult struct {
	StatusCode int
	Body       []byte
	Replayed   bool
}

type ValidateScheduleResult struct {
	ScheduleID   int64                     `json:"schedule_id"`
	Conflicts    []domain.ScheduleConflict `json:"conflicts"`
	HasConflicts bool                      `json:"has_conflicts"`
}

func (s *SchedulingService) List(ctx context.Context, filter repository.ExamScheduleFilter) ([]domain.ExamSchedule, error) {
	return s.schedules.List(ctx, filter)
}

func (s *SchedulingService) CreateIdempotent(ctx context.Context, input CreateExamScheduleInput) (*IdempotentCreateScheduleResult, error) {
	normalized, err := normalizeScheduleInput(input)
	if err != nil {
		return nil, err
	}

	requestHash := scheduleRequestHash(normalized)
	now := time.Now().UTC()
	if normalized.RouteKey == "" {
		normalized.RouteKey = "/api/v1/exam-schedules"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create exam schedule tx: %w", err)
	}
	defer tx.Rollback()

	existing, err := s.idempotency.GetActiveTx(ctx, tx, normalized.ActorID, normalized.RouteKey, normalized.IdempotencyKey, now)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.RequestHash != requestHash {
			return nil, fmt.Errorf("%w: same key with different payload", ErrIdempotencyConflict)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit idempotency replay tx: %w", err)
		}
		return &IdempotentCreateScheduleResult{StatusCode: existing.ResponseCode, Body: []byte(existing.ResponseBody), Replayed: true}, nil
	}

	conflicts, err := s.schedules.DetectConflictsTx(ctx, tx, nil, normalized.RoomID, normalized.ProctorID, normalized.CandidateIDs, normalized.StartAt, normalized.EndAt)
	if err != nil {
		return nil, err
	}

	if len(conflicts) > 0 {
		responseBody, err := marshalEnvelope(nil, normalized.RequestID, &apiErrorBody{
			Code:    "SCHEDULING_CONFLICT",
			Message: "Schedule conflicts detected",
			Details: map[string]any{"conflicts": conflicts},
		})
		if err != nil {
			return nil, err
		}

		record := &domain.IdempotencyKeyRecord{
			ActorID:      normalized.ActorID,
			RouteKey:     normalized.RouteKey,
			Key:          normalized.IdempotencyKey,
			RequestHash:  requestHash,
			ResponseCode: 409,
			ResponseBody: string(responseBody),
			ExpiresAt:    now.Add(24 * time.Hour),
			CreatedAt:    now,
		}

		if err := s.storeIdempotencyTx(ctx, tx, record, requestHash, now); err != nil {
			return nil, err
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit create exam schedule conflict tx: %w", err)
		}

		return &IdempotentCreateScheduleResult{StatusCode: record.ResponseCode, Body: []byte(record.ResponseBody), Replayed: false}, nil
	}

	schedule := &domain.ExamSchedule{
		ExamID:       normalized.ExamID,
		RoomID:       normalized.RoomID,
		ProctorID:    normalized.ProctorID,
		CandidateIDs: normalized.CandidateIDs,
		StartAt:      normalized.StartAt,
		EndAt:        normalized.EndAt,
		Status:       domain.ExamScheduleStatusScheduled,
		ActorID:      &normalized.ActorID,
	}

	if err := s.schedules.CreateTx(ctx, tx, schedule); err != nil {
		return nil, err
	}
	if err := s.schedules.ReplaceCandidatesTx(ctx, tx, schedule.ID, normalized.CandidateIDs); err != nil {
		return nil, err
	}

	auditPayload := map[string]any{
		"request_payload": map[string]any{
			"exam_id":         normalized.ExamID,
			"room_id":         normalized.RoomID,
			"proctor_id":      normalized.ProctorID,
			"candidate_ids":   normalized.CandidateIDs,
			"start_at":        normalized.StartAt.Format(time.RFC3339),
			"end_at":          normalized.EndAt.Format(time.RFC3339),
			"idempotency_key": normalized.IdempotencyKey,
		},
		"schedule": map[string]any{
			"id":            schedule.ID,
			"exam_id":       schedule.ExamID,
			"room_id":       schedule.RoomID,
			"proctor_id":    schedule.ProctorID,
			"candidate_ids": schedule.CandidateIDs,
			"start_at":      schedule.StartAt.Format(time.RFC3339),
			"end_at":        schedule.EndAt.Format(time.RFC3339),
		},
	}

	if err := s.audit.LogEventTx(ctx, tx, AuditLogInput{
		ActorID:      &normalized.ActorID,
		Action:       "exam_schedule.create",
		ResourceType: "exam_schedule",
		ResourceID:   fmt.Sprintf("%d", schedule.ID),
		After:        auditPayload,
		RequestID:    normalized.RequestID,
	}); err != nil {
		return nil, err
	}

	responseBody, err := marshalEnvelope(map[string]any{"schedule": schedule}, normalized.RequestID, nil)
	if err != nil {
		return nil, err
	}

	record := &domain.IdempotencyKeyRecord{
		ActorID:      normalized.ActorID,
		RouteKey:     normalized.RouteKey,
		Key:          normalized.IdempotencyKey,
		RequestHash:  requestHash,
		ResponseCode: 201,
		ResponseBody: string(responseBody),
		ExpiresAt:    now.Add(24 * time.Hour),
		CreatedAt:    now,
	}

	if err := s.storeIdempotencyTx(ctx, tx, record, requestHash, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create exam schedule tx: %w", err)
	}

	return &IdempotentCreateScheduleResult{StatusCode: record.ResponseCode, Body: []byte(record.ResponseBody), Replayed: false}, nil
}

func (s *SchedulingService) Validate(ctx context.Context, scheduleID int64) (*ValidateScheduleResult, error) {
	if scheduleID <= 0 {
		return nil, fmt.Errorf("%w: schedule_id must be positive", ErrValidation)
	}

	schedule, err := s.schedules.GetByID(ctx, scheduleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: schedule not found", ErrNotFound)
		}
		return nil, err
	}

	conflicts, err := s.schedules.DetectConflicts(ctx, &schedule.ID, schedule.RoomID, schedule.ProctorID, schedule.CandidateIDs, schedule.StartAt, schedule.EndAt)
	if err != nil {
		return nil, err
	}

	return &ValidateScheduleResult{
		ScheduleID:   schedule.ID,
		Conflicts:    conflicts,
		HasConflicts: len(conflicts) > 0,
	}, nil
}

func (s *SchedulingService) storeIdempotencyTx(ctx context.Context, tx *sql.Tx, record *domain.IdempotencyKeyRecord, requestHash string, now time.Time) error {
	err := s.idempotency.CreateTx(ctx, tx, record)
	if err == nil {
		return nil
	}
	if !isUniqueViolation(err) {
		return err
	}

	existing, lookupErr := s.idempotency.GetActiveTx(ctx, tx, record.ActorID, record.RouteKey, record.Key, now)
	if lookupErr != nil {
		return lookupErr
	}
	if existing == nil {
		return err
	}
	if existing.RequestHash != requestHash {
		return fmt.Errorf("%w: same key with different payload", ErrIdempotencyConflict)
	}

	record.ResponseCode = existing.ResponseCode
	record.ResponseBody = existing.ResponseBody

	return nil
}

func normalizeScheduleInput(input CreateExamScheduleInput) (CreateExamScheduleInput, error) {
	input.ExamID = strings.TrimSpace(input.ExamID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.RouteKey = strings.TrimSpace(input.RouteKey)
	input.RequestID = strings.TrimSpace(input.RequestID)

	if input.ExamID == "" {
		return input, fmt.Errorf("%w: exam_id is required", ErrValidation)
	}
	if input.RoomID <= 0 || input.ProctorID <= 0 {
		return input, fmt.Errorf("%w: room_id and proctor_id must be positive", ErrValidation)
	}
	if input.ActorID <= 0 {
		return input, fmt.Errorf("%w: actor_id must be positive", ErrValidation)
	}
	if input.IdempotencyKey == "" {
		return input, fmt.Errorf("%w: Idempotency-Key header is required", ErrValidation)
	}

	input.CandidateIDs = normalizeCandidateIDs(input.CandidateIDs)
	if len(input.CandidateIDs) == 0 {
		return input, fmt.Errorf("%w: candidate_ids must include at least one candidate", ErrValidation)
	}

	input.StartAt = input.StartAt.UTC()
	input.EndAt = input.EndAt.UTC()
	if !input.EndAt.After(input.StartAt) {
		return input, fmt.Errorf("%w: end_at must be greater than start_at", ErrValidation)
	}

	if input.RequestID == "" {
		input.RequestID = "system"
	}

	return input, nil
}

func normalizeCandidateIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) > 1 {
		for i := 0; i < len(out)-1; i++ {
			for j := i + 1; j < len(out); j++ {
				if out[j] < out[i] {
					out[i], out[j] = out[j], out[i]
				}
			}
		}
	}
	return out
}

func scheduleRequestHash(input CreateExamScheduleInput) string {
	payload := struct {
		ExamID       string  `json:"exam_id"`
		RoomID       int64   `json:"room_id"`
		ProctorID    int64   `json:"proctor_id"`
		CandidateIDs []int64 `json:"candidate_ids"`
		StartAt      string  `json:"start_at"`
		EndAt        string  `json:"end_at"`
	}{
		ExamID:       input.ExamID,
		RoomID:       input.RoomID,
		ProctorID:    input.ProctorID,
		CandidateIDs: input.CandidateIDs,
		StartAt:      input.StartAt.Format(time.RFC3339Nano),
		EndAt:        input.EndAt.Format(time.RFC3339Nano),
	}
	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

type apiEnvelope struct {
	Data  any           `json:"data"`
	Meta  apiMeta       `json:"meta"`
	Error *apiErrorBody `json:"error"`
}

type apiMeta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func marshalEnvelope(data any, requestID string, apiErr *apiErrorBody) ([]byte, error) {
	env := apiEnvelope{
		Data: data,
		Meta: apiMeta{
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Error: apiErr,
	}

	raw, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal schedule response envelope: %w", err)
	}
	return raw, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}
