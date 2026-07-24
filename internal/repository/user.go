package repository

import (
	"context"
	"time"

	trafficEntity "github.com/perfect-panel/server/internal/model/entity/traffic"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	cacheUserIdPrefix             = "cache:user:id:"
	cacheUserEmailPrefix          = "cache:user:email:v2:"
	cacheUserSubscribeTokenPrefix = "cache:user:subscribe:token:"
	// v3 stores the complete, status-unfiltered subscription history. Status-specific
	// callers filter this shared value in memory so cache entries cannot collide.
	cacheUserSubscribeUserPrefix = "cache:user:subscribe:user:v3:"
	cacheUserSubscribeIdPrefix   = "cache:user:subscribe:id:"
	cacheUserDeviceNumberPrefix  = "cache:user:device:number:"
	cacheUserDeviceIdPrefix      = "cache:user:device:id:"
)

// UserRepo provides user profile, account, reporting, and marketing queries.
// Related authentication, subscription, device, cache, withdrawal, and traffic
// operations live behind their own focused repository interfaces below.
type UserRepo interface {
	Insert(ctx context.Context, data *user.User, tx ...*gorm.DB) error
	FindOne(ctx context.Context, id int64) (*user.User, error)
	FindOneForUpdate(ctx context.Context, id int64) (*user.User, error)
	FindOneByEmail(ctx context.Context, email string) (*user.User, error)
	FindOneByReferCode(ctx context.Context, referCode string) (*user.User, error)
	Update(ctx context.Context, data *user.User, tx ...*gorm.DB) error
	UpgradePasswordHash(ctx context.Context, id int64, currentHash, password, algo, salt string) (bool, error)
	UpdateBalanceFields(ctx context.Context, data *user.User, tx ...*gorm.DB) error
	UpdateCommission(ctx context.Context, data *user.User, tx ...*gorm.DB) error
	Delete(ctx context.Context, id int64, tx ...*gorm.DB) error
	BatchDeleteUser(ctx context.Context, ids []int64, tx ...*gorm.DB) error
	QueryPageList(ctx context.Context, page, size int, filter *user.UserFilterParams) ([]*user.User, int64, error)
	FindUsersByIds(ctx context.Context, ids []int64) ([]*user.User, error)
	CountAffiliates(ctx context.Context, refererId int64) (int64, error)
	QueryAffiliateList(ctx context.Context, refererId int64, page, size int) ([]*user.User, int64, error)
	QueryAdminUsers(ctx context.Context) ([]*user.User, error)
	CountEnabledUsers(ctx context.Context) (int64, error)
	QueryResisterUserTotal(ctx context.Context) (int64, error)
	QueryResisterUserTotalByDate(ctx context.Context, date time.Time) (int64, error)
	QueryResisterUserTotalByMonthly(ctx context.Context, date time.Time) (int64, error)
	QueryEmailRecipients(ctx context.Context, filter *user.EmailRecipientFilter) ([]string, error)
	CountEmailRecipients(ctx context.Context, filter *user.EmailRecipientFilter) (int64, error)
	QueryDailyUserStatisticsList(ctx context.Context, date time.Time) ([]user.UserStatisticsWithDate, error)
	QueryMonthlyUserStatisticsList(ctx context.Context, date time.Time) ([]user.UserStatisticsWithDate, error)
}

// UserAuthRepo manages external authentication identities linked to users.
type UserAuthRepo interface {
	FindUserAuthMethods(ctx context.Context, userId int64) ([]*user.AuthMethods, error)
	FindUserAuthMethodByOpenID(ctx context.Context, method, openID string) (*user.AuthMethods, error)
	ValidateEmailIdentityUniqueness(ctx context.Context) error
	FindUserAuthMethodByPlatform(ctx context.Context, userId int64, platform string) (*user.AuthMethods, error)
	FindUserAuthMethodByUserId(ctx context.Context, method string, userId int64) (*user.AuthMethods, error)
	InsertUserAuthMethods(ctx context.Context, data *user.AuthMethods, tx ...*gorm.DB) error
	UpdateUserAuthMethods(ctx context.Context, data *user.AuthMethods, tx ...*gorm.DB) error
	DeleteUserAuthMethods(ctx context.Context, userId int64, platform string, tx ...*gorm.DB) error
	UpdateUserAuthMethodOwner(ctx context.Context, authType, identifier string, userId int64, tx ...*gorm.DB) error
	DeleteUserAuthMethodByIdentifier(ctx context.Context, authType, identifier string, tx ...*gorm.DB) error
	UpsertUserAuthMethod(ctx context.Context, data *user.AuthMethods) error
}

