PRAGMA foreign_keys = ON;

ALTER TABLE audit_events ADD COLUMN operator_username TEXT;
ALTER TABLE audit_events ADD COLUMN local_ip TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_events_resource_type ON audit_events(resource_type);
CREATE INDEX IF NOT EXISTS idx_audit_events_resource_id ON audit_events(resource_id);

CREATE TABLE IF NOT EXISTS care_quality_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    resident_id INTEGER NOT NULL,
    checkpoint_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pass', 'watch', 'fail')),
    notes TEXT,
    recorded_by INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(resident_id) REFERENCES patients(id) ON DELETE CASCADE,
    FOREIGN KEY(recorded_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_care_quality_resident_id ON care_quality_checkpoints(resident_id);
CREATE INDEX IF NOT EXISTS idx_care_quality_created_at ON care_quality_checkpoints(created_at DESC);

CREATE TABLE IF NOT EXISTS alert_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    resident_id INTEGER NOT NULL,
    alert_type TEXT NOT NULL,
    severity TEXT NOT NULL CHECK(severity IN ('low', 'medium', 'high', 'critical')),
    state TEXT NOT NULL CHECK(state IN ('open', 'acknowledged', 'resolved')),
    message TEXT NOT NULL,
    recorded_by INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(resident_id) REFERENCES patients(id) ON DELETE CASCADE,
    FOREIGN KEY(recorded_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_events_resident_id ON alert_events(resident_id);
CREATE INDEX IF NOT EXISTS idx_alert_events_state ON alert_events(state);
CREATE INDEX IF NOT EXISTS idx_alert_events_created_at ON alert_events(created_at DESC);

CREATE TABLE IF NOT EXISTS exercise_favorites (
    user_id INTEGER NOT NULL,
    exercise_id INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY(user_id, exercise_id),
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(exercise_id) REFERENCES exercises(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_exercise_favorites_user_id ON exercise_favorites(user_id);

CREATE TABLE IF NOT EXISTS exam_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    subject TEXT NOT NULL,
    duration_minutes INTEGER NOT NULL,
    room_id INTEGER NOT NULL,
    proctor_id INTEGER NOT NULL,
    candidate_ids_json TEXT NOT NULL,
    created_by INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_exam_templates_subject ON exam_templates(subject);

CREATE TABLE IF NOT EXISTS exam_template_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL,
    label TEXT,
    window_start_at INTEGER NOT NULL,
    window_end_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(template_id) REFERENCES exam_templates(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_exam_template_windows_template_id ON exam_template_windows(template_id);

CREATE TABLE IF NOT EXISTS exam_session_drafts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL,
    subject TEXT NOT NULL,
    room_id INTEGER NOT NULL,
    proctor_id INTEGER NOT NULL,
    candidate_ids_json TEXT NOT NULL,
    start_at INTEGER NOT NULL,
    end_at INTEGER NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('draft', 'published')),
    conflict_details_json TEXT,
    published_schedule_id INTEGER,
    created_by INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(template_id) REFERENCES exam_templates(id) ON DELETE CASCADE,
    FOREIGN KEY(published_schedule_id) REFERENCES exam_schedules(id) ON DELETE SET NULL,
    FOREIGN KEY(created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_exam_session_drafts_template_id ON exam_session_drafts(template_id);
CREATE INDEX IF NOT EXISTS idx_exam_session_drafts_status ON exam_session_drafts(status);

CREATE TABLE IF NOT EXISTS report_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_type TEXT NOT NULL,
    format TEXT NOT NULL CHECK(format IN ('csv', 'xlsx')),
    shared_folder_path TEXT NOT NULL,
    filters_json TEXT,
    interval_minutes INTEGER NOT NULL,
    next_run_at INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_report_schedules_next_run_at ON report_schedules(next_run_at);

CREATE TABLE IF NOT EXISTS config_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_key TEXT NOT NULL,
    config_payload_json TEXT NOT NULL,
    created_by INTEGER,
    created_at INTEGER NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_config_versions_key ON config_versions(config_key);
CREATE INDEX IF NOT EXISTS idx_config_versions_active ON config_versions(config_key, is_active);
