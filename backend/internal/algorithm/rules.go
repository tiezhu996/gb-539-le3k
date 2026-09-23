package algorithm

import (
	"fmt"
	"strings"
)

const RuleSetVersion = "kiln-safety-2026.08"

type Rule struct {
	ID                 string  `json:"id"`
	Version            string  `json:"version"`
	Stage              string  `json:"stage"`
	MaxTemperature     float64 `json:"max_temperature"`
	MinHumidity        float64 `json:"min_humidity"`
	MaxGradient        float64 `json:"max_gradient"`
	MaxDryingRate      float64 `json:"max_drying_rate"`
	MaxTemperatureStep float64 `json:"max_temperature_step"`
	MaxHumidityStep    float64 `json:"max_humidity_step"`
	Source             string  `json:"source"`
}

var DefaultRules = []Rule{
	{ID: "green-safe", Version: RuleSetVersion, Stage: "green", MaxTemperature: 55, MinHumidity: 65, MaxGradient: 12, MaxDryingRate: 0.35, MaxTemperatureStep: 1, MaxHumidityStep: 2, Source: "KilnCurve safety baseline, green wood"},
	{ID: "fiber-safe", Version: RuleSetVersion, Stage: "fiber_saturation", MaxTemperature: 62, MinHumidity: 52, MaxGradient: 9, MaxDryingRate: 0.50, MaxTemperatureStep: 1, MaxHumidityStep: 2, Source: "KilnCurve safety baseline, fiber saturation"},
	{ID: "bound-safe", Version: RuleSetVersion, Stage: "bound_water", MaxTemperature: 68, MinHumidity: 40, MaxGradient: 6, MaxDryingRate: 0.65, MaxTemperatureStep: 1, MaxHumidityStep: 2, Source: "KilnCurve safety baseline, bound water"},
	{ID: "target-safe", Version: RuleSetVersion, Stage: "target", MaxTemperature: 60, MinHumidity: 48, MaxGradient: 4, MaxDryingRate: 0.30, MaxTemperatureStep: 1, MaxHumidityStep: 2, Source: "KilnCurve safety baseline, equalization"},
}

func RuleFor(stage string) (Rule, bool) {
	for _, rule := range DefaultRules {
		if rule.Stage == stage {
			return rule, true
		}
	}
	return Rule{}, false
}

// SpeciesProfile is a curated material envelope. Profiles modify only the
// conservative rule ceiling, never a kiln's own hard safety boundary.
type SpeciesProfile struct {
	ID               string   `json:"id"`
	Names            []string `json:"names"`
	MinThicknessMM   float64  `json:"min_thickness_mm"`
	MaxThicknessMM   float64  `json:"max_thickness_mm"`
	TemperatureDelta float64  `json:"temperature_delta"`
	HumidityDelta    float64  `json:"humidity_delta"`
	DryingRateFactor float64  `json:"drying_rate_factor"`
	EqualizingHours  float64  `json:"equalizing_hours"`
	Notes            string   `json:"notes"`
}

