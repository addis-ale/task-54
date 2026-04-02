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

type ExerciseRepository struct {
	db *sql.DB
}

func NewExerciseRepository(db *sql.DB) *ExerciseRepository {
	return &ExerciseRepository{db: db}
}

func (r *ExerciseRepository) List(ctx context.Context, filter repository.ExerciseFilter) ([]domain.Exercise, error) {
	query := `
SELECT DISTINCT e.id, e.title, e.description, e.coaching_points, e.difficulty, e.search_text, e.version, e.created_at, e.updated_at
FROM exercises e
WHERE 1=1`
	args := make([]any, 0)

	if q := strings.TrimSpace(filter.Query); q != "" {
		query += ` AND e.id IN (SELECT rowid FROM exercises_fts WHERE exercises_fts MATCH ?)`
		args = append(args, q)
	}

	if difficulty := strings.TrimSpace(filter.Difficulty); difficulty != "" {
		query += ` AND e.difficulty = ?`
		args = append(args, difficulty)
	}

	for _, tag := range normalizeStrings(filter.Tags) {
		query += ` AND EXISTS (
SELECT 1 FROM exercise_tags et
JOIN tags t ON t.id = et.tag_id
WHERE et.exercise_id = e.id AND t.name = ?
)`
		args = append(args, tag)
	}

	for _, equipment := range normalizeStrings(filter.Equipment) {
		query += ` AND EXISTS (
SELECT 1 FROM exercise_tags et
JOIN tags t ON t.id = et.tag_id
WHERE et.exercise_id = e.id AND t.tag_type = 'equipment' AND t.name = ?
)`
		args = append(args, equipment)
	}

	for _, contraindication := range normalizeStrings(filter.Contraindications) {
		query += ` AND EXISTS (
SELECT 1 FROM exercise_contraindications ec
JOIN contraindications c ON c.id = ec.contraindication_id
WHERE ec.exercise_id = e.id AND c.label = ?
)`
		args = append(args, contraindication)
	}

	for _, bodyRegion := range normalizeStrings(filter.BodyRegions) {
		query += ` AND EXISTS (
SELECT 1 FROM exercise_body_regions ebr
JOIN body_regions br ON br.id = ebr.body_region_id
WHERE ebr.exercise_id = e.id AND br.name = ?
)`
		args = append(args, bodyRegion)
	}

	for _, coachingPoint := range normalizeStrings(filter.CoachingPoints) {
		query += ` AND lower(e.coaching_points) LIKE ?`
		args = append(args, "%"+strings.ToLower(coachingPoint)+"%")
	}

	query += ` ORDER BY e.updated_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Exercise, 0)
	for rows.Next() {
		exercise, err := scanExercise(rows)
		if err != nil {
			return nil, fmt.Errorf("scan exercise row: %w", err)
		}

		if err := r.loadRelations(ctx, r.db, exercise); err != nil {
			return nil, err
		}

		items = append(items, *exercise)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercises rows: %w", err)
	}

	return items, nil
}

func (r *ExerciseRepository) GetByID(ctx context.Context, id int64) (*domain.Exercise, error) {
	const q = `
SELECT id, title, description, coaching_points, difficulty, search_text, version, created_at, updated_at
FROM exercises
WHERE id = ?`

	exercise, err := scanExercise(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get exercise by id: %w", err)
	}

	if err := r.loadRelations(ctx, r.db, exercise); err != nil {
		return nil, err
	}

	return exercise, nil
}

func (r *ExerciseRepository) CreateTx(ctx context.Context, tx *sql.Tx, exercise *domain.Exercise) error {
	now := time.Now().UTC()
	const q = `
INSERT INTO exercises(title, description, coaching_points, difficulty, search_text, version, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, 1, ?, ?)`

	result, err := tx.ExecContext(ctx, q, exercise.Title, exercise.Description, exercise.CoachingPoints, exercise.Difficulty, exercise.SearchText, now.Unix(), now.Unix())
	if err != nil {
		return fmt.Errorf("create exercise: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("create exercise last insert id: %w", err)
	}

	exercise.ID = id
	exercise.Version = 1
	exercise.CreatedAt = now
	exercise.UpdatedAt = now
	return nil
}

func (r *ExerciseRepository) UpdateTx(ctx context.Context, tx *sql.Tx, exerciseID, expectedVersion int64, title, description, coachingPoints, difficulty, searchText string) (bool, error) {
	const q = `
UPDATE exercises
SET title = ?, description = ?, coaching_points = ?, difficulty = ?, search_text = ?, version = version + 1, updated_at = ?
WHERE id = ? AND version = ?`

	result, err := tx.ExecContext(ctx, q, title, description, coachingPoints, difficulty, searchText, time.Now().UTC().Unix(), exerciseID, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("update exercise: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update exercise rows affected: %w", err)
	}

	return affected == 1, nil
}

func (r *ExerciseRepository) ListTags(ctx context.Context, tagType string) ([]domain.Tag, error) {
	query := `SELECT id, tag_type, name FROM tags WHERE 1=1`
	args := make([]any, 0, 1)
	if strings.TrimSpace(tagType) != "" {
		query += ` AND tag_type = ?`
		args = append(args, strings.TrimSpace(tagType))
	}
	query += ` ORDER BY tag_type ASC, name ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Tag, 0)
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.TagType, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan tag row: %w", err)
		}
		items = append(items, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag rows: %w", err)
	}

	return items, nil
}

