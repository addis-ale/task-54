PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS users_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('admin', 'front_desk', 'charge_nurse', 'therapist', 'aide', 'training_coordinator', 'finance_clerk', 'auditor', 'clinician')),
    failed_login_count INTEGER NOT NULL DEFAULT 0,
    locked_until INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT OR IGNORE INTO users_new(id, username, password_hash, role, failed_login_count, locked_until, created_at, updated_at)
SELECT id, username, password_hash,
       CASE
         WHEN role = 'clinician' THEN 'therapist'
         ELSE role
       END,
       failed_login_count,
       locked_until,
       created_at,
       updated_at
FROM users;

DROP TABLE IF EXISTS users;
ALTER TABLE users_new RENAME TO users;

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);

PRAGMA foreign_keys = ON;
