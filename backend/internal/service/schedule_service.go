package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/algorithm"
	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

const AlgorithmVersion = "curve-v2.0"

// Supersession reasons recorded when reading maintenance invalidates plans.
const (
	SupersedeReadingSupplemented = "reading_supplemented"
	SupersedeReadingVoided       = "reading_voided"
	SupersedeReadingCorrected    = "reading_corrected"
)

func SupersedeReasonLabel(reason string) string {
	switch reason {
	case SupersedeReadingSupplemented:
		return "读数补录"
	case SupersedeReadingVoided:
		return "读数作废"
	case SupersedeReadingCorrected:
		return "读数修正"
	default:
		return reason
	}
}

type ScheduleService struct {
	Repo     repository.ScheduleRepository
	Lots     repository.LotRepository
	Kilns    repository.KilnRepository
	Readings repository.ReadingRepository
	Audit    AuditService
}

// ExpireActivePlans marks every active plan of a lot as superseded inside the
// caller's reading-maintenance transaction and returns the pre-change rows.
// Frozen plans keep their snapshot; only the supersession marker changes, and
// expired plans can no longer be acted on. Audit events are written after the
// transaction commits via RecordSupersessionAudit.
func (s ScheduleService) ExpireActivePlans(ctx context.Context, tx *gorm.DB, lotID, reason string, at time.Time) ([]model.DryingSchedule, error) {
	before, err := s.Repo.ActiveForLot(ctx, lotID)
	if err != nil {
		return nil, err
	}
	if _, err = s.Repo.SupersedeBatchWithDB(ctx, tx, lotID, reason, at); err != nil {
		return nil, err
	}
	return before, nil
}

// RecordSupersessionAudit writes one audit event per plan expired by a reading
// change. It must be called only after the surrounding transaction commits so
// the audit connection is not blocked by an open writer lock.
func (s ScheduleService) RecordSupersessionAudit(ctx context.Context, before []model.DryingSchedule, reason, actor, requestID string, at time.Time) {
	for _, item := range before {
		after := item
		after.ScheduleState = constants.ScheduleSuperseded
		after.SupersededReason = reason
		after.SupersededAt = &at
		after.Version = item.Version + 1
		_ = s.Audit.Record(ctx, requestID, "schedule", item.ID, "superseded", actor, item, after)
	}
}

func (s ScheduleService) List(ctx context.Context) ([]model.DryingSchedule, error) {
	return s.Repo.List(ctx)
}
func (s ScheduleService) Get(ctx context.Context, id string) (model.DryingSchedule, error) {
	item, err := s.Repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ErrNotFound
	}
	return item, err
}

