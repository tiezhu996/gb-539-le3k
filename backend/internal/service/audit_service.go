package service

import (
	"context"
	"timber-kiln-drying-optimizer/backend/internal/model"
	"timber-kiln-drying-optimizer/backend/internal/repository"
	"timber-kiln-drying-optimizer/backend/internal/util"
	"time"
)

type AuditService struct{ Repo repository.AuditRepository }

func (s AuditService) Record(ctx context.Context, requestID, entity, entityID, action, actor string, before, after any) error {
	return s.Repo.Create(ctx, &model.AuditEvent{ID: util.ID(), RequestID: requestID, Entity: entity, EntityID: entityID, Action: action, ActorID: actor, BeforeJSON: util.JSON(before), AfterJSON: util.JSON(after), CreatedAt: time.Now().UTC()})
}
func (s AuditService) List(ctx context.Context) ([]model.AuditEvent, error) { return s.Repo.List(ctx) }
