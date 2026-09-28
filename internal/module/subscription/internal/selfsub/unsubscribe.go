package selfsub

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

// Inbox consumers for the two unsubscribe stages (ADR-001 step 2), keyed by
// user-subscription id. The cancellation marker carries "orderID|remaining"
// so a replay can settle the refund without recomputing it.
const (
	unsubscribeCancelConsumer = "subscription.unsubscribe_cancel"
	unsubscribeRefundConsumer = "billing.unsubscribe_refund"
)

// errNotCancelable rejects cancelling a subscription that ended, was
// refunded or is stopped (usersub.CurrentStatuses may be cancelled).
var errNotCancelable = errors.New("subscription status invalid for cancellation")

// Unsubscribe cancels the subscription in a subscription-domain transaction,
// then settles the refund in a billing-domain transaction (gift amount first
// for balance-paid orders, then regular balance). A crash between the two is
// repaired when the user retries: a Deducted subscription whose refund marker
// is missing resumes at the refund stage.
func (s *Service) Unsubscribe(ctx context.Context, req *dto.UnsubscribeRequest) error {
	lg := logger.WithContext(ctx)
	u, ok := user.FromContext(ctx)
	if !ok {
		lg.Error("current user is not found in context")
		return xerr.NewErrCode(xerr.InvalidAccess)
	}
	userSub, err := s.deps.UserSubs.FindOneSubscribe(ctx, req.Id)
	if err != nil {
		lg.Errorw("[Unsubscribe] Find subscription failed", logger.Field("error", err.Error()), logger.Field("user_subscribe_id", req.Id))
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find subscription %d", req.Id)
	}
	if userSub.UserId != u.Id {
		lg.Errorw("[Unsubscribe] Subscription belongs to another user", logger.Field("user_subscribe_id", req.Id), logger.Field("user_id", u.Id))
		return errNotOwner
	}
	if userSub.EntitlementSource != "" {
		return usersub.ErrProviderManaged
	}

	subKey := strconv.FormatInt(req.Id, 10)
	if usersub.CurrentStatuses.Contains(userSub.Status) {
		if err := s.cancel(ctx, u.Id, req.Id, subKey); err != nil {
			lg.Errorw("[Unsubscribe] Cancel subscription failed", logger.Field("error", err.Error()), logger.Field("user_subscribe_id", req.Id))
			return err
		}
	} else {
		resumable, err := s.hasUnsettledRefund(ctx, userSub.Status, subKey)
		if err != nil {
			return err
		}
		if !resumable {
			lg.Errorw("[Unsubscribe] Subscription status invalid for cancellation", logger.Field("user_subscribe_id", req.Id), logger.Field("status", userSub.Status))
			return xerr.Wrapf(errNotCancelable, xerr.ERROR, "subscription %d has status %d", userSub.Id, userSub.Status)
		}
	}

	// Billing-domain transaction: settle the refund exactly once.
	if err := s.settleRefundOnce(ctx, u.Id, req.Id, subKey); err != nil {
		lg.Errorw("[Unsubscribe] Settle refund failed", logger.Field("error", err.Error()), logger.Field("user_subscribe_id", req.Id))
		return xerr.Wrapf(err, xerr.ERROR, "settle refund of subscription %d", req.Id)
	}
	if err := s.deps.Cache.ClearSubscribeCache(ctx, userSub); err != nil {
		lg.Errorw("[Unsubscribe] Clear subscription cache failed", logger.Field("error", err.Error()), logger.Field("user_subscribe_id", req.Id))
		return xerr.Wrapf(err, xerr.ERROR, "clear subscription cache")
	}
	if err := s.deps.Plans.ClearCache(ctx, userSub.SubscribeId); err != nil {
		lg.Errorw("[Unsubscribe] Clear plan cache failed", logger.Field("error", err.Error()), logger.Field("subscribe_id", userSub.SubscribeId))
		return xerr.Wrapf(err, xerr.ERROR, "clear plan cache")
	}
	return nil
}

