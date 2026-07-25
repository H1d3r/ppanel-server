package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/eventbus"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/queue/types"
)

// DeliverDomainEventLogic is the delivery worker: each task carries one
// outbox event, and the bus runs every subscriber of its topic. A failing
// subscriber fails the task so asynq retries with backoff and eventually
// archives it (the dead-letter queue); subscribers are idempotent, so
// retries and duplicate deliveries are safe.
type DeliverDomainEventLogic struct {
	svcCtx *svc.ServiceContext
}

func NewDeliverDomainEventLogic(svcCtx *svc.ServiceContext) *DeliverDomainEventLogic {
	return &DeliverDomainEventLogic{svcCtx: svcCtx}
}

func (l *DeliverDomainEventLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload types.EventDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// A malformed payload can never deliver; retrying cannot fix it.
		return fmt.Errorf("unmarshal event payload: %v: %w", err, asynq.SkipRetry)
	}
	return l.svcCtx.EventBus.Deliver(ctx, eventbus.Event{
		ID:      payload.ID,
		Topic:   payload.Topic,
		Key:     payload.Key,
		Payload: payload.Payload,
	})
}
