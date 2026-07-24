// Package quotatask processes the admin-scheduled quota grants: extending
// subscription time and crediting gift money for a scope of subscriptions,
// atomically with the task bookkeeping. Only the module facade may reach it.
package quotatask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/model/entity/log"
	"github.com/perfect-panel/server/internal/model/entity/task"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
)

const (
	UnitTimeNoLimit = "NoLimit" // Unlimited time subscription
	UnitTimeYear    = "Year"    // Annual subscription
	UnitTimeMonth   = "Month"   // Monthly subscription
	UnitTimeDay     = "Day"     // Daily subscription
	UnitTimeHour    = "Hour"    // Hourly subscription
	UnitTimeMinute  = "Minute"  // Per-minute subscription

)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	// Store carries the deliberate cross-domain transaction (subscription
	// state + billing gift + task bookkeeping) and the post-commit cache
	// invalidation.
	Store repository.Store
}

type QuotaTaskLogic struct {
	deps Deps
}

type ErrorInfo struct {
	UserSubscribeId int64  `json:"user_subscribe_id"`
	Error           string `json:"error"`
}

// ErrUnretryable marks failures a retry cannot fix (missing or malformed
// task data); the queue shell maps it to its skip-retry sentinel.
var ErrUnretryable = errors.New("quota task is not retryable")

func newQuotaTaskLogic(deps Deps) *QuotaTaskLogic {
	return &QuotaTaskLogic{deps: deps}
}

// Service is the quota-task entry point used by the subscription facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) ProcessQuotaTask(ctx context.Context, taskID int64) error {
	return newQuotaTaskLogic(s.deps).process(ctx, taskID)
}

func (l *QuotaTaskLogic) process(ctx context.Context, taskID int64) error {
	taskInfo, err := l.getTaskInfo(ctx, taskID)
	if err != nil {
		return err
	}

	if taskInfo.Status != 0 {
		logger.WithContext(ctx).Info("[QuotaTaskLogic.ProcessTask] task already processed",
			logger.Field("taskID", taskID),
			logger.Field("status", taskInfo.Status),
		)
		return nil
	}

	scope, content, err := l.parseTaskData(ctx, taskInfo)
	if err != nil {
		return err
	}

	subscribes, err := l.getSubscribes(ctx, scope.Objects)
	if err != nil {
		return err
	}
	if err = l.processSubscribes(ctx, subscribes, content, taskInfo); err != nil {
		return err
	}
	// 清理用户缓存（仅在有赠送金时清理）
	if content.GiftValue != 0 {
		var userIds []int64
		for _, sub := range subscribes {
			userIds = append(userIds, sub.UserId)
		}
		userIds = tool.RemoveDuplicateElements(userIds...)
		users, err := l.deps.Store.User().FindUsersByIds(ctx, userIds)
		if err != nil {
			logger.WithContext(ctx).Error("[QuotaTaskLogic.ProcessTask] find users error",
				logger.Field("error", err.Error()),
				logger.Field("userIDs", userIds))
		}
		err = l.deps.Store.UserCache().ClearUserCache(ctx, users...)
		if err != nil {
			logger.WithContext(ctx).Error("[QuotaTaskLogic.ProcessTask] clear user cache error",
				logger.Field("error", err.Error()),
				logger.Field("userIDs", userIds))
		}
	}

	// 清理用户订阅缓存
	err = l.deps.Store.UserCache().ClearSubscribeCache(ctx, subscribes...)
	if err != nil {
		logger.WithContext(ctx).Error("[QuotaTaskLogic.ProcessTask] clear subscribe cache error",
			logger.Field("error", err.Error()))
	}

	return nil
}

func (l *QuotaTaskLogic) getTaskInfo(ctx context.Context, taskID int64) (*task.Task, error) {
	taskInfo, err := l.deps.Store.Task().FindOne(ctx, taskID)
	if err != nil {
		logger.WithContext(ctx).Error("[QuotaTaskLogic.getTaskInfo] find task error",
			logger.Field("error", err.Error()),
			logger.Field("taskID", taskID),
		)
		return nil, ErrUnretryable
	}
	return taskInfo, nil
}

