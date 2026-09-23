package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
)

func testServices(t *testing.T) (context.Context, *model.DryingKiln, *model.TimberLot, LotService, ReadingService, ScheduleService) {
	t.Helper()
	db, err := model.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	kiln := &model.DryingKiln{ID: "kiln-" + t.Name(), KilnCode: "K-" + t.Name(), MaxTemperatureC: 70, MinHumidityPct: 35, KilnState: "ready"}
	lot := &model.TimberLot{ID: "lot-" + t.Name(), LotCode: "L-" + t.Name(), KilnID: kiln.ID, Species: "oak", ThicknessMM: 30, VolumeM3: 2, InitialMoisturePct: 45, TargetMoisturePct: 10, LoadedAt: now, LotState: constants.LotQueued, CreatedBy: "creator", Version: 1}
	if err := db.Create(kiln).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(lot).Error; err != nil {
		t.Fatal(err)
	}
	audit := AuditService{Repo: repository.AuditRepository{DB: db}}
	scheduleRepo := repository.ScheduleRepository{DB: db}
	lots := LotService{Repo: repository.LotRepository{DB: db}, Kilns: repository.KilnRepository{DB: db}, Schedules: scheduleRepo, Audit: audit}
	readings := ReadingService{Repo: repository.ReadingRepository{DB: db}, Lots: repository.LotRepository{DB: db}, Audit: audit}
	schedules := ScheduleService{Repo: scheduleRepo, Lots: repository.LotRepository{DB: db}, Kilns: repository.KilnRepository{DB: db}, Readings: repository.ReadingRepository{DB: db}, Audit: audit}
	readings.Schedules = schedules
	return context.Background(), kiln, lot, lots, readings, schedules
}

func TestReadingImportValidatesTimestampSamplesAndPhysicalBounds(t *testing.T) {
	ctx, _, lot, _, readings, _ := testServices(t)
	invalid := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: "not-a-time", MoisturePct: 30, DryBulbC: 50, WetBulbC: 42}}}
	if _, err := readings.Import(ctx, invalid, "analyst", "r1"); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid time error = %v", err)
	}
	measured := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	duplicate := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: measured, MoisturePct: 30, DryBulbC: 50, WetBulbC: 42}, {SamplePosition: "center", MeasuredAt: measured, MoisturePct: 31, DryBulbC: 50, WetBulbC: 42}}}
	if _, err := readings.Import(ctx, duplicate, "analyst", "r2"); !errors.Is(err, ErrValidation) {
		t.Fatalf("duplicate sample error = %v", err)
	}
	physical := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: measured, MoisturePct: 30, DryBulbC: 40, WetBulbC: 45}}}
	if _, err := readings.Import(ctx, physical, "analyst", "r3"); !errors.Is(err, ErrValidation) {
		t.Fatalf("wet bulb error = %v", err)
	}
	valid := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: measured, MoisturePct: 30, DryBulbC: 50, WetBulbC: 42}}}
	if created, err := readings.Import(ctx, valid, "analyst", "r4"); err != nil || len(created) != 1 || created[0].SamplePosition != "surface" {
		t.Fatalf("valid import = %+v, %v", created, err)
	}
	if _, err := readings.Import(ctx, valid, "analyst", "r5"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate checksum error = %v", err)
	}
}

func TestLotTransitionUsesVersionAndLegalStateMachine(t *testing.T) {
	ctx, _, lot, lots, _, _ := testServices(t)
	updated, err := lots.Transition(ctx, lot.ID, constants.LotConditioning, "engineer", "l1", 1)
	if err != nil || updated.Version != 2 || updated.LotState != constants.LotConditioning {
		t.Fatalf("valid transition = %+v, %v", updated, err)
	}
	if _, err := lots.Transition(ctx, lot.ID, constants.LotDrying, "engineer", "l2", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale transition error = %v", err)
	}
	if _, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "l3", 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("illegal transition error = %v", err)
	}
}

