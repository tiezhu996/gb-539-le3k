package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"time"
)

type ScheduleRepository struct{ DB *gorm.DB }

func (r ScheduleRepository) List(ctx context.Context) ([]model.DryingSchedule, error) {
	var items []model.DryingSchedule
	err := r.DB.WithContext(ctx).Order("calculated_at desc").Find(&items).Error
	return items, err
}
func (r ScheduleRepository) Get(ctx context.Context, id string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).First(&item, "id = ?", id).Error
	if err != nil {
		return item, fmt.Errorf("get schedule: %w", err)
	}
	return item, nil
}
func (r ScheduleRepository) ByHash(ctx context.Context, lotID, hash, algorithm string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).Where("timber_lot_id = ? AND input_hash = ? AND algorithm_version = ?", lotID, hash, algorithm).First(&item).Error
	return item, err
}

// ActiveForLot returns plans that still guide the shop floor: they have reached
// a reviewable state and have neither been manually voided nor superseded by a
// recalculation triggered by a reading change.
func (r ScheduleRepository) ActiveForLot(ctx context.Context, lotID string) ([]model.DryingSchedule, error) {
	var items []model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("timber_lot_id = ? AND schedule_state IN ?", lotID, []string{constants.ScheduleProposed, constants.ScheduleReviewed, constants.ScheduleAccepted}).
		Order("calculated_at asc").Find(&items).Error
	return items, err
}

// LatestForLot returns the most recently calculated plan for a lot, including
// voided and superseded history, or gorm.ErrRecordNotFound when none exists.
func (r ScheduleRepository) LatestForLot(ctx context.Context, lotID string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).Where("timber_lot_id = ?", lotID).Order("calculated_at desc").First(&item).Error
	return item, err
}

// LatestUnlinkedSuperseded returns the newest plan of a lot that was already
// marked superseded but does not yet point at a replacement. A recalculated
// plan links back to it as its predecessor.
func (r ScheduleRepository) LatestUnlinkedSuperseded(ctx context.Context, lotID string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("timber_lot_id = ? AND schedule_state = ? AND superseded_by_id = ''", lotID, constants.ScheduleSuperseded).
		Order("calculated_at desc").First(&item).Error
	return item, err
}

// SupersedeBatchWithDB marks every still-active plan of a lot as superseded in
// the caller's transaction. Frozen plans are not modified except for the
// supersession marker and version, so their frozen snapshot stays intact.
func (r ScheduleRepository) SupersedeBatchWithDB(ctx context.Context, db *gorm.DB, lotID, reason string, at time.Time) (int64, error) {
	result := db.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("timber_lot_id = ? AND schedule_state IN ?", lotID, []string{constants.ScheduleProposed, constants.ScheduleReviewed, constants.ScheduleAccepted}).
		Updates(map[string]any{"schedule_state": constants.ScheduleSuperseded, "superseded_reason": reason, "superseded_at": at, "version": gorm.Expr("version + 1")})
	return result.RowsAffected, result.Error
}

// MarkReplacedByWithDB records the recalculated plan that replaced an expired
// plan. The conditional id guard keeps the predecessor chain linear.
func (r ScheduleRepository) MarkReplacedByWithDB(ctx context.Context, db *gorm.DB, id, replacementID string, version int) (bool, error) {
	result := db.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND schedule_state = ? AND superseded_by_id = '' AND version = ?", id, constants.ScheduleSuperseded, version).
		Updates(map[string]any{"superseded_by_id": replacementID, "version": version + 1})
	return result.RowsAffected == 1, result.Error
}

func (r ScheduleRepository) ByIdempotencyKey(ctx context.Context, key string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).Where("idempotency_key = ?", key).First(&item).Error
	return item, err
}
func (r ScheduleRepository) Create(ctx context.Context, item *model.DryingSchedule) error {
	return r.DB.WithContext(ctx).Create(item).Error
}
func (r ScheduleRepository) Save(ctx context.Context, item *model.DryingSchedule) error {
	return r.DB.WithContext(ctx).Save(item).Error
}

func (r ScheduleRepository) Transition(ctx context.Context, id, current, next string, version int, updates map[string]any) (bool, error) {
	return r.TransitionWithDB(ctx, r.DB, id, current, next, version, updates)
}

func (r ScheduleRepository) TransitionWithDB(ctx context.Context, db *gorm.DB, id, current, next string, version int, updates map[string]any) (bool, error) {
	updates["schedule_state"] = next
	updates["version"] = version + 1
	result := db.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND schedule_state = ? AND version = ?", id, current, version).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func (r ScheduleRepository) Freeze(ctx context.Context, id string, version int, actor string, at time.Time, snapshot string) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND version = ? AND frozen_at IS NULL", id, version).
		Updates(map[string]any{"frozen_at": at, "frozen_by": actor, "frozen_snapshot": snapshot, "version": version + 1})
	return result.RowsAffected == 1, result.Error
}

func (r ScheduleRepository) SaveComparison(ctx context.Context, id string, version int, baselineID, comparison string) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND version = ?", id, version).
		Updates(map[string]any{"baseline_schedule_id": baselineID, "comparison_json": comparison, "version": version + 1})
	return result.RowsAffected == 1, result.Error
}
