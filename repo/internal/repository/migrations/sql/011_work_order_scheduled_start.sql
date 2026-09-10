PRAGMA foreign_keys = ON;

ALTER TABLE work_orders ADD COLUMN scheduled_start INTEGER;

CREATE INDEX IF NOT EXISTS idx_work_orders_scheduled_start ON work_orders(scheduled_start);