func TestScheduleCalculationIsIdempotentAndReviewIsVersioned(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	transitioned, err := lots.Transition(ctx, lot.ID, constants.LotConditioning, "creator", "s1", 1)
	if err != nil {
		t.Fatal(err)
	}
	lot = &transitioned
	measured := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	input := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: measured, MoisturePct: 44, DryBulbC: 50, WetBulbC: 44}, {SamplePosition: "surface", MeasuredAt: measured, MoisturePct: 43, DryBulbC: 50, WetBulbC: 44}}}
	if _, err := readings.Import(ctx, input, "creator", "s2"); err != nil {
		t.Fatal(err)
	}
	first, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID, IdempotencyKey: "schedule-key"}, "creator", "s3")
	if err != nil || first.ScheduleState != constants.ScheduleProposed {
		t.Fatalf("calculation = %+v, %v", first, err)
	}
	again, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID, IdempotencyKey: "schedule-key"}, "creator", "s4")
	if err != nil || again.ID != first.ID {
		t.Fatalf("idempotency = %+v, %v", again, err)
	}
	if _, err := schedules.Review(ctx, first.ID, constants.ScheduleAccepted, "", "creator", "s5", first.Version); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self approval error = %v", err)
	}
	if _, err := schedules.Review(ctx, first.ID, constants.ScheduleAccepted, "checked", "reviewer", "s6", first.Version-1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale review error = %v", err)
	}
	approved, err := schedules.Review(ctx, first.ID, constants.ScheduleAccepted, "checked", "reviewer", "s7", first.Version)
	if err != nil || approved.ScheduleState != constants.ScheduleAccepted || approved.Version != first.Version+1 {
		t.Fatalf("approval = %+v, %v", approved, err)
	}
}

func TestReadingCorrectionAtomicallyVoidsAndReimports(t *testing.T) {
	ctx, _, lot, _, readings, schedules := testServices(t)
	measured := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	input := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: measured, MoisturePct: 42, DryBulbC: 50, WetBulbC: 44}}}
	created, err := readings.Import(ctx, input, "analyst", "correct-1")
	if err != nil {
		t.Fatal(err)
	}
	replacements, err := readings.Correct(ctx, created[0].ID, input, "analyst", "correct-2", created[0].Version)
	if err != nil || len(replacements) != 1 || replacements[0].SupersedesID != created[0].ID {
		t.Fatalf("correction = %+v, %v", replacements, err)
	}
	original, err := readings.Repo.Get(ctx, created[0].ID)
	if err != nil || original.ReadingQuality != "voided" || original.VoidReason == "" {
		t.Fatalf("original after correction = %+v, %v", original, err)
	}
	accepted, err := readings.Repo.Accepted(ctx, lot.ID)
	if err != nil || len(accepted) != 1 || accepted[0].ID != replacements[0].ID {
		t.Fatalf("active readings = %+v, %v", accepted, err)
	}
	if _, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "analyst", "correct-3"); err != nil {
		t.Fatalf("calculation must use corrected active sample: %v", err)
	}
}

func TestScheduleFreezeAndHistoricalComparison(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	transitioned, err := lots.Transition(ctx, lot.ID, constants.LotConditioning, "engineer", "history-1", lot.Version)
	if err != nil {
		t.Fatal(err)
	}
	lot = &transitioned
	firstTime := time.Now().UTC().Add(-50 * time.Minute).Format(time.RFC3339)
	firstInput := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: firstTime, MoisturePct: 46, DryBulbC: 50, WetBulbC: 44}, {SamplePosition: "surface", MeasuredAt: firstTime, MoisturePct: 44, DryBulbC: 50, WetBulbC: 44}}}
	if _, err = readings.Import(ctx, firstInput, "analyst", "history-2"); err != nil {
		t.Fatal(err)
	}
	baseline, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "history-3")
	if err != nil {
		t.Fatal(err)
	}
	baseline, err = schedules.Review(ctx, baseline.ID, constants.ScheduleAccepted, "approved", "reviewer", "history-4", baseline.Version)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := schedules.Freeze(ctx, baseline.ID, "reviewer", "history-5", baseline.Version)
	if err != nil || frozen.FrozenAt == nil || frozen.FrozenSnapshot == "" {
		t.Fatalf("freeze = %+v, %v", frozen, err)
	}
	if _, err = schedules.Freeze(ctx, baseline.ID, "reviewer", "history-6", frozen.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("second freeze error = %v", err)
	}
	secondTime := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	secondInput := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: secondTime, MoisturePct: 30, DryBulbC: 51, WetBulbC: 43}, {SamplePosition: "surface", MeasuredAt: secondTime, MoisturePct: 28, DryBulbC: 51, WetBulbC: 43}}}
	if _, err = readings.Import(ctx, secondInput, "analyst", "history-7"); err != nil {
		t.Fatal(err)
	}
	current, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "history-8")
	if err != nil || current.ID == baseline.ID {
		t.Fatalf("second calculation = %+v, %v", current, err)
	}
	comparison, err := schedules.Compare(ctx, current.ID, baseline.ID, "analyst", "history-9")
	if err != nil || !comparison.FrozenBaseline || comparison.ScheduleID != current.ID {
		t.Fatalf("comparison = %+v, %v", comparison, err)
	}
	other := model.DryingSchedule{ID: "other-schedule-" + t.Name(), TimberLotID: "other-lot", ScheduleState: constants.ScheduleProposed, Version: 1}
	if err = schedules.Repo.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = schedules.Compare(ctx, current.ID, other.ID, "analyst", "history-10"); !errors.Is(err, ErrValidation) {
		t.Fatalf("cross-lot comparison error = %v", err)
	}
}

