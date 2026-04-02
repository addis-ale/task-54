package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"clinic-admin-suite/internal/domain"
)

var ErrNotFound = errors.New("repository: not found")

type UserRepository interface {
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) error
	RecordFailedLogin(ctx context.Context, userID int64, failedCount int, lockedUntil *time.Time) error
	ResetFailedLogins(ctx context.Context, userID int64) error
}

type SessionRepository interface {
	Create(ctx context.Context, session *domain.Session) error
	GetActiveByTokenHash(ctx context.Context, tokenHash string, now time.Time) (*domain.Session, *domain.User, error)
	TouchActivity(ctx context.Context, sessionID int64, lastSeenAt, newExpiresAt time.Time) error
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
}

type AuditRepository interface {
	LastHash(ctx context.Context) (*string, error)
	LastHashTx(ctx context.Context, tx *sql.Tx) (*string, error)
	Append(ctx context.Context, event *domain.AuditEvent) error
	AppendTx(ctx context.Context, tx *sql.Tx, event *domain.AuditEvent) error
}

type BedFilter struct {
	WardID *int64
	Status string
}

type AdmissionFilter struct {
	Status    string
	PatientID *int64
	BedID     *int64
}

type WorkOrderFilter struct {
	Status      string
	AssignedTo  *int64
	Priority    string
	ServiceType string
}

type KPIRollupFilter struct {
	BucketGranularity string
	From              *time.Time
	To                *time.Time
}

type ExerciseFilter struct {
	Query             string
	Difficulty        string
	Tags              []string
	Equipment         []string
	Contraindications []string
	BodyRegions       []string
	CoachingPoints    []string
}

type PatientRepository interface {
	List(ctx context.Context) ([]domain.Patient, error)
	GetByID(ctx context.Context, id int64) (*domain.Patient, error)
	GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.Patient, error)
	Create(ctx context.Context, patient *domain.Patient) error
}

type WardRepository interface {
	List(ctx context.Context) ([]domain.Ward, error)
	GetByID(ctx context.Context, id int64) (*domain.Ward, error)
	Create(ctx context.Context, ward *domain.Ward) error
}

type BedRepository interface {
	List(ctx context.Context, filter BedFilter) ([]domain.Bed, error)
	ListOccupancy(ctx context.Context) ([]domain.BedOccupancy, error)
	GetByID(ctx context.Context, id int64) (*domain.Bed, error)
	GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.Bed, error)
	Create(ctx context.Context, bed *domain.Bed) error
	UpdateStatusWithVersion(ctx context.Context, id int64, status string, expectedVersion int64) (bool, error)
	UpdateStatusWithVersionTx(ctx context.Context, tx *sql.Tx, id int64, status string, expectedVersion int64) (bool, error)
}

type AdmissionRepository interface {
	List(ctx context.Context, filter AdmissionFilter) ([]domain.Admission, error)
	GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.Admission, error)
	FindActiveByBedIDTx(ctx context.Context, tx *sql.Tx, bedID int64) (*domain.Admission, error)
	CreateTx(ctx context.Context, tx *sql.Tx, admission *domain.Admission) error
	UpdateBedAndVersionTx(ctx context.Context, tx *sql.Tx, admissionID, toBedID, expectedVersion int64) (bool, error)
	DischargeTx(ctx context.Context, tx *sql.Tx, admissionID, expectedVersion int64, dischargedAt time.Time) (bool, error)
	AddAssignmentHistoryTx(ctx context.Context, tx *sql.Tx, history *domain.BedAssignmentHistory) error
}

type WorkOrderRepository interface {
	List(ctx context.Context, filter WorkOrderFilter) ([]domain.WorkOrder, error)
	GetByID(ctx context.Context, id int64) (*domain.WorkOrder, error)
	GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.WorkOrder, error)
	Create(ctx context.Context, workOrder *domain.WorkOrder) error
	StartTx(ctx context.Context, tx *sql.Tx, id, expectedVersion int64, startedAt time.Time) (bool, error)
	CompleteTx(ctx context.Context, tx *sql.Tx, id, expectedVersion int64, completedAt time.Time) (bool, error)
	AggregateMetrics(ctx context.Context, from, to time.Time) ([]domain.KPIServiceRollup, error)
	AggregateMetricsTx(ctx context.Context, tx *sql.Tx, from, to time.Time) ([]domain.KPIServiceRollup, error)
}

type KPIRollupRepository interface {
	Upsert(ctx context.Context, rollup *domain.KPIServiceRollup) error
	UpsertTx(ctx context.Context, tx *sql.Tx, rollup *domain.KPIServiceRollup) error
	List(ctx context.Context, filter KPIRollupFilter) ([]domain.KPIServiceRollup, error)
}