// Calculate creates an immutable calculation record. The initial calculating
// state is persisted before evaluation and conditionally advanced so concurrent
// requests cannot overwrite a completed proposal.
func (s ScheduleService) Calculate(ctx context.Context, input dto.ScheduleCalculate, actor, requestID string) (model.DryingSchedule, error) {
	lot, err := s.Lots.Get(ctx, input.TimberLotID)
	if err != nil {
		return model.DryingSchedule{}, fmt.Errorf("lot: %w", ErrValidation)
	}
	if lot.LotState == constants.LotCompleted || lot.LotState == constants.LotAborted {
		return model.DryingSchedule{}, fmt.Errorf("terminal lot cannot be calculated: %w", ErrConflict)
	}
	kiln, err := s.Kilns.Get(ctx, lot.KilnID)
	if err != nil {
		return model.DryingSchedule{}, err
	}
	readings, err := s.Readings.Accepted(ctx, lot.ID)
	if err != nil {
		return model.DryingSchedule{}, err
	}
	inputHash := scheduleHash(lot, kiln, readings)
	if input.IdempotencyKey != "" {
		if existing, findErr := s.Repo.ByIdempotencyKey(ctx, input.IdempotencyKey); findErr == nil {
			if existing.InputHash != inputHash {
				return existing, fmt.Errorf("idempotency key was used for a different input: %w", ErrConflict)
			}
			return existing, nil
		}
	}
	if existing, findErr := s.Repo.ByHash(ctx, lot.ID, inputHash, AlgorithmVersion); findErr == nil {
		return existing, nil
	}

	now := time.Now().UTC()
	item := model.DryingSchedule{ID: util.ID(), TimberLotID: lot.ID, KilnSnapshot: util.JSON(kiln), AlgorithmVersion: AlgorithmVersion, RuleSetVersion: algorithm.RuleSetVersion, InputHash: inputHash, IdempotencyKey: input.IdempotencyKey, ScheduleState: constants.ScheduleCalculating, CalculatedAt: now, CreatedBy: actor, Version: 1}
	// A recalculated plan points back at the most recent expired plan of the
	// lot that has no replacement yet, keeping the version chain traceable.
	predecessorVersion := 0
	if predecessor, findErr := s.Repo.LatestUnlinkedSuperseded(ctx, lot.ID); findErr == nil {
		item.SupersedesScheduleID = predecessor.ID
		predecessorVersion = predecessor.Version
	}
	if err = s.Repo.Create(ctx, &item); err != nil {
		if existing, findErr := s.Repo.ByHash(ctx, lot.ID, inputHash, AlgorithmVersion); findErr == nil {
			return existing, nil
		}
		if input.IdempotencyKey != "" {
			if existing, findErr := s.Repo.ByIdempotencyKey(ctx, input.IdempotencyKey); findErr == nil {
				return existing, nil
			}
		}
		return item, fmt.Errorf("create calculation: %w", err)
	}

	result, evaluateErr := algorithm.Evaluate(lot, kiln, readings, now)
	if evaluateErr != nil {
		_, updateErr := s.Repo.Transition(ctx, item.ID, constants.ScheduleCalculating, constants.ScheduleFailed, item.Version, map[string]any{"failure_reason": evaluateErr.Error(), "explanation": "输入未通过安全计算校验"})
		if updateErr != nil {
			return item, updateErr
		}
		item.ScheduleState, item.FailureReason, item.Version = constants.ScheduleFailed, evaluateErr.Error(), item.Version+1
		_ = s.Audit.Record(ctx, requestID, "schedule", item.ID, "calculation_failed", actor, nil, item)
		return item, fmt.Errorf("calculate schedule: %w", ErrValidation)
	}
	stages, marshalErr := json.Marshal(result.Stages)
	if marshalErr != nil {
		return item, marshalErr
	}
	suggestions, marshalErr := json.Marshal(result.Suggestions)
	if marshalErr != nil {
		return item, marshalErr
	}
	evidence, marshalErr := json.Marshal(result.Evidence)
	if marshalErr != nil {
		return item, marshalErr
	}
	updates := map[string]any{"stages_json": string(stages), "recommended_changes_json": string(suggestions), "rule_evidence_json": string(evidence), "predicted_finish_at": result.FinishAt, "defect_risk_score": result.Risk, "explanation": result.Explanation}
	var proposed model.DryingSchedule
	if txErr := s.Repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated, txErr := s.Repo.TransitionWithDB(ctx, tx, item.ID, constants.ScheduleCalculating, constants.ScheduleProposed, item.Version, updates)
		if txErr != nil {
			return txErr
		}
		if !updated {
			return ErrConflict
		}
		if item.SupersedesScheduleID != "" {
			// If a concurrent recalculation already claimed the predecessor's
			// back-link, this plan still points at it one-way for traceability.
			if _, linkErr := s.Repo.MarkReplacedByWithDB(ctx, tx, item.SupersedesScheduleID, item.ID, predecessorVersion); linkErr != nil {
				return linkErr
			}
		}
		return tx.WithContext(ctx).First(&proposed, "id = ?", item.ID).Error
	}); txErr != nil {
		return item, txErr
	}
	item.ScheduleState, item.Version = constants.ScheduleProposed, proposed.Version
	item.StagesJSON, item.RecommendedChangesJSON, item.RuleEvidenceJSON = string(stages), string(suggestions), string(evidence)
	item.PredictedFinishAt, item.DefectRiskScore, item.Explanation = &result.FinishAt, result.Risk, result.Explanation
	item.SupersedesScheduleID = proposed.SupersedesScheduleID
	if item.SupersedesScheduleID != "" {
		_ = s.Audit.Record(ctx, requestID, "schedule", item.SupersedesScheduleID, "replacement_linked", actor, nil, map[string]any{"superseded_schedule_id": item.SupersedesScheduleID, "replacement_schedule_id": item.ID})
	}
	_ = s.Audit.Record(ctx, requestID, "schedule", item.ID, "calculated", actor, nil, item)
	return item, nil
}

