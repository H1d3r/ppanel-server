// Package orderLogic provides order processing logic for handling various types of orders
// including subscription purchases, renewals, traffic resets, and balance recharges.
package orderLogic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/perfect-panel/server/internal/model/entity/log"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/logic/telegram"
	"github.com/perfect-panel/server/internal/model/entity/order"
	"github.com/perfect-panel/server/internal/model/entity/subscribe"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/uuidx"
	"github.com/perfect-panel/server/queue/types"
)

// Order type constants define the different types of orders that can be processed
const (
	OrderTypeSubscribe    = 1 // New subscription purchase
	OrderTypeRenewal      = 2 // Subscription renewal
	OrderTypeResetTraffic = 3 // Traffic quota reset
	OrderTypeRecharge     = 4 // Balance recharge
)

// Order status constants define the lifecycle states of an order
const (
	OrderStatusPending  = 1 // Order created but not paid
	OrderStatusPaid     = 2 // Order paid and ready for processing
	OrderStatusClose    = 3 // Order closed/cancelled
	OrderStatusFailed   = 4 // Order processing failed
	OrderStatusFinished = 5 // Order successfully completed
)

// Predefined error variables for common error conditions
var (
	ErrInvalidOrderStatus = fmt.Errorf("invalid order status")
	ErrInvalidOrderType   = fmt.Errorf("invalid order type")
)

// ActivateOrderLogic handles the activation and processing of paid orders
type ActivateOrderLogic struct {
	svc *svc.ServiceContext // Service context containing dependencies
}

// activationResult contains only post-commit work.  All financial and
// subscription mutations are committed with the order transition first; cache
// invalidation and notifications are deliberately kept outside that
// transaction because they are retryable side effects rather than settlement
// state.
type activationResult struct {
	order               *order.Order
	user                *user.User
	subscribe           *subscribe.Subscribe
	userSub             *user.Subscribe
	commissionRecipient *user.User
	notifyType          string
}

// NewActivateOrderLogic creates a new instance of ActivateOrderLogic
func NewActivateOrderLogic(svc *svc.ServiceContext) *ActivateOrderLogic {
	return &ActivateOrderLogic{
		svc: svc,
	}
}

// Inbox consumers for the activation stages (ADR-001 step 2). Each stage runs
// in its own single-domain transaction and marks itself processed in the
// idempotent inbox, keyed by order number.
const (
	inboxGuestAccount = "identity.guest_account"
	inboxFulfillment  = "subscription.fulfillment"
	inboxRecharge     = "identity.balance_recharge"
	inboxCommission   = "identity.commission"
)

// ProcessTask activates a paid order as a sequence of single-domain
// transactions instead of one cross-domain transaction (ADR-001 step 2):
//
//  1. identity: create the guest account (subscribe orders without a user)
//  2. subscription/identity: fulfill (open/extend/reset subscription, or
//     credit the recharge balance)
//  3. identity: settle referral commission
//  4. billing: coupon accounting + the Paid -> Finished transition, which
//     also appends the order.fulfilled outbox event
//
// Idempotency: every stage marks itself in the domain event inbox inside its
// own transaction, so Asynq's at-least-once delivery and the paid-order
// reconciler can replay the task safely — completed stages are skipped, the
// remaining ones run. The order stays Paid until stage 4 commits, so a crash
// between stages leaves a durable signal that the reconciler re-drives.
func (l *ActivateOrderLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := l.parsePayload(ctx, task.Payload())
	if err != nil {
		return err
	}
	orderInfo, err := l.svc.Store.Order().FindOneByOrderNo(ctx, payload.OrderNo)
	if err != nil {
		return err
	}
	if orderInfo.Status == OrderStatusFinished {
		return nil
	}
	if orderInfo.Status != OrderStatusPaid {
		return ErrInvalidOrderStatus
	}

	if orderInfo.Type == OrderTypeSubscribe && orderInfo.UserId == 0 {
		if err := l.ensureGuestAccount(ctx, orderInfo); err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Guest account stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
	}

	result, err := l.fulfillOrder(ctx, orderInfo)
	if err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Fulfillment stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
		return err
	}

	if orderInfo.Type == OrderTypeSubscribe || orderInfo.Type == OrderTypeRenewal {
		result.commissionRecipient, err = l.settleCommission(ctx, result.user, orderInfo)
		if err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Commission stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
	}

	if err := l.finalizeOrder(ctx, orderInfo); err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Finalize stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
		return err
	}

	l.afterActivationCommit(ctx, result)
	return nil
}

