# Clinic Administration Suite API Spec

## Document Control

- Status: Draft
- Version: 0.1
- Scope: Local network-only deployment (offline-first)
- Stack context: Go (Fiber), HTMX, SQLite

## API Style and Conventions

### Base Paths

- JSON API: `/api/v1`
- HTMX fragment endpoints: `/ui`

### Transport and Auth

- Protocol: HTTP on local network (HTTPS recommended when LAN certificate is available)
- Auth mode: cookie-based session for UI, bearer token optional for machine clients
- Session timeout: configurable (default 8 hours idle)

### Request Headers

- `Idempotency-Key`: required for mutating financial and scheduling operations
- `If-Match-Version`: required for optimistic-lock protected updates
- `X-Request-ID`: optional client-supplied correlation id; server generates if absent
- `HX-Request`: sent by HTMX clients for fragment responses

### Response Format (JSON)

```json
{
  "data": {},
  "meta": {
    "request_id": "req_01J...",
    "timestamp": "2026-03-31T08:15:30Z"
  },
  "error": null
}
```

Error shape:

```json
{
  "data": null,
  "meta": {
    "request_id": "req_01J..."
  },
  "error": {
    "code": "CONFLICT",
    "message": "Version mismatch",
    "details": {
      "resource": "admission",
      "id": "adm_01J..."
    }
  }
}
```

### Common Query Parameters

- `page` (default 1)
- `page_size` (default 25, max 200)
- `sort` (example: `created_at:desc`)
- `from`, `to` for time windows (ISO-8601 UTC)

## Core Resources

### 1) Authentication

#### `POST /api/v1/auth/login`

- Purpose: authenticate local user
- Request: `{ "username": "...", "password": "..." }`
- Response: user profile + session cookie

#### `POST /api/v1/auth/logout`

- Purpose: terminate session

#### `GET /api/v1/auth/me`

- Purpose: current user, role, permissions

#### `POST /api/v1/auth/change-password`

- Request: `{ "current_password": "...", "new_password": "..." }`

### 2) Beds and Admissions

#### `GET /api/v1/beds`

- Filters: `ward_id`, `status` (`available|occupied|cleaning|maintenance`)

#### `POST /api/v1/beds`

- Creates bed metadata

#### `PATCH /api/v1/beds/{bed_id}`

- Optimistic lock via `If-Match-Version`

#### `GET /api/v1/admissions`

- Filters: `status`, `patient_id`, `bed_id`

#### `POST /api/v1/admissions`

- Assigns patient to bed atomically

#### `POST /api/v1/admissions/{admission_id}/transfer`

- Transfers active admission between beds

#### `POST /api/v1/admissions/{admission_id}/discharge`

- Ends active admission and frees bed

#### `GET /ui/occupancy/board`

- Returns HTML fragment for bed board (HTMX)

### 3) Work Orders and Service KPIs

#### `GET /api/v1/work-orders`

- Filters: `status`, `assigned_to`, `priority`, `service_type`

#### `POST /api/v1/work-orders`

- Creates queued work order

#### `POST /api/v1/work-orders/{work_order_id}/start`

- Marks execution start timestamp

#### `POST /api/v1/work-orders/{work_order_id}/complete`

- Marks completion and records latency

#### `GET /api/v1/kpis/service-delivery`

- Returns execution rate and <=15-minute timeliness
- Params: `from`, `to`, `group_by` (`hour|day|week|service_type`)

#### `POST /api/v1/kpis/recompute`

- Rebuilds KPI aggregates for specified interval (admin)

### 4) Exercise Library CMS

#### `GET /api/v1/exercises`

- Filters: `q`, `tags`, `difficulty`, `contraindications`, `body_region`

#### `POST /api/v1/exercises`

- Creates exercise record with metadata

#### `PATCH /api/v1/exercises/{exercise_id}`

- Updates metadata and searchable fields

#### `GET /api/v1/exercises/{exercise_id}`

- Returns exercise details and media references

