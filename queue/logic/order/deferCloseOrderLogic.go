package orderLogic

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/perfect-panel/server/pkg/logger"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/queue/types"
)

type DeferCloseOrderLogic struct {
	svc *svc.ServiceContext
}

func NewDeferCloseOrderLogic(svc *svc.ServiceContext) *DeferCloseOrderLogic {
	return &DeferCloseOrderLogic{
		svc: svc,
	}
}

func (l *DeferCloseOrderLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload := types.DeferCloseOrderPayload{}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.WithContext(ctx).Error("[DeferCloseOrderLogic] Unmarshal payload failed",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(task.Payload())),
		)
		return nil
	}

	err := l.svc.Billing.CloseOrder(ctx, &dto.CloseOrderRequest{
		OrderNo: payload.OrderNo,
	})
	if err != nil && errors.Is(err, billing.ErrGatewayUnconfirmed) {
		// Expected for EPay orders the gateway cannot confirm as paid: the
		// order stays pending and the reconciler keeps watching it, so
		// retrying this task would only repeat the same refusal.
		logger.WithContext(ctx).Infow("[DeferCloseOrderLogic] order stays pending until the gateway confirms payment",
			logger.Field("orderNo", payload.OrderNo),
		)
		return nil
	}
	count, ok := asynq.GetRetryCount(ctx)
	if !ok {
		return nil
	}
	if err != nil && count < 3 {
		return err
	}
	return nil
}
