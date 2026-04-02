package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
)

type ExerciseService struct {
	db        *sql.DB
	exercises repository.ExerciseRepository
	media     repository.MediaRepository
}

func NewExerciseService(db *sql.DB, exercises repository.ExerciseRepository, media repository.MediaRepository) *ExerciseService {
	return &ExerciseService{db: db, exercises: exercises, media: media}
}

type CreateExerciseInput struct {
	Title             string
	Description       string
	CoachingPoints    string
	Difficulty        string
	Tags              []string
	Equipment         []string
	Contraindications []string
	BodyRegions       []string
}

type UpdateExerciseInput struct {
	ExerciseID        int64
	ExpectedVersion   int64
	Title             string
	Description       string
	CoachingPoints    string
	Difficulty        string
	Tags              []string
	Equipment         []string
	Contraindications []string
	BodyRegions       []string
}

type UpdateExerciseTagsInput struct {
	ExerciseID int64
	TagType    string
	Attach     []string
	Detach     []string
}

func (s *ExerciseService) List(ctx context.Context, filter repository.ExerciseFilter) ([]domain.Exercise, error) {
	if strings.TrimSpace(filter.Difficulty) != "" && !domain.IsValidExerciseDifficulty(strings.TrimSpace(filter.Difficulty)) {
		return nil, fmt.Errorf("%w: invalid exercise difficulty", ErrValidation)
	}
	return s.exercises.List(ctx, filter)
}

func (s *ExerciseService) GetByID(ctx context.Context, id int64) (*domain.Exercise, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: exercise_id must be positive", ErrValidation)
	}

	exercise, err := s.exercises.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: exercise not found", ErrNotFound)
		}
		return nil, err
	}

	media, err := s.media.ListByExerciseID(ctx, id)
	if err != nil {
		return nil, err
	}
	exercise.MediaAssets = media

	return exercise, nil
}

func (s *ExerciseService) Create(ctx context.Context, input CreateExerciseInput) (*domain.Exercise, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrValidation)
	}

	difficulty := strings.TrimSpace(strings.ToLower(input.Difficulty))
	if !domain.IsValidExerciseDifficulty(difficulty) {
		return nil, fmt.Errorf("%w: invalid difficulty", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create exercise tx: %w", err)
	}
	defer tx.Rollback()

	exercise := &domain.Exercise{
		Title:          title,
		Description:    strings.TrimSpace(input.Description),
		CoachingPoints: strings.TrimSpace(input.CoachingPoints),
		Difficulty:     difficulty,
		SearchText:     buildExerciseSearchText(title, input.Description, input.CoachingPoints, input.Tags, input.Equipment, input.Contraindications, input.BodyRegions),
	}

	if err := s.exercises.CreateTx(ctx, tx, exercise); err != nil {
		return nil, err
	}

	if err := s.syncRelationsTx(ctx, tx, exercise.ID, input.Tags, input.Equipment, input.Contraindications, input.BodyRegions); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create exercise tx: %w", err)
	}

	return s.GetByID(ctx, exercise.ID)
}

func (s *ExerciseService) Update(ctx context.Context, input UpdateExerciseInput) (*domain.Exercise, error) {
	if input.ExerciseID <= 0 || input.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: exercise_id and expected version must be positive", ErrValidation)
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrValidation)
	}

	difficulty := strings.TrimSpace(strings.ToLower(input.Difficulty))
	if !domain.IsValidExerciseDifficulty(difficulty) {
		return nil, fmt.Errorf("%w: invalid difficulty", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update exercise tx: %w", err)
	}
	defer tx.Rollback()

	updated, err := s.exercises.UpdateTx(
		ctx,
		tx,
		input.ExerciseID,
		input.ExpectedVersion,
		title,
		strings.TrimSpace(input.Description),
		strings.TrimSpace(input.CoachingPoints),
		difficulty,
		buildExerciseSearchText(title, input.Description, input.CoachingPoints, input.Tags, input.Equipment, input.Contraindications, input.BodyRegions),
	)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("%w: stale exercise version", ErrVersionConflict)
	}

	if err := s.syncRelationsTx(ctx, tx, input.ExerciseID, input.Tags, input.Equipment, input.Contraindications, input.BodyRegions); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update exercise tx: %w", err)
	}

	return s.GetByID(ctx, input.ExerciseID)
}

