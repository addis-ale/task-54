# CareOps Clinic Administration Suite

A rehab and long-term care facility operations platform with server-rendered HTMX UI, built on Go/Fiber and SQLite — fully self-hosted with no internet dependency.

## Project Structure

- `cmd/server` — executable entrypoint
- `internal/api` — Fiber routes, handlers, middleware
- `internal/service` — application services and security policies
- `internal/repository` — repository interfaces, SQLite implementations, migrations
- `internal/domain` — core entities and RBAC definitions

## Implemented Foundations

- SQLite initialization and migration runner
- `users`, `sessions`, and append-only `audit_events` tables
- Clinical core tables: `patients`, `wards`, `beds`, `admissions`, `bed_assignment_history`
- `POST /api/v1/auth/login` with bcrypt password verification
- Session cookies configured as `HttpOnly` and `Secure`
- RBAC middleware with role-to-permission mapping
- AuditService login success/failure audit events with hash chain support
- AdmissionsService with transactional assignment, transfer, and discharge
- Version-based optimistic locking for bed status updates via `If-Match-Version`
- HTMX occupancy board fragment at `GET /ui/occupancy/board`
- Work order lifecycle (queue/start/complete) with latency capture
- Hourly KPI rollup background ticker with hybrid real-time + rollup querying
- Job run observability (`job_runs`) for work order completion and KPI rollups
- Exercise Library CMS with tags, body-region/contraindication joins, and FTS5-backed search
- Local media ingestion under `/data/media/{yy}/{mm}/{media_id}_{variant}.{ext}` with checksum metadata
- Media streaming endpoint with HTTP Range support and UI LRU cache simulator
- Exam scheduling engine with overlap conflict detection (room/proctor/candidate)
- Strict 24-hour idempotency for authenticated write endpoints (`POST/PATCH/PUT/DELETE`) via `Idempotency-Key`
- Unified payments + local gateway adapters and encrypted PII storage for payment references
- Additional local offline gateways for finance workflows: cash/check/facility charge/imported card batch/card
- Payment refund workflow via `POST /api/v1/payments/{payment_id}/refunds`
- Shift settlement batch reconciliation with discrepancy logging and settlement items
- Diagnostics bundle export with structured logs, schema versions, and health/audit-chain snapshot
- Finance report exports in CSV/XLSX formats and ops summary endpoint for admin/auditor dashboards
- Frontend app shell at `GET /app` with static assets under `GET /assets/*`

---