type JobRunRepository interface {
	Create(ctx context.Context, run *domain.JobRun) error
	CreateTx(ctx context.Context, tx *sql.Tx, run *domain.JobRun) error
}

type ExerciseRepository interface {
	List(ctx context.Context, filter ExerciseFilter) ([]domain.Exercise, error)
	GetByID(ctx context.Context, id int64) (*domain.Exercise, error)
	CreateTx(ctx context.Context, tx *sql.Tx, exercise *domain.Exercise) error
	UpdateTx(ctx context.Context, tx *sql.Tx, exerciseID, expectedVersion int64, title, description, coachingPoints, difficulty, searchText string) (bool, error)
	ListTags(ctx context.Context, tagType string) ([]domain.Tag, error)
	EnsureTagsTx(ctx context.Context, tx *sql.Tx, tagType string, names []string) ([]domain.Tag, error)
	ReplaceExerciseTagsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, tagIDs []int64) error
	EnsureContraindicationsTx(ctx context.Context, tx *sql.Tx, labels []string) ([]domain.Contraindication, error)
	ReplaceExerciseContraindicationsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, contraindicationIDs []int64) error
	EnsureBodyRegionsTx(ctx context.Context, tx *sql.Tx, names []string) ([]domain.BodyRegion, error)
	ReplaceExerciseBodyRegionsTx(ctx context.Context, tx *sql.Tx, exerciseID int64, bodyRegionIDs []int64) error
	DeleteExerciseTagLinksByTypeTx(ctx context.Context, tx *sql.Tx, exerciseID int64, tagType string) error
}

type MediaRepository interface {
	Create(ctx context.Context, asset *domain.MediaAsset) error
	GetByID(ctx context.Context, id int64) (*domain.MediaAsset, error)
	ListByExerciseID(ctx context.Context, exerciseID int64) ([]domain.MediaAsset, error)
}

type ExamScheduleFilter struct {
	Date        *time.Time
	RoomID      *int64
	ProctorID   *int64
	CandidateID *int64
}

type ExamScheduleRepository interface {
	List(ctx context.Context, filter ExamScheduleFilter) ([]domain.ExamSchedule, error)
	GetByID(ctx context.Context, id int64) (*domain.ExamSchedule, error)
	GetByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*domain.ExamSchedule, error)
	CreateTx(ctx context.Context, tx *sql.Tx, schedule *domain.ExamSchedule) error
	ReplaceCandidatesTx(ctx context.Context, tx *sql.Tx, scheduleID int64, candidateIDs []int64) error
	ListCandidatesTx(ctx context.Context, tx *sql.Tx, scheduleID int64) ([]int64, error)
	ListCandidates(ctx context.Context, scheduleID int64) ([]int64, error)
	DetectConflictsTx(ctx context.Context, tx *sql.Tx, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error)
	DetectConflicts(ctx context.Context, excludeScheduleID *int64, roomID, proctorID int64, candidateIDs []int64, startAt, endAt time.Time) ([]domain.ScheduleConflict, error)
}

type IdempotencyRepository interface {
	GetActive(ctx context.Context, actorID int64, routeKey, key string, now time.Time) (*domain.IdempotencyKeyRecord, error)
	Create(ctx context.Context, record *domain.IdempotencyKeyRecord) error
	GetActiveTx(ctx context.Context, tx *sql.Tx, actorID int64, routeKey, key string, now time.Time) (*domain.IdempotencyKeyRecord, error)
	CreateTx(ctx context.Context, tx *sql.Tx, record *domain.IdempotencyKeyRecord) error
}

type PaymentFilter struct {
	Status  string
	Method  string
	Gateway string
	ShiftID string
}

type PaymentRepository interface {
	List(ctx context.Context, filter PaymentFilter) ([]domain.Payment, error)
	CreateTx(ctx context.Context, tx *sql.Tx, payment *domain.Payment) error
	GetByID(ctx context.Context, id int64) (*domain.Payment, error)
	ListSucceededByShiftTx(ctx context.Context, tx *sql.Tx, shiftID string) ([]domain.Payment, int64, error)
}

type PaymentEventRepository interface {
	CreateTx(ctx context.Context, tx *sql.Tx, event *domain.PaymentEvent) error
}

type SettlementRepository interface {
	CreateTx(ctx context.Context, tx *sql.Tx, settlement *domain.Settlement) error
	AddItemTx(ctx context.Context, tx *sql.Tx, item *domain.SettlementItem) error
}