func (r *ExerciseRepository) EnsureTagsTx(ctx context.Context, tx *sql.Tx, tagType string, names []string) ([]domain.Tag, error) {
	clean := normalizeStrings(names)
	if len(clean) == 0 {
		return nil, nil
	}

	for _, name := range clean {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(tag_type, name) VALUES(?, ?)`, tagType, name); err != nil {
			return nil, fmt.Errorf("ensure tag: %w", err)
		}
	}

	placeholders := strings.Repeat("?,", len(clean))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := `SELECT id, tag_type, name FROM tags WHERE tag_type = ? AND name IN (` + placeholders + `)`
	args := make([]any, 0, len(clean)+1)
	args = append(args, tagType)
	for _, name := range clean {
		args = append(args, name)
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select ensured tags: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Tag, 0, len(clean))
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.TagType, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan ensured tag row: %w", err)
		}
		result = append(result, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ensured tags: %w", err)
	}

	return result, nil
}

func (r *ExerciseRepository) ReplaceExerciseTagsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, tagIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exercise_tags WHERE exercise_id = ?`, exerciseID); err != nil {
		return fmt.Errorf("clear exercise tags: %w", err)
	}

	for _, tagID := range tagIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_tags(exercise_id, tag_id) VALUES(?, ?)`, exerciseID, tagID); err != nil {
			return fmt.Errorf("insert exercise tag: %w", err)
		}
	}

	return nil
}

func (r *ExerciseRepository) EnsureContraindicationsTx(ctx context.Context, tx *sql.Tx, labels []string) ([]domain.Contraindication, error) {
	clean := normalizeStrings(labels)
	if len(clean) == 0 {
		return nil, nil
	}

	for _, label := range clean {
		code := contraCode(label)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO contraindications(code, label) VALUES(?, ?)`, code, label); err != nil {
			return nil, fmt.Errorf("ensure contraindication: %w", err)
		}
	}

	placeholders := strings.Repeat("?,", len(clean))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := `SELECT id, code, label FROM contraindications WHERE label IN (` + placeholders + `)`
	args := make([]any, 0, len(clean))
	for _, label := range clean {
		args = append(args, label)
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select ensured contraindications: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Contraindication, 0, len(clean))
	for rows.Next() {
		var item domain.Contraindication
		if err := rows.Scan(&item.ID, &item.Code, &item.Label); err != nil {
			return nil, fmt.Errorf("scan contraindication row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contraindication rows: %w", err)
	}

	return items, nil
}

func (r *ExerciseRepository) ReplaceExerciseContraindicationsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, contraindicationIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exercise_contraindications WHERE exercise_id = ?`, exerciseID); err != nil {
		return fmt.Errorf("clear exercise contraindications: %w", err)
	}

	for _, id := range contraindicationIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_contraindications(exercise_id, contraindication_id) VALUES(?, ?)`, exerciseID, id); err != nil {
			return fmt.Errorf("insert exercise contraindication: %w", err)
		}
	}

	return nil
}

