package algorithm

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

type StageMetric struct {
	Stage           string  `json:"stage"`
	AverageMoisture float64 `json:"average_moisture"`
	Gradient        float64 `json:"gradient"`
	DryingRate      float64 `json:"drying_rate"`
	Anomalies       int     `json:"anomalies"`
}

type RuleEvidence struct {
	RuleID     string  `json:"rule_id"`
	Version    string  `json:"version"`
	Constraint string  `json:"constraint"`
	Observed   float64 `json:"observed"`
	Limit      float64 `json:"limit"`
	Passed     bool    `json:"passed"`
	Source     string  `json:"source"`
}

type Suggestion struct {
	Parameter string         `json:"parameter"`
	Current   float64        `json:"current"`
	Suggested float64        `json:"suggested"`
	Rule      string         `json:"rule"`
	RiskDelta float64        `json:"risk_delta"`
	Evidence  []RuleEvidence `json:"evidence"`
}

type Result struct {
	Stages      []StageMetric  `json:"stages"`
	Suggestions []Suggestion   `json:"suggestions"`
	Evidence    []RuleEvidence `json:"evidence"`
	Risk        float64        `json:"risk"`
	Explanation string         `json:"explanation"`
	FinishAt    time.Time      `json:"finish_at"`
}

type SeriesCoverage struct {
	SampleCount      int       `json:"sample_count"`
	EventCount       int       `json:"event_count"`
	CoreSamples      int       `json:"core_samples"`
	SurfaceSamples   int       `json:"surface_samples"`
	FirstMeasuredAt  time.Time `json:"first_measured_at"`
	LastMeasuredAt   time.Time `json:"last_measured_at"`
	SpanHours        float64   `json:"span_hours"`
	LargestGapHours  float64   `json:"largest_gap_hours"`
	PairCompleteness float64   `json:"pair_completeness"`
	Warnings         []string  `json:"warnings"`
}

func StageFor(moisture, target float64) string {
	switch {
	case moisture <= target+1:
		return constants.MoistureTarget
	case moisture <= 25:
		return constants.MoistureBoundWater
	case moisture <= 35:
		return constants.MoistureFiberSaturation
	default:
		return constants.MoistureGreen
	}
}