// cancel flips the subscription to Deducted and durably records what the
// billing stage owes, in one subscription-domain transaction.
func (s *Service) cancel(ctx context.Context, userID, subID int64, subKey string) error {
	// The refund is the unused share of the subscription's time and traffic.
	remaining, err := CalculateRemainingAmount(ctx, s.deps, subID)
	if err != nil {
		return err
	}
	err = s.deps.Store.InSubscriptionTx(ctx, func(store repository.SubscriptionStore) error {
		// Re-read the subscription under a row lock. The context user is
		// only an authorization principal and can be stale.
		locked, err := store.UserSubscription().FindOneSubscribeForUpdate(ctx, subID)
		if err != nil {
			return err
		}
		if locked.UserId != userID {
			return errNotOwner
		}
		if locked.EntitlementSource != "" {
			return usersub.ErrProviderManaged
		}
		if !usersub.CurrentStatuses.Contains(locked.Status) {
			return xerr.Wrapf(errNotCancelable, xerr.ERROR, "subscription %d has status %d", locked.Id, locked.Status)
		}
		locked.Status = usersub.SubscribeStatusDeducted
		if err := store.UserSubscription().UpdateSubscribeColumns(ctx, locked, "status"); err != nil {
			return err
		}
		return store.Inbox().Insert(ctx, unsubscribeCancelConsumer, subKey, fmt.Sprintf("%d|%d", locked.OrderId, remaining))
	})
	return xerr.Wrapf(err, xerr.ERROR, "cancel subscription %d", subID)
}

// hasUnsettledRefund reports whether a non-cancelable subscription is a
// Deducted one whose cancellation committed but whose refund never did.
func (s *Service) hasUnsettledRefund(ctx context.Context, status uint8, subKey string) (bool, error) {
	if status != usersub.SubscribeStatusDeducted {
		return false, nil
	}
	cancelled, err := s.deps.Inbox.Find(ctx, unsubscribeCancelConsumer, subKey)
	if err != nil {
		return false, xerr.Wrapf(err, xerr.DatabaseQueryError, "find cancellation marker")
	}
	if cancelled == nil {
		return false, nil
	}
	refunded, err := s.deps.Inbox.Find(ctx, unsubscribeRefundConsumer, subKey)
	if err != nil {
		return false, xerr.Wrapf(err, xerr.DatabaseQueryError, "find refund marker")
	}
	return refunded == nil, nil
}

// settleRefundOnce credits the refund recorded by the cancellation marker in
// a billing-domain transaction, guarded by the refund marker.
func (s *Service) settleRefundOnce(ctx context.Context, userID, subID int64, subKey string) error {
	cancelled, err := s.deps.Inbox.Find(ctx, unsubscribeCancelConsumer, subKey)
	if err != nil {
		return err
	}
	if cancelled == nil {
		return fmt.Errorf("cancellation marker missing for subscription %s", subKey)
	}
	refunded, err := s.deps.Inbox.Find(ctx, unsubscribeRefundConsumer, subKey)
	if err != nil {
		return err
	}
	if refunded != nil {
		return nil
	}
	orderID, remainingAmount, err := parseCancellationMarker(cancelled.Result)
	if err != nil {
		return err
	}
	return s.deps.Store.InRefundTx(ctx, func(ledger RefundLedger) error {
		// Subscriptions created by an administrator have no associated order.
		// They can be cancelled, but there is no payment to refund.
		if orderID != 0 {
			if err := s.refund(ctx, ledger, userID, subID, orderID, remainingAmount); err != nil {
				return err
			}
		}
		return ledger.MarkRefunded(ctx, subKey)
	})
}

