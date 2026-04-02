PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS patients (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mrn TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    dob TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_patients_name ON patients(name);

CREATE TABLE IF NOT EXISTS wards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS beds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ward_id INTEGER NOT NULL,
    bed_code TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('available', 'occupied', 'cleaning', 'maintenance')),
    version INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(ward_id) REFERENCES wards(id) ON DELETE RESTRICT,
    UNIQUE(ward_id, bed_code)
);

CREATE INDEX IF NOT EXISTS idx_beds_ward_id ON beds(ward_id);
CREATE INDEX IF NOT EXISTS idx_beds_status ON beds(status);

CREATE TABLE IF NOT EXISTS admissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    patient_id INTEGER NOT NULL,
    bed_id INTEGER NOT NULL,
    admitted_at INTEGER NOT NULL,
    discharged_at INTEGER,
    status TEXT NOT NULL CHECK(status IN ('active', 'discharged')),
    version INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE RESTRICT,
    FOREIGN KEY(bed_id) REFERENCES beds(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_admissions_patient_id ON admissions(patient_id);
CREATE INDEX IF NOT EXISTS idx_admissions_bed_id ON admissions(bed_id);
CREATE INDEX IF NOT EXISTS idx_admissions_status ON admissions(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_admissions_active_bed
    ON admissions(bed_id)
    WHERE status = 'active' AND discharged_at IS NULL;

CREATE TABLE IF NOT EXISTS bed_assignment_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admission_id INTEGER NOT NULL,
    from_bed_id INTEGER,
    to_bed_id INTEGER,
    changed_at INTEGER NOT NULL,
    actor_id INTEGER,
    FOREIGN KEY(admission_id) REFERENCES admissions(id) ON DELETE CASCADE,
    FOREIGN KEY(from_bed_id) REFERENCES beds(id) ON DELETE SET NULL,
    FOREIGN KEY(to_bed_id) REFERENCES beds(id) ON DELETE SET NULL,
    FOREIGN KEY(actor_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_bed_assignment_history_admission_id ON bed_assignment_history(admission_id);
CREATE INDEX IF NOT EXISTS idx_bed_assignment_history_changed_at ON bed_assignment_history(changed_at DESC);