#### `GET /api/v1/tags`

- Lists tags by type (`focus|equipment|contraindication`)

#### `POST /api/v1/exercises/{exercise_id}/tags`

- Bulk attach or detach tags

### 5) Local Media

#### `POST /api/v1/media`

- Upload metadata registration (chunked upload optional)

#### `GET /api/v1/media/{media_id}`

- Returns metadata, checksum, variants

#### `GET /api/v1/media/{media_id}/stream`

- Streams media with `Range` support

#### `GET /api/v1/media/{media_id}/download`

- Controlled static file response with audit trail

### 6) Exam Scheduling

#### `GET /api/v1/exam-schedules`

- Filters: `date`, `room_id`, `proctor_id`, `candidate_id`

#### `POST /api/v1/exam-schedules`

- Creates schedule item and runs conflict detection
- Idempotency required

#### `POST /api/v1/exam-schedules/{schedule_id}/validate`

- Re-checks constraints and returns conflicts

#### `PATCH /api/v1/exam-schedules/{schedule_id}`

- Reschedules with optimistic lock

#### `GET /api/v1/exam-conflicts`

- Query potential overlaps within a window

### 7) Financial Transactions and Settlement

#### `GET /api/v1/payments`

- Filters: `status`, `method`, `gateway`, `shift_id`

#### `POST /api/v1/payments`

- Creates payment attempt through unified gateway abstraction
- Idempotency required

#### `POST /api/v1/payments/{payment_id}/void`

- Voids eligible payment

#### `POST /api/v1/settlements/run`

- Executes shift settlement batch (scheduled or manual)

#### `GET /api/v1/settlements`

- Lists settlement batches and reconciliation totals

#### `GET /api/v1/settlements/{settlement_id}`

- Detailed settlement report and discrepancies

### 8) Audit and Observability

#### `GET /api/v1/audit/events`

- Filters: `actor_id`, `action`, `resource_type`, `from`, `to`

#### `GET /api/v1/jobs`

- Background jobs and status (KPI rollups, settlements, exports)

#### `POST /api/v1/diagnostics/export`

- Generates offline diagnostics bundle

#### `GET /api/v1/diagnostics/export/{export_id}`

- Download ready bundle

#### `GET /api/v1/health`

- Local readiness and dependency checks

## HTMX Endpoint Guidance

- Fragment endpoints return HTML only and avoid full layout rendering
- Use narrow resource routes, for example `/ui/beds/{bed_id}/card`
- Include ETag or version marker in fragment payload for lightweight refresh checks
- Prefer server-side sorting/filtering over client-side heavy logic

## Idempotency Contract

- Applicable to: payment creation, settlement run, schedule creation, other high-risk writes
- Key scope: `actor_id + route + idempotency_key`
- TTL: 24 hours
- Replay response: exact original status and payload
- Conflict case: same key with different payload returns `409 IDEMPOTENCY_CONFLICT`

## Optimistic Locking Contract

- Every mutable table includes `version INTEGER NOT NULL DEFAULT 1`
- Clients read `version` and send via `If-Match-Version`
- Server updates with predicate `WHERE id = ? AND version = ?`
- On zero rows affected, return `409 VERSION_CONFLICT`

## Standard Error Codes

- `AUTH_INVALID_CREDENTIALS` (401)
- `AUTH_FORBIDDEN` (403)
- `VALIDATION_ERROR` (422)
- `NOT_FOUND` (404)
- `VERSION_CONFLICT` (409)
- `IDEMPOTENCY_CONFLICT` (409)
- `SCHEDULING_CONFLICT` (409)
- `PAYMENT_GATEWAY_ERROR` (502 mapped to local adapter failure)
- `INTERNAL_ERROR` (500)

## Minimal Audit Events per Endpoint

- Login success/failure
- Bed assignment/transfer/discharge
- Work-order completion
- Payment create/void
- Settlement run and reconciliation outcome
- Exam schedule create/update/cancel
- Admin operations (recompute, diagnostic export)