// ensureGuestAccount creates the guest's account in an identity-domain
// transaction, then binds it to the order in a billing write. The inbox
// marker stores the created user id so a replay re-binds the same account
// instead of creating a second one.
func (l *ActivateOrderLogic) ensureGuestAccount(ctx context.Context, orderInfo *order.Order) error {
	var userID int64
	mark, err := l.svc.Store.Inbox().Find(ctx, inboxGuestAccount, orderInfo.OrderNo)
	if err != nil {
		return err
	}
	if mark != nil {
		userID, err = strconv.ParseInt(mark.Result, 10, 64)
		if err != nil {
			return fmt.Errorf("corrupt guest account marker %q: %w", mark.Result, err)
		}
	} else {
		tempOrder, err := l.getGuestOrderInfo(ctx, orderInfo)
		if err != nil {
			return err
		}
		passwordHash := tempOrder.PasswordHash
		if passwordHash == "" {
			// Compatibility for an already-created guest checkout from an older
			// release. New records only retain PasswordHash in Redis.
			passwordHash = tool.EncodePassWord(tempOrder.Password)
		}
		if passwordHash == "" {
			return fmt.Errorf("guest order password hash is missing")
		}
		userInfo := &user.User{Password: passwordHash, Algo: tool.PasswordAlgoForHash(passwordHash)}
		err = l.svc.Store.InIdentityTx(ctx, func(store repository.IdentityStore) error {
			if err := store.User().Insert(ctx, userInfo); err != nil {
				return err
			}
			userInfo.ReferCode = uuidx.UserInviteCode(userInfo.Id)
			if err := store.User().Update(ctx, userInfo); err != nil {
				return err
			}
			if err := store.UserAuth().InsertUserAuthMethods(ctx, &user.AuthMethods{
				UserId:         userInfo.Id,
				AuthType:       tempOrder.AuthType,
				AuthIdentifier: tempOrder.Identifier,
			}); err != nil {
				return err
			}
			if tempOrder.InviteCode != "" {
				if referer, findErr := store.User().FindOneByReferCode(ctx, tempOrder.InviteCode); findErr == nil {
					userInfo.RefererId = referer.Id
					if err := store.User().Update(ctx, userInfo); err != nil {
						return err
					}
				} else {
					logger.WithContext(ctx).Error("Find referer failed", logger.Field("error", findErr.Error()), logger.Field("refer_code", tempOrder.InviteCode))
				}
			}
			return store.Inbox().Insert(ctx, inboxGuestAccount, orderInfo.OrderNo, strconv.FormatInt(userInfo.Id, 10))
		})
		if err != nil {
			return err
		}
		userID = userInfo.Id
	}
	// Billing write: bind the account to the order. Replays write the same
	// value, so this needs no transaction with the identity mutations above.
	orderInfo.UserId = userID
	return l.svc.Store.Order().Update(ctx, orderInfo)
}

