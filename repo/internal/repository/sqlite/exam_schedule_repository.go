package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type ExamScheduleRepository struct {
	db *sql.DB
}

func NewExamScheduleRepository(db *sql.DB) *ExamScheduleRepository {
	return &ExamScheduleRepository{db: db}
}

func (r *ExamScheduleRepository) List(ctx context.Context, filter repository.ExamScheduleFilter) ([]domain.ExamSchedule, error) {
	query := `
SELECT id, exam_id, room_id, proctor_id, start_at, end_at, status, version, actor_id, created_at, updated_at
FROM exam_schedules
WHERE 1=1`
	args := make([]any, 0, 5)

	if filter.Date != nil {
		dayStart := time.Date(filter.Date.UTC().Year(), filter.Date.UTC().Month(), filter.Date.UTC().Day(), 0, 0, 0, 0, time.UTC)
		dayEnd := dayStart.Add(24 * time.Hour)
		query += ` AND start_at >= ? AND start_at < ?`
		args = append(args, dayStart.Unix(), dayEnd.Unix())
	}

	if filter.RoomID != nil {
		query += ` AND room_id = ?`
		args = append(args, *filter.RoomID)
	}
	if filter.ProctorID != nil {
		query += ` AND proctor_id = ?`
		args = append(args, *filter.ProctorID)
	}
	if filter.CandidateID != nil {
		query += ` AND EXISTS (SELECT 1 FROM exam_candidates ec WHERE ec.schedule_id = exam_schedules.id AND ec.candidate_id = ?)`
		args = append(args, *filter.CandidateID)
	}

	query += ` ORDER BY start_at ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list exam schedules: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ExamSchedule, 0)
	for rows.Next() {
		schedule, err := scanExamSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan exam schedule row: %w", err)
		}

		candidateIDs, err := r.ListCandidates(ctx, schedule.ID)
		if err != nil {
			return nil, err
		}
		schedule.CandidateIDs = candidateIDs

		items = append(items, *schedule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exam schedule rows: %w", err)
	}

	return items, nil
}

func (r *ExamScheduleRepository) GetByID(ctx context.Context, id int64) (*domain.ExamSchedule, error) {
	schedule, err := r.getByID(ctx, r.db, id)
	if err != nil {
		return nil, err
	}

	candidateIDs, err := r.ListCandidates(ctx, id)
	if err != nil {
		return nil, err
	}
	schedule.CandidateIDs = candidateIDs

	return schedule, nil
}

func (r *ExamScheduleRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.ExamSchedule, error) {
	schedule, err := r.getByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	candidateIDs, err := r.ListCandidatesTx(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	schedule.CandidateIDs = candidateIDs

	return schedule, nil
}

func (r *ExamScheduleRepository) getByID(ctx context.Context, runner queryRowRunner, id int64) (*domain.ExamSchedule, error) {
	const q = `
SELECT id, exam_id, room_id, proctor_id, start_at, end_at, status, version, actor_id, created_at, updated_at
FROM exam_schedules
WHERE id = ?`

	schedule, err := scanExamSchedule(runner.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get exam schedule by id: %w", err)
	}

	return schedule, nil
}

func (r *ExamScheduleRepository) CreateTx(ctx context.Context, tx *sql.Tx, schedule *domain.ExamSchedule) error {
	now := time.Now().UTC()
	if schedule.Status == "" {
		schedule.Status = domain.ExamScheduleStatusScheduled
	}

	const q = `
INSERT INTO exam_schedules(exam_id, room_id, proctor_id, start_at, end_at, status, version, actor_id, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`

	var actorID sql.NullInt64
	if schedule.ActorID != nil {
		actorID = sql.NullInt64{Int64: *schedule.ActorID, Valid: true}
	}

	result, err := tx.ExecContext(ctx, q, schedule.ExamID, schedule.RoomID, schedule.ProctorID, schedule.StartAt.UTC().Unix(), schedule.EndAt.UTC().Unix(), schedule.Status, actorID, now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("create exam schedule: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create exam schedule last insert id: %w", err)
	}

	schedule.ID = id
	schedule.Version = 1
	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	return nil
}

func (r *ExamScheduleRepository) ReplaceCandidatesTx(ctx context.Context, tx *sql.Tx, scheduleID int64, candidateIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exam_candidates WHERE schedule_id = ?`, scheduleID); err != nil {
		return fmt.Errorf("clear exam candidates: %w", err)
	}

	for _, candidateID := range candidateIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exam_candidates(schedule_id, candidate_id) VALUES(?, ?)`, scheduleID, candidateID); err != nil {
			return fmt.Errorf("insert exam candidate: %w", err)
		}
	}

	return nil
}

func (r *ExamScheduleRepository) ListCandidatesTx(ctx context.Context, tx *sql.Tx, scheduleID int64) ([]int64, error) {
	return r.listCandidates(ctx, tx, scheduleID)
}

func (r *ExamScheduleRepository) ListCandidates(ctx context.Context, scheduleID int64) ([]int64, error) {
	return r.listCandidates(ctx, r.db, scheduleID)
}

func (r *ExamScheduleRepository) listCandidates(ctx context.Context, runner queryRunner, scheduleID int64) ([]int64, error) {
	rows, err := runner.QueryContext(ctx, `SELECT candidate_id FROM exam_candidates WHERE schedule_id = ? ORDER BY candidate_id ASC`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list exam candidates: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan exam candidate row: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exam candidates rows: %w", err)
	}

	return ids, nil
}

func (r *ExamScheduleRepository) DetectConflictsTx(ctx context.Context, tx *sql.Tx, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error) {
	return r.detectConflicts(ctx, tx, excludeScheduleID, roomID, proctorID, candidateIDs, startAt, endAt)
}

func (r *ExamScheduleRepository) DetectConflicts(ctx context.Context, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error) {
	return r.detectConflicts(ctx, r.db, excludeScheduleID, roomID, proctorID, candidateIDs, startAt, endAt)
}

func (r *ExamScheduleRepository) detectConflicts(ctx context.Context, runner queryRunner, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error) {
	conflicts := make([]domain.ScheduleConflict, 0)

	roomRows, err := runner.QueryContext(ctx, buildConflictQuery("room_id", excludeScheduleID), buildConflictArgs(roomID, excludeScheduleID, startAt, endAt)...)
	if err != nil {
		return nil, fmt.Errorf("query room conflicts: %w", err)
	}
	for roomRows.Next() {
		var (
			scheduleID    int64
			existingStart int64
			existingEnd   int64
		)
		if err := roomRows.Scan(&scheduleID, &existingStart, &existingEnd); err != nil {
			roomRows.Close()
			return nil, fmt.Errorf("scan room conflict row: %w", err)
		}
		conflicts = append(conflicts, domain.ScheduleConflict{
			ConflictType:          "room",
			EntityID:              roomID,
			ConflictingScheduleID: scheduleID,
			ExistingStartAt:       time.Unix(existingStart, 0).UTC(),
			ExistingEndAt:         time.Unix(existingEnd, 0).UTC(),
			Message:               "Room is already booked for overlapping interval",
		})
	}
	if err := roomRows.Err(); err != nil {
		roomRows.Close()
		return nil, fmt.Errorf("iterate room conflicts: %w", err)
	}
	roomRows.Close()

	proctorRows, err := runner.QueryContext(ctx, buildConflictQuery("proctor_id", excludeScheduleID), buildConflictArgs(proctorID, excludeScheduleID, startAt, endAt)...)
	if err != nil {
		return nil, fmt.Errorf("query proctor conflicts: %w", err)
	}
	for proctorRows.Next() {
		var (
			scheduleID    int64
			existingStart int64
			existingEnd   int64
		)
		if err := proctorRows.Scan(&scheduleID, &existingStart, &existingEnd); err != nil {
			proctorRows.Close()
			return nil, fmt.Errorf("scan proctor conflict row: %w", err)
		}
		conflicts = append(conflicts, domain.ScheduleConflict{
			ConflictType:          "proctor",
			EntityID:              proctorID,
			ConflictingScheduleID: scheduleID,
			ExistingStartAt:       time.Unix(existingStart, 0).UTC(),
			ExistingEndAt:         time.Unix(existingEnd, 0).UTC(),
			Message:               "Proctor has an overlapping schedule",
		})
	}
	if err := proctorRows.Err(); err != nil {
		proctorRows.Close()
		return nil, fmt.Errorf("iterate proctor conflicts: %w", err)
	}
	proctorRows.Close()

	normalizedCandidates := normalizeInt64(candidateIDs)
	if len(normalizedCandidates) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(normalizedCandidates)), ",")
		query := `
SELECT DISTINCT es.id, ec.candidate_id, es.start_at, es.end_at
FROM exam_schedules es
JOIN exam_candidates ec ON ec.schedule_id = es.id
WHERE es.status = 'scheduled'
AND ec.candidate_id IN (` + placeholders + `)
AND ? < es.end_at
AND ? > es.start_at`
		args := make([]any, 0, len(normalizedCandidates)+3)
		for _, id := range normalizedCandidates {
			args = append(args, id)
		}
		args = append(args, startAt.UTC().Unix(), endAt.UTC().Unix())
		if excludeScheduleID != nil {
			query += ` AND es.id <> ?`
			args = append(args, *excludeScheduleID)
		}
		query += ` ORDER BY es.start_at ASC`

		candidateRows, err := runner.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query candidate conflicts: %w", err)
		}
		for candidateRows.Next() {
			var (
				scheduleID    int64
				candidateID   int64
				existingStart int64
				existingEnd   int64
			)
			if err := candidateRows.Scan(&scheduleID, &candidateID, &existingStart, &existingEnd); err != nil {
				candidateRows.Close()
				return nil, fmt.Errorf("scan candidate conflict row: %w", err)
			}
			conflicts = append(conflicts, domain.ScheduleConflict{
				ConflictType:          "candidate",
				EntityID:              candidateID,
				ConflictingScheduleID: scheduleID,
				ExistingStartAt:       time.Unix(existingStart, 0).UTC(),
				ExistingEndAt:         time.Unix(existingEnd, 0).UTC(),
				Message:               "Candidate has an overlapping schedule",
			})
		}
		if err := candidateRows.Err(); err != nil {
			candidateRows.Close()
			return nil, fmt.Errorf("iterate candidate conflicts: %w", err)
		}
		candidateRows.Close()
	}

	return conflicts, nil
}

func buildConflictQuery(field string, excludeScheduleID *int64) string {
	query := `
SELECT id, start_at, end_at
FROM exam_schedules
WHERE status = 'scheduled'
AND ` + field + ` = ?
AND ? < end_at
AND ? > start_at`
	if excludeScheduleID != nil {
		query += ` AND id <> ?`
	}
	query += ` ORDER BY start_at ASC`
	return query
}

func buildConflictArgs(entityID int64, excludeScheduleID *int64, startAt, endAt time.Time) []any {
	args := []any{entityID, startAt.UTC().Unix(), endAt.UTC().Unix()}
	if excludeScheduleID != nil {
		args = append(args, *excludeScheduleID)
	}
	return args
}

func scanExamSchedule(scanner rowScanner) (*domain.ExamSchedule, error) {
	var (
		item      domain.ExamSchedule
		startAt   int64
		endAt     int64
		createdAt int64
		updatedAt int64
		actorID   sql.NullInt64
	)

	err := scanner.Scan(
		&item.ID,
		&item.ExamID,
		&item.RoomID,
		&item.ProctorID,
		&startAt,
		&endAt,
		&item.Status,
		&item.Version,
		&actorID,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	item.StartAt = time.Unix(startAt, 0).UTC()
	item.EndAt = time.Unix(endAt, 0).UTC()
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if actorID.Valid {
		v := actorID.Int64
		item.ActorID = &v
	}

	return &item, nil
}

func normalizeInt64(values []int64) []int64 {
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

var _ repository.ExamScheduleRepository = (*ExamScheduleRepository)(nil)
