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

type ScheduleService struct {
	Repo     repository.ScheduleRepository
	Lots     repository.LotRepository
	Kilns    repository.KilnRepository
	Readings repository.ReadingRepository
	Audit    AuditService
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

// ExpireStalePlans marks every plan of the lot that can still drive work as
// expired because its reading basis changed. Frozen plans keep their snapshot
// and lifecycle state; only the expiry metadata changes. The operation joins
// the caller's transaction (or runs on the shared connection when db is nil),
// and each expired plan receives its own audit event.
func (s ScheduleService) ExpireStalePlans(ctx context.Context, db *gorm.DB, lotID, actor, requestID, reason string) error {
	plans, err := s.Repo.WithDB(s.connection(db)).ExpirableForLot(ctx, lotID, constants.ScheduleExpirableStates)
	if err != nil {
		return fmt.Errorf("list expirable schedules: %w", err)
	}
	if len(plans) == 0 {
		return nil
	}
	ids := make([]string, len(plans))
	for index, plan := range plans {
		ids[index] = plan.ID
	}
	now := time.Now().UTC()
	if _, err := s.Repo.WithDB(s.connection(db)).ExpireMany(ctx, ids, actor, reason, now); err != nil {
		return fmt.Errorf("expire stale schedules: %w", err)
	}
	for _, plan := range plans {
		before := plan
		plan.ExpiredAt, plan.ExpiredBy, plan.ExpiryReason, plan.Version = &now, actor, reason, plan.Version+1
		if err := s.recordInTx(ctx, db, requestID, "schedule", plan.ID, "expired", actor, before, plan); err != nil {
			return err
		}
	}
	return nil
}

// RequireLatestFrozenPlan gates the equalizing -> completed move. The lot may
// only complete while a frozen plan exists for the current measurement basis;
// an expired frozen plan no longer satisfies the gate.
func (s ScheduleService) RequireLatestFrozenPlan(ctx context.Context, lotID string) error {
	plan, err := s.Repo.LatestFrozenForLot(ctx, lotID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("equalizing lot requires the latest schedule to be frozen before completion: %w", ErrMissingFrozenPlan)
		}
		return err
	}
	if plan.ExpiredAt != nil {
		return fmt.Errorf("the frozen schedule for this lot is expired: %w", ErrMissingFrozenPlan)
	}
	return nil
}

func (s ScheduleService) connection(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return s.Repo.DB
}

func (s ScheduleService) recordInTx(ctx context.Context, db *gorm.DB, requestID, entity, entityID, action, actor string, before, after any) error {
	repo := s.Audit.Repo
	if db != nil {
		repo = repo.WithDB(db)
	}
	return repo.Create(ctx, &model.AuditEvent{ID: util.ID(), RequestID: requestID, Entity: entity, EntityID: entityID, Action: action, ActorID: actor, BeforeJSON: util.JSON(before), AfterJSON: util.JSON(after), CreatedAt: time.Now().UTC()})
}

// Calculate creates an immutable calculation record. The initial calculating
// state is persisted before evaluation and conditionally advanced so concurrent
// requests cannot overwrite a completed proposal. When reading changes have
// expired prior plans, the new proposal links itself to the most recent one.
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

	// A new calculation may only claim the expired chain tip that belongs to
	// this lot and is not yet linked to another successor.
	supersedesID := ""
	expiredTip, tipErr := s.Repo.LatestExpiredUnclaimed(ctx, lot.ID)
	if tipErr == nil {
		supersedesID = expiredTip.ID
	} else if !errors.Is(tipErr, gorm.ErrRecordNotFound) {
		return model.DryingSchedule{}, tipErr
	}

	now := time.Now().UTC()
	item := model.DryingSchedule{ID: util.ID(), TimberLotID: lot.ID, KilnSnapshot: util.JSON(kiln), AlgorithmVersion: AlgorithmVersion, RuleSetVersion: algorithm.RuleSetVersion, InputHash: inputHash, IdempotencyKey: input.IdempotencyKey, SupersedesID: supersedesID, ScheduleState: constants.ScheduleCalculating, CalculatedAt: now, CreatedBy: actor, Version: 1}
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
		updated, updateErr := s.Repo.Transition(ctx, item.ID, constants.ScheduleCalculating, constants.ScheduleFailed, item.Version, map[string]any{"failure_reason": evaluateErr.Error(), "explanation": "输入未通过安全计算校验"})
		if updateErr != nil {
			return item, updateErr
		}
		if !updated {
			return item, ErrConflict
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

	var proposed model.DryingSchedule
	var linked model.DryingSchedule
	if err = s.Repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"stages_json": string(stages), "recommended_changes_json": string(suggestions), "rule_evidence_json": string(evidence), "predicted_finish_at": result.FinishAt, "defect_risk_score": result.Risk, "explanation": result.Explanation}
		ok, txErr := s.Repo.WithDB(tx).TransitionWithDB(ctx, tx, item.ID, constants.ScheduleCalculating, constants.ScheduleProposed, item.Version, updates)
		if txErr != nil {
			return txErr
		}
		if !ok {
			return ErrConflict
		}
		if supersedesID != "" {
			if claimed, claimErr := s.Repo.WithDB(tx).LinkSupersessionWithDB(ctx, tx, supersedesID, item.ID); claimErr != nil {
				return claimErr
			} else if !claimed {
				// Another recalculation already claimed the chain tip; this
				// proposal still proceeds but must not point at that plan.
				if resetErr := tx.Model(&model.DryingSchedule{}).Where("id = ?", item.ID).Update("supersedes_id", "").Error; resetErr != nil {
					return resetErr
				}
				item.SupersedesID = ""
			}
		}
		if txErr := tx.First(&proposed, "id = ?", item.ID).Error; txErr != nil {
			return txErr
		}
		if supersedesID != "" && item.SupersedesID != "" {
			if txErr := tx.First(&linked, "id = ?", supersedesID).Error; txErr != nil {
				return txErr
			}
			if txErr := s.recordInTx(ctx, tx, requestID, "schedule", linked.ID, "superseded", actor, struct {
				Old model.DryingSchedule
			}{linked}, struct {
				Old model.DryingSchedule
				New model.DryingSchedule
			}{linked, proposed}); txErr != nil {
				return txErr
			}
		}
		return s.recordInTx(ctx, tx, requestID, "schedule", item.ID, "calculated", actor, nil, proposed)
	}); err != nil {
		return item, err
	}
	return proposed, nil
}

func (s ScheduleService) Review(ctx context.Context, id, decision, note, actor, requestID string, version int) (model.DryingSchedule, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	if version != item.Version {
		return item, ErrConflict
	}
	if item.ExpiredAt != nil {
		return item, fmt.Errorf("schedule is expired and can no longer be reviewed: %w", ErrConflict)
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
// if later rules or kiln records change; expiry preserves that snapshot.
func (s ScheduleService) Freeze(ctx context.Context, id, actor, requestID string, version int) (model.DryingSchedule, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	if version != item.Version || item.FrozenAt != nil {
		return item, ErrConflict
	}
	if item.ScheduleState != constants.ScheduleAccepted {
		return item, ErrConflict
	}
	if item.ExpiredAt != nil {
		return item, fmt.Errorf("schedule is expired and can no longer be frozen: %w", ErrConflict)
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