// UserSubscriptionRepo manages user subscription records and their lifecycle.
type UserSubscriptionRepo interface {
	InsertSubscribe(ctx context.Context, data *user.Subscribe, tx ...*gorm.DB) error
	FindOneSubscribe(ctx context.Context, id int64) (*user.Subscribe, error)
	FindOneSubscribeForUpdate(ctx context.Context, id int64) (*user.Subscribe, error)
	FindOneSubscribeByOrderId(ctx context.Context, orderId int64) (*user.Subscribe, error)
	FindOneSubscribeByToken(ctx context.Context, token string) (*user.Subscribe, error)
	FindOneSubscribeByTokenForUpdate(ctx context.Context, token string) (*user.Subscribe, error)
	UpdateSubscribe(ctx context.Context, data *user.Subscribe, tx ...*gorm.DB) error
	DeleteSubscribe(ctx context.Context, token string, tx ...*gorm.DB) error
	DeleteSubscribeById(ctx context.Context, id int64, tx ...*gorm.DB) error
	UpdateUserSubscribeWithTraffic(ctx context.Context, id, download, upload int64, tx ...*gorm.DB) error
	BatchUpdateUserSubscribeWithTraffic(ctx context.Context, deltas []trafficEntity.SubscribeTrafficDelta, tx ...*gorm.DB) error
	FindUsersSubscribeBySubscribeId(ctx context.Context, subscribeId int64) ([]*user.Subscribe, error)
	FindUserSubscribesByStatus(ctx context.Context, status ...int64) ([]*user.Subscribe, error)
	FindSubscribesByIds(ctx context.Context, ids []int64) ([]*user.Subscribe, error)
	ActivatePendingSubscribesBySubscribeId(ctx context.Context, subscribeId int64) error
	CountQuotaConsumingSubscriptions(ctx context.Context, userId, subscribeId int64) (int64, error)
	HasBlockingSubscription(ctx context.Context, userId int64) (bool, error)
	CountUserSubscribesBySubscribeIdAndStatus(ctx context.Context, subscribeId int64, status ...int64) (int64, error)
	QueryActiveSubscriptions(ctx context.Context, subscribeId ...int64) (map[int64]int64, error)
	QueryUserSubscribe(ctx context.Context, userId int64, status ...int64) ([]*user.SubscribeDetails, error)
	FindOneSubscribeDetailsById(ctx context.Context, id int64) (*user.SubscribeDetails, error)
	FindOneUserSubscribe(ctx context.Context, id int64) (*user.SubscribeDetails, error)
	FindTrafficExceededSubscribes(ctx context.Context) ([]*user.Subscribe, error)
	FindExpiredSubscribes(ctx context.Context, now time.Time) ([]*user.Subscribe, error)
	MarkSubscribesFinished(ctx context.Context, ids []int64, status uint8, finishedAt time.Time, tx ...*gorm.DB) error
	QuerySubscribeIdsByFilter(ctx context.Context, filter *user.SubscribeFilter) ([]int64, error)
	CountSubscribesByFilter(ctx context.Context, filter *user.SubscribeFilter) (int64, error)
}

// UserDeviceRepo manages registered devices and their online records.
type UserDeviceRepo interface {
	InsertDevice(ctx context.Context, data *user.Device, tx ...*gorm.DB) error
	FindOneDevice(ctx context.Context, id int64) (*user.Device, error)
	FindOneDeviceByIdentifier(ctx context.Context, id string) (*user.Device, error)
	UpdateDevice(ctx context.Context, data *user.Device, tx ...*gorm.DB) error
	DeleteDevice(ctx context.Context, id int64, tx ...*gorm.DB) error
	QueryDeviceList(ctx context.Context, userid int64) ([]*user.Device, int64, error)
	QueryDevicePageList(ctx context.Context, userid, subscribeId int64, page, size int) ([]*user.Device, int64, error)
	FindDeviceOnlineRecord(ctx context.Context, userId int64, startTime, endTime string) (*user.DeviceOnlineRecord, error)
	InsertDeviceOnlineRecord(ctx context.Context, data *user.DeviceOnlineRecord, tx ...*gorm.DB) error
}

