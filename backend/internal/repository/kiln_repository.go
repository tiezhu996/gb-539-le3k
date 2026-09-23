package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

type KilnRepository struct{ DB *gorm.DB }

func (r KilnRepository) List(ctx context.Context) ([]model.DryingKiln, error) {
	var items []model.DryingKiln
	err := r.DB.WithContext(ctx).Order("updated_at desc").Find(&items).Error
	return items, err
}
func (r KilnRepository) Get(ctx context.Context, id string) (model.DryingKiln, error) {
	var item model.DryingKiln
	err := r.DB.WithContext(ctx).Preload("Lots").First(&item, "id = ?", id).Error
	if err != nil {
		return item, fmt.Errorf("get kiln: %w", err)
	}
	return item, nil
}
func (r KilnRepository) Create(ctx context.Context, item *model.DryingKiln) error {
	return r.DB.WithContext(ctx).Create(item).Error
}
func (r KilnRepository) Save(ctx context.Context, item *model.DryingKiln) error {
	return r.DB.WithContext(ctx).Save(item).Error
}
