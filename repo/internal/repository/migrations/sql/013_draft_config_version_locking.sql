PRAGMA foreign_keys = ON;

ALTER TABLE exam_session_drafts ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE config_versions ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
