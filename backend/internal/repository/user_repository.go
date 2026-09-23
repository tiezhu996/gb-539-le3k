package repository

import (
	"context"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/model"
)

type UserRepository struct{ DB *gorm.DB }

func (r UserRepository) FindByEmail(ctx context.Context, email string) (model.User, error) {
	var user model.User
	err := r.DB.WithContext(ctx).Where("email = ?", email).First(&user).Error
	return user, err
}
func (r UserRepository) FindByID(ctx context.Context, id string) (model.User, error) {
	var user model.User
	err := r.DB.WithContext(ctx).First(&user, "id = ?", id).Error
	return user, err
}
