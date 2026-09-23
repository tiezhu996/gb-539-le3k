package algorithm

import (
	"testing"
	"time"

	"timber-kiln-drying-optimizer/backend/internal/model"
)

func TestMaterialProfilesConstrainRulesAndRejectUnsupportedStock(t *testing.T) {
	oak, err := ProfileFor("白橡", 32)
	if err != nil || oak.ID != "oak" {
		t.Fatalf("white oak profile = %+v, %v", oak, err)
	}
	base, ok := RuleFor("green")
	if !ok {
		t.Fatal("missing green rule")
	}
	adjusted := ApplyProfile(base, oak)
	if adjusted.MaxTemperature >= base.MaxTemperature || adjusted.MinHumidity <= base.MinHumidity || adjusted.MaxDryingRate >= base.MaxDryingRate {
		t.Fatalf("oak profile did not tighten the base rule: base=%+v adjusted=%+v", base, adjusted)
	}
	if _, err := ProfileFor("unknown species", 30); err == nil {
		t.Fatal("unsupported species must be rejected")
	}
	if _, err := ProfileFor("oak", 120); err == nil {
		t.Fatal("out-of-range thickness must be rejected")
	}
}

func TestCatalogAndCoverageValidation(t *testing.T) {
	badRules := append([]Rule(nil), DefaultRules...)
	badRules[1].Stage = badRules[0].Stage
	if err := ValidateRuleCatalog(badRules, SpeciesProfiles); err == nil {
		t.Fatal("duplicate stage catalog must be rejected")
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	coverage := AnalyzeCoverage([]model.MoistureReading{
		{MeasuredAt: now.Add(-30 * time.Hour), SamplePosition: "core"},
		{MeasuredAt: now, SamplePosition: "core"},
	})
	if coverage.PairCompleteness != 0 || coverage.LargestGapHours != 30 || len(coverage.Warnings) < 3 {
		t.Fatalf("unexpected weak-series coverage: %+v", coverage)
	}
}
