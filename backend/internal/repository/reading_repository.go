package repository

import (
	"context"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"time"
)

type ReadingRepository struct{ DB *gorm.DB }

func (r ReadingRepository) List(ctx context.Context, lotID string) ([]model.MoistureReading, error) {
	var items []model.MoistureReading
	q := r.DB.WithContext(ctx).Order("measured_at asc")
	if lotID != "" {
		q = q.Where("timber_lot_id = ?", lotID)
	}
	return items, q.Find(&items).Error
}

// Accepted returns only readings that remain eligible for a safety calculation.
// Voided and analyst-flagged rows stay visible in List for audit purposes.
func (r ReadingRepository) Accepted(ctx context.Context, lotID string) ([]model.MoistureReading, error) {
	var items []model.MoistureReading
	query := r.DB.WithContext(ctx).Where("reading_quality = ?", "accepted").Order("measured_at asc")
	if lotID != "" {
		query = query.Where("timber_lot_id = ?", lotID)
	}
	return items, query.Find(&items).Error
}
func (r ReadingRepository) Get(ctx context.Context, id string) (model.MoistureReading, error) {
	var item model.MoistureReading
	err := r.DB.WithContext(ctx).First(&item, "id = ?", id).Error
	return item, err
}
func (r ReadingRepository) Create(ctx context.Context, item *model.MoistureReading) error {
	return r.DB.WithContext(ctx).Create(item).Error
}
func (r ReadingRepository) CreateBatch(ctx context.Context, items []model.MoistureReading) error {
	return r.CreateBatchWithDB(ctx, r.DB, items)
}

func (r ReadingRepository) CreateBatchWithDB(ctx context.Context, db *gorm.DB, items []model.MoistureReading) error {
	return db.WithContext(ctx).Create(&items).Error
}

func (r ReadingRepository) ExistsChecksum(ctx context.Context, lotID, checksum string) (bool, error) {
	return r.ExistsChecksumWithDB(ctx, r.DB, lotID, checksum)
}

// ExistsChecksumWithDB treats voided readings as history, rather than active
// measurements. This permits a corrected import to retain identical telemetry.
func (r ReadingRepository) ExistsChecksumWithDB(ctx context.Context, db *gorm.DB, lotID, checksum string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&model.MoistureReading{}).
		Where("timber_lot_id = ? AND source_checksum = ? AND reading_quality <> ?", lotID, checksum, "voided").Count(&count).Error
	return count > 0, err
}
func (r ReadingRepository) Delete(ctx context.Context, id string) error {
	return r.DB.WithContext(ctx).Delete(&model.MoistureReading{}, "id = ?", id).Error
}

func (r ReadingRepository) Void(ctx context.Context, id string, version int, actor, reason string, at time.Time) (bool, error) {
	return r.VoidWithDB(ctx, r.DB, id, version, actor, reason, at)
}

func (r ReadingRepository) VoidWithDB(ctx context.Context, db *gorm.DB, id string, version int, actor, reason string, at time.Time) (bool, error) {
	result := db.WithContext(ctx).Model(&model.MoistureReading{}).
		Where("id = ? AND reading_quality = ? AND version = ?", id, "accepted", version).
		Updates(map[string]any{"reading_quality": "voided", "voided_at": at, "voided_by": actor, "void_reason": reason, "version": version + 1})
	return result.RowsAffected == 1, result.Error
}
