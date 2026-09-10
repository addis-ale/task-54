package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type JobRunRepository struct {
	db *sql.DB
}

func NewJobRunRepository(db *sql.DB) *JobRunRepository {
	return &JobRunRepository{db: db}
}

func (r *JobRunRepository) Create(ctx context.Context, run *domain.JobRun) error {
	return r.create(ctx, r.db, run)
}

func (r *JobRunRepository) CreateTx(ctx context.Context, tx *sql.Tx, run *domain.JobRun) error {
	return r.create(ctx, tx, run)
}

func (r *JobRunRepository) create(ctx context.Context, runner execRunner, run *domain.JobRun) error {
	const q = `
INSERT INTO job_runs(job_type, started_at, finished_at, status, summary_json, failure_root_cause_notes)
VALUES(?, ?, ?, ?, ?, ?)`

	var summary sql.NullString
	if run.SummaryJSON != nil {
		summary = sql.NullString{String: *run.SummaryJSON, Valid: true}
	}

	var rootCause sql.NullString
	if run.FailureRootCauseNotes != nil {
		rootCause = sql.NullString{String: *run.FailureRootCauseNotes, Valid: true}
	}

	result, err := runner.ExecContext(ctx, q, run.JobType, run.StartedAt.UTC().Unix(), run.FinishedAt.UTC().Unix(), run.Status, summary, rootCause)
	if err != nil {
		return fmt.Errorf("create job run: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create job run last insert id: %w", err)
	}
	run.ID = id

	return nil
}

var _ repository.JobRunRepository = (*JobRunRepository)(nil)