## Getting Started with Docker

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) ≥ 24.0
- [Docker Compose](https://docs.docker.com/compose/) ≥ 2.0

### 1. Configure Environment

Copy the example environment file and adjust values as needed:

```bash
cp .env.example .env
```

Key variables (see [Environment Variables](#environment-variables) below for full reference).

### 2. Build and Start

```bash
docker compose up --build
```

The server will be available at **http://localhost:8080**. You can access the UI at **http://localhost:8080/app**.

#### Default Credentials
On the first boot, an administrator account is automatically generated based on your `.env` configuration. If you used the provided `.env.example`, the default login is:
- **Username**: `admin`
- **Password**: `AdminPassword1!`

> **Note:** It is highly recommended to change this password or set your own secure credentials in the `.env` file before deploying.

To run in the background:

```bash
docker compose up --build -d
```

### 3. Seed Demo Data

Run the seed command inside the container after the service is up:

```bash
docker compose exec app ./seed
```

This seeds default wards, patients, and beds if missing.

### 4. Stop the Suite

```bash
docker compose down
```

To also remove persisted volumes (database, media, logs):

```bash
docker compose down -v
```

---

## Environment Variables

Set these in your `.env` file or via `docker compose` environment overrides.

| Variable | Default | Description |
|---|---|---|
| `APP_ADDR` | `:8080` | Server listen address |
| `APP_DB_PATH` | `/data/clinic.db` | SQLite database file path (inside container) |
| `APP_MEDIA_ROOT` | `/data/media` | Media storage root (inside container) |
| `APP_STRUCTURED_LOG_PATH` | `/data/logs/structured.log` | Structured log output path |
| `APP_DIAGNOSTICS_ROOT` | `/data/diagnostics` | Diagnostics bundle output path |
| `SESSION_COOKIE_NAME` | `clinic_session` | Session cookie name |
| `SESSION_TTL` | `15m` | Session idle timeout |
| `SESSION_COOKIE_SECURE` | `true` | Set `false` only behind HTTP (non-TLS) reverse proxy |
| `BCRYPT_COST` | `12` | bcrypt work factor |
| `BOOTSTRAP_ADMIN_USERNAME` | *(none)* | Auto-created admin username on first boot |
| `BOOTSTRAP_ADMIN_PASSWORD` | *(none)* | Must satisfy 12-character minimum policy |
| `APP_MASTER_KEY_B64` | *(none)* | Base64-encoded AES-256-GCM key for field encryption |

> **Note:** `APP_MASTER_KEY_B64` is required for encrypted fields (e.g., payer reference numbers). Generate a key with:
> ```bash
> docker compose run --rm app ./keygen
> ```

---

## Key API Endpoints

| Category | Endpoints |
|---|---|
| **Auth** | `POST /api/v1/auth/login`, `GET /api/v1/auth/me` |
| **Wards** | `GET /api/v1/wards`, `POST /api/v1/wards` |
| **Patients** | `GET /api/v1/patients`, `POST /api/v1/patients` |
| **Beds** | `GET /api/v1/beds`, `POST /api/v1/beds`, `PATCH /api/v1/beds/{bed_id}` |
| **Admissions** | `GET /api/v1/admissions`, `POST /api/v1/admissions`, `POST /api/v1/admissions/{id}/transfer`, `POST /api/v1/admissions/{id}/discharge` |
| **Work Orders** | `GET /api/v1/work-orders`, `POST /api/v1/work-orders`, `POST /api/v1/work-orders/{id}/start`, `POST /api/v1/work-orders/{id}/complete` |
| **KPIs** | `GET /api/v1/kpis/service-delivery` |
| **Exercises** | `GET /api/v1/exercises`, `POST /api/v1/exercises`, `PATCH /api/v1/exercises/{id}`, `GET /api/v1/exercises/{id}` |
| **Tags** | `GET /api/v1/tags`, `POST /api/v1/exercises/{id}/tags` |
| **Media** | `POST /api/v1/media`, `GET /api/v1/media/{id}`, `GET /api/v1/media/{id}/stream` |
| **Exam Scheduling** | `GET /api/v1/exam-schedules`, `POST /api/v1/exam-schedules`, `POST /api/v1/exam-schedules/{id}/validate` |
| **Payments** | `GET /api/v1/payments`, `POST /api/v1/payments` |
| **Refunds** | `POST /api/v1/payments/{payment_id}/refunds` |
| **Settlements** | `POST /api/v1/settlements/run` |
| **Diagnostics** | `POST /api/v1/diagnostics/export` |
| **Reports** | `GET /api/v1/reports/ops/summary`, `GET /api/v1/reports/finance/export?format=csv\|xlsx` |
| **UI Shell** | `GET /app`, `GET /assets/*` |
| **UI Fragments** | `GET /ui/occupancy/board`, `GET /ui/cache/lru` |

---

## Data Persistence

Docker volumes are used to persist all facility data across container restarts:

| Volume | Purpose |
|---|---|
| `clinic_data` | SQLite database |
| `clinic_media` | Exercise library media files |
| `clinic_logs` | Structured logs |
| `clinic_diagnostics` | Diagnostic bundle exports |
| `clinic_reports` | Scheduled report output folder |

Volumes are defined in `docker-compose.yml` and survive `docker compose down` unless `-v` is passed.
