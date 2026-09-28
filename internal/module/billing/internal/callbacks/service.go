// Package callbacks implements the payment gateway callback subdomain of the
// billing module: it authenticates gateway notifications, verifies them
// against the order's immutable payment expectation, re-confirms with the
// gateway and settles the payment. The gateway-specific protocol lives in
// the gateway package; this is the one flow every gateway goes through. Only
// the module facade may reach it.
package callbacks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/gateway"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	pkgerrors "github.com/pkg/errors"
	"gorm.io/gorm"
)

type Service struct {
	orders   settle.Orders
	queue    settle.Queue
	gateways *gateway.Registry
}

// NewService builds the callback flow; a nil registry selects the production
// gateways.
func NewService(orders settle.Orders, queue settle.Queue, gateways *gateway.Registry) *Service {
	if gateways == nil {
		gateways = gateway.NewRegistry()
	}
	return &Service{orders: orders, queue: queue, gateways: gateways}
}

// Notify authenticates and settles a callback delivered to the notify URL of
// the payment method the notify middleware put in ctx.
func (s *Service) Notify(ctx context.Context, n gateway.Notification) error {
	method, ok := ctx.Value(requestctx.CtxKeyPayment).(*payment.Payment)
	if !ok {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.ERROR), "payment config not found")
	}
	if err := s.notify(ctx, method, n); err != nil {
		logger.WithContext(ctx).Errorw("[PaymentNotify] Callback rejected",
			logger.Field("platform", method.Platform), logger.Field("payment", method.Id), logger.Field("error", err.Error()))
		return err
	}
	return nil
}

func (s *Service) notify(ctx context.Context, method *payment.Payment, n gateway.Notification) error {
	gw, err := s.gateways.Open(method)
	if err != nil {
		return err
	}
	notice, err := gw.ParseCallback(ctx, n)
	if err != nil {
		return err
	}
	if notice.Ignore {
		return nil
	}
	orderInfo, err := s.orders.FindOneByOrderNo(ctx, notice.OrderNo)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order not exist: %v", notice.OrderNo)
	}
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find order %s", notice.OrderNo)
	}
	if err := validateOrderPayment(orderInfo, method); err != nil {
		return err
	}
	if err := gw.CheckOrder(orderInfo, notice); err != nil {
		return err
	}
	if !notice.Paid {
		return acknowledgeLifecycle(ctx, orderInfo, notice)
	}
	if finished, err := finishedOrderDuplicate(ctx, orderInfo, notice.TradeNo); err != nil || finished {
		return err
	}
	if err := validateOrderCanSettle(orderInfo); err != nil {
		return err
	}
	if err := validatePaymentExpectation(orderInfo, notice.Amount, notice.Currency); err != nil {
		return err
	}
	if err := gw.ConfirmPayment(ctx, orderInfo, notice); err != nil {
		return err
	}
	if err := settle.VerifiedPayment(ctx, s.orders, s.queue, orderInfo, notice.TradeNo); err != nil {
		return err
	}
	logger.WithContext(ctx).Infow("[PaymentNotify] Notify processed", logger.Field("platform", method.Platform), logger.Field("orderNo", orderInfo.OrderNo))
	return nil
}

// acknowledgeLifecycle accepts a valid lifecycle event that does not
// announce a payment. It is not a failed payment callback: it is
// acknowledged without settling or downgrading the local order, even when
// delivery is out of order or a cancelled order is already closed.
func acknowledgeLifecycle(ctx context.Context, orderInfo *order.Order, notice *gateway.Notice) error {
	if err := validatePaymentExpectation(orderInfo, notice.Amount, notice.Currency); err != nil {
		return err
	}
	fields := append([]logger.LogField{
		logger.Field("orderNo", orderInfo.OrderNo),
		logger.Field("status", notice.Status),
		logger.Field("order_status", orderInfo.Status),
	}, notice.Fields...)
	if notice.ManualReview {
		logger.WithContext(ctx).Errorw("[PaymentNotify] Payment requires manual review", append(fields, logger.Field("requires_manual_review", true))...)
		return nil
	}
	logger.WithContext(ctx).Infow("[PaymentNotify] Payment status received without settlement", fields...)
	return nil
}

func validateOrderPayment(orderInfo *order.Order, method *payment.Payment) error {
	if orderInfo.PaymentId != method.Id {
		return errors.New("payment method mismatch")
	}
	if orderInfo.Method != method.Platform {
		return errors.New("payment platform mismatch")
	}
	return nil
}

func validatePaymentExpectation(orderInfo *order.Order, amount int64, currency string) error {
	if orderInfo.PaymentCurrency == "" {
		return errors.New("payment amount snapshot is missing; restart checkout")
	}
	if orderInfo.PaymentAmount != amount {
		return errors.New("payment amount mismatch")
	}
	if !strings.EqualFold(orderInfo.PaymentCurrency, currency) {
		return errors.New("payment currency mismatch")
	}
	return nil
}

// finishedOrderDuplicate reports whether the order is already in the finished
// state and the incoming callback is a safe duplicate.
//
// Historical orders created before trade_no persistence was introduced may
// have an empty TradeNo field.  Blocking those retried callbacks would
// permanently prevent them from being acknowledged.  Instead, a warning is
// emitted so the gap can be audited, and the callback is treated as a known
// duplicate so the gateway stops retrying.
func finishedOrderDuplicate(ctx context.Context, orderInfo *order.Order, tradeNo string) (bool, error) {
	if orderInfo.Status != order.StatusFinished {
		return false, nil
	}
	if err := settle.ValidateTradeNo(tradeNo); err != nil {
		return false, err
	}
	if orderInfo.TradeNo == "" {
		// Legacy order: trade_no was not persisted at payment time.
		// Warn for audit purposes and accept the duplicate gracefully.
		logger.WithContext(ctx).Infow("[finishedOrderDuplicate] finished order has no trade_no recorded; treating callback as duplicate",
			logger.Field("orderNo", orderInfo.OrderNo),
			logger.Field("incomingTradeNo", tradeNo),
		)
		return true, nil
	}
	if orderInfo.TradeNo != tradeNo {
		return false, errors.New("order trade number mismatch")
	}
	return true, nil
}

func validateOrderCanSettle(orderInfo *order.Order) error {
	if !order.CanSettle(orderInfo.Status) {
		return fmt.Errorf("invalid order status transition: %d", orderInfo.Status)
	}
	return nil
}
