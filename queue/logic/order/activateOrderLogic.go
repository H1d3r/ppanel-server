// Package order hosts the queue shell of the paid-order activation saga.
// The domain stages live behind the module facades — identity guest-account
// glue and notification dispatch stay here as orchestration concerns
// (ADR-001 step 6 preparation):
//
//	guest account (identity tx + order bind) -> subscription.FulfillPaidOrder
//	-> billing.SettleOrderCommission -> billing.FinalizeOrder -> notify
package orderLogic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/uuidx"
	types "github.com/perfect-panel/server/queue/types"
)

// Order type and status constants for the shell's dispatch decisions.
const (
	OrderTypeSubscribe    = 1
	OrderTypeRenewal      = 2
	OrderTypeResetTraffic = 3
	OrderTypeRecharge     = 4

	OrderStatusPending  = 1
	OrderStatusPaid     = 2
	OrderStatusClose    = 3
	OrderStatusFailed   = 4
	OrderStatusFinished = 5

	inboxGuestAccount = "identity.guest_account"
)

var (
	ErrInvalidOrderStatus = fmt.Errorf("invalid order status")
	ErrInvalidOrderType   = fmt.Errorf("invalid order type")
)

// ActivateOrderLogic sequences the activation saga stages.
type ActivateOrderLogic struct {
	svc *svc.ServiceContext
}

func NewActivateOrderLogic(svc *svc.ServiceContext) *ActivateOrderLogic {
	return &ActivateOrderLogic{svc: svc}
}

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

	if orderInfo.Type == OrderTypeRecharge {
		balance, err := l.svc.Billing.ActivateRecharge(ctx, orderInfo.OrderNo)
		if err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Recharge stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
		// Load the notification context BEFORE the finalize CAS: once the
		// order is Finished a retry short-circuits, so failing here (all
		// prior stages are idempotent) keeps the notice at-least-once.
		userInfo, err := l.svc.Store.User().FindOne(ctx, orderInfo.UserId)
		if err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Load user for recharge notify failed", logger.Field("error", err.Error()))
			return err
		}
		if err := l.svc.Billing.FinalizeOrder(ctx, orderInfo.OrderNo); err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Finalize stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
		l.sendRechargeNotifications(ctx, orderInfo, userInfo, balance)
		return nil
	}

	outcome, err := l.svc.Subscription.FulfillPaidOrder(ctx, orderInfo.OrderNo)
	if err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Fulfillment stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
		return err
	}

	if orderInfo.Type == OrderTypeSubscribe || orderInfo.Type == OrderTypeRenewal {
		if err := l.svc.Billing.SettleOrderCommission(ctx, orderInfo.OrderNo, outcome.UserID); err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Commission stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
	}

	// Load the notification context BEFORE the finalize CAS (see the
	// recharge branch above for why).
	userInfo, err := l.svc.Store.User().FindOne(ctx, orderInfo.UserId)
	if err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Load user for notify failed", logger.Field("error", err.Error()))
		return err
	}

	if err := l.svc.Billing.FinalizeOrder(ctx, orderInfo.OrderNo); err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Finalize stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
		return err
	}

	l.notifyFulfillment(ctx, orderInfo, userInfo, outcome)
	return nil
}

