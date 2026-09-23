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
)

func approvedFrozenSchedule(t *testing.T, ctx context.Context, lot *model.TimberLot, readings ReadingService, schedules ScheduleService, measuredAt string, moisture float64) model.DryingSchedule {
	t.Helper()
	input := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: measuredAt, MoisturePct: moisture, DryBulbC: 52, WetBulbC: 42}, {SamplePosition: "core", MeasuredAt: measuredAt, MoisturePct: moisture + 3, DryBulbC: 52, WetBulbC: 42}}}
	if _, err := readings.Import(ctx, input, "analyst", "expiry-import"); err != nil {
		t.Fatalf("import readings: %v", err)
	}
	plan, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "expiry-calc")
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	plan, err = schedules.Review(ctx, plan.ID, constants.ScheduleAccepted, "approved", "reviewer", "expiry-review", plan.Version)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	plan, err = schedules.Freeze(ctx, plan.ID, "reviewer", "expiry-freeze", plan.Version)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	return plan
}

func TestReadingChangeExpiresPlanAndKeepsFrozenSnapshot(t *testing.T) {
	ctx, _, lot, _, readings, schedules := testServices(t)
	firstAt := time.Now().UTC().Add(-40 * time.Minute).Format(time.RFC3339)
	plan := approvedFrozenSchedule(t, ctx, lot, readings, schedules, firstAt, 38)

	// A later backfill must mark the frozen plan expired while preserving the
	// lifecycle state and canonical frozen snapshot.
	laterAt := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
	backfill := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: laterAt, MoisturePct: 30, DryBulbC: 53, WetBulbC: 42}, {SamplePosition: "core", MeasuredAt: laterAt, MoisturePct: 32, DryBulbC: 53, WetBulbC: 42}}}
	if _, err := readings.Import(ctx, backfill, "analyst", "expiry-backfill"); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	stale, err := schedules.Get(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.ExpiredAt == nil || stale.ExpiryReason != constants.ScheduleExpiryReadingBackfilled {
		t.Fatalf("plan should be expired with backfill reason, got %+v", stale)
	}
	if stale.ScheduleState != constants.ScheduleAccepted || stale.FrozenAt == nil || stale.FrozenSnapshot == "" {
		t.Fatalf("expired frozen plan must retain state and snapshot, got %+v", stale)
	}
	if stale.SupersededByID != "" {
		t.Fatalf("expired plan must wait for a successor, got %+v", stale)
	}

	// Expired plans can no longer be reviewed or frozen.
	if _, err := schedules.Review(ctx, stale.ID, "reviewed", "", "reviewer", "expiry-stale-review", stale.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("review expired plan error = %v", err)
	}
	if _, err := schedules.Freeze(ctx, stale.ID, "reviewer", "expiry-stale-freeze", stale.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("freeze expired plan error = %v", err)
	}

	// The recalculated successor points back at the expired plan and closes
	// the relation on both records.
	current, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "expiry-recalc")
	if err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if current.SupersedesID != stale.ID {
		t.Fatalf("successor should point to expired plan, got %+v", current)
	}
	closed, err := schedules.Get(ctx, stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.SupersededByID != current.ID {
		t.Fatalf("expired plan should point forward to successor, got %+v", closed)
	}
}

func TestVoidingAndCorrectingReadingsExpiresPlans(t *testing.T) {
	ctx, _, lot, _, readings, schedules := testServices(t)
	firstAt := time.Now().UTC().Add(-40 * time.Minute).Format(time.RFC3339)
	first := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: firstAt, MoisturePct: 42, DryBulbC: 50, WetBulbC: 44}}}
	created, err := readings.Import(ctx, first, "analyst", "void-import")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "void-calc")
	if err != nil || plan.ScheduleState != constants.ScheduleProposed {
		t.Fatalf("calculation = %+v, %v", plan, err)
	}
	voided := dto.ReadingVoid{Reason: "instrument probe drifted after calibration", Version: created[0].Version}
	if _, err := readings.Void(ctx, created[0].ID, voided, "analyst", "void-action"); err != nil {
		t.Fatalf("void: %v", err)
	}
	stale, err := schedules.Get(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.ExpiredAt == nil || stale.ExpiryReason != constants.ScheduleExpiryReadingVoided {
		t.Fatalf("void should expire plan, got %+v", stale)
	}

	// Correction of another lot's reading marks plans with the corrected
	// reason and the replacement import remains the active basis.
	correctAt := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	replacement := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: correctAt, MoisturePct: 40, DryBulbC: 51, WetBulbC: 44}}}
	replaced, err := readings.Import(ctx, replacement, "analyst", "correct-import")
	if err != nil {
		t.Fatal(err)
	}
	next, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "correct-calc-1")
	if err != nil {
		t.Fatal(err)
	}
	correction := dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "core", MeasuredAt: correctAt, MoisturePct: 39, DryBulbC: 51, WetBulbC: 44}}}
	if _, err := readings.Correct(ctx, replaced[0].ID, correction, "analyst", "correct-action", replaced[0].Version); err != nil {
		t.Fatalf("correct: %v", err)
	}
	expiredNext, err := schedules.Get(ctx, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if expiredNext.ExpiredAt == nil || expiredNext.ExpiryReason != constants.ScheduleExpiryReadingCorrected {
		t.Fatalf("correction should expire plan, got %+v", expiredNext)
	}
}