// fulfillOrder applies the order's business effect in the owning domain's
// transaction. A replayed delivery whose fulfillment already committed only
// rebuilds the in-memory context for the post-commit side effects.
func (l *ActivateOrderLogic) fulfillOrder(ctx context.Context, orderInfo *order.Order) (*activationResult, error) {
	consumer := inboxFulfillment
	if orderInfo.Type == OrderTypeRecharge {
		consumer = inboxRecharge
	}
	mark, err := l.svc.Store.Inbox().Find(ctx, consumer, orderInfo.OrderNo)
	if err != nil {
		return nil, err
	}
	if mark != nil {
		return l.loadActivationResult(ctx, orderInfo)
	}
	var result *activationResult
	if orderInfo.Type == OrderTypeRecharge {
		// Recharge is a wallet credit: a pure billing-domain transaction.
		err = l.svc.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
			var txErr error
			result, txErr = l.activateRechargeTx(ctx, store, orderInfo)
			if txErr != nil {
				return txErr
			}
			return store.Inbox().Insert(ctx, consumer, orderInfo.OrderNo, "")
		})
	} else {
		// Subscription fulfillment still runs on the generic transaction: the
		// user row lock inside serialises per-user quota checks across
		// domains until the subscription module owns that concern (ADR-001
		// step 5).
		err = l.svc.Store.InTx(ctx, func(store repository.Store) error {
			var txErr error
			result, txErr = l.processOrderByTypeInTx(ctx, store, orderInfo)
			if txErr != nil {
				return txErr
			}
			// A duplicate key here means a concurrent delivery fulfilled
			// first; this transaction rolls back and the retry takes the
			// replay path.
			return store.Inbox().Insert(ctx, consumer, orderInfo.OrderNo, "")
		})
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// loadActivationResult rebuilds the post-commit context (caches,
// notifications) for a replayed delivery whose fulfillment already committed.
func (l *ActivateOrderLogic) loadActivationResult(ctx context.Context, orderInfo *order.Order) (*activationResult, error) {
	userInfo, err := l.svc.Store.User().FindOne(ctx, orderInfo.UserId)
	if err != nil {
		return nil, err
	}
	result := &activationResult{order: orderInfo, user: userInfo}
	switch orderInfo.Type {
	case OrderTypeRecharge:
		return result, nil
	case OrderTypeSubscribe, OrderTypeRenewal, OrderTypeResetTraffic:
		token := orderInfo.SubscribeToken
		if orderInfo.Type == OrderTypeSubscribe {
			// New-purchase tokens are derived from the order number.
			token = uuidx.SubscribeToken(orderInfo.OrderNo)
		}
		userSub, err := l.svc.Store.UserSubscription().FindOneSubscribeByToken(ctx, token)
		if err != nil {
			return nil, err
		}
		subID := orderInfo.SubscribeId
		if orderInfo.Type == OrderTypeResetTraffic {
			subID = userSub.SubscribeId
		}
		sub, err := l.svc.Store.Subscribe().FindOne(ctx, subID)
		if err != nil {
			return nil, err
		}
		result.subscribe, result.userSub = sub, userSub
		switch orderInfo.Type {
		case OrderTypeSubscribe:
			result.notifyType = telegram.PurchaseNotify
		case OrderTypeRenewal:
			result.notifyType = telegram.RenewalNotify
		case OrderTypeResetTraffic:
			result.notifyType = telegram.ResetTrafficNotify
		}
		return result, nil
	default:
		return nil, ErrInvalidOrderType
	}
}

// settleCommission credits the referral commission in an identity-domain
// transaction. The inbox marker also covers the "no commission applies"
// outcome so replays skip the referer lock entirely.
func (l *ActivateOrderLogic) settleCommission(ctx context.Context, userInfo *user.User, orderInfo *order.Order) (*user.User, error) {
	mark, err := l.svc.Store.Inbox().Find(ctx, inboxCommission, orderInfo.OrderNo)
	if err != nil {
		return nil, err
	}
	if mark != nil {
		return nil, nil
	}
	var recipient *user.User
	err = l.svc.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
		var txErr error
		recipient, txErr = l.handleCommissionTx(ctx, store, userInfo, orderInfo)
		if txErr != nil {
			return txErr
		}
		return store.Inbox().Insert(ctx, inboxCommission, orderInfo.OrderNo, "")
	})
	if err != nil {
		return nil, err
	}
	return recipient, nil
}

