package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ExerciseFavoriteService struct {
	db *sql.DB
}

func NewExerciseFavoriteService(db *sql.DB) *ExerciseFavoriteService {
	return &ExerciseFavoriteService{db: db}
}

func (s *ExerciseFavoriteService) Toggle(ctx context.Context, userID, exerciseID int64) (bool, error) {
	if userID <= 0 || exerciseID <= 0 {
		return false, fmt.Errorf("%w: user_id and exercise_id must be positive", ErrValidation)
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM exercise_favorites WHERE user_id = ? AND exercise_id = ?`, userID, exerciseID)
	if err != nil {
		return false, fmt.Errorf("delete exercise favorite: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return false, nil
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO exercise_favorites(user_id, exercise_id, created_at) VALUES(?, ?, ?)`, userID, exerciseID, time.Now().UTC().Unix())
	if err != nil {
		return false, fmt.Errorf("insert exercise favorite: %w", err)
	}
	return true, nil
}

func (s *ExerciseFavoriteService) ListIDs(ctx context.Context, userID int64) (map[int64]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT exercise_id FROM exercise_favorites WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("list exercise favorites: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan exercise favorite row: %w", err)
		}
		out[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercise favorite rows: %w", err)
	}
	return out, nil
}