// refund credits the order's refund to the buyer's wallet, logs the movement
// and reverses the referral commission the order earned.
func (s *Service) refund(ctx context.Context, ledger RefundLedger, userID, subID, orderID, remainingAmount int64) error {
	lockedUser, err := ledger.LockWallet(ctx, userID)
	if err != nil {
		return err
	}
	// Query the original order information to determine refund strategy
	orderInfo, err := ledger.OrderDetails(ctx, orderID)
	if err != nil {
		return err
	}
	// A refund never exceeds what was paid, whatever amount an older
	// cancellation marker recorded.
	remainingAmount = min(remainingAmount, refundBasis(orderInfo))
	// Calculate refund distribution based on payment method and gift amount priority
	var balance, gift int64
	if orderInfo.Method == "balance" {
		// For balance-paid orders, prioritize refunding to gift amount first
		if orderInfo.GiftAmount >= remainingAmount {
			// Gift amount covers the entire refund - refund all to gift balance
			gift = remainingAmount
			balance = lockedUser.Balance // Regular balance remains unchanged
		} else {
			// Gift amount insufficient - refund to gift first, remainder to regular balance
			gift = orderInfo.GiftAmount
			balance = lockedUser.Balance + (remainingAmount - orderInfo.GiftAmount)
		}
	} else {
		// For non-balance payment orders, refund entirely to regular balance
		balance = remainingAmount + lockedUser.Balance
		gift = 0
	}

	// Create balance log entry only if there's an actual regular balance refund
	balanceRefundAmount := balance - lockedUser.Balance
	if balanceRefundAmount > 0 {
		balanceLog := log.Balance{
			OrderNo:   orderInfo.OrderNo,
			Amount:    balanceRefundAmount,
			Type:      log.BalanceTypeRefund, // Type 4 represents refund transaction
			Balance:   balance,
			Timestamp: timeutil.Now().UnixMilli(),
		}
		content, _ := balanceLog.Marshal()

		if err := ledger.AppendLog(ctx, &log.SystemLog{
			Type:     log.TypeBalance.Uint8(),
			Date:     timeutil.Now().Format(time.DateOnly),
			ObjectID: lockedUser.UserId,
			Content:  string(content),
		}); err != nil {
			return err
		}
	}

	// Create gift amount log entry if there's a gift balance refund
	if gift > 0 {
		giftLog := log.Gift{
			SubscribeId: subID,
			OrderNo:     orderInfo.OrderNo,
			Type:        log.GiftTypeIncrease, // Type 1 represents gift amount increase
			Amount:      gift,
			Balance:     lockedUser.GiftAmount + gift,
			Remark:      "Unsubscribe refund",
		}
		content, _ := giftLog.Marshal()

		if err := ledger.AppendLog(ctx, &log.SystemLog{
			Type:     log.TypeGift.Uint8(),
			Date:     timeutil.Now().Format(time.DateOnly),
			ObjectID: lockedUser.UserId,
			Content:  string(content),
		}); err != nil {
			return err
		}
		// Update user's gift amount
		lockedUser.GiftAmount += gift
	}

	// Update only financial fields so this refund cannot overwrite a
	// concurrent profile/auth update.
	lockedUser.Balance = balance
	if err := ledger.SaveBalance(ctx, lockedUser); err != nil {
		return err
	}
	return s.reverseCommission(ctx, ledger, userID, orderInfo, remainingAmount)
}

// reverseCommission takes back the referral commission the refunded orders
// earned, in proportion to the refund, so recycled balance cannot farm
// commission through buy-and-refund loops. A referrer who already withdrew it
// goes negative, which blocks withdrawals until it is earned back.
func (s *Service) reverseCommission(ctx context.Context, ledger RefundLedger, buyerID int64, details *order.Details, refund int64) error {
	commission := details.Commission
	for _, subOrder := range details.SubOrders {
		if isPaidRenewal(subOrder) {
			commission += subOrder.Commission
		}
	}
	basis := refundBasis(details)
	if commission <= 0 || refund <= 0 || basis <= 0 {
		return nil
	}
	reversed := commission
	if refund < basis {
		reversed = int64(float64(commission) * float64(refund) / float64(basis))
	}
	if reversed <= 0 {
		return nil
	}
	buyer, err := s.deps.Users.FindOne(ctx, buyerID)
	if err != nil {
		return err
	}
	if buyer.RefererId == 0 {
		return nil
	}
	referer, err := ledger.LockWallet(ctx, buyer.RefererId)
	if err != nil {
		return err
	}
	referer.Commission -= reversed
	if err := ledger.SaveCommission(ctx, referer); err != nil {
		return err
	}
	// Negative like withdrawals, so summed commission logs stay net.
	content, err := (&log.Commission{
		Type:      log.CommissionTypeRefund,
		Amount:    -reversed,
		OrderNo:   details.OrderNo,
		Timestamp: timeutil.Now().UnixMilli(),
	}).Marshal()
	if err != nil {
		return err
	}
	return ledger.AppendLog(ctx, &log.SystemLog{
		Type:     log.TypeCommission.Uint8(),
		Date:     timeutil.Now().Format(time.DateOnly),
		ObjectID: referer.UserId,
		Content:  string(content),
	})
}

func parseCancellationMarker(result string) (orderID, remainingAmount int64, err error) {
	parts := strings.SplitN(result, "|", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("corrupt cancellation marker %q", result)
	}
	if orderID, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		return 0, 0, fmt.Errorf("corrupt cancellation marker %q: %w", result, err)
	}
	if remainingAmount, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
		return 0, 0, fmt.Errorf("corrupt cancellation marker %q: %w", result, err)
	}
	return orderID, remainingAmount, nil
}