// UserWithdrawalRepo manages affiliate withdrawal records.
type UserWithdrawalRepo interface {
	InsertWithdrawal(ctx context.Context, data *user.Withdrawal, tx ...*gorm.DB) error
}

// SubscriptionTrafficRepo manages scheduled subscription traffic resets.
type SubscriptionTrafficRepo interface {
	QueryMonthlyResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error)
	QueryFirstResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error)
	QueryYearlyResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error)
	ResetSubscribeTrafficByIds(ctx context.Context, ids []int64, tx ...*gorm.DB) error
}

// UserCacheRepo manages cached user-related projections.
type UserCacheRepo interface {
	ClearUserCache(ctx context.Context, data ...*user.User) error
	ClearSubscribeCache(ctx context.Context, data ...*user.Subscribe) error
	ClearDeviceCache(ctx context.Context, data ...*user.Device) error
	ClearAuthMethodCache(ctx context.Context, data ...*user.AuthMethods) error
	BatchClearRelatedCache(ctx context.Context, data *user.User) error
	UpdateUserCache(ctx context.Context, data *user.User) error
	UpdateUserSubscribeCache(ctx context.Context, data *user.Subscribe) error
}

var _ UserRepo = (*userRepo)(nil)
var _ UserAuthRepo = (*userRepo)(nil)
var _ UserSubscriptionRepo = (*userRepo)(nil)
var _ UserDeviceRepo = (*userRepo)(nil)
var _ UserWithdrawalRepo = (*userRepo)(nil)
var _ SubscriptionTrafficRepo = (*userRepo)(nil)
var _ UserCacheRepo = (*userRepo)(nil)

type userRepo struct {
	cache.CachedConn
	table string
}

func newUserRepo(db *gorm.DB, c *redis.Client, invalidations ...*cache.InvalidationQueue) *userRepo {
	return &userRepo{
		CachedConn: newCachedConn(db, c, invalidations...),
		table:      "user",
	}
}

// Code organization (ADR-001 step 5): the shared *userRepo implementation is
// being split along domain seams. Each user_*.go file holds one domain's
// methods; the struct itself splits once the cross-domain cascades
// (Delete/BatchDeleteUser/BatchClearRelatedCache) are untangled.

// --- internal helpers ---

func (m *userRepo) getCacheKeys(data *user.User) []string {
	if data == nil {
		return []string{}
	}
	return data.GetCacheKeys()
}

func (m *userRepo) batchGetCacheKeys(users ...*user.User) []string {
	var keys []string
	for _, u := range users {
		keys = append(keys, u.GetCacheKeys()...)
	}
	return keys
}

// --- cache helpers ---

func (m *userRepo) ClearUserCache(ctx context.Context, users ...*user.User) error {
	if len(users) == 0 {
		return nil
	}
	var keys []string
	for _, u := range users {
		if u != nil {
			keys = append(keys, u.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

func (m *userRepo) ClearDeviceCache(ctx context.Context, devices ...*user.Device) error {
	if len(devices) == 0 {
		return nil
	}
	var keys []string
	for _, d := range devices {
		if d != nil {
			keys = append(keys, d.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

func (m *userRepo) ClearAuthMethodCache(ctx context.Context, authMethods ...*user.AuthMethods) error {
	if len(authMethods) == 0 {
		return nil
	}
	var keys []string
	for _, a := range authMethods {
		if a != nil {
			keys = append(keys, a.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

func (m *userRepo) BatchClearRelatedCache(ctx context.Context, u *user.User) error {
	if u == nil {
		return nil
	}
	var allKeys []string
	allKeys = append(allKeys, u.GetCacheKeys()...)

	for _, auth := range u.AuthMethods {
		allKeys = append(allKeys, auth.GetCacheKeys()...)
	}

	for _, device := range u.UserDevices {
		allKeys = append(allKeys, device.GetCacheKeys()...)
	}

	subscribes, err := m.QueryUserSubscribe(ctx, u.Id)
	if err != nil {
		logger.Errorf("failed to query user subscribes for cache clearing: %v", err)
	} else {
		for _, sub := range subscribes {
			subModel := &user.Subscribe{
				Id:          sub.Id,
				UserId:      sub.UserId,
				Token:       sub.Token,
				SubscribeId: sub.SubscribeId,
			}
			allKeys = append(allKeys, subModel.GetCacheKeys()...)
		}
	}

	return m.CachedConn.DelCacheCtx(ctx, allKeys...)
}
