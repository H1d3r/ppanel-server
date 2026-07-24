package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	trafficEntity "github.com/perfect-panel/server/internal/model/entity/traffic"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Subscription-domain methods of the user repository: user_subscribe rows,
// traffic accounting, expiry/reset sweeps (ADR-001 step 5).

// --- subscribe ---

func (m *userSubscriptionRepo) UpdateUserSubscribeCache(ctx context.Context, data *user.Subscribe) error {
	return m.ClearSubscribeCache(ctx, data)
}

// QueryActiveSubscriptions returns the number of active subscriptions.
func (m *userSubscriptionRepo) QueryActiveSubscriptions(ctx context.Context, subscribeId ...int64) (map[int64]int64, error) {
	type SubscriptionCount struct {
		SubscribeId int64
		Total       int64
	}
	var result []SubscriptionCount
	err := m.QueryNoCacheCtx(ctx, &result, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).
			Where("subscribe_id IN ? AND status IN ?", subscribeId, []int64{1, 0}).
			Select("subscribe_id, COUNT(id) as total").
			Group("subscribe_id").
			Scan(&result).
			Error
	})

	if err != nil {
		return nil, err
	}

	resultMap := make(map[int64]int64)
	for _, item := range result {
		resultMap[item.SubscribeId] = item.Total
	}

	return resultMap, nil
}

func (m *userSubscriptionRepo) FindOneSubscribeByOrderId(ctx context.Context, orderId int64) (*user.Subscribe, error) {
	var data user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Where("order_id = ?", orderId).First(&data).Error
	})
	return &data, err
}

func (m *userSubscriptionRepo) FindOneSubscribe(ctx context.Context, id int64) (*user.Subscribe, error) {
	var data user.Subscribe
	key := fmt.Sprintf("%s%d", cacheUserSubscribeIdPrefix, id)
	err := m.QueryCtx(ctx, &data, key, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Where("id = ?", id).First(&data).Error
	})
	return &data, err
}

func (m *userSubscriptionRepo) FindOneSubscribeForUpdate(ctx context.Context, id int64) (*user.Subscribe, error) {
	var data user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&user.Subscribe{}).
			Where("id = ?", id).
			First(v).Error
	})
	return &data, err
}

func (m *userSubscriptionRepo) FindUsersSubscribeBySubscribeId(ctx context.Context, subscribeId int64) ([]*user.Subscribe, error) {
	var data []*user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Where("subscribe_id = ? AND status IN ?", subscribeId, []int64{1, 0}).Find(v).Error
	})
	return data, err
}

func (m *userSubscriptionRepo) FindUserSubscribesByStatus(ctx context.Context, status ...int64) ([]*user.Subscribe, error) {
	var data []*user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		conn = conn.Model(&user.Subscribe{})
		if len(status) > 0 {
			conn = conn.Where("status IN ?", status)
		}
		return conn.Find(v).Error
	})
	return data, err
}

func (m *userSubscriptionRepo) ActivatePendingSubscribesBySubscribeId(ctx context.Context, subscribeId int64) error {
	var pending []*user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &pending, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Where("subscribe_id = ? AND status = ?", subscribeId, 0).Find(v).Error
	})
	if err != nil || len(pending) == 0 {
		return err
	}

	cacheKeys := make([]string, 0)
	for _, sub := range pending {
		cacheKeys = append(cacheKeys, sub.GetCacheKeys()...)
	}

	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Model(&user.Subscribe{}).Where("subscribe_id = ? AND status = ?", subscribeId, 0).Update("status", 1).Error
	}, cacheKeys...)
}

func (m *userSubscriptionRepo) CountUserSubscribesBySubscribeIdAndStatus(ctx context.Context, subscribeId int64, status ...int64) (int64, error) {
	var total int64
	err := m.QueryNoCacheCtx(ctx, &total, func(conn *gorm.DB, v interface{}) error {
		conn = conn.Model(&user.Subscribe{}).Where("subscribe_id = ?", subscribeId)
		if len(status) > 0 {
			conn = conn.Where("status IN ?", status)
		}
		return conn.Count(&total).Error
	})
	return total, err
}