func (s ScheduleService) Review(ctx context.Context, id, decision, note, actor, requestID string, version int) (model.DryingSchedule, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	if version != item.Version {
		return item, ErrConflict
	}
	if item.ScheduleState == constants.ScheduleSuperseded {
		return item, fmt.Errorf("superseded schedule cannot be reviewed: %w", ErrConflict)
	}
	if item.CreatedBy == actor && decision == constants.ScheduleAccepted {
		return item, ErrForbidden
	}
	next, valid := reviewTransition(item.ScheduleState, decision)
	if !valid {
		return item, ErrConflict
	}
	before := item
	explanation := item.Explanation
	if note != "" {
		explanation += "\n审核备注：" + note
	}
	updated, err := s.Repo.Transition(ctx, item.ID, item.ScheduleState, next, item.Version, map[string]any{"reviewed_by": actor, "explanation": explanation})
	if err != nil {
		return item, err
	}
	if !updated {
		return item, ErrConflict
	}
	item.ScheduleState, item.ReviewedBy, item.Explanation, item.Version = next, actor, explanation, item.Version+1
	_ = s.Audit.Record(ctx, requestID, "schedule", id, "reviewed", actor, before, item)
	return item, nil
}

func reviewTransition(current, decision string) (string, bool) {
	switch current {
	case constants.ScheduleProposed:
		switch decision {
		case "reviewed":
			return constants.ScheduleReviewed, true
		case constants.ScheduleAccepted:
			return constants.ScheduleAccepted, true
		case "void", "rejected":
			return constants.ScheduleVoided, true
		}
	case constants.ScheduleReviewed:
		switch decision {
		case constants.ScheduleAccepted:
			return constants.ScheduleAccepted, true
		case "void", "rejected":
			return constants.ScheduleVoided, true
		}
	}
	return "", false
}

type ScheduleComparison struct {
	ScheduleID          string   `json:"schedule_id"`
	BaselineScheduleID  string   `json:"baseline_schedule_id"`
	RiskDelta           float64  `json:"risk_delta"`
	FinishDeltaHours    float64  `json:"finish_delta_hours"`
	StageChanged        bool     `json:"stage_changed"`
	RuleSetChanged      bool     `json:"rule_set_changed"`
	RecommendationCount int      `json:"recommendation_count"`
	BaselineCount       int      `json:"baseline_count"`
	FailedRules         []string `json:"failed_rules"`
	BaselineFailedRules []string `json:"baseline_failed_rules"`
	ImprovedRules       []string `json:"improved_rules"`
	DegradedRules       []string `json:"degraded_rules"`
	FrozenBaseline      bool     `json:"frozen_baseline"`
}

// Freeze stores a canonical JSON snapshot before an approved schedule is used
// as a shop-floor reference. A frozen plan remains historically readable even
// if later rules or kiln records change.
func (s ScheduleService) Freeze(ctx context.Context, id, actor, requestID string, version int) (model.DryingSchedule, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	if version != item.Version || item.FrozenAt != nil {
		return item, ErrConflict
	}
	if item.ScheduleState == constants.ScheduleSuperseded {
		return item, fmt.Errorf("superseded schedule cannot be frozen: %w", ErrConflict)
	}
	if item.ScheduleState != constants.ScheduleAccepted {
		return item, ErrConflict
	}
	snapshot := struct {
		RuleSetVersion string
		Algorithm      string
		KilnSnapshot   string
		Stages         string
		Evidence       string
		Changes        string
		FinishAt       *time.Time
	}{item.RuleSetVersion, item.AlgorithmVersion, item.KilnSnapshot, item.StagesJSON, item.RuleEvidenceJSON, item.RecommendedChangesJSON, item.PredictedFinishAt}
	now := time.Now().UTC()
	frozen, err := s.Repo.Freeze(ctx, item.ID, item.Version, actor, now, util.JSON(snapshot))
	if err != nil {
		return item, err
	}
	if !frozen {
		return item, ErrConflict
	}
	before := item
	item.FrozenAt, item.FrozenBy, item.FrozenSnapshot, item.Version = &now, actor, util.JSON(snapshot), item.Version+1
	_ = s.Audit.Record(ctx, requestID, "schedule", item.ID, "frozen", actor, before, item)
	return item, nil
}