func TestCompletionRequiresCurrentFrozenPlan(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	for index, state := range []string{constants.LotConditioning, constants.LotDrying, constants.LotEqualizing} {
		updated, err := lots.Transition(ctx, lot.ID, state, "engineer", fmt.Sprintf("gate-%d", index), lot.Version)
		if err != nil {
			t.Fatalf("transition %s: %v", state, err)
		}
		lot = &updated
	}
	// No plan at all: completion stays in equalizing with missing_plan.
	if _, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "gate-missing", lot.Version); !errors.Is(err, ErrMissingFrozenPlan) {
		t.Fatalf("missing plan completion error = %v", err)
	}

	// A proposed plan that is not frozen still does not satisfy the gate.
	at := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	if _, err := readings.Import(ctx, dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: at, MoisturePct: 22, DryBulbC: 52, WetBulbC: 42}, {SamplePosition: "core", MeasuredAt: at, MoisturePct: 24, DryBulbC: 52, WetBulbC: 42}}}, "analyst", "gate-import-1"); err != nil {
		t.Fatal(err)
	}
	plan, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "gate-calc-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "gate-unfrozen", lot.Version); !errors.Is(err, ErrMissingFrozenPlan) {
		t.Fatalf("unfrozen plan completion error = %v", err)
	}

	// Frozen current plan unlocks completion.
	plan, err = schedules.Review(ctx, plan.ID, constants.ScheduleAccepted, "ok", "reviewer", "gate-review", plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = schedules.Freeze(ctx, plan.ID, "reviewer", "gate-freeze", plan.Version); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	completed, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "gate-complete", lot.Version)
	if err != nil || completed.LotState != constants.LotCompleted {
		t.Fatalf("completion = %+v, %v", completed, err)
	}
}

func TestExpiredFrozenPlanDoesNotUnlockCompletion(t *testing.T) {
	ctx, _, lot, lots, readings, schedules := testServices(t)
	for index, state := range []string{constants.LotConditioning, constants.LotDrying, constants.LotEqualizing} {
		updated, err := lots.Transition(ctx, lot.ID, state, "engineer", fmt.Sprintf("egate-%d", index), lot.Version)
		if err != nil {
			t.Fatalf("transition %s: %v", state, err)
		}
		lot = &updated
	}
	firstAt := time.Now().UTC().Add(-50 * time.Minute).Format(time.RFC3339)
	approvedFrozenSchedule(t, ctx, lot, readings, schedules, firstAt, 26)
	laterAt := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := readings.Import(ctx, dto.ReadingImport{TimberLotID: lot.ID, Readings: []dto.ReadingInput{{SamplePosition: "surface", MeasuredAt: laterAt, MoisturePct: 18, DryBulbC: 53, WetBulbC: 42}, {SamplePosition: "core", MeasuredAt: laterAt, MoisturePct: 19, DryBulbC: 53, WetBulbC: 42}}}, "analyst", "egate-backfill"); err != nil {
		t.Fatal(err)
	}
	// The frozen plan is now expired, so completion must be retained.
	if _, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "egate-rejected", lot.Version); !errors.Is(err, ErrMissingFrozenPlan) {
		t.Fatalf("expired frozen plan completion error = %v", err)
	}
	current, err := schedules.Calculate(ctx, dto.ScheduleCalculate{TimberLotID: lot.ID}, "engineer", "egate-recalc")
	if err != nil {
		t.Fatal(err)
	}
	current, err = schedules.Review(ctx, current.ID, constants.ScheduleAccepted, "ok", "reviewer", "egate-review", current.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = schedules.Freeze(ctx, current.ID, "reviewer", "egate-freeze", current.Version); err != nil {
		t.Fatalf("freeze current: %v", err)
	}
	completed, err := lots.Transition(ctx, lot.ID, constants.LotCompleted, "engineer", "egate-complete", lot.Version)
	if err != nil || completed.LotState != constants.LotCompleted {
		t.Fatalf("completion = %+v, %v", completed, err)
	}
}
