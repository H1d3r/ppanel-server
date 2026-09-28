package portal

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/xerr"
)

// QueryPurchaseOrder returns the guest order's status snapshot and, once the
// account exists, the exchanged session token.
func (s *Service) QueryPurchaseOrder(ctx context.Context, req *dto.QueryPurchaseOrderRequest) (*dto.QueryPurchaseOrderResponse, error) {
	orderInfo, err := s.findOrder(ctx, req.OrderNo)
	if err != nil {
		return nil, err
	}
	if err := s.authorizePurchaseOrder(ctx, orderInfo, req.CheckoutToken); err != nil {
		return nil, err
	}
	var token string
	if order.IsSettled(orderInfo.Status) {
		if orderInfo.UserId == 0 {
			return nil, xerr.Errorf(xerr.OrderStatusError, "guest account is not ready")
		}
		if token, err = s.IssueSession(ctx, orderInfo.UserId); err != nil {
			return nil, err
		}
	}
	subscribeInfo, paymentInfo, err := s.fetchOrderDetails(ctx, orderInfo)
	if err != nil {
		return nil, err
	}
	return &dto.QueryPurchaseOrderResponse{
		OrderNo:        orderInfo.OrderNo,
		Subscribe:      subscribeInfo,
		Quantity:       orderInfo.Quantity,
		Price:          orderInfo.Price,
		Amount:         orderInfo.Amount,
		Discount:       orderInfo.Discount,
		Coupon:         orderInfo.Coupon,
		CouponDiscount: orderInfo.CouponDiscount,
		FeeAmount:      orderInfo.FeeAmount,
		Payment:        paymentInfo,
		Status:         orderInfo.Status,
		CreatedAt:      orderInfo.CreatedAt.UnixMilli(),
		Token:          token,
	}, nil
}

// authorizePurchaseOrder accepts either the authenticated owner of a completed
// guest order or the unguessable checkout capability issued when that order was
// created.  An email/identifier is not authentication and must never be used to
// mint a session token.
func (s *Service) authorizePurchaseOrder(ctx context.Context, orderInfo *order.Order, checkoutToken string) error {
	if orderInfo.UserId != 0 {
		if currentUser, ok := user.FromContext(ctx); ok && currentUser.Id == orderInfo.UserId {
			return nil
		}
	}
	return s.authorizeGuest(ctx, orderInfo, checkoutToken)
}

// fetchOrderDetails reads the plan and the payment method of the order for
// its status snapshot.
func (s *Service) fetchOrderDetails(ctx context.Context, orderInfo *order.Order) (dto.BillingSubscribeSnapshot, dto.PaymentMethod, error) {
	sub, err := s.deps.Plans.FindOne(ctx, orderInfo.SubscribeId)
	if err != nil {
		return dto.BillingSubscribeSnapshot{}, dto.PaymentMethod{}, xerr.Wrapf(err, xerr.DatabaseQueryError, "find subscribe %d", orderInfo.SubscribeId)
	}
	var subscribeInfo dto.BillingSubscribeSnapshot
	if err := mapping.Copy(&subscribeInfo, sub); err != nil {
		return dto.BillingSubscribeSnapshot{}, dto.PaymentMethod{}, xerr.Wrapf(err, xerr.ERROR, "map subscribe %d", orderInfo.SubscribeId)
	}

	method, err := s.deps.Payments.FindOne(ctx, orderInfo.PaymentId)
	if err != nil {
		return dto.BillingSubscribeSnapshot{}, dto.PaymentMethod{}, xerr.Wrapf(err, xerr.DatabaseQueryError, "find payment method %d", orderInfo.PaymentId)
	}
	return subscribeInfo, paymentMethodDTO(method), nil
}