var SpeciesProfiles = []SpeciesProfile{
	{ID: "oak", Names: []string{"oak", "white oak", "red oak", "白橡", "红橡", "橡木"}, MinThicknessMM: 16, MaxThicknessMM: 80, TemperatureDelta: -5, HumidityDelta: 8, DryingRateFactor: 0.72, EqualizingHours: 4, Notes: "ring-porous hardwood: preserve humidity while moisture is high"},
	{ID: "beech", Names: []string{"beech", "山毛榉", "榉木"}, MinThicknessMM: 16, MaxThicknessMM: 70, TemperatureDelta: -4, HumidityDelta: 6, DryingRateFactor: 0.76, EqualizingHours: 4, Notes: "check end splitting and use gentle early drying"},
	{ID: "pine", Names: []string{"pine", "松木", "辐射松"}, MinThicknessMM: 12, MaxThicknessMM: 100, TemperatureDelta: 0, HumidityDelta: 2, DryingRateFactor: 0.92, EqualizingHours: 2, Notes: "softwood permits the base envelope when samples agree"},
	{ID: "maple", Names: []string{"maple", "枫木"}, MinThicknessMM: 16, MaxThicknessMM: 65, TemperatureDelta: -6, HumidityDelta: 9, DryingRateFactor: 0.68, EqualizingHours: 5, Notes: "dense hardwood requires a low gradient envelope"},
	{ID: "ash", Names: []string{"ash", "白蜡木"}, MinThicknessMM: 16, MaxThicknessMM: 75, TemperatureDelta: -3, HumidityDelta: 5, DryingRateFactor: 0.80, EqualizingHours: 3, Notes: "monitor ring-porous variation between boards"},
	{ID: "birch", Names: []string{"birch", "桦木"}, MinThicknessMM: 12, MaxThicknessMM: 60, TemperatureDelta: -3, HumidityDelta: 5, DryingRateFactor: 0.78, EqualizingHours: 3, Notes: "avoid aggressive early temperature rise"},
	{ID: "cherry", Names: []string{"cherry", "樱桃木"}, MinThicknessMM: 16, MaxThicknessMM: 65, TemperatureDelta: -4, HumidityDelta: 7, DryingRateFactor: 0.70, EqualizingHours: 4, Notes: "protect colour and reduce checking risk"},
	{ID: "walnut", Names: []string{"walnut", "黑胡桃"}, MinThicknessMM: 16, MaxThicknessMM: 75, TemperatureDelta: -4, HumidityDelta: 6, DryingRateFactor: 0.73, EqualizingHours: 4, Notes: "use a restrained drying rate through the fibre point"},
	{ID: "spruce", Names: []string{"spruce", "云杉"}, MinThicknessMM: 12, MaxThicknessMM: 100, TemperatureDelta: 0, HumidityDelta: 1, DryingRateFactor: 0.95, EqualizingHours: 2, Notes: "verify resin pockets in manual review"},
	{ID: "fir", Names: []string{"fir", "冷杉", "杉木"}, MinThicknessMM: 12, MaxThicknessMM: 100, TemperatureDelta: 0, HumidityDelta: 1, DryingRateFactor: 0.94, EqualizingHours: 2, Notes: "softwood base envelope with sample-pair checks"},
	{ID: "poplar", Names: []string{"poplar", "杨木"}, MinThicknessMM: 12, MaxThicknessMM: 80, TemperatureDelta: -1, HumidityDelta: 3, DryingRateFactor: 0.86, EqualizingHours: 2, Notes: "watch collapse risk when initial moisture is high"},
	{ID: "teak", Names: []string{"teak", "柚木"}, MinThicknessMM: 16, MaxThicknessMM: 80, TemperatureDelta: -3, HumidityDelta: 5, DryingRateFactor: 0.74, EqualizingHours: 4, Notes: "dense extractive-rich stock benefits from equalization"},
}

func ProfileFor(species string, thicknessMM float64) (SpeciesProfile, error) {
	name := strings.ToLower(strings.TrimSpace(species))
	for _, profile := range SpeciesProfiles {
		for _, alias := range profile.Names {
			if name == strings.ToLower(alias) {
				if thicknessMM < profile.MinThicknessMM || thicknessMM > profile.MaxThicknessMM {
					return SpeciesProfile{}, fmt.Errorf("%s thickness %.1fmm is outside %.1f..%.1fmm profile range", profile.ID, thicknessMM, profile.MinThicknessMM, profile.MaxThicknessMM)
				}
				return profile, nil
			}
		}
	}
	return SpeciesProfile{}, fmt.Errorf("no approved species profile for %q", species)
}

func ApplyProfile(base Rule, profile SpeciesProfile) Rule {
	base.MaxTemperature += profile.TemperatureDelta
	base.MinHumidity += profile.HumidityDelta
	base.MaxDryingRate *= profile.DryingRateFactor
	base.Source += "; material profile " + profile.ID
	return base
}