func (s ScheduleService) Compare(ctx context.Context, id, baselineID, actor, requestID string) (ScheduleComparison, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return ScheduleComparison{}, err
	}
	baseline, err := s.Get(ctx, baselineID)
	if err != nil {
		return ScheduleComparison{}, ErrNotFound
	}
	if item.TimberLotID != baseline.TimberLotID || baseline.ID == item.ID {
		return ScheduleComparison{}, ErrValidation
	}
	comparison := compareSchedules(item, baseline)
	encoded := util.JSON(comparison)
	updated, err := s.Repo.SaveComparison(ctx, item.ID, item.Version, baseline.ID, encoded)
	if err != nil {
		return comparison, err
	}
	if !updated {
		return comparison, ErrConflict
	}
	_ = s.Audit.Record(ctx, requestID, "schedule", item.ID, "compared", actor, nil, comparison)
	return comparison, nil
}

func compareSchedules(item, baseline model.DryingSchedule) ScheduleComparison {
	comparison := ScheduleComparison{ScheduleID: item.ID, BaselineScheduleID: baseline.ID, RiskDelta: item.DefectRiskScore - baseline.DefectRiskScore, RuleSetChanged: item.RuleSetVersion != baseline.RuleSetVersion, FrozenBaseline: baseline.FrozenAt != nil}
	if item.PredictedFinishAt != nil && baseline.PredictedFinishAt != nil {
		comparison.FinishDeltaHours = item.PredictedFinishAt.Sub(*baseline.PredictedFinishAt).Hours()
	}
	var stages, oldStages []algorithm.StageMetric
	var changes, oldChanges []algorithm.Suggestion
	_ = json.Unmarshal([]byte(item.StagesJSON), &stages)
	_ = json.Unmarshal([]byte(baseline.StagesJSON), &oldStages)
	_ = json.Unmarshal([]byte(item.RecommendedChangesJSON), &changes)
	_ = json.Unmarshal([]byte(baseline.RecommendedChangesJSON), &oldChanges)
	comparison.RecommendationCount, comparison.BaselineCount = len(changes), len(oldChanges)
	if len(stages) > 0 && len(oldStages) > 0 {
		comparison.StageChanged = stages[0].Stage != oldStages[0].Stage
	}
	currentRules := failedEvidence(item.RuleEvidenceJSON)
	baselineRules := failedEvidence(baseline.RuleEvidenceJSON)
	comparison.FailedRules, comparison.BaselineFailedRules = sortedRuleNames(currentRules), sortedRuleNames(baselineRules)
	for rule := range baselineRules {
		if !currentRules[rule] {
			comparison.ImprovedRules = append(comparison.ImprovedRules, rule)
		}
	}
	for rule := range currentRules {
		if !baselineRules[rule] {
			comparison.DegradedRules = append(comparison.DegradedRules, rule)
		}
	}
	sort.Strings(comparison.ImprovedRules)
	sort.Strings(comparison.DegradedRules)
	return comparison
}

func failedEvidence(raw string) map[string]bool {
	failed := map[string]bool{}
	var evidence []algorithm.RuleEvidence
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		return failed
	}
	for _, item := range evidence {
		if !item.Passed {
			failed[item.RuleID+":"+item.Constraint] = true
		}
	}
	return failed
}

func sortedRuleNames(rules map[string]bool) []string {
	items := make([]string, 0, len(rules))
	for item := range rules {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

func scheduleHash(lot model.TimberLot, kiln model.DryingKiln, readings []model.MoistureReading) string {
	payload := struct {
		Lot              model.TimberLot
		Kiln             model.DryingKiln
		Readings         []model.MoistureReading
		Algorithm, Rules string
	}{lot, kiln, readings, AlgorithmVersion, algorithm.RuleSetVersion}
	encoded, _ := json.Marshal(payload)
	return util.Hash(string(encoded))
}
