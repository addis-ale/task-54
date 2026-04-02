package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type JobRunService struct {
	runs repository.JobRunRepository
}

func NewJobRunService(runs repository.JobRunRepository) *JobRunService {
	return &JobRunService{runs: runs}
}

type JobRunInput struct {
	JobType    string
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string
	Summary    any
}

func (s *JobRunService) Record(ctx context.Context, input JobRunInput) error {
	return s.record(ctx, nil, input)
}

func (s *JobRunService) RecordTx(ctx context.Context, tx *sql.Tx, input JobRunInput) error {
	return s.record(ctx, tx, input)
}

func (s *JobRunService) record(ctx context.Context, tx *sql.Tx, input JobRunInput) error {
	var summaryJSON *string
	if input.Summary != nil {
		raw, err := json.Marshal(input.Summary)
		if err != nil {
			return fmt.Errorf("marshal job run summary: %w", err)
		}
		text := string(raw)
		summaryJSON = &text
	}

	run := &domain.JobRun{
		JobType:     input.JobType,
		StartedAt:   input.StartedAt.UTC(),
		FinishedAt:  input.FinishedAt.UTC(),
		Status:      input.Status,
		SummaryJSON: summaryJSON,
	}

	if tx != nil {
		if err := s.runs.CreateTx(ctx, tx, run); err != nil {
			return fmt.Errorf("create job run tx: %w", err)
		}
		return nil
	}

	if err := s.runs.Create(ctx, run); err != nil {
		return fmt.Errorf("create job run: %w", err)
	}

	return nil
}