func ValidateRuleCatalog(rules []Rule, profiles []SpeciesProfile) error {
	if len(rules) == 0 || len(profiles) == 0 {
		return fmt.Errorf("rule catalog must contain stage rules and material profiles")
	}
	stageIDs, stageNames, aliases := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, rule := range rules {
		if rule.ID == "" || rule.Version == "" || rule.Stage == "" || rule.Source == "" {
			return fmt.Errorf("rule identity, version, stage, and source are required")
		}
		if stageIDs[rule.ID] || stageNames[rule.Stage] {
			return fmt.Errorf("duplicate rule %q or stage %q", rule.ID, rule.Stage)
		}
		stageIDs[rule.ID], stageNames[rule.Stage] = true, true
		if rule.MaxTemperature <= 0 || rule.MaxTemperature > 120 || rule.MinHumidity < 0 || rule.MinHumidity > 100 {
			return fmt.Errorf("rule %s has an invalid temperature or humidity boundary", rule.ID)
		}
		if rule.MaxGradient <= 0 || rule.MaxDryingRate <= 0 || rule.MaxTemperatureStep <= 0 || rule.MaxHumidityStep <= 0 {
			return fmt.Errorf("rule %s has a non-positive process limit", rule.ID)
		}
	}
	for _, profile := range profiles {
		if profile.ID == "" || len(profile.Names) == 0 || profile.MinThicknessMM <= 0 || profile.MaxThicknessMM < profile.MinThicknessMM {
			return fmt.Errorf("profile %q has incomplete material range", profile.ID)
		}
		if profile.DryingRateFactor <= 0 || profile.DryingRateFactor > 1 || profile.EqualizingHours < 0 {
			return fmt.Errorf("profile %s has an unsafe process factor", profile.ID)
		}
		for _, alias := range profile.Names {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key == "" || aliases[key] {
				return fmt.Errorf("profile %s has duplicate or empty alias %q", profile.ID, alias)
			}
			aliases[key] = true
		}
	}
	for _, stage := range []string{"green", "fiber_saturation", "bound_water", "target"} {
		if !stageNames[stage] {
			return fmt.Errorf("catalog is missing required stage %s", stage)
		}
	}
	return nil
}

func ValidateKilnEnvelope(kilnMaxTemperature, kilnMinHumidity float64, rule Rule) error {
	if kilnMaxTemperature <= 0 || kilnMaxTemperature > 120 || kilnMinHumidity < 0 || kilnMinHumidity > 100 {
		return fmt.Errorf("kiln safety envelope is invalid")
	}
	if kilnMaxTemperature < 35 {
		return fmt.Errorf("kiln maximum temperature %.1fC cannot support controlled drying", kilnMaxTemperature)
	}
	if kilnMinHumidity > 90 {
		return fmt.Errorf("kiln minimum humidity %.1f%% cannot support controlled drying", kilnMinHumidity)
	}
	if rule.MaxTemperature <= 0 || rule.MinHumidity < 0 {
		return fmt.Errorf("material rule is invalid")
	}
	return nil
}

type RuleLimit struct {
	Name        string  `json:"name"`
	RuleLimit   float64 `json:"rule_limit"`
	KilnLimit   float64 `json:"kiln_limit"`
	Effective   float64 `json:"effective"`
	Constrained string  `json:"constrained_by"`
}

func EffectiveLimits(kilnMaxTemperature, kilnMinHumidity float64, rule Rule) []RuleLimit {
	temperature := RuleLimit{Name: "temperature", RuleLimit: rule.MaxTemperature, KilnLimit: kilnMaxTemperature, Effective: rule.MaxTemperature, Constrained: "rule"}
	if kilnMaxTemperature < rule.MaxTemperature {
		temperature.Effective, temperature.Constrained = kilnMaxTemperature, "kiln"
	}
	humidity := RuleLimit{Name: "relative_humidity", RuleLimit: rule.MinHumidity, KilnLimit: kilnMinHumidity, Effective: rule.MinHumidity, Constrained: "rule"}
	if kilnMinHumidity > rule.MinHumidity {
		humidity.Effective, humidity.Constrained = kilnMinHumidity, "kiln"
	}
	return []RuleLimit{temperature, humidity}
}
