package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

type LotRepository struct{ DB *gorm.DB }

func (r LotRepository) List(ctx context.Context) ([]model.TimberLot, error) {
	var items []model.TimberLot
	err := r.DB.WithContext(ctx).Preload("Kiln").Order("loaded_at desc").Find(&items).Error
	return items, err
}
func (r LotRepository) Get(ctx context.Context, id string) (model.TimberLot, error) {
	var item model.TimberLot
	err := r.DB.WithContext(ctx).Preload("Kiln").First(&item, "id = ?", id).Error
	if err != nil {
		return item, fmt.Errorf("get lot: %w", err)
	}
	return item, nil
}
func (r LotRepository) Create(ctx context.Context, item *model.TimberLot) error {
	return r.DB.WithContext(ctx).Create(item).Error
}
func (r LotRepository) Save(ctx context.Context, item *model.TimberLot) error {
	return r.DB.WithContext(ctx).Save(item).Error
}

// Transition changes a lot only when the caller still holds the version it read.
func (r LotRepository) Transition(ctx context.Context, id, current, next string, version int) (bool, error) {
	result := r.DB.WithContext(ctx).Model(&model.TimberLot{}).
		Where("id = ? AND lot_state = ? AND version = ?", id, current, version).
		Updates(map[string]any{"lot_state": next, "version": version + 1})
	return result.RowsAffected == 1, result.Error
}
