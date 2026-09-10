PRAGMA foreign_keys = ON;

ALTER TABLE work_orders ADD COLUMN patient_id INTEGER REFERENCES patients(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_work_orders_patient_id ON work_orders(patient_id);