func (m *userSubscriptionRepo) CountQuotaConsumingSubscriptions(ctx context.Context, userId, subscribeId int64) (int64, error) {
	var total int64
	err := m.QueryNoCacheCtx(ctx, &total, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).
			Where("user_id = ? AND subscribe_id = ? AND status <> ?", userId, subscribeId, user.SubscribeStatusDeducted).
			Count(&total).Error
	})
	return total, err
}

func (m *userSubscriptionRepo) HasBlockingSubscription(ctx context.Context, userId int64) (bool, error) {
	var total int64
	err := m.QueryNoCacheCtx(ctx, &total, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).
			Where("user_id = ? AND status <> ?", userId, user.SubscribeStatusDeducted).
			Count(&total).Error
	})
	return total > 0, err
}

// QueryUserSubscribe returns the complete cached subscription history for a user.
func (m *userSubscriptionRepo) QueryUserSubscribe(ctx context.Context, userId int64, status ...int64) ([]*user.SubscribeDetails, error) {
	var all []*user.SubscribeDetails
	key := fmt.Sprintf("%s%d", cacheUserSubscribeUserPrefix, userId)
	err := m.QueryCtx(ctx, &all, key, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).
			Where("user_id = ?", userId).
			Preload("Subscribe").
			Order("CASE WHEN status = 1 THEN 0 ELSE 1 END ASC").
			Order("expire_time DESC").
			Order("id DESC").
			Find(&all).Error
	})
	if err != nil {
		return nil, err
	}
	return filterUserSubscribeByStatus(all, status), nil
}

func filterUserSubscribeByStatus(list []*user.SubscribeDetails, status []int64) []*user.SubscribeDetails {
	if len(status) == 0 {
		return list
	}

	allowed := make(map[int64]struct{}, len(status))
	for _, value := range status {
		allowed[value] = struct{}{}
	}

	filtered := make([]*user.SubscribeDetails, 0, len(list))
	for _, item := range list {
		if item == nil {
			continue
		}
		if _, ok := allowed[int64(item.Status)]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FindOneUserSubscribe  finds a subscribeDetails by id.
func (m *userSubscriptionRepo) FindOneUserSubscribe(ctx context.Context, id int64) (subscribeDetails *user.SubscribeDetails, err error) {
	//TODO cache
	//key := fmt.Sprintf("%s%d", cacheUserSubscribeUserPrefix, userId)
	subscribeDetails = new(user.SubscribeDetails)
	err = m.QueryNoCacheCtx(ctx, subscribeDetails, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Preload("Subscribe").Where("id = ?", id).First(v).Error
	})
	return
}

func (m *userSubscriptionRepo) UpdateUserSubscribeWithTraffic(ctx context.Context, id, download, upload int64, tx ...*gorm.DB) error {
	sub, err := m.FindOneSubscribe(ctx, id)
	if err != nil {
		return err
	}

	// 使用 defer 确保更新后清理缓存
	defer func() {
		if clearErr := m.ClearSubscribeCache(ctx, sub); clearErr != nil {
			// 记录清理缓存错误
		}
	}()

	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.Subscribe{}).Where("id = ?", id).Updates(map[string]interface{}{
			"download": gorm.Expr("download + ?", download),
			"upload":   gorm.Expr("upload + ?", upload),
		}).Error
	})
}

func (m *userSubscriptionRepo) BatchUpdateUserSubscribeWithTraffic(ctx context.Context, deltas []trafficEntity.SubscribeTrafficDelta, tx ...*gorm.DB) error {
	deltas = mergeSubscribeTrafficDeltas(deltas)
	if len(deltas) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(deltas))
	for _, delta := range deltas {
		ids = append(ids, delta.SubscribeId)
	}
	subs, err := m.FindSubscribesByIds(ctx, ids)
	if err != nil {
		return err
	}

	defer func() {
		_ = m.ClearSubscribeCache(ctx, subs...)
	}()

	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		downloadExpr, downloadArgs := userSubscribeTrafficIncrementExpr(conn, "download", deltas)
		uploadExpr, uploadArgs := userSubscribeTrafficIncrementExpr(conn, "upload", deltas)
		return conn.Model(&user.Subscribe{}).Where("id IN ?", ids).Updates(map[string]interface{}{
			"download": gorm.Expr(downloadExpr, downloadArgs...),
			"upload":   gorm.Expr(uploadExpr, uploadArgs...),
		}).Error
	})
}