func (s *ExerciseService) UpdateTags(ctx context.Context, input UpdateExerciseTagsInput) (*domain.Exercise, error) {
	if input.ExerciseID <= 0 {
		return nil, fmt.Errorf("%w: exercise_id must be positive", ErrValidation)
	}
	tagType := strings.TrimSpace(strings.ToLower(input.TagType))
	if !domain.IsValidTagType(tagType) {
		return nil, fmt.Errorf("%w: invalid tag_type", ErrValidation)
	}

	current, err := s.exercises.GetByID(ctx, input.ExerciseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: exercise not found", ErrNotFound)
		}
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update exercise tags tx: %w", err)
	}
	defer tx.Rollback()

	attachTags, err := s.exercises.EnsureTagsTx(ctx, tx, tagType, input.Attach)
	if err != nil {
		return nil, err
	}

	detachSet := make(map[string]struct{})
	for _, name := range normalizeSlice(input.Detach) {
		detachSet[name] = struct{}{}
	}

	finalTagIDs := make([]int64, 0)
	selectedByName := make(map[string]int64)
	for _, tag := range current.Tags {
		if tag.TagType != tagType {
			finalTagIDs = append(finalTagIDs, tag.ID)
			continue
		}
		if _, remove := detachSet[tag.Name]; remove {
			continue
		}
		selectedByName[tag.Name] = tag.ID
	}

	for _, tag := range attachTags {
		if _, remove := detachSet[tag.Name]; remove {
			continue
		}
		selectedByName[tag.Name] = tag.ID
	}

	for _, id := range selectedByName {
		finalTagIDs = append(finalTagIDs, id)
	}

	if err := s.exercises.ReplaceExerciseTagsTx(ctx, tx, input.ExerciseID, finalTagIDs); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update exercise tags tx: %w", err)
	}

	return s.GetByID(ctx, input.ExerciseID)
}

func (s *ExerciseService) ListTags(ctx context.Context, tagType string) ([]domain.Tag, error) {
	tagType = strings.TrimSpace(strings.ToLower(tagType))
	if tagType != "" && !domain.IsValidTagType(tagType) {
		return nil, fmt.Errorf("%w: invalid tag_type", ErrValidation)
	}
	return s.exercises.ListTags(ctx, tagType)
}

func (s *ExerciseService) syncRelationsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, tags, equipment, contraindications, bodyRegions []string) error {
	generalTags, err := s.exercises.EnsureTagsTx(ctx, tx, domain.TagTypeGeneral, tags)
	if err != nil {
		return err
	}
	equipmentTags, err := s.exercises.EnsureTagsTx(ctx, tx, domain.TagTypeEquipment, equipment)
	if err != nil {
		return err
	}

	tagIDs := make([]int64, 0, len(generalTags)+len(equipmentTags))
	for _, tag := range generalTags {
		tagIDs = append(tagIDs, tag.ID)
	}
	for _, tag := range equipmentTags {
		tagIDs = append(tagIDs, tag.ID)
	}

	if err := s.exercises.ReplaceExerciseTagsTx(ctx, tx, exerciseID, tagIDs); err != nil {
		return err
	}

	contraRows, err := s.exercises.EnsureContraindicationsTx(ctx, tx, contraindications)
	if err != nil {
		return err
	}
	contraIDs := make([]int64, 0, len(contraRows))
	for _, row := range contraRows {
		contraIDs = append(contraIDs, row.ID)
	}
	if err := s.exercises.ReplaceExerciseContraindicationsTx(ctx, tx, exerciseID, contraIDs); err != nil {
		return err
	}

	bodyRows, err := s.exercises.EnsureBodyRegionsTx(ctx, tx, bodyRegions)
	if err != nil {
		return err
	}
	bodyIDs := make([]int64, 0, len(bodyRows))
	for _, row := range bodyRows {
		bodyIDs = append(bodyIDs, row.ID)
	}
	if err := s.exercises.ReplaceExerciseBodyRegionsTx(ctx, tx, exerciseID, bodyIDs); err != nil {
		return err
	}

	return nil
}

func buildExerciseSearchText(title, description, coachingPoints string, tags, equipment, contraindications, bodyRegions []string) string {
	parts := []string{strings.TrimSpace(title), strings.TrimSpace(description), strings.TrimSpace(coachingPoints)}
	parts = append(parts, normalizeSlice(tags)...)
	parts = append(parts, normalizeSlice(equipment)...)
	parts = append(parts, normalizeSlice(contraindications)...)
	parts = append(parts, normalizeSlice(bodyRegions)...)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func normalizeSlice(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
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