func (l *QuotaTaskLogic) parseTaskData(ctx context.Context, taskInfo *task.Task) (task.QuotaScope, task.QuotaContent, error) {
	var scope task.QuotaScope
	if err := scope.Unmarshal([]byte(taskInfo.Scope)); err != nil {
		logger.WithContext(ctx).Error("[QuotaTaskLogic.parseTaskData] unmarshal scope error",
			logger.Field("error", err.Error()),
		)
		return scope, task.QuotaContent{}, ErrUnretryable
	}

	var content task.QuotaContent
	if err := content.Unmarshal([]byte(taskInfo.Content)); err != nil {
		logger.WithContext(ctx).Error("[QuotaTaskLogic.parseTaskData] unmarshal content error",
			logger.Field("error", err.Error()),
		)
		return scope, content, ErrUnretryable
	}
	return scope, content, nil
}

func (l *QuotaTaskLogic) getSubscribes(ctx context.Context, subscriberIDs []int64) ([]*user.Subscribe, error) {
	subscribes, err := l.deps.Store.UserSubscription().FindSubscribesByIds(ctx, subscriberIDs)
	if err != nil {
		logger.WithContext(ctx).Error("[QuotaTaskLogic.getSubscribes] find subscribes error",
			logger.Field("error", err.Error()),
			logger.Field("subscribers", subscriberIDs),
		)
		return nil, ErrUnretryable
	}
	return subscribes, nil
}

func (l *QuotaTaskLogic) processSubscribes(ctx context.Context, subscribes []*user.Subscribe, content task.QuotaContent, taskInfo *task.Task) error {
	// Deliberate cross-domain transaction: the scheduled gift grant reads
	// subscription state and moves billing money atomically with its task
	// bookkeeping; it splits into an event-driven flow when the domains
	// become services.
	return l.deps.Store.InTx(ctx, func(store repository.Store) error {
		var errors []ErrorInfo
		now := timeutil.Now()

		for _, sub := range subscribes {
			if err := l.processSubscription(ctx, store, sub, content, now, &errors); err != nil {
				return err
			}
		}

		// 根据错误情况决定任务状态
		status := int8(2) // Completed
		if len(errors) > 0 {
			logger.WithContext(ctx).Error("[QuotaTaskLogic.processSubscribes] some subscriptions failed",
				logger.Field("total", len(subscribes)),
				logger.Field("failed", len(errors)),
			)
			// 如果所有订阅都失败，标记为失败状态
			if len(errors) == len(subscribes) {
				status = 3 // Failed
			}
			errs, err := json.Marshal(errors)
			if err != nil {
				logger.WithContext(ctx).Error("[QuotaTaskLogic.processSubscribes] marshal errors failed",
					logger.Field("error", err.Error()),
				)
				return err
			}
			taskInfo.Errors = string(errs)
		}

		taskInfo.Current = uint64(len(subscribes))
		taskInfo.Status = status
		if err := store.Task().Update(ctx, taskInfo); err != nil {
			logger.WithContext(ctx).Error("[QuotaTaskLogic.processSubscribes] update task status error",
				logger.Field("error", err.Error()),
				logger.Field("taskID", taskInfo.Id),
			)
			return err
		}

		return nil
	})
}

func (l *QuotaTaskLogic) processSubscription(ctx context.Context, store repository.Store, sub *user.Subscribe, content task.QuotaContent, now time.Time, errors *[]ErrorInfo) error {
	// 验证订阅数据
	if sub == nil {
		*errors = append(*errors, ErrorInfo{
			UserSubscribeId: 0,
			Error:           "subscription is nil",
		})
		return nil
	}

	updated := false

	// 处理时间延长 - 修复逻辑：只要Days不为0就处理，不管ExpireTime是否为0
	if content.Days != 0 {
		if sub.ExpireTime.Unix() == 0 || sub.ExpireTime.Before(now) {
			// 如果没有过期时间或已过期，从现在开始计算
			sub.ExpireTime = now.AddDate(0, 0, int(content.Days))
		} else {
			// 在原有过期时间基础上延长
			sub.ExpireTime = sub.ExpireTime.AddDate(0, 0, int(content.Days))
		}
		// 如果订阅延长到未来时间，设置为激活状态
		if sub.ExpireTime.After(now) && sub.Status != 1 {
			sub.Status = 1 // Active
		}
		updated = true
	}

	// 处理流量重置
	if content.ResetTraffic {
		sub.Download = 0
		sub.Upload = 0
		updated = true
		if err := l.createResetTrafficLog(ctx, store, sub.Id, sub.UserId, now); err != nil {
			// 记录错误但不阻断整个任务,日志失败不影响主流程
			*errors = append(*errors, ErrorInfo{
				UserSubscribeId: sub.Id,
				Error:           "create reset traffic log error: " + err.Error(),
			})
		}
	}

	// 处理赠送金
	if content.GiftValue != 0 {
		if err := l.processGift(ctx, store, sub, content, now, errors); err != nil {
			return err
		}
	}

	// 只有在有更新时才保存订阅信息
	if updated {
		if err := store.UserSubscription().UpdateSubscribe(ctx, sub); err != nil {
			*errors = append(*errors, ErrorInfo{
				UserSubscribeId: sub.Id,
				Error:           "update subscription error: " + err.Error(),
			})
			return nil
		}
	}

	return nil
}