func mergeSubscribeTrafficDeltas(deltas []trafficEntity.SubscribeTrafficDelta) []trafficEntity.SubscribeTrafficDelta {
	if len(deltas) == 0 {
		return nil
	}
	merged := make(map[int64]trafficEntity.SubscribeTrafficDelta, len(deltas))
	for _, delta := range deltas {
		if delta.SubscribeId <= 0 {
			continue
		}
		current := merged[delta.SubscribeId]
		current.SubscribeId = delta.SubscribeId
		current.Download += delta.Download
		current.Upload += delta.Upload
		merged[delta.SubscribeId] = current
	}
	result := make([]trafficEntity.SubscribeTrafficDelta, 0, len(merged))
	for _, delta := range merged {
		result = append(result, delta)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SubscribeId < result[j].SubscribeId
	})
	return result
}

func userSubscribeTrafficIncrementExpr(db *gorm.DB, column string, deltas []trafficEntity.SubscribeTrafficDelta) (string, []interface{}) {
	idColumn := userSubscribeColumn(db, "id")
	targetColumn := userSubscribeColumn(db, column)
	parts := make([]string, 0, len(deltas))
	args := make([]interface{}, 0, len(deltas)*2)
	for _, delta := range deltas {
		parts = append(parts, "WHEN ? THEN ?")
		args = append(args, delta.SubscribeId)
		if column == "download" {
			args = append(args, delta.Download)
		} else {
			args = append(args, delta.Upload)
		}
	}
	return fmt.Sprintf(
		"%s + CASE %s %s ELSE %s END",
		targetColumn,
		idColumn,
		strings.Join(parts, " "),
		userSubscribeTrafficZeroExpr(db),
	), args
}

func userSubscribeTrafficZeroExpr(db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return "0"
	}
	switch db.Dialector.Name() {
	case "postgres":
		return "0::bigint"
	case "mysql":
		return "CAST(0 AS SIGNED)"
	default:
		return "0"
	}
}

// FindOneSubscribeByToken  finds a record by token.
func (m *userSubscriptionRepo) FindOneSubscribeByToken(ctx context.Context, token string) (*user.Subscribe, error) {
	var data user.Subscribe
	key := fmt.Sprintf("%s%s", cacheUserSubscribeTokenPrefix, token)
	err := m.QueryCtx(ctx, &data, key, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Where("token = ?", token).First(&data).Error
	})
	return &data, err
}

func (m *userSubscriptionRepo) FindOneSubscribeByTokenForUpdate(ctx context.Context, token string) (*user.Subscribe, error) {
	var data user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&user.Subscribe{}).
			Where("token = ?", token).
			First(v).Error
	})
	return &data, err
}

// UpdateSubscribe updates a record.
func (m *userSubscriptionRepo) UpdateSubscribe(ctx context.Context, data *user.Subscribe, tx ...*gorm.DB) error {
	old, err := m.FindOneSubscribe(ctx, data.Id)
	if err != nil {
		return err
	}

	// 使用 defer 确保更新后清理缓存
	defer func() {
		if clearErr := m.ClearSubscribeCache(ctx, old, data); clearErr != nil {
			// 记录清理缓存错误
		}
	}()

	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.Subscribe{}).Where("id = ?", data.Id).Save(data).Error
	})
}

// DeleteSubscribe deletes a record.
func (m *userSubscriptionRepo) DeleteSubscribe(ctx context.Context, token string, tx ...*gorm.DB) error {
	data, err := m.FindOneSubscribeByToken(ctx, token)
	if err != nil {
		return err
	}

	// 使用 defer 确保删除后清理缓存
	defer func() {
		if clearErr := m.ClearSubscribeCache(ctx, data); clearErr != nil {
			// 记录清理缓存错误
		}
	}()

	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Where("token = ?", token).Delete(&user.Subscribe{}).Error
	})
}

// InsertSubscribe insert Subscribe into the database.
func (m *userSubscriptionRepo) InsertSubscribe(ctx context.Context, data *user.Subscribe, tx ...*gorm.DB) error {
	// 使用 defer 确保插入后清理相关缓存
	defer func() {
		if clearErr := m.ClearSubscribeCache(ctx, data); clearErr != nil {
			// 记录清理缓存错误
		}
	}()

	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Create(data).Error
	})
}

