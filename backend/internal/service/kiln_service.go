package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"timber-kiln-drying-optimizer/backend/internal/util"
	"time"
)

type KilnService struct {
	Repo  repository.KilnRepository
	Audit AuditService
}

func (s KilnService) List(ctx context.Context) ([]model.DryingKiln, error) { return s.Repo.List(ctx) }
func (s KilnService) Get(ctx context.Context, id string) (model.DryingKiln, error) {
	item, err := s.Repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ErrNotFound
	}
	return item, err
}
func (s KilnService) Create(ctx context.Context, input dto.KilnCreate, actor, requestID string) (model.DryingKiln, error) {
	item := model.DryingKiln{ID: util.ID(), KilnCode: input.KilnCode, Name: input.Name, CapacityM3: input.CapacityM3, MaxTemperatureC: input.MaxTemperatureC, MinHumidityPct: input.MinHumidityPct, AirflowClass: input.AirflowClass, OwnerTeam: input.OwnerTeam, KilnState: "ready", CommissionedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := s.Repo.Create(ctx, &item); err != nil {
		return item, fmt.Errorf("create kiln: %w", err)
	}
	_ = s.Audit.Record(ctx, requestID, "kiln", item.ID, "created", actor, nil, item)
	return item, nil
}
func (s KilnService) Update(ctx context.Context, id string, input dto.KilnUpdate, actor, requestID string) (model.DryingKiln, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	before := item
	if input.Name != "" {
		item.Name = input.Name
	}
	if input.MaxTemperatureC > 0 {
		item.MaxTemperatureC = input.MaxTemperatureC
	}
	if input.MinHumidityPct > 0 {
		item.MinHumidityPct = input.MinHumidityPct
	}
	if input.AirflowClass != "" {
		item.AirflowClass = input.AirflowClass
	}
	if input.OwnerTeam != "" {
		item.OwnerTeam = input.OwnerTeam
	}
	if input.KilnState != "" {
		item.KilnState = input.KilnState
	}
	item.UpdatedAt = time.Now().UTC()
	if err = s.Repo.Save(ctx, &item); err != nil {
		return item, err
	}
	_ = s.Audit.Record(ctx, requestID, "kiln", id, "updated", actor, before, item)
	return item, nil
}
