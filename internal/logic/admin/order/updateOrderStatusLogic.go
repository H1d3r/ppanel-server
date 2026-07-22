package order

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/logger"
	queue "github.com/perfect-panel/server/queue/types"
)

type UpdateOrderStatusLogic struct {
	logger.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// Update order status
func NewUpdateOrderStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateOrderStatusLogic {
	return &UpdateOrderStatusLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateOrderStatusLogic) UpdateOrderStatus(req *dto.UpdateOrderStatusRequest) error {
	store := l.svcCtx.Store
	info, err := store.Order().FindOne(l.ctx, req.Id)
	if err != nil {
		l.Errorw("[UpdateOrderStatus] FindOne error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOne error: %v", err.Error())
	}

	// Orders have a deliberately narrow state machine. Arbitrary status writes
	// could resurrect terminal orders or skip the activation workflow.
	if req.Status != 2 && req.Status != 3 {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_ORDER_TRANSITION"), "only pending orders may be marked paid or closed")
	}
	if req.Status == 2 && req.TradeNo == "" {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "TRADE_NO_REQUIRED"), "trade_no is required when marking an order paid")
	}
	if req.Status == 3 && (req.PaymentId != 0 || req.TradeNo != "") {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_ORDER_CLOSE_REQUEST"), "payment_id and trade_no are not allowed when closing an order")
	}

	var transitioned bool
	err = store.InTx(l.ctx, func(txStore repository.Store) error {
		orderStore := txStore.Order()
		current, err := orderStore.FindOneByOrderNoForUpdate(l.ctx, info.OrderNo)
		if err != nil {
			return err
		}
		if current.Status != 1 {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}

		if req.Status == 2 {
			if req.PaymentId != 0 {
				paymentMethod, err := txStore.Payment().FindOne(l.ctx, req.PaymentId)
				if err != nil {
					return errors.Wrapf(xerr.NewErrCode(xerr.PaymentMethodNotFound), "payment method not found: %v", err)
				}
				current.PaymentId = paymentMethod.Id
				current.Method = paymentMethod.Platform
				if err := orderStore.Update(l.ctx, current); err != nil {
					return err
				}
			}
			transitioned, err = orderStore.MarkOrderPaid(l.ctx, current.OrderNo, req.TradeNo)
		} else {
			transitioned, err = orderStore.UpdateOrderStatusFrom(l.ctx, current.OrderNo, 1, 3)
		}
		if err != nil {
			return err
		}
		if !transitioned {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}
		return nil
	})
	if err != nil {
		l.Errorw("[UpdateOrderStatus] Transaction error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Transaction error: %v", err.Error())
	}
	if req.Status != 2 || !transitioned {
		return nil
	}
	payload, err := json.Marshal(queue.ForthwithActivateOrderPayload{OrderNo: info.OrderNo})
	if err != nil {
		return err
	}
	task := asynq.NewTask(queue.ForthwithActivateOrder, payload)
	_, err = l.svcCtx.Queue.EnqueueContext(l.ctx, task, asynq.TaskID(queue.ActivationTaskID(info.OrderNo)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	if err != nil {
		// The committed Paid state is a durable outbox; reconciliation will
		// repair an enqueue failure without reversing a real payment.
		return errors.Wrapf(xerr.NewErrCode(xerr.QueueEnqueueError), "enqueue activation: %v", err)
	}
	return nil
}
