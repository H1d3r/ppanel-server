package selfsub

import (
	"context"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"

	"github.com/perfect-panel/server/pkg/logger"

	"github.com/perfect-panel/server/internal/module/subscription/internal/deduction"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// orderTypeRenewal is the billing order type of a paid renewal.
const orderTypeRenewal = 2

// refundBasis is what was paid for the subscription term: the original order
// plus paid renewals. Traffic resets buy traffic, not time, and are not
// refunded.
func refundBasis(details *order.Details) int64 {
	basis := details.Amount + details.GiftAmount
	for _, subOrder := range details.SubOrders {
		if isPaidRenewal(subOrder) {
			basis += subOrder.Amount + subOrder.GiftAmount
		}
	}
	return basis
}

func isPaidRenewal(o *order.Order) bool {
	return o.Type == orderTypeRenewal && (o.Status == 2 || o.Status == 5)
}

func CalculateRemainingAmount(ctx context.Context, deps Deps, userSubscribeId int64) (int64, error) {
	// Find User Subscribe
	userSubscribe, err := deps.UserSubs.FindOneUserSubscribe(ctx, userSubscribeId)
	if err != nil {
		logger.WithContext(ctx).Error("[func CalculateRemainingAmount(ctx context.Context, deps Deps, userSubscribeId int64) (int64, error) {\n] FindOneUserSubscribe", logger.Field("err", err.Error()), logger.Field("id", userSubscribeId))
		return 0, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneUserSubscribe failed, id: %d", userSubscribeId)
	}
	if userSubscribe.EntitlementSource != "" {
		return 0, usersub.ErrProviderManaged
	}
	if userSubscribe.OrderId == 0 {
		return 0, nil
	}
	if !*userSubscribe.Subscribe.AllowDeduction && !deps.SingleModel() {
		return 0, errors.New("The subscription package does not support deductions")
	}

	if userSubscribe.Status != 1 {
		return 0, errors.New("The subscription package is not in use")
	}
	// Find Order Details
	orderDetails, err := deps.Orders.FindOneDetails(ctx, userSubscribe.OrderId)
	if err != nil {
		logger.WithContext(ctx).Error("[PreUnsubscribe] FindOneDetails", logger.Field("err", err.Error()), logger.Field("id", userSubscribe.OrderId))
		return 0, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneDetails failed, id: %d", userSubscribe.OrderId)
	}
	// Calculate Remaining Amount
	remainingAmount, err := deduction.CalculateRemainingAmount(
		deduction.Subscribe{
			StartTime:      userSubscribe.StartTime,
			ExpireTime:     userSubscribe.ExpireTime,
			Traffic:        userSubscribe.Traffic,
			Download:       userSubscribe.Download,
			Upload:         userSubscribe.Upload,
			UnitTime:       userSubscribe.Subscribe.UnitTime,
			ResetCycle:     userSubscribe.Subscribe.ResetCycle,
			DeductionRatio: userSubscribe.Subscribe.DeductionRatio,
		},
		deduction.Order{Amount: refundBasis(orderDetails)},
	)
	if err != nil {
		return 0, errors.Wrapf(xerr.NewErrCode(500), "CalculateRemainingAmount failed, userSubscribeId: %d, err: %v", userSubscribeId, err)
	}
	return remainingAmount, nil
}
