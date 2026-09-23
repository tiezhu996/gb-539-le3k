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
)

var MoistureStages = []string{MoistureGreen, MoistureFiberSaturation, MoistureBoundWater, MoistureTarget}
var LotStates = []string{LotQueued, LotConditioning, LotDrying, LotEqualizing, LotCompleted, LotAborted}
var ScheduleStates = []string{ScheduleDraft, ScheduleCalculating, ScheduleProposed, ScheduleFailed, ScheduleReviewed, ScheduleAccepted, ScheduleVoided}

// Schedule expiry reasons describe why a plan no longer reflects a lot's
// current measurement basis. They are stored verbatim so the UI can show them
// without replaying the originating reading change.
const (
	ScheduleExpiryReadingBackfilled = "reading_backfilled"
	ScheduleExpiryReadingVoided     = "reading_voided"
	ScheduleExpiryReadingCorrected  = "reading_corrected"
)

var ScheduleExpiryReasons = []string{ScheduleExpiryReadingBackfilled, ScheduleExpiryReadingVoided, ScheduleExpiryReadingCorrected}

// ScheduleExpirableStates are the lifecycle states a plan can be referenced and
// acted upon from. Terminal and transient states are never marked expired.
var ScheduleExpirableStates = []string{ScheduleProposed, ScheduleReviewed, ScheduleAccepted}

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
