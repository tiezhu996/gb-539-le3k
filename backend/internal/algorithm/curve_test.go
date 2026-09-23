package algorithm

import (
	"testing"
	"time"

	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

func TestStageForBoundaries(t *testing.T) {
	cases := []struct {
		moisture, target float64
		want             string
	}{{11, 10, constants.MoistureTarget}, {25, 10, constants.MoistureBoundWater}, {35, 10, constants.MoistureFiberSaturation}, {35.1, 10, constants.MoistureGreen}}
	for _, tc := range cases {
		if got := StageFor(tc.moisture, tc.target); got != tc.want {
			t.Fatalf("StageFor(%v) = %q, want %q", tc.moisture, got, tc.want)
		}
	}
}

func TestEvaluateUsesTimeSeriesAndSafetyEnvelope(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	lot := model.TimberLot{Species: "oak", ThicknessMM: 30, TargetMoisturePct: 10, InitialMoisturePct: 48, LoadedAt: now.Add(-8 * time.Hour)}
	kiln := model.DryingKiln{MaxTemperatureC: 58, MinHumidityPct: 55}
	readings := []model.MoistureReading{
		{MeasuredAt: now.Add(-6 * time.Hour), SamplePosition: "core", MoisturePct: 46, DryBulbC: 50, WetBulbC: 44},
		{MeasuredAt: now.Add(-2 * time.Hour), SamplePosition: "core", MoisturePct: 45.2, DryBulbC: 50, WetBulbC: 44},
		{MeasuredAt: now.Add(-2 * time.Hour), SamplePosition: "surface", MoisturePct: 44.4, DryBulbC: 50, WetBulbC: 44},
	}
	result, err := Evaluate(lot, kiln, readings, now)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.Stages[0].DryingRate <= 0 || result.Stages[0].Gradient != 0.8 {
		t.Fatalf("unexpected time-series metrics: %+v", result.Stages[0])
	}
	if len(result.Evidence) != 4 {
		t.Fatalf("got %d evidence rows", len(result.Evidence))
	}
	for _, evidence := range result.Evidence {
		if evidence.Version != RuleSetVersion {
			t.Fatalf("evidence is not versioned: %+v", evidence)
		}
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.Parameter == "干燥段温度" && suggestion.Suggested > kiln.MaxTemperatureC {
			t.Fatalf("unsafe recommendation: %+v", suggestion)
		}
	}
	if !result.FinishAt.After(now.Add(-2 * time.Hour)) {
		t.Fatalf("finish estimate must derive from newest reading: %s", result.FinishAt)
	}
}

func TestEvaluateBlocksChangesWhenDryingRateViolatesRule(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	lot := model.TimberLot{Species: "oak", ThicknessMM: 30, TargetMoisturePct: 10, InitialMoisturePct: 70, LoadedAt: now.Add(-3 * time.Hour)}
	kiln := model.DryingKiln{MaxTemperatureC: 72, MinHumidityPct: 32}
	readings := []model.MoistureReading{{MeasuredAt: now.Add(-2 * time.Hour), SamplePosition: "core", MoisturePct: 70, DryBulbC: 52, WetBulbC: 46}, {MeasuredAt: now.Add(-time.Hour), SamplePosition: "core", MoisturePct: 40, DryBulbC: 52, WetBulbC: 46}, {MeasuredAt: now.Add(-time.Hour), SamplePosition: "surface", MoisturePct: 38, DryBulbC: 52, WetBulbC: 46}}
	result, err := Evaluate(lot, kiln, readings, now)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.Suggestions[0].Parameter != "工艺处置" {
		t.Fatalf("unsafe rate should not produce setpoint change: %+v", result.Suggestions)
	}
	foundFailure := false
	for _, evidence := range result.Evidence {
		if evidence.Constraint == "drying_rate_pct_per_hour" && !evidence.Passed {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Fatal("expected failing drying-rate evidence")
	}
}
