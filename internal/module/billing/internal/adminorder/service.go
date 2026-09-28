// Package adminorder implements the admin-side order management subdomain of
// the billing module. Only the module facade may reach it.
package adminorder

import (
	"context"
	"errors"
	"time"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/gateway"
	"github.com/perfect-panel/server/internal/module/billing/internal/orderaudit"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// Orders is the order persistence administration reads outside a
// transaction.
type Orders interface {
	FindOne(ctx context.Context, id int64) (*order.Order, error)
	QueryOrderListByPage(ctx context.Context, page, size int, status uint8, user, subscribe int64, search string) (int64, []*order.Details, error)
	QueryDailyReport(ctx context.Context, date time.Time) (*order.DailyReport, error)
}

// Transactor mirrors the facade's billing-scoped transaction port.
type Transactor interface {
	InBillingTx(ctx context.Context, fn func(repository.BillingStore) error) error
}

// ActivationEnqueuer mirrors the facade's activation queue port.
type ActivationEnqueuer interface {
	EnqueueActivation(ctx context.Context, orderNo string) error
}

// PlanNameReader is the read port onto the subscription domain's plan
// catalogue, used to label the daily report's plan breakdown.
type PlanNameReader interface {
	FindOne(ctx context.Context, id int64) (*subscribe.Subscribe, error)
}

// OrderCloser is the checkout close flow shared with owner and expiry closes;
// it reports whether the call closed the order.
type OrderCloser interface {
	CloseByAdmin(ctx context.Context, orderNo string, adminID int64) (bool, error)
}

type Deps struct {
	Orders   Orders
	Payments gateway.MethodFinder
	Tx       Transactor
	Queue    ActivationEnqueuer
	// Plans resolves plan names for the daily report; optional so callers
	// that only manage orders need not provide it.
	Plans  PlanNameReader
	Closer OrderCloser
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) Create(ctx context.Context, req *dto.CreateOrderRequest) error {
	if req.Status != 0 && req.Status != order.StatusPending {
		return xerr.Errorf(xerr.InvalidInitialOrderStatus, "admin-created orders must start pending")
	}
	paymentMethod, err := findPaymentMethod(ctx, s.deps.Payments, req.PaymentId)
	if err != nil {
		return err
	}
	orderInfo := &order.Order{
		UserId:         req.UserId,
		OrderNo:        order.GenerateTradeNo(),
		Type:           req.Type,
		Quantity:       req.Quantity,
		Price:          req.Price,
		Amount:         req.Amount,
		Discount:       req.Discount,
		Coupon:         req.Coupon,
		CouponDiscount: req.CouponDiscount,
		PaymentId:      req.PaymentId,
		Method:         paymentMethod.Platform,
		FeeAmount:      req.FeeAmount,
		TradeNo:        req.TradeNo,
		Status:         order.StatusPending,
		SubscribeId:    req.SubscribeId,
	}
	if err := s.deps.Tx.InBillingTx(ctx, func(txStore repository.BillingStore) error {
		if err := txStore.Order().Insert(ctx, orderInfo); err != nil {
			return err
		}
		return orderaudit.InsertCreated(ctx, txStore.Log(), orderInfo, orderaudit.SourceAdmin)
	}); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseInsertError, "create order")
	}
	return nil
}

func (s *Service) List(ctx context.Context, req *dto.GetOrderListRequest) (*dto.GetOrderListResponse, error) {
	total, list, err := s.deps.Orders.QueryOrderListByPage(ctx, int(req.Page), int(req.Size), req.Status, req.UserId, req.SubscribeId, req.Search)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "query order list")
	}
	resp := &dto.GetOrderListResponse{Total: total, List: make([]dto.Order, 0)}
	if err := mapping.Copy(&resp.List, list); err != nil {
		return nil, xerr.Wrapf(err, xerr.ERROR, "map order list")
	}
	return resp, nil
}