// Evaluate is advisory-only. It searches only small changes and rejects every
// point outside the kiln and versioned stage-rule safety envelope.
func Evaluate(lot model.TimberLot, kiln model.DryingKiln, readings []model.MoistureReading, evaluatedAt time.Time) (Result, error) {
	if err := ValidateRuleCatalog(DefaultRules, SpeciesProfiles); err != nil {
		return Result{}, err
	}
	ordered := append([]model.MoistureReading(nil), readings...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].MeasuredAt.Before(ordered[j].MeasuredAt) })
	if err := validateSeries(lot, ordered, evaluatedAt); err != nil {
		return Result{}, err
	}
	if len(ordered) == 0 {
		ordered = []model.MoistureReading{{MoisturePct: lot.InitialMoisturePct, MeasuredAt: lot.LoadedAt, SamplePosition: "initial"}}
	}
	coverage := AnalyzeCoverage(ordered)
	avg := averageMoisture(ordered)
	gradient := sampleGradient(ordered)
	rate := dryingRate(ordered)
	stage := StageFor(avg, lot.TargetMoisturePct)
	rule, ok := RuleFor(stage)
	if !ok {
		return Result{}, fmt.Errorf("no safety rule for stage %q", stage)
	}
	profile, err := ProfileFor(lot.Species, lot.ThicknessMM)
	if err != nil {
		return Result{}, err
	}
	rule = ApplyProfile(rule, profile)
	if err := ValidateKilnEnvelope(kiln.MaxTemperatureC, kiln.MinHumidityPct, rule); err != nil {
		return Result{}, err
	}
	currentTemperature, currentHumidity := currentEnvironment(ordered, kiln)
	tempLimit, humidityLimit := math.Min(rule.MaxTemperature, kiln.MaxTemperatureC), math.Max(rule.MinHumidity, kiln.MinHumidityPct)
	evidence := []RuleEvidence{
		{RuleID: rule.ID, Version: rule.Version, Constraint: "temperature", Observed: currentTemperature, Limit: tempLimit, Passed: currentTemperature <= tempLimit, Source: rule.Source},
		{RuleID: rule.ID, Version: rule.Version, Constraint: "relative_humidity", Observed: currentHumidity, Limit: humidityLimit, Passed: currentHumidity >= humidityLimit, Source: rule.Source},
		{RuleID: rule.ID, Version: rule.Version, Constraint: "moisture_gradient", Observed: gradient, Limit: rule.MaxGradient, Passed: gradient <= rule.MaxGradient, Source: rule.Source},
		{RuleID: rule.ID, Version: rule.Version, Constraint: "drying_rate_pct_per_hour", Observed: rate, Limit: rule.MaxDryingRate, Passed: rate <= rule.MaxDryingRate, Source: rule.Source},
	}
	if coverage.PairCompleteness < 0.5 {
		evidence = append(evidence, RuleEvidence{RuleID: rule.ID, Version: rule.Version, Constraint: "sample_pair_coverage", Observed: coverage.PairCompleteness, Limit: 0.5, Passed: false, Source: "measurement coverage policy"})
	}
	anomalies := anomalyCount(ordered, avg)
	risk := riskScore(evidence, anomalies)
	suggestions := constrainedSuggestions(lot, kiln, rule, currentTemperature, currentHumidity, avg, gradient, rate, evidence)
	duration := estimateDuration(lot, avg, rate)
	finishBase := ordered[len(ordered)-1].MeasuredAt
	if finishBase.IsZero() {
		finishBase = evaluatedAt.UTC()
	}
	explanation := fmt.Sprintf("规则集 %s 使用 %s/%0.0fmm 材料目录，在 %s 阶段核验温度、相对湿度、含水率梯度与干燥速率。采样覆盖 %d 个事件、%.0f%% 成对中心/表层读数，最长间隔 %.1f 小时。%s。建议仅是离线工艺建议，设备联锁和人工复核仍为最终边界。", rule.Version, profile.ID, lot.ThicknessMM, stage, coverage.EventCount, coverage.PairCompleteness*100, coverage.LargestGapHours, profile.Notes)
	return Result{Stages: []StageMetric{{Stage: stage, AverageMoisture: round(avg, 1), Gradient: round(gradient, 1), DryingRate: round(rate, 3), Anomalies: anomalies}}, Suggestions: suggestions, Evidence: evidence, Risk: risk, Explanation: explanation, FinishAt: finishBase.Add(duration)}, nil
}

func AnalyzeCoverage(readings []model.MoistureReading) SeriesCoverage {
	coverage := SeriesCoverage{SampleCount: len(readings), Warnings: []string{}}
	if len(readings) == 0 {
		coverage.Warnings = append(coverage.Warnings, "no measured samples; initial lot moisture is being used")
		return coverage
	}
	ordered := append([]model.MoistureReading(nil), readings...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].MeasuredAt.Before(ordered[j].MeasuredAt) })
	coverage.FirstMeasuredAt, coverage.LastMeasuredAt = ordered[0].MeasuredAt, ordered[len(ordered)-1].MeasuredAt
	coverage.SpanHours = coverage.LastMeasuredAt.Sub(coverage.FirstMeasuredAt).Hours()
	events := map[time.Time]map[string]bool{}
	for index, reading := range ordered {
		if events[reading.MeasuredAt] == nil {
			events[reading.MeasuredAt] = map[string]bool{}
		}
		position := strings.ToLower(reading.SamplePosition)
		events[reading.MeasuredAt][position] = true
		if position == "core" || position == "center" || position == "中心" {
			coverage.CoreSamples++
		}
		if position == "surface" || position == "表层" {
			coverage.SurfaceSamples++
		}
		if index > 0 {
			gap := reading.MeasuredAt.Sub(ordered[index-1].MeasuredAt).Hours()
			if gap > coverage.LargestGapHours {
				coverage.LargestGapHours = gap
			}
		}
	}
	coverage.EventCount = len(events)
	paired := 0
	for _, positions := range events {
		if (positions["core"] || positions["center"] || positions["中心"]) && (positions["surface"] || positions["表层"]) {
			paired++
		}
	}
	coverage.PairCompleteness = float64(paired) / float64(coverage.EventCount)
	if coverage.EventCount < 2 {
		coverage.Warnings = append(coverage.Warnings, "one sampling event cannot establish time-based drying rate")
	}
	if coverage.PairCompleteness < 0.5 {
		coverage.Warnings = append(coverage.Warnings, "less than half of sampling events contain a core/surface pair")
	}
	if coverage.LargestGapHours > 24 {
		coverage.Warnings = append(coverage.Warnings, "sampling gap exceeds 24 hours")
	}
	if coverage.CoreSamples == 0 || coverage.SurfaceSamples == 0 {
		coverage.Warnings = append(coverage.Warnings, "gradient calculation lacks one controlled sample position")
	}
	return coverage
}