// finalizeOrder is the billing-domain settlement: coupon accounting and the
// Paid -> Finished transition (which appends the order.fulfilled outbox
// event) commit atomically. Losing the status CAS rolls the coupon count
// back, so it stays exactly-once.
//
// Known transitional window: an admin closing a Paid order between the
// fulfillment stage and this CAS leaves the fulfillment committed while the
// order ends Closed. The pre-split code had the same conflict resolved by row
// locks; compensation for admin closes of paid orders is a billing concern
// tracked in ADR-001 step 2.
func (l *ActivateOrderLogic) finalizeOrder(ctx context.Context, orderInfo *order.Order) error {
	return l.svc.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
		if orderInfo.Coupon != "" && !orderInfo.CouponReserved {
			if err := store.Coupon().UpdateCount(ctx, orderInfo.Coupon); err != nil {
				return err
			}
		}
		updated, err := store.Order().UpdateOrderStatusFrom(ctx, orderInfo.OrderNo, OrderStatusPaid, OrderStatusFinished)
		if err != nil {
			return err
		}
		if !updated {
			return ErrInvalidOrderStatus
		}
		return nil
	})
}

func (l *ActivateOrderLogic) processOrderByTypeInTx(ctx context.Context, store repository.Store, orderInfo *order.Order) (*activationResult, error) {
	switch orderInfo.Type {
	case OrderTypeSubscribe:
		return l.activateNewPurchaseTx(ctx, store, orderInfo)
	case OrderTypeRenewal:
		return l.activateRenewalTx(ctx, store, orderInfo)
	case OrderTypeResetTraffic:
		return l.activateResetTrafficTx(ctx, store, orderInfo)
	default:
		return nil, ErrInvalidOrderType
	}
}

func (l *ActivateOrderLogic) activateNewPurchaseTx(ctx context.Context, store repository.Store, orderInfo *order.Order) (*activationResult, error) {
	// Guest accounts are created by ensureGuestAccount before this stage, so
	// UserId is always set here. The user row lock is a transitional
	// serialisation point for per-user quota checks; the subscription module
	// takes over that concern once it owns its data (ADR-001 step 5).
	userInfo, err := store.User().FindOneForUpdate(ctx, orderInfo.UserId)
	if err != nil {
		return nil, err
	}

	sub, err := store.Subscribe().FindOne(ctx, orderInfo.SubscribeId)
	if err != nil {
		return nil, err
	}
	userSub, err := l.createUserSubscriptionTx(ctx, store, orderInfo, sub)
	if err != nil {
		return nil, err
	}
	return &activationResult{order: orderInfo, user: userInfo, subscribe: sub, userSub: userSub, notifyType: telegram.PurchaseNotify}, nil
}

func (l *ActivateOrderLogic) createUserSubscriptionTx(ctx context.Context, store repository.Store, orderInfo *order.Order, sub *subscribe.Subscribe) (*user.Subscribe, error) {
	if l.svc.Config.Subscribe.SingleModel {
		hasBlockingSubscription, err := store.UserSubscription().HasBlockingSubscription(ctx, orderInfo.UserId)
		if err != nil {
			return nil, err
		}
		if hasBlockingSubscription {
			return nil, fmt.Errorf("single subscription mode exceeds limit")
		}
	}
	if sub.Quota > 0 {
		count, err := store.UserSubscription().CountQuotaConsumingSubscriptions(ctx, orderInfo.UserId, orderInfo.SubscribeId)
		if err != nil {
			return nil, err
		}
		if count >= sub.Quota {
			return nil, fmt.Errorf("subscribe quota limit exceeded")
		}
	}
	now := timeutil.Now()
	userSub := &user.Subscribe{
		UserId:      orderInfo.UserId,
		OrderId:     orderInfo.Id,
		SubscribeId: orderInfo.SubscribeId,
		StartTime:   now,
		ExpireTime:  tool.AddTime(sub.UnitTime, orderInfo.Quantity, now),
		Traffic:     sub.Traffic,
		Token:       uuidx.SubscribeToken(orderInfo.OrderNo),
		UUID:        uuid.New().String(),
		Status:      1,
	}
	if err := store.UserSubscription().InsertSubscribe(ctx, userSub); err != nil {
		return nil, err
	}
	return userSub, nil
}