// UpdateStatus applies an administrator's decision on a pending order: mark
// it paid with the gateway's trade number, or close it.
func (s *Service) UpdateStatus(ctx context.Context, req *dto.UpdateOrderStatusRequest) error {
	info, err := s.deps.Orders.FindOne(ctx, req.Id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Errorf(xerr.OrderNotExist, "order %d not found", req.Id)
	}
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find order %d", req.Id)
	}
	// Orders have a deliberately narrow state machine. Arbitrary status writes
	// could resurrect terminal orders or skip the activation workflow.
	if req.Status != order.StatusPaid && req.Status != order.StatusClosed {
		return xerr.Errorf(xerr.InvalidOrderTransition, "only pending orders may be marked paid or closed")
	}
	if req.Status == order.StatusPaid && req.TradeNo == "" {
		return xerr.Errorf(xerr.TradeNoRequired, "trade_no is required when marking an order paid")
	}
	if req.Status == order.StatusClosed && (req.PaymentId != 0 || req.TradeNo != "") {
		return xerr.Errorf(xerr.InvalidOrderCloseRequest, "payment_id and trade_no are not allowed when closing an order")
	}
	if req.Status == order.StatusClosed {
		return s.close(ctx, info)
	}

	err = s.deps.Tx.InBillingTx(ctx, func(txStore repository.BillingStore) error {
		orderStore := txStore.Order()
		current, err := orderStore.FindOneByOrderNoForUpdate(ctx, info.OrderNo)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "lock order %s", info.OrderNo)
		}
		if current.Status != order.StatusPending {
			return xerr.Errorf(xerr.OrderStatusError, "order is no longer pending")
		}
		if req.PaymentId != 0 {
			paymentMethod, err := findPaymentMethod(ctx, txStore.Payment(), req.PaymentId)
			if err != nil {
				return err
			}
			current.PaymentId = paymentMethod.Id
			current.Method = paymentMethod.Platform
			if err := orderStore.Update(ctx, current); err != nil {
				return xerr.Wrapf(err, xerr.DatabaseUpdateError, "rebind order %s", info.OrderNo)
			}
		}
		transitioned, err := orderStore.MarkOrderPaid(ctx, current.OrderNo, req.TradeNo)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "mark order %s paid", info.OrderNo)
		}
		if !transitioned {
			return xerr.Errorf(xerr.OrderStatusError, "order is no longer pending")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.deps.Queue.EnqueueActivation(ctx, info.OrderNo); err != nil {
		// The order is committed as paid, which is what the administrator
		// asked for; the paid-order reconciler re-drives the activation. A
		// reported failure would only invite a retry that the order, no
		// longer pending, refuses.
		logger.WithContext(ctx).Errorw("[UpdateOrderStatus] enqueue activation failed; the paid-order reconciler retries it",
			logger.Field("order_no", info.OrderNo), logger.Field("error", err.Error()))
	}
	return nil
}

// close routes the administrator's close through the checkout close flow, so
// the coupon, gift deduction and plan inventory are released and a
// cancellable gateway payment is voided first. A payment the gateway confirms
// is settled instead, and the close reports the order as no longer pending.
func (s *Service) close(ctx context.Context, info *order.Order) error {
	var adminID int64
	if admin, ok := user.FromContext(ctx); ok {
		adminID = admin.Id
	}
	closed, err := s.deps.Closer.CloseByAdmin(ctx, info.OrderNo, adminID)
	if err != nil {
		// A gateway that cannot confirm the payment carries its own code
		// (PaymentStatusUnconfirmed) through the generic wrap.
		return xerr.Wrapf(err, xerr.ERROR, "close order %s", info.OrderNo)
	}
	if !closed {
		return xerr.Errorf(xerr.OrderStatusError, "order is no longer pending")
	}
	return nil
}

// findPaymentMethod loads the method an administrator binds an order to. A
// disabled method may still be bound by hand; a missing one may not.
func findPaymentMethod(ctx context.Context, methods gateway.MethodFinder, id int64) (*paymentEntity.Payment, error) {
	method, err := methods.FindOne(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.Errorf(xerr.PaymentMethodNotFound, "payment method %d not found", id)
	}
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find payment method %d", id)
	}
	return method, nil
}
