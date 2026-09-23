package repository

import (
	"context"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

type AuditRepository struct{ DB *gorm.DB }

func (r AuditRepository) Create(ctx context.Context, event *model.AuditEvent) error {
	return r.DB.WithContext(ctx).Create(event).Error
}
func (r AuditRepository) List(ctx context.Context) ([]model.AuditEvent, error) {
	var events []model.AuditEvent
	return events, r.DB.WithContext(ctx).Order("created_at desc").Limit(200).Find(&events).Error
}
