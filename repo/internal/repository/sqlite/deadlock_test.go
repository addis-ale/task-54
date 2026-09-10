package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"clinic-admin-suite/internal/domain"
	"clinic-admin-suite/internal/repository"
	"clinic-admin-suite/internal/repository/migrations"
)

func TestExerciseListDeadlockReproduction(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := migrations.Run(ctx, db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Ensure there is at least one exercise
	repo := NewExerciseRepository(db)
	
	exercise := &domain.Exercise{
		Title:      "Test Exercise",
		Difficulty: "beginner",
	}
	
	tx, _ := db.BeginTx(ctx, nil)
	if err := repo.CreateTx(ctx, tx, exercise); err != nil {
		t.Fatalf("failed to create exercise: %v", err)
	}
	tx.Commit()

	// The List call should not hang even if MaxOpenConns is 1 (though we increased it to 25)
	// Because we refactored List to load rows into memory first.
	
	// To be extra sure, we can temporarily set it back to 1 for this test
	db.SetMaxOpenConns(1)
	
	done := make(chan struct{})
	go func() {
		_, err := repo.List(ctx, repository.ExerciseFilter{})
		if err != nil {
			t.Errorf("List failed: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-context.Background().Done():
		// Should not happen
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected: repo.List timed out")
	}
}