func (l *ActivateOrderLogic) activateRenewalTx(ctx context.Context, store repository.Store, orderInfo *order.Order) (*activationResult, error) {
	userInfo, err := store.User().FindOne(ctx, orderInfo.UserId)
	if err != nil {
		return nil, err
	}
	userSub, err := store.UserSubscription().FindOneSubscribeByTokenForUpdate(ctx, orderInfo.SubscribeToken)
	if err != nil {
		return nil, err
	}
	if userSub.UserId != orderInfo.UserId {
		return nil, fmt.Errorf("renewal subscription ownership mismatch")
	}
	sub, err := store.Subscribe().FindOne(ctx, orderInfo.SubscribeId)
	if err != nil {
		return nil, err
	}
	if err := l.updateSubscriptionForRenewalTx(ctx, store, userSub, sub, orderInfo); err != nil {
		return nil, err
	}
	return &activationResult{order: orderInfo, user: userInfo, subscribe: sub, userSub: userSub, notifyType: telegram.RenewalNotify}, nil
}

func (l *ActivateOrderLogic) updateSubscriptionForRenewalTx(ctx context.Context, store repository.Store, userSub *user.Subscribe, sub *subscribe.Subscribe, orderInfo *order.Order) error {
	now := timeutil.Now()
	if userSub.ExpireTime.Before(now) {
		userSub.ExpireTime = now
	}
	today := now.Day()
	resetDay := userSub.ExpireTime.Day()
	if (sub.RenewalReset != nil && *sub.RenewalReset) || today == resetDay {
		userSub.Download = 0
		userSub.Upload = 0
	}
	if userSub.FinishedAt != nil {
		if userSub.FinishedAt.Before(now) && today > resetDay {
			userSub.Download = 0
			userSub.Upload = 0
		}
		userSub.FinishedAt = nil
	}
	userSub.ExpireTime = tool.AddTime(sub.UnitTime, orderInfo.Quantity, userSub.ExpireTime)
	userSub.Status = 1
	return store.UserSubscription().UpdateSubscribe(ctx, userSub)
}

func (l *ActivateOrderLogic) activateResetTrafficTx(ctx context.Context, store repository.Store, orderInfo *order.Order) (*activationResult, error) {
	userInfo, err := store.User().FindOne(ctx, orderInfo.UserId)
	if err != nil {
		return nil, err
	}
	userSub, err := store.UserSubscription().FindOneSubscribeByTokenForUpdate(ctx, orderInfo.SubscribeToken)
	if err != nil {
		return nil, err
	}
	if userSub.UserId != orderInfo.UserId {
		return nil, fmt.Errorf("reset subscription ownership mismatch")
	}
	userSub.Download = 0
	userSub.Upload = 0
	userSub.Status = 1
	userSub.FinishedAt = nil
	if err := store.UserSubscription().UpdateSubscribe(ctx, userSub); err != nil {
		return nil, err
	}
	sub, err := store.Subscribe().FindOne(ctx, userSub.SubscribeId)
	if err != nil {
		return nil, err
	}
	resetLog := &log.ResetSubscribe{
		Type:      log.ResetSubscribeTypePaid,
		UserId:    userInfo.Id,
		OrderNo:   orderInfo.OrderNo,
		Timestamp: timeutil.Now().UnixMilli(),
	}
	content, err := resetLog.Marshal()
	if err != nil {
		return nil, err
	}
	if err := store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeResetSubscribe.Uint8(),
		Date:     timeutil.Now().Format(time.DateOnly),
		ObjectID: userSub.Id,
		Content:  string(content),
	}); err != nil {
		return nil, err
	}
	return &activationResult{order: orderInfo, user: userInfo, subscribe: sub, userSub: userSub, notifyType: telegram.ResetTrafficNotify}, nil
}

