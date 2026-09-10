PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS exercises (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    difficulty TEXT NOT NULL CHECK(difficulty IN ('beginner', 'intermediate', 'advanced')),
    search_text TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exercises_difficulty ON exercises(difficulty);

CREATE VIRTUAL TABLE IF NOT EXISTS exercises_fts USING fts5(
    title,
    description,
    search_text,
    content='exercises',
    content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS exercises_ai
AFTER INSERT ON exercises
BEGIN
    INSERT INTO exercises_fts(rowid, title, description, search_text)
    VALUES (new.id, new.title, new.description, new.search_text);
END;

CREATE TRIGGER IF NOT EXISTS exercises_ad
AFTER DELETE ON exercises
BEGIN
    INSERT INTO exercises_fts(exercises_fts, rowid, title, description, search_text)
    VALUES('delete', old.id, old.title, old.description, old.search_text);
END;

CREATE TRIGGER IF NOT EXISTS exercises_au
AFTER UPDATE ON exercises
BEGIN
    INSERT INTO exercises_fts(exercises_fts, rowid, title, description, search_text)
    VALUES('delete', old.id, old.title, old.description, old.search_text);
    INSERT INTO exercises_fts(rowid, title, description, search_text)
    VALUES (new.id, new.title, new.description, new.search_text);
END;

CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_type TEXT NOT NULL CHECK(tag_type IN ('focus', 'equipment', 'general')),
    name TEXT NOT NULL,
    UNIQUE(tag_type, name)
);

CREATE INDEX IF NOT EXISTS idx_tags_tag_type ON tags(tag_type);

CREATE TABLE IF NOT EXISTS exercise_tags (
    exercise_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY(exercise_id, tag_id),
    FOREIGN KEY(exercise_id) REFERENCES exercises(id) ON DELETE CASCADE,
    FOREIGN KEY(tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_exercise_tags_tag_id ON exercise_tags(tag_id);

CREATE TABLE IF NOT EXISTS contraindications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS exercise_contraindications (
    exercise_id INTEGER NOT NULL,
    contraindication_id INTEGER NOT NULL,
    PRIMARY KEY(exercise_id, contraindication_id),
    FOREIGN KEY(exercise_id) REFERENCES exercises(id) ON DELETE CASCADE,
    FOREIGN KEY(contraindication_id) REFERENCES contraindications(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_exercise_contraindications_id ON exercise_contraindications(contraindication_id);

CREATE TABLE IF NOT EXISTS body_regions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS exercise_body_regions (
    exercise_id INTEGER NOT NULL,
    body_region_id INTEGER NOT NULL,
    PRIMARY KEY(exercise_id, body_region_id),
    FOREIGN KEY(exercise_id) REFERENCES exercises(id) ON DELETE CASCADE,
    FOREIGN KEY(body_region_id) REFERENCES body_regions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_exercise_body_regions_id ON exercise_body_regions(body_region_id);

CREATE TABLE IF NOT EXISTS media_assets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    exercise_id INTEGER NOT NULL,
    media_type TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    checksum_sha256 TEXT NOT NULL,
    duration_ms INTEGER,
    bytes INTEGER NOT NULL,
    variant TEXT NOT NULL DEFAULT 'original',
    created_at INTEGER NOT NULL,
    FOREIGN KEY(exercise_id) REFERENCES exercises(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_media_assets_exercise_id ON media_assets(exercise_id);
CREATE INDEX IF NOT EXISTS idx_media_assets_created_at ON media_assets(created_at DESC);
