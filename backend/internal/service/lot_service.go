package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"timber-kiln-drying-optimizer/backend/internal/constants"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"timber-kiln-drying-optimizer/backend/internal/util"
	"time"
)

type LotService struct {
	Repo      repository.LotRepository
	Kilns     repository.KilnRepository
	Schedules repository.ScheduleRepository
	Audit     AuditService
}

func (s LotService) List(ctx context.Context) ([]model.TimberLot, error) { return s.Repo.List(ctx) }
func (s LotService) Get(ctx context.Context, id string) (model.TimberLot, error) {
	item, err := s.Repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ErrNotFound
	}
	return item, err
}
func (s LotService) Create(ctx context.Context, input dto.LotCreate, actor, requestID string) (model.TimberLot, error) {
	if _, err := s.Kilns.Get(ctx, input.KilnID); err != nil {
		return model.TimberLot{}, fmt.Errorf("kiln: %w", ErrValidation)
	}
	if input.TargetMoisturePct >= input.InitialMoisturePct {
		return model.TimberLot{}, fmt.Errorf("target moisture must be below initial moisture: %w", ErrValidation)
	}
	item := model.TimberLot{ID: util.ID(), LotCode: input.LotCode, KilnID: input.KilnID, Species: input.Species, ThicknessMM: input.ThicknessMM, VolumeM3: input.VolumeM3, InitialMoisturePct: input.InitialMoisturePct, TargetMoisturePct: input.TargetMoisturePct, QualityGrade: input.QualityGrade, LoadedAt: time.Now().UTC(), LotState: constants.LotQueued, CreatedBy: actor, Version: 1}
	if err := s.Repo.Create(ctx, &item); err != nil {
		return item, err
	}
	_ = s.Audit.Record(ctx, requestID, "lot", item.ID, "created", actor, nil, item)
	return item, nil
}
func (s LotService) Transition(ctx context.Context, id, next, actor, requestID string, version int) (model.TimberLot, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	if !validLotTransition(item.LotState, next) {
		return item, ErrConflict
	}
	// Leaving equalizing closes the batch, so the shop-floor reference must be
	// the newest frozen plan. If it is missing or stale the batch stays put and
	// the caller is told a frozen plan is required.
	if next == constants.LotCompleted {
		latest, err := s.Schedules.LatestForLot(ctx, item.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, fmt.Errorf("latest schedule must be frozen before completion: %w", ErrConflict)
		}
		if err != nil {
			return item, err
		}
		if latest.ScheduleState != constants.ScheduleAccepted || latest.FrozenAt == nil || latest.SupersededAt != nil {
			return item, fmt.Errorf("latest schedule must be frozen before completion: %w", ErrConflict)
		}
	}
	if version != item.Version {
		return item, ErrConflict
	}
	before := item
	updated, err := s.Repo.Transition(ctx, item.ID, item.LotState, next, item.Version)
	if err != nil {
		return item, err
	}
	if !updated {
		return item, ErrConflict
	}
	item.LotState, item.Version = next, item.Version+1
	_ = s.Audit.Record(ctx, requestID, "lot", id, "state_changed", actor, before, item)
	return item, nil
}
func validLotTransition(current, next string) bool {
	if next == constants.LotAborted && current != constants.LotCompleted && current != constants.LotAborted {
		return true
	}
	switch current {
	case constants.LotQueued:
		return next == constants.LotConditioning
	case constants.LotConditioning:
		return next == constants.LotDrying
	case constants.LotDrying:
		return next == constants.LotEqualizing
	case constants.LotEqualizing:
		return next == constants.LotCompleted
	}
	return false
}
