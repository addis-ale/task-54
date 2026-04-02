PRAGMA foreign_keys = ON;

ALTER TABLE exercises ADD COLUMN coaching_points TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_exercises_coaching_points ON exercises(coaching_points);
