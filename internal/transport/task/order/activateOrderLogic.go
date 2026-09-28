// Package order contains task adapters for the billing workflow.
package order

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/pkg/logger"
)

// Order types and statuses of the billing order entity, under the names the
// task adapters have always used.
const (
	OrderTypeSubscribe    = orderEntity.TypeSubscribe
	OrderTypeRenewal      = orderEntity.TypeRenewal
	OrderTypeResetTraffic = orderEntity.TypeResetTraffic
	OrderTypeRecharge     = orderEntity.TypeRecharge
	OrderStatusPending    = orderEntity.StatusPending
	OrderStatusPaid       = orderEntity.StatusPaid
	OrderStatusClose      = orderEntity.StatusClosed
	OrderStatusFailed     = orderEntity.StatusFailed
	OrderStatusFinished   = orderEntity.StatusFinished
)

type PaidOrderActivator interface {
	ActivatePaidOrder(context.Context, string) error
}

type ActivateOrderLogic struct{ activator PaidOrderActivator }

func NewActivateOrderLogic(activator PaidOrderActivator) *ActivateOrderLogic {
	return &ActivateOrderLogic{activator: activator}
}

func (l *ActivateOrderLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := l.parsePayload(ctx, task.Payload())
	if err != nil {
		return err
	}
	return l.activator.ActivatePaidOrder(ctx, payload.OrderNo)
}

// parsePayload unMarshals the task payload into a structured format
func (l *ActivateOrderLogic) parsePayload(ctx context.Context, payload []byte) (*taskqueue.ForthwithActivateOrderPayload, error) {
	var p taskqueue.ForthwithActivateOrderPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Unmarshal payload failed",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(payload)),
		)
		return nil, err
	}
	return &p, nil
}
