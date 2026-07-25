// Package events hosts the queue shell of the in-process domain-event bus.
package events

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/svc"
)

// DispatchDomainEventsLogic drains the generic domain-event outbox through
// the bus; delivery is at-least-once and subscribers are idempotent, so the
// task can retry freely.
type DispatchDomainEventsLogic struct {
	svcCtx *svc.ServiceContext
}

func NewDispatchDomainEventsLogic(svcCtx *svc.ServiceContext) *DispatchDomainEventsLogic {
	return &DispatchDomainEventsLogic{svcCtx: svcCtx}
}

func (l *DispatchDomainEventsLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.svcCtx.EventBus.Dispatch(ctx, 500)
}