func (l *ActivateOrderLogic) activateRechargeTx(ctx context.Context, store repository.BillingStore, orderInfo *order.Order) (*activationResult, error) {
	userInfo, err := store.Wallet().FindOneForUpdate(ctx, orderInfo.UserId)
	if err != nil {
		return nil, err
	}
	userInfo.Balance += orderInfo.Price
	if err := store.Wallet().UpdateBalanceFields(ctx, userInfo); err != nil {
		return nil, err
	}
	balanceLog := &log.Balance{
		Amount:    orderInfo.Price,
		Type:      log.BalanceTypeRecharge,
		OrderNo:   orderInfo.OrderNo,
		Balance:   userInfo.Balance,
		Timestamp: timeutil.Now().UnixMilli(),
	}
	content, err := balanceLog.Marshal()
	if err != nil {
		return nil, err
	}
	if err := store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeBalance.Uint8(),
		Date:     timeutil.Now().Format(time.DateOnly),
		ObjectID: userInfo.Id,
		Content:  string(content),
	}); err != nil {
		return nil, err
	}
	return &activationResult{order: orderInfo, user: userInfo}, nil
}

func (l *ActivateOrderLogic) afterActivationCommit(ctx context.Context, result *activationResult) {
	if result == nil || result.order == nil || result.user == nil {
		return
	}
	switch result.order.Type {
	case OrderTypeSubscribe, OrderTypeRenewal, OrderTypeResetTraffic:
		if result.userSub != nil {
			if err := l.svc.Store.UserCache().ClearSubscribeCache(ctx, result.userSub); err != nil {
				logger.WithContext(ctx).Error("Clear user subscribe cache failed", logger.Field("error", err.Error()))
			}
		}
		if result.subscribe != nil {
			l.clearServerCache(ctx, result.subscribe)
		}
		if result.commissionRecipient != nil {
			if err := l.svc.Store.UserCache().UpdateUserCache(ctx, result.commissionRecipient); err != nil {
				logger.WithContext(ctx).Error("Update referer cache failed", logger.Field("error", err.Error()))
			}
		}
		if result.subscribe != nil {
			l.sendNotifications(ctx, result.order, result.user, result.subscribe, result.userSub, result.notifyType)
		}
	case OrderTypeRecharge:
		if err := l.svc.Store.UserCache().UpdateUserCache(ctx, result.user); err != nil {
			logger.WithContext(ctx).Error("[Recharge] Update user cache failed", logger.Field("error", err.Error()))
		}
		l.sendRechargeNotifications(ctx, result.order, result.user)
	}
}

// parsePayload unMarshals the task payload into a structured format
func (l *ActivateOrderLogic) parsePayload(ctx context.Context, payload []byte) (*types.ForthwithActivateOrderPayload, error) {
	var p types.ForthwithActivateOrderPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Unmarshal payload failed",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(payload)),
		)
		return nil, err
	}
	return &p, nil
}

// getTempOrderInfo retrieves temporary order information from Redis cache
func (l *ActivateOrderLogic) getTempOrderInfo(ctx context.Context, orderNo string) (*constant.TemporaryOrderInfo, error) {
	cacheKey := fmt.Sprintf(constant.TempOrderCacheKey, orderNo)
	data, err := l.svc.Redis.Get(ctx, cacheKey).Result()
	if err != nil {
		logger.WithContext(ctx).Error("Get temp order cache failed",
			logger.Field("error", err.Error()),
			logger.Field("cache_key", cacheKey),
		)
		return nil, err
	}

	var tempOrder constant.TemporaryOrderInfo
	if err = tempOrder.Unmarshal([]byte(data)); err != nil {
		logger.WithContext(ctx).Error("Unmarshal temp order cache failed",
			logger.Field("error", err.Error()),
			logger.Field("cache_key", cacheKey),
		)
		return nil, err
	}

	return &tempOrder, nil
}

