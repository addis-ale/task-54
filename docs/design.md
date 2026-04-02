# Clinic Administration Suite Design

## 1. Goals and Constraints

- Fully local, offline-first deployment for clinic operations
- Reliable behavior on LAN without cloud dependencies
- Fast UI interactions using HTMX and server-rendered fragments
- Strong auditability for clinical and financial actions
- Maintainable module boundaries for long-term extension

## 2. High-Level Architecture

### Layers

1. Presentation layer
   - Fiber routes for full pages and HTMX fragment endpoints
   - Server-side templates for deterministic rendering and simple client logic
2. Application layer
   - Use-case services (Admissions, Scheduling, Finance, CMS, Metrics)
   - Validation, business rules, conflict checks, transaction orchestration
3. Domain/data layer
   - Repositories over SQLite
   - Explicit transaction boundaries and optimistic locking
4. Infrastructure layer
   - Local media file store, audit logger, background job runner, diagnostics exporter

### Service Boundaries

- `AuthService`: login, session, password policy, authorization checks
- `AdmissionsService`: bed assignment lifecycle and occupancy rules
- `WorkOrderService`: creation/execution/completion
- `KPIService`: metric rollups and query APIs
- `ExerciseService`: CMS metadata, tags, search index management
- `MediaService`: file ingestion, checksum, serving policies
- `SchedulingService`: constraints and overlap detection
- `PaymentsService`: unified transaction model over local gateway adapters
- `SettlementService`: shift close, reconciliation, discrepancy handling
- `AuditService`: append-only event recording and retrieval

## 3. HTMX UI Strategy

- Build page shells for major modules; load dynamic regions as fragments
- Keep endpoints coarse enough for business operations, fine enough for targeted refresh
- Use HTMX swap patterns for low-latency updates:
  - row/card replacement after mutation
  - partial table refresh after filter changes
  - inline conflict banners on failed optimistic updates
- Return compact HTML fragments from `/ui/*` routes and JSON from `/api/v1/*`
- Use polling only for truly live panels (for example occupancy board every 10-15s)

## 4. Data Model Design

### Core Clinical Tables

- `patients(id, mrn, name, dob, ... )`
- `wards(id, name)`
- `beds(id, ward_id, bed_code, status, version, updated_at)`
- `admissions(id, patient_id, bed_id, admitted_at, discharged_at, status, version)`
- `bed_assignment_history(id, admission_id, from_bed_id, to_bed_id, changed_at, actor_id)`

Occupancy is derived as active admissions joined to beds; optional materialized snapshot can be maintained for dashboards.

### Work and Metrics Tables

- `work_orders(id, service_type, created_at, started_at, completed_at, status, assignee_id, version)`
- `kpi_service_rollups(id, bucket_start, bucket_granularity, service_type, total, on_time_15m, execution_rate)`

Use a hybrid strategy:

- Real-time metrics for short windows
- Precomputed rollups for day/week/month views

### CMS and Media Tables

- `exercises(id, title, description, difficulty, body_region, search_text, version)`
- `tags(id, tag_type, name)`
- `exercise_tags(exercise_id, tag_id)`
- `contraindications(id, code, label)`
- `exercise_contraindications(exercise_id, contraindication_id)`
- `media_assets(id, exercise_id, media_type, path, checksum_sha256, duration_ms, bytes, created_at)`

Use indexes on tag joins and difficulty fields. Add FTS5 virtual table for free-text exercise lookup.

### Scheduling Tables

- `exam_schedules(id, exam_id, room_id, proctor_id, start_at, end_at, status, version)`
- `exam_candidates(schedule_id, candidate_id)`

Conflict checks use interval overlap predicates:

- overlap when `start_at < existing.end_at AND end_at > existing.start_at`

### Finance and Settlement Tables