func (m *userSubscriptionRepo) DeleteSubscribeById(ctx context.Context, id int64, tx ...*gorm.DB) error {
	data, err := m.FindOneSubscribe(ctx, id)
	if err != nil {
		return err
	}

	// 使用 defer 确保删除后清理缓存
	defer func() {
		if clearErr := m.ClearSubscribeCache(ctx, data); clearErr != nil {
			// 记录清理缓存错误
		}
	}()

	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Where("id = ?", id).Delete(&user.Subscribe{}).Error
	})
}

func (m *userSubscriptionRepo) ClearSubscribeCache(ctx context.Context, data ...*user.Subscribe) error {
	if len(data) == 0 {
		return nil
	}
	var keys []string
	for _, s := range data {
		if s != nil {
			keys = append(keys, s.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

// --- subscription checks (expired / traffic exceeded) ---

func (m *userSubscriptionRepo) FindTrafficExceededSubscribes(ctx context.Context) ([]*user.Subscribe, error) {
	var list []*user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &list, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).
			Where("upload + download >= traffic AND status IN ? AND traffic > 0", []int64{0, 1}).
			Find(&list).Error
	})
	return list, err
}

func (m *userSubscriptionRepo) FindExpiredSubscribes(ctx context.Context, now time.Time) ([]*user.Subscribe, error) {
	var list []*user.Subscribe
	err := m.QueryNoCacheCtx(ctx, &list, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).
			Where("status IN ? AND expire_time < ? AND expire_time != ? AND finished_at IS NULL", []int64{0, 1}, now, time.UnixMilli(0)).
			Find(&list).Error
	})
	return list, err
}

func (m *userSubscriptionRepo) MarkSubscribesFinished(ctx context.Context, ids []int64, status uint8, finishedAt time.Time, tx ...*gorm.DB) error {
	if len(ids) == 0 {
		return nil
	}
	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.Subscribe{}).Where("id IN ?", ids).Updates(map[string]interface{}{
			"status":      status,
			"finished_at": finishedAt,
		}).Error
	})
}

// --- reset traffic ---

func (m *userSubscriptionRepo) QueryMonthlyResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error) {
	var ids []int64
	if len(subscribeIds) == 0 {
		return ids, nil
	}
	err := m.QueryNoCacheCtx(ctx, &ids, func(conn *gorm.DB, v interface{}) error {
		return userMonthlyResetSubscribeQuery(conn, subscribeIds, now).Find(&ids).Error
	})
	return ids, err
}

func (m *userSubscriptionRepo) QueryFirstResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error) {
	var ids []int64
	if len(subscribeIds) == 0 {
		return ids, nil
	}
	err := m.QueryNoCacheCtx(ctx, &ids, func(conn *gorm.DB, v interface{}) error {
		return userResettableSubscribeQuery(conn, subscribeIds, now).Find(&ids).Error
	})
	return ids, err
}

func (m *userSubscriptionRepo) QueryYearlyResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error) {
	var ids []int64
	if len(subscribeIds) == 0 {
		return ids, nil
	}
	err := m.QueryNoCacheCtx(ctx, &ids, func(conn *gorm.DB, v interface{}) error {
		return userYearlyResetSubscribeQuery(conn, subscribeIds, now).Find(&ids).Error
	})
	return ids, err
}

func (m *userSubscriptionRepo) ResetSubscribeTrafficByIds(ctx context.Context, ids []int64, tx ...*gorm.DB) error {
	if len(ids) == 0 {
		return nil
	}
	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.Subscribe{}).Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"upload":      0,
				"download":    0,
				"status":      1,
				"finished_at": nil,
			}).Error
	})
}

func userExtractColumnDatePart(db *gorm.DB, column, part string) string {
	if db.Dialector.Name() == "postgres" {
		return fmt.Sprintf("EXTRACT(%s FROM %s)", part, column)
	}
	switch part {
	case "month":
		return fmt.Sprintf("MONTH(%s)", column)
	default:
		return fmt.Sprintf("DAY(%s)", column)
	}
}

func userMonthlyResetSubscribeQuery(conn *gorm.DB, subscribeIds []int64, now time.Time) *gorm.DB {
	query := userResettableSubscribeQuery(conn, subscribeIds, now)
	condition, args := userMonthlyResetDateCondition(conn, now)
	return query.Where(condition, args...)
}