func (r *ExerciseRepository) EnsureBodyRegionsTx(ctx context.Context, tx *sql.Tx, names []string) ([]domain.BodyRegion, error) {
	clean := normalizeStrings(names)
	if len(clean) == 0 {
		return nil, nil
	}

	for _, name := range clean {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO body_regions(name) VALUES(?)`, name); err != nil {
			return nil, fmt.Errorf("ensure body region: %w", err)
		}
	}

	placeholders := strings.Repeat("?,", len(clean))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query := `SELECT id, name FROM body_regions WHERE name IN (` + placeholders + `)`
	args := make([]any, 0, len(clean))
	for _, name := range clean {
		args = append(args, name)
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select ensured body regions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.BodyRegion, 0, len(clean))
	for rows.Next() {
		var item domain.BodyRegion
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, fmt.Errorf("scan body region row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate body region rows: %w", err)
	}

	return items, nil
}

func (r *ExerciseRepository) ReplaceExerciseBodyRegionsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, bodyRegionIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exercise_body_regions WHERE exercise_id = ?`, exerciseID); err != nil {
		return fmt.Errorf("clear exercise body regions: %w", err)
	}

	for _, id := range bodyRegionIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_body_regions(exercise_id, body_region_id) VALUES(?, ?)`, exerciseID, id); err != nil {
			return fmt.Errorf("insert exercise body region: %w", err)
		}
	}

	return nil
}

func (r *ExerciseRepository) DeleteExerciseTagLinksByTypeTx(ctx context.Context, tx *sql.Tx, exerciseID int64, tagType string) error {
	const q = `
DELETE FROM exercise_tags
WHERE exercise_id = ? AND tag_id IN (SELECT id FROM tags WHERE tag_type = ?)`

	if _, err := tx.ExecContext(ctx, q, exerciseID, tagType); err != nil {
		return fmt.Errorf("delete exercise tags by type: %w", err)
	}

	return nil
}

func (r *ExerciseRepository) loadRelations(ctx context.Context, runner queryRunner, exercise *domain.Exercise) error {
	tags, err := loadExerciseTags(ctx, runner, exercise.ID)
	if err != nil {
		return err
	}
	contra, err := loadExerciseContraindications(ctx, runner, exercise.ID)
	if err != nil {
		return err
	}
	bodyRegions, err := loadExerciseBodyRegions(ctx, runner, exercise.ID)
	if err != nil {
		return err
	}

	exercise.Tags = tags
	exercise.Contraindications = contra
	exercise.BodyRegions = bodyRegions
	return nil
}

func loadExerciseTags(ctx context.Context, runner queryRunner, exerciseID int64) ([]domain.Tag, error) {
	const q = `
SELECT t.id, t.tag_type, t.name
FROM exercise_tags et
JOIN tags t ON t.id = et.tag_id
WHERE et.exercise_id = ?
ORDER BY t.tag_type ASC, t.name ASC`

	rows, err := runner.QueryContext(ctx, q, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("list exercise tags: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Tag, 0)
	for rows.Next() {
		var item domain.Tag
		if err := rows.Scan(&item.ID, &item.TagType, &item.Name); err != nil {
			return nil, fmt.Errorf("scan exercise tag row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercise tag rows: %w", err)
	}

	return items, nil
}

func loadExerciseContraindications(ctx context.Context, runner queryRunner, exerciseID int64) ([]domain.Contraindication, error) {
	const q = `
SELECT c.id, c.code, c.label
FROM exercise_contraindications ec
JOIN contraindications c ON c.id = ec.contraindication_id
WHERE ec.exercise_id = ?
ORDER BY c.label ASC`

	rows, err := runner.QueryContext(ctx, q, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("list exercise contraindications: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Contraindication, 0)
	for rows.Next() {
		var item domain.Contraindication
		if err := rows.Scan(&item.ID, &item.Code, &item.Label); err != nil {
			return nil, fmt.Errorf("scan exercise contraindication row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercise contraindication rows: %w", err)
	}

	return items, nil
}

func loadExerciseBodyRegions(ctx context.Context, runner queryRunner, exerciseID int64) ([]domain.BodyRegion, error) {
	const q = `
SELECT br.id, br.name
FROM exercise_body_regions ebr
JOIN body_regions br ON br.id = ebr.body_region_id
WHERE ebr.exercise_id = ?
ORDER BY br.name ASC`

	rows, err := runner.QueryContext(ctx, q, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("list exercise body regions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.BodyRegion, 0)
	for rows.Next() {
		var item domain.BodyRegion
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, fmt.Errorf("scan exercise body region row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercise body region rows: %w", err)
	}

	return items, nil
}

func scanExercise(scanner rowScanner) (*domain.Exercise, error) {
	var (
		item      domain.Exercise
		createdAt int64
		updatedAt int64
	)

	err := scanner.Scan(&item.ID, &item.Title, &item.Description, &item.CoachingPoints, &item.Difficulty, &item.SearchText, &item.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	return &item, nil
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func contraCode(label string) string {
	label = strings.ToUpper(strings.TrimSpace(label))
	label = strings.ReplaceAll(label, " ", "_")
	return label
}

var _ repository.ExerciseRepository = (*ExerciseRepository)(nil)