func (l *QuotaTaskLogic) processGift(ctx context.Context, store repository.Store, sub *user.Subscribe, content task.QuotaContent, now time.Time, errors *[]ErrorInfo) error {
	// 验证赠送类型
	if content.GiftType != 1 && content.GiftType != 2 {
		*errors = append(*errors, ErrorInfo{
			UserSubscribeId: sub.Id,
			Error:           fmt.Sprintf("invalid gift type: %d", content.GiftType),
		})
		return nil
	}

	userInfo, err := store.Wallet().FindOneForUpdate(ctx, sub.UserId)
	if err != nil {
		*errors = append(*errors, ErrorInfo{
			UserSubscribeId: sub.Id,
			Error:           "find user error: " + err.Error(),
		})
		return nil
	}

	var giftAmount int64
	switch content.GiftType {
	case 1:
		giftAmount = int64(content.GiftValue)
	case 2:
		// 获取订阅对应的套餐信息
		subscribeInfo, err := store.Subscribe().FindOne(ctx, sub.SubscribeId)
		if err != nil {
			*errors = append(*errors, ErrorInfo{
				UserSubscribeId: sub.Id,
				Error:           "find subscribe error: " + err.Error(),
			})
			return nil
		}
		if subscribeInfo.UnitPrice > 0 {
			giftAmount = int64(float64(subscribeInfo.UnitPrice) * (float64(content.GiftValue) / 100))
		}
	}

	if giftAmount > 0 {
		userInfo.GiftAmount += giftAmount
		if err := store.Wallet().UpdateBalanceFields(ctx, userInfo); err != nil {
			return fmt.Errorf("update user gift amount: %w", err)
		}

		if err := l.createGiftLog(ctx, store, sub.Id, userInfo.UserId, giftAmount, userInfo.GiftAmount, now); err != nil {
			return fmt.Errorf("create gift log: %w", err)
		}
	}

	return nil
}

func (l *QuotaTaskLogic) getStartTime(sub *user.Subscribe, now time.Time) time.Time {
	if sub.StartTime.Unix() == 0 {
		return now
	}
	return sub.StartTime
}

func (l *QuotaTaskLogic) createGiftLog(ctx context.Context, store repository.Store, subscribeId, userId, amount, balance int64, now time.Time) error {
	giftLog := &log.Gift{
		Type:        log.GiftTypeIncrease,
		OrderNo:     "",
		SubscribeId: subscribeId,
		Amount:      amount,
		Balance:     balance,
		Remark:      "Quota task gift",
		Timestamp:   now.UnixMilli(),
	}

	logString, err := giftLog.Marshal()
	if err != nil {
		return fmt.Errorf("marshal gift log error: %v", err)
	}
	return store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeGift.Uint8(),
		Content:  string(logString),
		ObjectID: userId,
		Date:     now.Format(time.DateOnly),
	})
}

func (l *QuotaTaskLogic) createResetTrafficLog(ctx context.Context, store repository.Store, subscribeId, userId int64, now time.Time) error {
	trafficLog := &log.ResetSubscribe{
		Type:      log.ResetSubscribeTypeQuota,
		UserId:    userId,
		OrderNo:   "",
		Timestamp: now.UnixMilli(),
	}

	logString, err := trafficLog.Marshal()
	if err != nil {
		return fmt.Errorf("marshal traffic log error: %v", err)
	}
	return store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeResetSubscribe.Uint8(),
		Content:  string(logString),
		ObjectID: subscribeId,
		Date:     now.Format(time.DateOnly),
	})
}