func (l *ActivateOrderLogic) getGuestOrderInfo(ctx context.Context, orderInfo *order.Order) (*constant.TemporaryOrderInfo, error) {
	if orderInfo.GuestAuthType != "" && orderInfo.GuestIdentifier != "" && orderInfo.GuestPasswordHash != "" {
		return &constant.TemporaryOrderInfo{
			OrderNo:      orderInfo.OrderNo,
			Identifier:   orderInfo.GuestIdentifier,
			AuthType:     orderInfo.GuestAuthType,
			PasswordHash: orderInfo.GuestPasswordHash,
			InviteCode:   orderInfo.GuestInviteCode,
		}, nil
	}
	return l.getTempOrderInfo(ctx, orderInfo.OrderNo)
}

func (l *ActivateOrderLogic) handleCommissionTx(ctx context.Context, store repository.BillingStore, userInfo *user.User, orderInfo *order.Order) (*user.User, error) {
	if userInfo == nil || userInfo.RefererId == 0 || (orderInfo.Type != OrderTypeSubscribe && orderInfo.Type != OrderTypeRenewal) {
		return nil, nil
	}
	referer, err := store.Wallet().FindOneForUpdate(ctx, userInfo.RefererId)
	if err != nil {
		return nil, err
	}
	percentage := referer.ReferralPercentage
	if percentage != 0 {
		if referer.OnlyFirstPurchase != nil && *referer.OnlyFirstPurchase && !orderInfo.IsNew {
			return nil, nil
		}
	} else {
		if l.svc.Config.Invite.ReferralPercentage == 0 || (l.svc.Config.Invite.OnlyFirstPurchase && !orderInfo.IsNew) {
			return nil, nil
		}
		percentage = uint8(l.svc.Config.Invite.ReferralPercentage)
	}
	amount := l.calculateCommission(orderInfo.Amount-orderInfo.FeeAmount, percentage)
	if amount <= 0 {
		return nil, nil
	}
	referer.Commission += amount
	if err := store.Wallet().UpdateCommission(ctx, referer); err != nil {
		return nil, err
	}
	commissionType := log.CommissionTypePurchase
	if orderInfo.Type == OrderTypeRenewal {
		commissionType = log.CommissionTypeRenewal
	}
	content, err := (&log.Commission{
		Type:      commissionType,
		Amount:    amount,
		OrderNo:   orderInfo.OrderNo,
		Timestamp: orderInfo.CreatedAt.UnixMilli(),
	}).Marshal()
	if err != nil {
		return nil, err
	}
	if err := store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeCommission.Uint8(),
		Date:     timeutil.Now().Format(time.DateOnly),
		ObjectID: referer.Id,
		Content:  string(content),
	}); err != nil {
		return nil, err
	}
	return referer, nil
}

// calculateCommission computes the commission amount based on order price and referral percentage
func (l *ActivateOrderLogic) calculateCommission(price int64, percentage uint8) int64 {
	return int64(float64(price) * (float64(percentage) / 100))
}

// clearServerCache clears user list cache for all servers associated with the subscription
func (l *ActivateOrderLogic) clearServerCache(ctx context.Context, sub *subscribe.Subscribe) {
	if err := l.svc.Store.Subscribe().ClearCache(ctx, sub.Id); err != nil {
		logger.WithContext(ctx).Error("[Order Queue] Clear subscribe cache failed", logger.Field("error", err.Error()))
	}
}

// sendNotifications sends both user and admin notifications for order completion
func (l *ActivateOrderLogic) sendNotifications(ctx context.Context, orderInfo *order.Order, userInfo *user.User, sub *subscribe.Subscribe, userSub *user.Subscribe, notifyType string) {
	// Send user notification
	if telegramId, ok := findTelegram(userInfo); ok {
		templateData := l.buildUserNotificationData(orderInfo, sub, userSub)
		if text, err := tool.RenderTemplateToString(notifyType, templateData); err == nil {
			l.sendUserNotifyWithTelegram(telegramId, text)
		}
	}

	// Send admin notification
	adminData := l.buildAdminNotificationData(orderInfo, sub)
	if text, err := tool.RenderTemplateToString(telegram.AdminOrderNotify, adminData); err == nil {
		l.sendAdminNotifyWithTelegram(ctx, text)
	}
}

