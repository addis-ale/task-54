PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    external_ref TEXT,
    method TEXT NOT NULL,
    gateway TEXT NOT NULL,
    amount_cents INTEGER NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('succeeded', 'failed', 'voided')),
    received_at INTEGER NOT NULL,
    shift_id TEXT NOT NULL,
    idempotency_key TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    pii_reference_enc TEXT,
    pii_key_version INTEGER,
    failure_reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_payments_shift_id ON payments(shift_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_gateway ON payments(gateway);
CREATE INDEX IF NOT EXISTS idx_payments_received_at ON payments(received_at DESC);

CREATE TABLE IF NOT EXISTS payment_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    payment_id INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(payment_id) REFERENCES payments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_payment_events_payment_id ON payment_events(payment_id);
CREATE INDEX IF NOT EXISTS idx_payment_events_created_at ON payment_events(created_at DESC);

CREATE TABLE IF NOT EXISTS settlements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    shift_id TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    finished_at INTEGER NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('matched', 'discrepancy')),
    expected_total_cents INTEGER NOT NULL,
    actual_total_cents INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_settlements_shift_id ON settlements(shift_id);
CREATE INDEX IF NOT EXISTS idx_settlements_started_at ON settlements(started_at DESC);

CREATE TABLE IF NOT EXISTS settlement_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    settlement_id INTEGER NOT NULL,
    payment_id INTEGER NOT NULL,
    result TEXT NOT NULL CHECK(result IN ('matched', 'discrepancy')),
    discrepancy_reason TEXT,
    FOREIGN KEY(settlement_id) REFERENCES settlements(id) ON DELETE CASCADE,
    FOREIGN KEY(payment_id) REFERENCES payments(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_settlement_items_settlement_id ON settlement_items(settlement_id);
CREATE INDEX IF NOT EXISTS idx_settlement_items_payment_id ON settlement_items(payment_id);