// notifyFulfillment dispatches the post-activation notices using the
// fulfillment outcome's notification context.
func (l *ActivateOrderLogic) notifyFulfillment(ctx context.Context, orderInfo *order.Order, userInfo *user.User, outcome *subscription.FulfillmentOutcome) {
	if outcome == nil {
		return
	}
	notifyType := ""
	switch outcome.NotifyKind {
	case subscription.NotifyKindPurchase:
		notifyType = notification.PurchaseNotify
	case subscription.NotifyKindRenewal:
		notifyType = notification.RenewalNotify
	case subscription.NotifyKindResetTraffic:
		notifyType = notification.ResetTrafficNotify
	default:
		return
	}
	l.sendNotifications(ctx, orderInfo, userInfo, outcome, notifyType)
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

// sendNotifications sends both user and admin notifications for order completion
func (l *ActivateOrderLogic) sendNotifications(ctx context.Context, orderInfo *order.Order, userInfo *user.User, outcome *subscription.FulfillmentOutcome, notifyType string) {
	// Send user notification
	templateData := l.buildUserNotificationData(orderInfo, outcome)
	if text, err := notification.RenderTelegramMarkdown(notifyType, templateData); err == nil {
		l.sendUserNotifyWithTelegram(ctx, userInfo.Id, text)
	}

	// Send admin notification
	adminData := l.buildAdminNotificationData(orderInfo, userInfo, outcome)
	if text, err := notification.RenderTelegramMarkdown(notification.AdminOrderNotify, adminData); err == nil {
		l.sendAdminNotifyWithTelegram(ctx, text)
	}
}

// sendRechargeNotifications sends specific notifications for balance recharge orders
func (l *ActivateOrderLogic) sendRechargeNotifications(ctx context.Context, orderInfo *order.Order, userInfo *user.User, balance int64) {
	// Send user notification
	templateData := map[string]string{
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"PaymentMethod": orderInfo.Method,
		"Time":          orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"Balance":       fmt.Sprintf("%.2f", float64(balance)/100),
	}
	if text, err := notification.RenderTelegramMarkdown(notification.RechargeNotify, templateData); err == nil {
		l.sendUserNotifyWithTelegram(ctx, userInfo.Id, text)
	}

	// Send admin notification
	adminData := map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"TradeNo":       orderInfo.TradeNo,
		"UserEmail":     findEmail(userInfo),
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"SubscribeName": "余额充值",
		"OrderStatus":   "已支付",
		"OrderTime":     orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaymentMethod": orderInfo.Method,
	}
	if text, err := notification.RenderTelegramMarkdown(notification.AdminOrderNotify, adminData); err == nil {
		l.sendAdminNotifyWithTelegram(ctx, text)
	}
}

// buildUserNotificationData creates template data for user notifications
func (l *ActivateOrderLogic) buildUserNotificationData(orderInfo *order.Order, outcome *subscription.FulfillmentOutcome) map[string]string {
	data := map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"SubscribeName": outcome.PlanName,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
	}

	if outcome.HasSub {
		data["ExpireTime"] = outcome.ExpireAt.Format("2006-01-02 15:04:05")
		data["ResetTime"] = timeutil.Now().Format("2006-01-02 15:04:05")
	}

	return data
}

// buildAdminNotificationData creates template data for admin notifications
func (l *ActivateOrderLogic) buildAdminNotificationData(orderInfo *order.Order, userInfo *user.User, outcome *subscription.FulfillmentOutcome) map[string]string {
	subscribeName := outcome.PlanName
	if orderInfo.Type == OrderTypeResetTraffic {
		subscribeName = "流量重置"
	}

	return map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"TradeNo":       orderInfo.TradeNo,
		"UserEmail":     findEmail(userInfo),
		"SubscribeName": subscribeName,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"OrderStatus":   "已支付",
		"OrderTime":     orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaymentMethod": orderInfo.Method,
	}
}

// sendUserNotifyWithTelegram delivers rendered MarkdownV2 to the buyer's
// bound Telegram; "no binding" and "no bot" both just mean nothing to send.
func (l *ActivateOrderLogic) sendUserNotifyWithTelegram(ctx context.Context, userID int64, text string) {
	if !l.svc.Config.Telegram.EnableNotify {
		return
	}
	if err := l.svc.Notification.NotifyTelegramUser(ctx, userID, text); err != nil {
		logger.WithContext(ctx).Info("Telegram user notice skipped",
			logger.Field("reason", err.Error()), logger.Field("user_id", userID))
	}
}

// sendAdminNotifyWithTelegram posts into the admin group's notification
// topic - the group is the only administrator channel, so an unconfigured
// group means the notice is skipped.
func (l *ActivateOrderLogic) sendAdminNotifyWithTelegram(ctx context.Context, text string) {
	if !l.svc.Config.Telegram.EnableNotify {
		return
	}
	if err := l.svc.Notification.NotifyAdminsTelegram(ctx, text); err != nil {
		logger.WithContext(ctx).Info("Telegram admin notice skipped", logger.Field("reason", err.Error()))
	}
}

// findEmail returns the user's email auth identifier, falling back to the
// numeric id so the admin notification always names the buyer.
func findEmail(u *user.User) string {
	for _, item := range u.AuthMethods {
		if item.AuthType == "email" {
			return item.AuthIdentifier
		}
	}
	return fmt.Sprintf("ID:%d", u.Id)
}
