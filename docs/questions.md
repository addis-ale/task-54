1. 01_system_architecture.md
# Question 1: Overall System Architecture

## Context
The system is a fully local, offline-first clinic administration suite using Go (Fiber), HTMX, and SQLite.

## Question
How would you design the overall system architecture to ensure modularity, maintainability, and scalability within a local network-only environment?

## Expected Focus
- Layered architecture (UI, API, DB)
- Separation of concerns
- HTMX + server-rendered patterns
- Service boundaries
2. 02_htmx_ui_design.md
# Question 2: HTMX UI Strategy

## Context
Front-desk staff use HTMX-driven server-rendered interfaces for real-time operations.

## Question
How should the HTMX UI be structured to ensure fast updates, minimal latency, and maintainability?

## Expected Focus
- Partial rendering patterns
- Endpoint design for fragments
- UX responsiveness without SPA complexity
3. 03_bed_occupancy_model.md
# Question 3: Bed and Occupancy Data Modeling

## Context
Users need to view beds, admissions, and occupancy at a glance.

## Question
How would you design the database schema to efficiently track bed assignments, occupancy status, and admissions?

## Expected Focus
- Table relationships
- Status transitions
- Query performance
4. 04_service_delivery_metrics.md
# Question 4: Service Delivery Metrics Calculation

## Context
Dashboards track execution rate and work-order timeliness (within 15 minutes).

## Question
How would you design the logic and data model to compute and store service delivery KPIs?

## Expected Focus
- Time-based calculations
- Aggregation strategies
- Precompute vs real-time
5. 05_exercise_library_cms.md
# Question 5: Exercise Library CMS Design

## Context
Therapists search exercises by tags, difficulty, contraindications, etc.

## Question
How would you design the CMS schema and search functionality for efficient filtering and retrieval?

## Expected Focus
- Tagging system
- Indexing
- Flexible querying
6. 06_local_media_storage.md
# Question 6: Local Media Storage Strategy

## Context
Exercise content includes mixed media stored locally.

## Question
How should media files be stored, indexed, and served efficiently without internet access?

## Expected Focus
- File storage structure
- Metadata handling
- Streaming vs static serving
7. 07_client_cache_strategy.md
# Question 7: Client-Side Caching Strategy

## Context
Clients cache recently viewed content using LRU (2GB or 200 items).

## Question
How would you implement and manage this caching mechanism in a shared workstation environment?

## Expected Focus
- LRU design
- Storage limits
- Cache invalidation
8. 08_exam_scheduling_system.md
# Question 8: Exam Scheduling and Conflict Detection

## Context
Training coordinators schedule exams with constraints on rooms, proctors, and candidates.

## Question
How would you design the scheduling engine to detect and resolve conflicts?

## Expected Focus
- Constraint modeling
- Time overlap detection
- UI adjustment mechanisms
9. 09_financial_transactions.md
# Question 9: Financial Transaction Handling

## Context
Payments come from multiple offline gateways.

## Question
How would you design a robust system to handle diverse payment methods and ensure accurate records?

## Expected Focus
- Unified transaction model
- Gateway abstraction
- Validation rules
10. 10_shift_settlement.md
# Question 10: Shift Settlement Process

## Context
Settlements occur at fixed times daily.

## Question
How would you design the settlement workflow to ensure accuracy and auditability?

## Expected Focus
- Batch processing
- Reconciliation logic
- Error handling
11. 11_idempotency_design.md
# Question 11: Idempotency Key Implementation

## Context
Write operations use idempotency keys valid for 24 hours.

## Question
How would you implement idempotency to prevent duplicate operations in a local system?

## Expected Focus
- Key storage
- Expiration handling
- Conflict resolution
12. 12_optimistic_locking.md
# Question 12: Optimistic Locking Strategy

## Context
The system uses row version checks to prevent conflicting updates.

## Question
How would you implement optimistic locking in SQLite and handle conflicts gracefully?

## Expected Focus
- Version columns
- Update checks
- UX for conflicts
13. 13_audit_logging.md
# Question 13: Audit Logging System

## Context
All privileged actions are logged immutably.

## Question
How would you design an audit logging system that ensures traceability and integrity?

## Expected Focus
- Append-only logs
- Query patterns
- Storage considerations
14. 14_auth_security.md
# Question 14: Authentication and Security Design

## Context
The system uses local authentication with bcrypt and AES-256-GCM encryption.

## Question
How would you design the authentication and encryption system to ensure strong security in an offline environment?

## Expected Focus
- Password policies
- Key management
- Field-level encryption
15. 15_observability_diagnostics.md
# Question 15: Observability and Diagnostics

## Context
The system includes logs, request IDs, job history, and diagnostic exports.

## Question
How would you design observability features to support effective offline debugging and maintenance?

## Expected Focus
- Structured logging
- Correlation IDs
- Diagnostic packaging