// setupCalculatedPlan moves a lot into conditioning, imports readings and
// produces a proposed schedule that the reviewer can accept in tests.
func setupCalculatedPlan(ctx context.Context, t *testing.T, lot *model.TimberLot, lots LotService, readings ReadingService, schedules ScheduleService) model.DryingSchedule {
	t.Helper()
	measured := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	input := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: measured, MoisturePct: 44, DryBulbC: 50, WetBulbC: 44}, {SamplePosition: "surface", MeasuredAt: measured, MoisturePct: 43, DryBulbC: 50, WetBulbC: 44}}}
	if _, err := readings.Import(ctx, input, "analyst", "setup-1"); err != nil {
		t.Fatal(err)
	}
	plan, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "setup-2")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestReadingChangesExpireActiveAndFrozenPlans(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	transitioned, err := lots.Transition(ctx, lot.ID, constants.LotConditioning, "engineer", "expire-1", lot.Version)
	if err != nil {
		t.Fatal(err)
	}
	lot = &transitioned
	plan := setupCalculatedPlan(ctx, t, lot, lots, readings, schedules)
	plan, err = schedules.Review(ctx, plan.ID, constants.ScheduleAccepted, "approved", "reviewer", "expire-2", plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = schedules.Freeze(ctx, plan.ID, "reviewer", "expire-3", plan.Version)
	if err != nil || plan.FrozenSnapshot == "" {
		t.Fatalf("freeze = %+v, %v", plan, err)
	}

	later := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	supplement := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: later, MoisturePct: 40, DryBulbC: 52, WetBulbC: 43}}}
	if _, err = readings.Import(ctx, supplement, "analyst", "expire-4"); err != nil {
		t.Fatal(err)
	}
	expired, err := schedules.Get(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if expired.ScheduleState != constants.ScheduleSuperseded || expired.SupersededAt == nil || expired.SupersededReason != SupersedeReadingSupplemented {
		t.Fatalf("expired plan = %+v", expired)
	}
	// A frozen plan keeps its snapshot after expiration.
	if expired.FrozenAt == nil || expired.FrozenBy == "" || expired.FrozenSnapshot == "" {
		t.Fatalf("frozen snapshot must survive supersession: %+v", expired)
	}
	// Expired plans can no longer be reviewed or frozen.
	if _, err = schedules.Review(ctx, expired.ID, "reviewed", "", "reviewer", "expire-5", expired.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("review expired error = %v", err)
	}
	if _, err = schedules.Freeze(ctx, expired.ID, "reviewer", "expire-6", expired.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("freeze expired error = %v", err)
	}

	current, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "expire-7")
	if err != nil || current.ID == plan.ID {
		t.Fatalf("recalculation = %+v, %v", current, err)
	}
	if current.SupersedesScheduleID != plan.ID {
		t.Fatalf("new plan must point at superseded plan, got %q", current.SupersedesScheduleID)
	}
	linked, err := schedules.Get(ctx, plan.ID)
	if err != nil || linked.SupersededByID != current.ID {
		t.Fatalf("old plan replacement link = %+v, %v", linked, err)
	}
	var supersessionEvents int64
	if err = schedules.Repo.DB.Model(&model.AuditEvent{}).Where("entity = ? AND entity_id = ? AND action = ?", "schedule", plan.ID, "superseded").Count(&supersessionEvents).Error; err != nil || supersessionEvents != 1 {
		t.Fatalf("superseded audit events = %d, %v", supersessionEvents, err)
	}
}

