PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS work_orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_type TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low', 'normal', 'high', 'urgent')),
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    completed_at INTEGER,
    status TEXT NOT NULL CHECK(status IN ('queued', 'in_progress', 'completed')),
    assignee_id INTEGER,
    version INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(assignee_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_work_orders_status ON work_orders(status);
CREATE INDEX IF NOT EXISTS idx_work_orders_service_type ON work_orders(service_type);
CREATE INDEX IF NOT EXISTS idx_work_orders_assignee_id ON work_orders(assignee_id);
CREATE INDEX IF NOT EXISTS idx_work_orders_started_at ON work_orders(started_at);
CREATE INDEX IF NOT EXISTS idx_work_orders_completed_at ON work_orders(completed_at);

CREATE TABLE IF NOT EXISTS kpi_service_rollups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bucket_start INTEGER NOT NULL,
    bucket_granularity TEXT NOT NULL,
    service_type TEXT NOT NULL,
    total INTEGER NOT NULL,
    completed INTEGER NOT NULL,
    on_time_15m INTEGER NOT NULL,
    execution_rate REAL NOT NULL,
    UNIQUE(bucket_start, bucket_granularity, service_type)
);

CREATE INDEX IF NOT EXISTS idx_kpi_rollups_bucket_start ON kpi_service_rollups(bucket_start);
CREATE INDEX IF NOT EXISTS idx_kpi_rollups_granularity ON kpi_service_rollups(bucket_granularity);
CREATE INDEX IF NOT EXISTS idx_kpi_rollups_service_type ON kpi_service_rollups(service_type);

CREATE TABLE IF NOT EXISTS job_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_type TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    finished_at INTEGER NOT NULL,
    status TEXT NOT NULL,
    summary_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_job_runs_job_type ON job_runs(job_type);
CREATE INDEX IF NOT EXISTS idx_job_runs_started_at ON job_runs(started_at DESC);