// sendRechargeNotifications sends specific notifications for balance recharge orders
func (l *ActivateOrderLogic) sendRechargeNotifications(ctx context.Context, orderInfo *order.Order, userInfo *user.User) {
	// Send user notification
	if telegramId, ok := findTelegram(userInfo); ok {
		templateData := map[string]string{
			"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
			"PaymentMethod": orderInfo.Method,
			"Time":          orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
			"Balance":       fmt.Sprintf("%.2f", float64(userInfo.Balance)/100),
		}
		if text, err := tool.RenderTemplateToString(telegram.RechargeNotify, templateData); err == nil {
			l.sendUserNotifyWithTelegram(telegramId, text)
		}
	}

	// Send admin notification
	adminData := map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"TradeNo":       orderInfo.TradeNo,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"SubscribeName": "余额充值",
		"OrderStatus":   "已支付",
		"OrderTime":     orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaymentMethod": orderInfo.Method,
	}
	if text, err := tool.RenderTemplateToString(telegram.AdminOrderNotify, adminData); err == nil {
		l.sendAdminNotifyWithTelegram(ctx, text)
	}
}

// buildUserNotificationData creates template data for user notifications
func (l *ActivateOrderLogic) buildUserNotificationData(orderInfo *order.Order, sub *subscribe.Subscribe, userSub *user.Subscribe) map[string]string {
	data := map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"SubscribeName": sub.Name,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
	}

	if userSub != nil {
		data["ExpireTime"] = userSub.ExpireTime.Format("2006-01-02 15:04:05")
		data["ResetTime"] = timeutil.Now().Format("2006-01-02 15:04:05")
	}

	return data
}

// buildAdminNotificationData creates template data for admin notifications
func (l *ActivateOrderLogic) buildAdminNotificationData(orderInfo *order.Order, sub *subscribe.Subscribe) map[string]string {
	subscribeName := sub.Name
	if orderInfo.Type == OrderTypeResetTraffic {
		subscribeName = "流量重置"
	}

	return map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"TradeNo":       orderInfo.TradeNo,
		"SubscribeName": subscribeName,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"OrderStatus":   "已支付",
		"OrderTime":     orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaymentMethod": orderInfo.Method,
	}
}

// sendUserNotifyWithTelegram sends a notification message to a user via Telegram
func (l *ActivateOrderLogic) sendUserNotifyWithTelegram(chatId int64, text string) {
	if !l.svc.Config.Telegram.EnableNotify {
		return
	}
	msg := tgbotapi.NewMessage(chatId, text)
	msg.ParseMode = "markdown"
	if _, err := l.svc.TelegramBot.Send(msg); err != nil {
		logger.Error("Send telegram user message failed", logger.Field("error", err.Error()))
	}
}

// sendAdminNotifyWithTelegram sends a notification message to all admin users via Telegram
func (l *ActivateOrderLogic) sendAdminNotifyWithTelegram(ctx context.Context, text string) {
	if !l.svc.Config.Telegram.EnableNotify {
		return
	}
	admins, err := l.svc.Store.User().QueryAdminUsers(ctx)
	if err != nil {
		logger.WithContext(ctx).Error("Query admin users failed", logger.Field("error", err.Error()))
		return
	}

	for _, admin := range admins {
		if telegramId, ok := findTelegram(admin); ok {
			msg := tgbotapi.NewMessage(telegramId, text)
			msg.ParseMode = "markdown"
			if _, err := l.svc.TelegramBot.Send(msg); err != nil {
				logger.WithContext(ctx).Error("Send telegram admin message failed", logger.Field("error", err.Error()))
			}
		}
	}
}

// findTelegram extracts Telegram chat ID from user authentication methods.
// Returns the chat ID and a boolean indicating if Telegram auth was found.
func findTelegram(u *user.User) (int64, bool) {
	for _, item := range u.AuthMethods {
		if item.AuthType == "telegram" {
			if telegramId, err := strconv.ParseInt(item.AuthIdentifier, 10, 64); err == nil {
				return telegramId, true
			}
		}
	}
	return 0, false
}