// Calculate retains the original convenience API for callers that cannot surface validation errors.
func Calculate(lot model.TimberLot, kiln model.DryingKiln, readings []model.MoistureReading) Result {
	result, err := Evaluate(lot, kiln, readings, time.Now().UTC())
	if err == nil {
		return result
	}
	return Result{Explanation: "无法生成安全建议：" + err.Error(), Risk: 100}
}

func validateSeries(lot model.TimberLot, readings []model.MoistureReading, evaluatedAt time.Time) error {
	for _, reading := range readings {
		if reading.MeasuredAt.IsZero() || reading.MeasuredAt.After(evaluatedAt.Add(5*time.Minute)) {
			return fmt.Errorf("reading timestamp is missing or in the future")
		}
		if !lot.LoadedAt.IsZero() && reading.MeasuredAt.Before(lot.LoadedAt.Add(-5*time.Minute)) {
			return fmt.Errorf("reading predates lot loading")
		}
		if reading.MoisturePct < 0 || reading.MoisturePct > 100 || reading.DryBulbC < 0 || reading.DryBulbC > 120 || reading.WetBulbC < 0 || reading.WetBulbC > reading.DryBulbC {
			return fmt.Errorf("reading has invalid physical bounds")
		}
	}
	return nil
}

func averageMoisture(readings []model.MoistureReading) float64 {
	total := 0.0
	for _, item := range readings {
		total += item.MoisturePct
	}
	return total / float64(len(readings))
}

func sampleGradient(readings []model.MoistureReading) float64 {
	latest := readings[len(readings)-1].MeasuredAt
	core, surface, values := []float64{}, []float64{}, []float64{}
	for _, item := range readings {
		if !item.MeasuredAt.Equal(latest) {
			continue
		}
		values = append(values, item.MoisturePct)
		switch strings.ToLower(item.SamplePosition) {
		case "core", "center", "中心":
			core = append(core, item.MoisturePct)
		case "surface", "表层":
			surface = append(surface, item.MoisturePct)
		}
	}
	if len(core) > 0 && len(surface) > 0 {
		return math.Abs(mean(core) - mean(surface))
	}
	if len(values) < 2 {
		return 0
	}
	min, max := values[0], values[0]
	for _, value := range values {
		min = math.Min(min, value)
		max = math.Max(max, value)
	}
	return max - min
}

func dryingRate(readings []model.MoistureReading) float64 {
	if len(readings) < 2 {
		return 0
	}
	start, end := readings[0], readings[len(readings)-1]
	hours := end.MeasuredAt.Sub(start.MeasuredAt).Hours()
	if hours <= 0 {
		return 0
	}
	return math.Max(0, start.MoisturePct-end.MoisturePct) / hours
}

func currentEnvironment(readings []model.MoistureReading, kiln model.DryingKiln) (float64, float64) {
	latest := readings[len(readings)-1]
	if latest.DryBulbC == 0 && latest.WetBulbC == 0 {
		return math.Min(kiln.MaxTemperatureC, 50), math.Max(kiln.MinHumidityPct, 60)
	}
	return latest.DryBulbC, relativeHumidity(latest.DryBulbC, latest.WetBulbC)
}

