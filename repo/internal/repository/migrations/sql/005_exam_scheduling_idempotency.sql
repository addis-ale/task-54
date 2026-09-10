PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS exam_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    exam_id TEXT NOT NULL,
    room_id INTEGER NOT NULL,
    proctor_id INTEGER NOT NULL,
    start_at INTEGER NOT NULL,
    end_at INTEGER NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('scheduled', 'cancelled')),
    version INTEGER NOT NULL DEFAULT 1,
    actor_id INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY(actor_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_exam_schedules_room_time ON exam_schedules(room_id, start_at, end_at);
CREATE INDEX IF NOT EXISTS idx_exam_schedules_proctor_time ON exam_schedules(proctor_id, start_at, end_at);
CREATE INDEX IF NOT EXISTS idx_exam_schedules_status ON exam_schedules(status);

CREATE TABLE IF NOT EXISTS exam_candidates (
    schedule_id INTEGER NOT NULL,
    candidate_id INTEGER NOT NULL,
    PRIMARY KEY(schedule_id, candidate_id),
    FOREIGN KEY(schedule_id) REFERENCES exam_schedules(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_exam_candidates_candidate_id ON exam_candidates(candidate_id);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER NOT NULL,
    route_key TEXT NOT NULL,
    key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    response_body TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(actor_id, route_key, key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_expires_at ON idempotency_keys(expires_at);