func TestVoidAndCorrectExpirePlansWithDistinctReasons(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	transitioned, err := lots.Transition(ctx, lot.ID, constants.LotConditioning, "engineer", "reason-1", lot.Version)
	if err != nil {
		t.Fatal(err)
	}
	lot = &transitioned
	plan := setupCalculatedPlan(ctx, t, lot, lots, readings, schedules)
	accepted, err := readings.Repo.Accepted(ctx, lot.ID)
	if err != nil || len(accepted) == 0 {
		t.Fatalf("accepted readings = %+v, %v", accepted, err)
	}
	if _, err = readings.Void(ctx, accepted[0].ID, dto.ReadingVoid{Reason: "instrument probe failed on site", Version: accepted[0].Version}, "analyst", "reason-2"); err != nil {
		t.Fatal(err)
	}
	voidedPlan, err := schedules.Get(ctx, plan.ID)
	if err != nil || voidedPlan.ScheduleState != constants.ScheduleSuperseded || voidedPlan.SupersededReason != SupersedeReadingVoided {
		t.Fatalf("void expiry = %+v, %v", voidedPlan, err)
	}

	secondInput := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339), MoisturePct: 41, DryBulbC: 50, WetBulbC: 44}}}
	second, err := readings.Import(ctx, secondInput, "analyst", "reason-3")
	if err != nil {
		t.Fatal(err)
	}
	replan, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "reason-4")
	if err != nil {
		t.Fatal(err)
	}
	correction := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339), MoisturePct: 39, DryBulbC: 50, WetBulbC: 44}}}
	if _, err = readings.Correct(ctx, second[0].ID, correction, "analyst", "reason-5", second[0].Version); err != nil {
		t.Fatal(err)
	}
	correctedPlan, err := schedules.Get(ctx, replan.ID)
	if err != nil || correctedPlan.ScheduleState != constants.ScheduleSuperseded || correctedPlan.SupersededReason != SupersedeReadingCorrected {
		t.Fatalf("correct expiry = %+v, %v", correctedPlan, err)
	}
}

func TestCompletionRequiresLatestPlanFrozen(t *testing.T) {
	ctx, _, lot, lots, _, _ := testServices(t)
	for _, next := range []string{constants.LotConditioning, constants.LotDrying, constants.LotEqualizing} {
		moved, err := lots.Transition(ctx, lot.ID, next, "engineer", "complete-guard", lot.Version)
		if err != nil {
			t.Fatal(err)
		}
		lot = &moved
	}
	if _, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "complete-guard", lot.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("completion without frozen plan error = %v", err)
	}
	if stayed, err := lots.Get(ctx, lot.ID); err != nil || stayed.LotState != constants.LotEqualizing {
		t.Fatalf("lot must stay equalizing, got %+v, %v", stayed, err)
	}
}

func TestCompletionSucceedsWithFrozenLatestPlan(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	moved, err := lots.Transition(ctx, lot.ID, constants.LotConditioning, "engineer", "complete-ok-1", lot.Version)
	if err != nil {
		t.Fatal(err)
	}
	lot = &moved
	plan := setupCalculatedPlan(ctx, t, lot, lots, readings, schedules)
	plan, err = schedules.Review(ctx, plan.ID, constants.ScheduleAccepted, "approved", "reviewer", "complete-ok-2", plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = schedules.Freeze(ctx, plan.ID, "reviewer", "complete-ok-3", plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{constants.LotDrying, constants.LotEqualizing} {
		moved, err = lots.Transition(ctx, lot.ID, next, "engineer", "complete-ok-move", lot.Version)
		if err != nil {
			t.Fatal(err)
		}
		lot = &moved
	}
	done, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "complete-ok-4", lot.Version)
	if err != nil || done.LotState != constants.LotCompleted {
		t.Fatalf("completion = %+v, %v", done, err)
	}
}