- `payments(id, external_ref, method, gateway, amount, currency, status, received_at, shift_id, idempotency_key, version)`
- `payment_events(id, payment_id, event_type, payload_json, created_at)`
- `settlements(id, shift_id, started_at, finished_at, status, expected_total, actual_total)`
- `settlement_items(id, settlement_id, payment_id, result, discrepancy_reason)`

### Cross-Cutting Tables

- `idempotency_keys(id, actor_id, route_key, key, request_hash, response_code, response_body, expires_at)`
- `audit_events(id, occurred_at, actor_id, action, resource_type, resource_id, before_json, after_json, request_id, hash_prev, hash_self)`
- `job_runs(id, job_type, started_at, finished_at, status, summary_json)`

## 5. Local Media Storage Strategy

- File layout: `/data/media/{yy}/{mm}/{media_id}_{variant}.{ext}`
- Persist metadata in `media_assets`; file path remains opaque to clients
- Validate upload with checksum and size before commit
- Support static serving for small assets and range streaming for video/audio
- Keep a maintenance job for orphan detection and integrity recheck

## 6. Client Cache Strategy (Shared Workstations)

- Use local browser storage with app-level LRU index keyed by user + device profile
- Hard limits: max 2 GB or 200 items, whichever is hit first
- Track `last_accessed_at`, `size_bytes`, and content hash
- Evict least recently used entries atomically on insert when over threshold
- Invalidate on content-version mismatch from server metadata

## 7. Concurrency, Integrity, and Safety

### Idempotency

- Enforce on high-risk writes for 24 hours
- Request hash protects against same key with changed payload
- Return cached result on retry to prevent duplicates

### Optimistic Locking

- Include `version` on mutable rows
- Update with version predicate and increment on success
- On conflict, return latest row snapshot for user merge/retry UX

### Transactions

- Wrap multi-entity operations in SQLite transactions:
  - admission + bed status update
  - payment + payment_event write
  - settlement batch + item writes

## 8. Security Design (Offline)

- Passwords: bcrypt with strong work factor and lockout/backoff policy
- Sessions: secure, httpOnly cookies with rotation on privilege changes
- Field encryption: AES-256-GCM for sensitive columns (for example PII/payment references)
- Key management: master key from OS-protected secret store; local rotation plan with key versioning
- RBAC: role-scoped route guards and UI feature flags from server permissions

## 9. Audit Logging and Diagnostics

- Append-only audit table; no updates/deletes in normal operation
- Chain events using `hash_prev/hash_self` for tamper-evident sequence
- Attach `request_id` to app logs, audit events, and job runs
- Diagnostic bundle includes:
  - recent structured logs
  - schema/migration version
  - job history
  - system health snapshot

## 10. Operational Patterns

- Background jobs:
  - KPI rollup computation
  - settlement scheduler (fixed shift times)
  - cache/media maintenance
  - diagnostic export generation
- Recovery:
  - idempotent rerun of failed jobs
  - checkpointed batch progress for settlements
  - startup consistency checks for critical tables

## 11. Suggested Implementation Order

1. Core auth/session + RBAC scaffolding
2. Beds/admissions workflow and occupancy UI fragments
3. Work orders and KPI rollups
4. Exercise CMS + media pipeline
5. Scheduling conflict engine
6. Payments and settlement batches
7. Audit hardening + diagnostics exporter

## 12. Mapping to Question Set

- Q1: layered modular architecture and service boundaries
- Q2: HTMX fragment-first rendering and endpoint split
- Q3: bed/admission schema and lifecycle transitions
- Q4: KPI model with real-time + precompute hybrid
- Q5: tagging model + indexed/FTS search
- Q6: local media layout and serving strategy
- Q7: shared-workstation LRU cache rules
- Q8: scheduling constraints and overlap detection
- Q9: unified payment abstraction and validation
- Q10: auditable shift settlement workflow
- Q11: idempotency key store and conflict handling
- Q12: optimistic locking with version checks
- Q13: append-only, tamper-evident audit events
- Q14: offline auth and encryption controls
- Q15: logs, request ids, jobs, and diagnostic export
