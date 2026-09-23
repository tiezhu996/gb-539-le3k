package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

type ScheduleRepository struct{ DB *gorm.DB }

// WithDB returns a repository bound to a transaction so schedule writes can
// join the same unit of work as the reading change that triggered them.
func (r ScheduleRepository) WithDB(tx *gorm.DB) ScheduleRepository {
	return ScheduleRepository{DB: tx}
}

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

// ByHash returns the most recent non-expired plan for an input tuple. Plans
// whose basis was invalidated keep their historical hash but must never be
// reused as the current proposal.
func (r ScheduleRepository) ByHash(ctx context.Context, lotID, hash, algorithm string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("timber_lot_id = ? AND input_hash = ? AND algorithm_version = ? AND expired_at IS NULL", lotID, hash, algorithm).
		Order("calculated_at desc").First(&item).Error
	return item, err
}
func (r ScheduleRepository) ByIdempotencyKey(ctx context.Context, key string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("idempotency_key = ? AND expired_at IS NULL", key).
		Order("calculated_at desc").First(&item).Error
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

// TransitionWithDB applies a conditional lifecycle update and refuses rows
// that were marked expired after the caller read them.
func (r ScheduleRepository) TransitionWithDB(ctx context.Context, db *gorm.DB, id, current, next string, version int, updates map[string]any) (bool, error) {
	updates["schedule_state"] = next
	updates["version"] = version + 1
	result := db.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND schedule_state = ? AND version = ? AND expired_at IS NULL", id, current, version).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

func (r ScheduleRepository) Freeze(ctx context.Context, id string, version int, actor string, at time.Time, snapshot string) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND version = ? AND frozen_at IS NULL AND expired_at IS NULL", id, version).
		Updates(map[string]any{"frozen_at": at, "frozen_by": actor, "frozen_snapshot": snapshot, "version": gorm.Expr("version + 1")})
	return result.RowsAffected == 1, result.Error
}

func (r ScheduleRepository) SaveComparison(ctx context.Context, id string, version int, baselineID, comparison string) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND version = ?", id, version).
		Updates(map[string]any{"baseline_schedule_id": baselineID, "comparison_json": comparison, "version": version + 1})
	return result.RowsAffected == 1, result.Error
}

// ExpirableForLot returns the lot's plans that still drive shop-floor work and
// therefore become stale when readings change.
func (r ScheduleRepository) ExpirableForLot(ctx context.Context, lotID string, states []string) ([]model.DryingSchedule, error) {
	var items []model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("timber_lot_id = ? AND expired_at IS NULL AND schedule_state IN ?", lotID, states).
		Order("calculated_at desc").
		Find(&items).Error
	return items, err
}

// ExpireMany marks every still-current plan in the batch as expired. The
// version bump is evaluated in SQL so a concurrent lifecycle transition loses
// its optimistic lock instead of being silently overwritten.
func (r ScheduleRepository) ExpireMany(ctx context.Context, ids []string, actor, reason string, at time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.DB.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id IN ? AND expired_at IS NULL", ids).
		Updates(map[string]any{"expired_at": at, "expired_by": actor, "expiry_reason": reason, "version": gorm.Expr("version + 1")})
	return result.RowsAffected, result.Error
}

// LatestExpiredUnclaimed returns the tip of the lot's expired plan chain: the
// newest expired plan not yet linked to a recalculated successor.
func (r ScheduleRepository) LatestExpiredUnclaimed(ctx context.Context, lotID string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("timber_lot_id = ? AND expired_at IS NOT NULL AND superseded_by_id = ''", lotID).
		Order("calculated_at desc").First(&item).Error
	return item, err
}

// LinkSupersession closes the bidirectional replacement relation. It succeeds
// only while the old plan is still unclaimed, so concurrent recalculations
// cannot attach two successors to one expired plan.
func (r ScheduleRepository) LinkSupersession(ctx context.Context, oldID, newID string) (bool, error) {
	return r.LinkSupersessionWithDB(ctx, r.DB, oldID, newID)
}

func (r ScheduleRepository) LinkSupersessionWithDB(ctx context.Context, db *gorm.DB, oldID, newID string) (bool, error) {
	result := db.WithContext(ctx).Model(&model.DryingSchedule{}).
		Where("id = ? AND expired_at IS NOT NULL AND superseded_by_id = ''", oldID).
		Updates(map[string]any{"superseded_by_id": newID, "version": gorm.Expr("version + 1")})
	return result.RowsAffected == 1, result.Error
}

// LatestFrozenForLot returns the newest frozen, non-expired plan, which is the
// plan a lot must have before it may leave equalizing for completion.
func (r ScheduleRepository) LatestFrozenForLot(ctx context.Context, lotID string) (model.DryingSchedule, error) {
	var item model.DryingSchedule
	err := r.DB.WithContext(ctx).
		Where("timber_lot_id = ? AND frozen_at IS NOT NULL AND expired_at IS NULL", lotID).
		Order("calculated_at desc").First(&item).Error
	return item, err
}
