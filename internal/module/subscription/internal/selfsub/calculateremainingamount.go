package selfsub

import (
	"context"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"

	"github.com/perfect-panel/server/pkg/logger"

	"github.com/perfect-panel/server/internal/module/subscription/internal/deduction"
	"github.com/perfect-panel/server/internal/module/subscription/internal/period"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

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
	return o.Type == order.TypeRenewal && (o.Status == order.StatusPaid || o.Status == order.StatusFinished)
}

func CalculateRemainingAmount(ctx context.Context, deps Deps, userSubscribeId int64) (int64, error) {
	// Find User Subscribe
	userSubscribe, err := deps.UserSubs.FindOneUserSubscribe(ctx, userSubscribeId)
	if err != nil {
		logger.WithContext(ctx).Error("[CalculateRemainingAmount] FindOneUserSubscribe", logger.Field("err", err.Error()), logger.Field("id", userSubscribeId))
		return 0, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneUserSubscribe failed, id: %d", userSubscribeId)
	}
	if userSubscribe.EntitlementSource != "" {
		return 0, usersub.ErrProviderManaged
	}
	if userSubscribe.OrderId == 0 {
		return 0, nil
	}
	plan := userSubscribe.Subscribe
	if plan == nil {
		// The plan was deleted, and its refund rules with it.
		return 0, xerr.Errorf(xerr.SubscribeNotAvailable, "plan %d of subscription %d no longer exists", userSubscribe.SubscribeId, userSubscribeId)
	}
	// An unset flag (a row from before the column had a default) allows no
	// deduction, the safe reading.
	if (plan.AllowDeduction == nil || !*plan.AllowDeduction) && !deps.SingleModel() {
		return 0, xerr.Errorf(xerr.SubscribeNotAvailable, "plan %d does not allow deductions", plan.Id)
	}

	if userSubscribe.Status != usersub.SubscribeStatusActive {
		return 0, errors.New("The subscription package is not in use")
	}
	// Find Order Details
	orderDetails, err := deps.Orders.FindOneDetails(ctx, userSubscribe.OrderId)
	if err != nil {
		logger.WithContext(ctx).Error("[CalculateRemainingAmount] FindOneDetails", logger.Field("err", err.Error()), logger.Field("id", userSubscribe.OrderId))
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
			UnitTime:       period.Unit(plan.UnitTime),
			ResetCycle:     period.Cycle(plan.ResetCycle),
			DeductionRatio: plan.DeductionRatio,
		},
		deduction.Order{Amount: refundBasis(orderDetails)},
	)
	if err != nil {
		return 0, xerr.Wrapf(err, xerr.ERROR, "calculate the refund of subscription %d: %v", userSubscribeId, err)
	}
	return remainingAmount, nil
}
