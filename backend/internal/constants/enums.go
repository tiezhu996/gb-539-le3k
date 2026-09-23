package constants

const (
	MoistureGreen           = "green"
	MoistureFiberSaturation = "fiber_saturation"
	MoistureBoundWater      = "bound_water"
	MoistureTarget          = "target"
)

const (
	LotQueued       = "queued"
	LotConditioning = "conditioning"
	LotDrying       = "drying"
	LotEqualizing   = "equalizing"
	LotCompleted    = "completed"
	LotAborted      = "aborted"
)

const (
	ScheduleDraft       = "draft"
	ScheduleCalculating = "calculating"
	ScheduleProposed    = "proposed"
	ScheduleFailed      = "failed"
	ScheduleReviewed    = "reviewed"
	ScheduleAccepted    = "accepted"
	ScheduleVoided      = "voided"
	ScheduleSuperseded  = "superseded"
)

var MoistureStages = []string{MoistureGreen, MoistureFiberSaturation, MoistureBoundWater, MoistureTarget}
var LotStates = []string{LotQueued, LotConditioning, LotDrying, LotEqualizing, LotCompleted, LotAborted}
var ScheduleStates = []string{ScheduleDraft, ScheduleCalculating, ScheduleProposed, ScheduleFailed, ScheduleReviewed, ScheduleAccepted, ScheduleVoided, ScheduleSuperseded}

func ValidRole(role string) bool {
	switch role {
	case "admin", "kiln_engineer", "quality_analyst", "reviewer", "auditor":
		return true
	}
	return false
}

type MoistureStage string

func (s MoistureStage) Valid() bool {
	for _, item := range MoistureStages {
		if string(s) == item {
			return true
		}
	}
	return false
}

type ScheduleState string

func (s ScheduleState) Valid() bool {
	for _, item := range ScheduleStates {
		if string(s) == item {
			return true
		}
	}
	return false
}