func userYearlyResetSubscribeQuery(conn *gorm.DB, subscribeIds []int64, now time.Time) *gorm.DB {
	query := userResettableSubscribeQuery(conn, subscribeIds, now)
	condition, args := userYearlyResetDateCondition(conn, now)
	return query.Where(condition, args...)
}

func userResettableSubscribeQuery(conn *gorm.DB, subscribeIds []int64, now time.Time) *gorm.DB {
	return conn.Model(&user.Subscribe{}).Select("id").
		Where("subscribe_id IN ?", subscribeIds).
		Where("status IN ?", []int64{1, 2}).
		Where("start_time <= ?", now).
		Where("(expire_time IS NULL OR expire_time = ? OR expire_time > ?)", time.UnixMilli(0), now)
}

func userMonthlyResetDateCondition(db *gorm.DB, now time.Time) (string, []interface{}) {
	dayExpr := userExtractColumnDatePart(db, "start_time", "day")
	if userIsLastDayOfMonth(now) {
		return dayExpr + " >= ?", []interface{}{now.Day()}
	}
	return dayExpr + " = ?", []interface{}{now.Day()}
}

func userYearlyResetDateCondition(db *gorm.DB, now time.Time) (string, []interface{}) {
	monthExpr := userExtractColumnDatePart(db, "start_time", "month")
	dayExpr := userExtractColumnDatePart(db, "start_time", "day")
	if now.Month() == time.February && now.Day() == 28 && !userIsLeapYear(now.Year()) {
		return fmt.Sprintf("%s = ? AND %s IN ?", monthExpr, dayExpr), []interface{}{int(time.February), []int{28, 29}}
	}
	return fmt.Sprintf("%s = ? AND %s = ?", monthExpr, dayExpr), []interface{}{int(now.Month()), now.Day()}
}

func userIsLastDayOfMonth(t time.Time) bool {
	return t.AddDate(0, 0, 1).Month() != t.Month()
}

func userIsLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func subscribeFilterQuery(conn *gorm.DB, filter *user.SubscribeFilter) *gorm.DB {
	query := conn.Model(&user.Subscribe{})
	if filter == nil {
		return query
	}
	if len(filter.Subscribers) > 0 {
		query = query.Where("subscribe_id IN ?", filter.Subscribers)
	}
	if filter.IsActive != nil && *filter.IsActive {
		query = query.Where("status IN ?", []int64{0, 1, 2})
	}
	if filter.StartTime != 0 {
		query = query.Where("start_time <= ?", time.UnixMilli(filter.StartTime))
	}
	if filter.EndTime != 0 {
		query = query.Where("expire_time >= ?", time.UnixMilli(filter.EndTime))
	}
	return query
}

func (m *userSubscriptionRepo) QuerySubscribeIdsByFilter(ctx context.Context, filter *user.SubscribeFilter) ([]int64, error) {
	var ids []int64
	err := m.QueryNoCacheCtx(ctx, &ids, func(conn *gorm.DB, v interface{}) error {
		return subscribeFilterQuery(conn, filter).Pluck("id", v).Error
	})
	return ids, err
}

func (m *userSubscriptionRepo) CountSubscribesByFilter(ctx context.Context, filter *user.SubscribeFilter) (int64, error) {
	var total int64
	err := m.QueryNoCacheCtx(ctx, &total, func(conn *gorm.DB, v interface{}) error {
		return subscribeFilterQuery(conn, filter).Count(&total).Error
	})
	return total, err
}

func (m *userSubscriptionRepo) FindSubscribesByIds(ctx context.Context, ids []int64) ([]*user.Subscribe, error) {
	var subscribes []*user.Subscribe
	if len(ids) == 0 {
		return subscribes, nil
	}
	err := m.QueryNoCacheCtx(ctx, &subscribes, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&user.Subscribe{}).Where("id IN ?", ids).Find(&subscribes).Error
	})
	return subscribes, err
}

func (m *userSubscriptionRepo) FindOneSubscribeDetailsById(ctx context.Context, id int64) (*user.SubscribeDetails, error) {
	var data user.SubscribeDetails
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		// Subscribe is a same-domain association; the identity-domain User
		// row is no longer preloaded (no consumer read it — ADR-001 step 5).
		return conn.Model(&user.Subscribe{}).Preload("Subscribe").Where("id = ?", id).First(&data).Error
	})
	return &data, err
}