func relativeHumidity(dry, wet float64) float64 { return math.Max(0, math.Min(100, 100-5*(dry-wet))) }
func mean(values []float64) float64 {
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
func round(value float64, places int) float64 {
	power := math.Pow(10, float64(places))
	return math.Round(value*power) / power
}
func anomalyCount(readings []model.MoistureReading, average float64) int {
	count := 0
	for _, item := range readings {
		if math.Abs(item.MoisturePct-average) > 12 {
			count++
		}
	}
	return count
}
func riskScore(evidence []RuleEvidence, anomalies int) float64 {
	risk := float64(anomalies * 8)
	for _, item := range evidence {
		if !item.Passed {
			risk += 22
		}
	}
	return math.Min(100, round(risk, 1))
}

func constrainedSuggestions(lot model.TimberLot, kiln model.DryingKiln, rule Rule, currentTemp, currentHumidity, moisture, gradient, rate float64, evidence []RuleEvidence) []Suggestion {
	tempCap, humidityFloor := math.Min(rule.MaxTemperature, kiln.MaxTemperatureC), math.Max(rule.MinHumidity, kiln.MinHumidityPct)
	type candidate struct{ temp, humidity, score float64 }
	candidates := []candidate{}
	for temp := math.Max(0, currentTemp-4); temp <= math.Min(tempCap, currentTemp+4); temp += rule.MaxTemperatureStep {
		for humidity := math.Max(humidityFloor, currentHumidity-8); humidity <= math.Min(100, currentHumidity+8); humidity += rule.MaxHumidityStep {
			if temp > tempCap || humidity < humidityFloor || gradient > rule.MaxGradient || rate > rule.MaxDryingRate {
				continue
			}
			dryNeed := math.Max(0, moisture-lot.TargetMoisturePct)
			score := temp*dryNeed/100 - math.Abs(temp-currentTemp)*0.4 - math.Abs(humidity-currentHumidity)*0.08
			candidates = append(candidates, candidate{temp, humidity, score})
		}
	}
	if len(candidates) == 0 {
		return []Suggestion{{Parameter: "工艺处置", Rule: "当前梯度或干燥速率超过规则上限；不提供升温或降湿建议，应先均衡并复测。", Evidence: evidence}}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	best := candidates[0]
	suggestions := []Suggestion{}
	if round(best.temp, 1) != round(currentTemp, 1) {
		suggestions = append(suggestions, Suggestion{Parameter: "干燥段温度", Current: round(currentTemp, 1), Suggested: round(best.temp, 1), Rule: fmt.Sprintf("%s: 不超过 %.1fC，单次调整不超过 %.1fC", rule.ID, tempCap, rule.MaxTemperatureStep), RiskDelta: -4, Evidence: evidence})
	}
	if round(best.humidity, 1) != round(currentHumidity, 1) {
		suggestions = append(suggestions, Suggestion{Parameter: "相对湿度", Current: round(currentHumidity, 1), Suggested: round(best.humidity, 1), Rule: fmt.Sprintf("%s: 不低于 %.1f%%，单次调整不超过 %.1f%%", rule.ID, humidityFloor, rule.MaxHumidityStep), RiskDelta: -3, Evidence: evidence})
	}
	if gradient > rule.MaxGradient*0.8 {
		suggestions = append(suggestions, Suggestion{Parameter: "均衡时长", Current: 2, Suggested: 3, Rule: "梯度接近规则上限，增加均衡后再评估。", RiskDelta: -8, Evidence: evidence})
	}
	return suggestions
}

func estimateDuration(lot model.TimberLot, moisture, rate float64) time.Duration {
	remaining := math.Max(0, moisture-lot.TargetMoisturePct)
	if rate <= 0 {
		return time.Duration(math.Ceil(remaining*2)) * time.Hour
	}
	return time.Duration(math.Ceil(remaining/rate)) * time.Hour
}
