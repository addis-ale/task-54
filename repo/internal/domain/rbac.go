package domain

type Role string

const (
	RoleAdmin               Role = "admin"
	RoleFrontDesk           Role = "front_desk"
	RoleChargeNurse         Role = "charge_nurse"
	RoleTherapist           Role = "therapist"
	RoleAide                Role = "aide"
	RoleTrainingCoordinator Role = "training_coordinator"
	RoleFinanceClerk        Role = "finance_clerk"
	RoleAuditor             Role = "auditor"
	RoleClinician           Role = "clinician"

	RoleLegacyClinician Role = RoleClinician
)

type Permission string

const (
	PermissionAuthLogin         Permission = "auth.login"
	PermissionAuthReadMe        Permission = "auth.me.read"
	PermissionAuditRead         Permission = "audit.read"
	PermissionBedsRead          Permission = "beds.read"
	PermissionBedsWrite         Permission = "beds.write"
	PermissionWardsRead         Permission = "wards.read"
	PermissionWardsWrite        Permission = "wards.write"
	PermissionPatientsRead      Permission = "patients.read"
	PermissionPatientsWrite     Permission = "patients.write"
	PermissionAdmissionsRead    Permission = "admissions.read"
	PermissionAdmissionsWrite   Permission = "admissions.write"
	PermissionOccupancyRead     Permission = "occupancy.read"
	PermissionWorkOrdersRead    Permission = "work_orders.read"
	PermissionWorkOrdersWrite   Permission = "work_orders.write"
	PermissionKPIRead           Permission = "kpis.read"
	PermissionExercisesRead     Permission = "exercises.read"
	PermissionExercisesWrite    Permission = "exercises.write"
	PermissionMediaRead         Permission = "media.read"
	PermissionMediaWrite        Permission = "media.write"
	PermissionUICacheRead       Permission = "ui.cache.read"
	PermissionSchedulingRead    Permission = "scheduling.read"
	PermissionSchedulingWrite   Permission = "scheduling.write"
	PermissionCareQualityRead   Permission = "care_quality.read"
	PermissionCareQualityWrite  Permission = "care_quality.write"
	PermissionAlertsRead        Permission = "alerts.read"
	PermissionAlertsWrite       Permission = "alerts.write"
	PermissionPaymentsRead      Permission = "payments.read"
	PermissionPaymentsWrite     Permission = "payments.write"
	PermissionSettlementsRun    Permission = "settlements.run"
	PermissionDiagnosticsExport Permission = "diagnostics.export"
	PermissionReportsRead       Permission = "reports.read"
	PermissionReportsExport     Permission = "reports.export"
	PermissionConfigManage      Permission = "config.manage"
)

var rolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionAuditRead,
		PermissionBedsRead,
		PermissionBedsWrite,
		PermissionWardsRead,
		PermissionWardsWrite,
		PermissionPatientsRead,
		PermissionPatientsWrite,
		PermissionAdmissionsRead,
		PermissionAdmissionsWrite,
		PermissionOccupancyRead,
		PermissionWorkOrdersRead,
		PermissionWorkOrdersWrite,
		PermissionKPIRead,
		PermissionExercisesRead,
		PermissionExercisesWrite,
		PermissionMediaRead,
		PermissionMediaWrite,
		PermissionUICacheRead,
		PermissionSchedulingRead,
		PermissionSchedulingWrite,
		PermissionCareQualityRead,
		PermissionCareQualityWrite,
		PermissionAlertsRead,
		PermissionAlertsWrite,
		PermissionPaymentsRead,
		PermissionPaymentsWrite,
		PermissionSettlementsRun,
		PermissionDiagnosticsExport,
		PermissionReportsRead,
		PermissionReportsExport,
		PermissionConfigManage,
	},
	RoleFrontDesk: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionBedsRead,
		PermissionBedsWrite,
		PermissionWardsRead,
		PermissionPatientsRead,
		PermissionPatientsWrite,
		PermissionAdmissionsRead,
		PermissionAdmissionsWrite,
		PermissionOccupancyRead,
		PermissionWorkOrdersRead,
		PermissionWorkOrdersWrite,
		PermissionKPIRead,
		PermissionExercisesRead,
		PermissionMediaRead,
		PermissionUICacheRead,
		PermissionSchedulingRead,
		PermissionCareQualityRead,
		PermissionAlertsRead,
	},
	RoleChargeNurse: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionBedsRead,
		PermissionBedsWrite,
		PermissionPatientsRead,
		PermissionAdmissionsRead,
		PermissionAdmissionsWrite,
		PermissionOccupancyRead,
		PermissionWorkOrdersRead,
		PermissionWorkOrdersWrite,
		PermissionKPIRead,
		PermissionSchedulingRead,
		PermissionCareQualityRead,
		PermissionCareQualityWrite,
		PermissionAlertsRead,
		PermissionAlertsWrite,
	},
	RoleTherapist: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionPatientsRead,
		PermissionAdmissionsRead,
		PermissionWorkOrdersRead,
		PermissionWorkOrdersWrite,
		PermissionExercisesRead,
		PermissionExercisesWrite,
		PermissionMediaRead,
		PermissionMediaWrite,
		PermissionUICacheRead,
		PermissionCareQualityRead,
		PermissionAlertsRead,
		PermissionAlertsWrite,
	},
	RoleAide: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionPatientsRead,
		PermissionAdmissionsRead,
		PermissionWorkOrdersRead,
		PermissionWorkOrdersWrite,
		PermissionExercisesRead,
		PermissionMediaRead,
		PermissionUICacheRead,
		PermissionCareQualityRead,
		PermissionAlertsRead,
	},
	RoleTrainingCoordinator: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionSchedulingRead,
		PermissionSchedulingWrite,
		PermissionExercisesRead,
		PermissionExercisesWrite,
		PermissionReportsRead,
		PermissionReportsExport,
		PermissionCareQualityRead,
		PermissionAlertsRead,
	},
	RoleFinanceClerk: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionPaymentsRead,
		PermissionPaymentsWrite,
		PermissionSettlementsRun,
		PermissionReportsRead,
		PermissionReportsExport,
		PermissionDiagnosticsExport,
		PermissionCareQualityRead,
		PermissionAlertsRead,
	},
	RoleAuditor: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionAuditRead,
		PermissionReportsRead,
		PermissionReportsExport,
		PermissionDiagnosticsExport,
		PermissionKPIRead,
		PermissionPaymentsRead,
		PermissionSchedulingRead,
		PermissionCareQualityRead,
		PermissionAlertsRead,
	},

	RoleLegacyClinician: {
		PermissionAuthLogin,
		PermissionAuthReadMe,
		PermissionPatientsRead,
		PermissionAdmissionsRead,
		PermissionWorkOrdersRead,
		PermissionWorkOrdersWrite,
		PermissionExercisesRead,
		PermissionMediaRead,
		PermissionUICacheRead,
		PermissionCareQualityRead,
		PermissionAlertsRead,
	},
}

func PermissionsForRole(role string) []Permission {
	permissions, ok := rolePermissions[Role(role)]
	if !ok {
		return nil
	}
	dup := make([]Permission, len(permissions))
	copy(dup, permissions)
	return dup
}

func HasPermissions(role string, required ...Permission) bool {
	granted := PermissionsForRole(role)
	if len(required) == 0 {
		return true
	}
	if len(granted) == 0 {
		return false
	}
	lookup := make(map[Permission]struct{}, len(granted))
	for _, p := range granted {
		lookup[p] = struct{}{}
	}
	for _, need := range required {
		if _, ok := lookup[need]; !ok {
			return false
		}
	}
	return true
}
