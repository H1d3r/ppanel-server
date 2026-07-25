// Package events hosts the queue shells of the domain-event bus: the publish
// pump moving outbox rows onto the asynq broker, and the delivery worker
// running subscribers.
package events

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/svc"
)

// DispatchDomainEventsLogic is the publish pump: it drains the generic
// domain-event outbox onto the asynq queue. Enqueues deduplicate by outbox
// event id, so the tick can retry freely.
type DispatchDomainEventsLogic struct {
	svcCtx *svc.ServiceContext
}

func NewDispatchDomainEventsLogic(svcCtx *svc.ServiceContext) *DispatchDomainEventsLogic {
	return &DispatchDomainEventsLogic{svcCtx: svcCtx}
}

func (l *DispatchDomainEventsLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.svcCtx.EventBus.Publish(ctx, 500)
}
