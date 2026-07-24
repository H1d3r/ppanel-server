package orderLogic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	inboxEntity "github.com/perfect-panel/server/internal/model/entity/inbox"
	logEntity "github.com/perfect-panel/server/internal/model/entity/log"
	orderEntity "github.com/perfect-panel/server/internal/model/entity/order"
	subscribeEntity "github.com/perfect-panel/server/internal/model/entity/subscribe"
	userEntity "github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/queue/types"
	"gorm.io/gorm"
)

type activationStore struct {
	repository.Store
	orders     *activationOrderRepo
	users      *activationUserRepo
	subscribes *activationSubscribeRepo
	logs       *activationLogRepo
	inbox      *activationInboxRepo
}

func (s *activationStore) InTx(_ context.Context, fn func(repository.Store) error) error {
	return fn(s)
}

func (s *activationStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s)
}

func (s *activationStore) InIdentityTx(_ context.Context, fn func(repository.IdentityStore) error) error {
	return fn(s)
}

func (s *activationStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	return fn(s)
}

func (s *activationStore) Wallet() repository.WalletRepo { return s.users }
func (s *activationStore) Order() repository.OrderRepo   { return s.orders }
func (s *activationStore) User() repository.UserRepo     { return s.users }
func (s *activationStore) UserSubscription() repository.UserSubscriptionRepo {
	return s.users
}
func (s *activationStore) UserCache() repository.UserCacheRepo { return s.users }
func (s *activationStore) Log() repository.LogRepo             { return s.logs }
func (s *activationStore) Subscribe() repository.SubscribeRepo { return s.subscribes }
func (s *activationStore) Inbox() repository.InboxRepo         { return s.inbox }

type activationInboxRepo struct {
	repository.InboxRepo
	records map[string]*inboxEntity.Record
}

func newActivationInboxRepo() *activationInboxRepo {
	return &activationInboxRepo{records: map[string]*inboxEntity.Record{}}
}

func (r *activationInboxRepo) Find(_ context.Context, consumer, eventKey string) (*inboxEntity.Record, error) {
	record, ok := r.records[consumer+"|"+eventKey]
	if !ok {
		return nil, nil
	}
	copy := *record
	return &copy, nil
}

func (r *activationInboxRepo) Insert(_ context.Context, consumer, eventKey, result string) error {
	key := consumer + "|" + eventKey
	if _, ok := r.records[key]; ok {
		return fmt.Errorf("duplicate inbox record %s", key)
	}
	r.records[key] = &inboxEntity.Record{Consumer: consumer, EventKey: eventKey, Result: result}
	return nil
}

type activationOrderRepo struct {
	repository.OrderRepo
	order *orderEntity.Order
}

func (r *activationOrderRepo) FindOneByOrderNo(_ context.Context, orderNo string) (*orderEntity.Order, error) {
	if r.order.OrderNo != orderNo {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.order
	return &copy, nil
}

func (r *activationOrderRepo) FindOneByOrderNoForUpdate(ctx context.Context, orderNo string) (*orderEntity.Order, error) {
	return r.FindOneByOrderNo(ctx, orderNo)
}

func (r *activationOrderRepo) UpdateOrderStatusFrom(_ context.Context, orderNo string, from, to uint8, _ ...*gorm.DB) (bool, error) {
	if r.order.OrderNo != orderNo || r.order.Status != from {
		return false, nil
	}
	r.order.Status = to
	return true, nil
}

type activationUserRepo struct {
	repository.UserRepo
	repository.UserSubscriptionRepo
	repository.UserCacheRepo
	user             *userEntity.User
	updateCacheCalls int
	quotaCount       int64
	quotaCountCalls  int
	blocking         bool
	hasBlockingCalls int
	subscription     *userEntity.Subscribe
}

func (r *activationUserRepo) FindOne(_ context.Context, id int64) (*userEntity.User, error) {
	if r.user == nil || r.user.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.user
	return &copy, nil
}

func (r *activationUserRepo) FindOneForUpdate(_ context.Context, id int64) (*userEntity.User, error) {
	if r.user.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.user
	return &copy, nil
}

func (r *activationUserRepo) Update(_ context.Context, data *userEntity.User, _ ...*gorm.DB) error {
	r.user.Balance = data.Balance
	return nil
}

func (r *activationUserRepo) UpdateBalanceFields(_ context.Context, data *userEntity.User, _ ...*gorm.DB) error {
	r.user.Balance = data.Balance
	r.user.GiftAmount = data.GiftAmount
	return nil
}

func (r *activationUserRepo) UpdateUserCache(_ context.Context, _ *userEntity.User) error {
	r.updateCacheCalls++
	return nil
}

func (r *activationUserRepo) CountQuotaConsumingSubscriptions(_ context.Context, _ int64, _ int64) (int64, error) {
	r.quotaCountCalls++
	return r.quotaCount, nil
}

func (r *activationUserRepo) HasBlockingSubscription(_ context.Context, _ int64) (bool, error) {
	r.hasBlockingCalls++
	return r.blocking, nil
}

func (r *activationUserRepo) FindOneSubscribeByToken(_ context.Context, token string) (*userEntity.Subscribe, error) {
	if r.subscription == nil || r.subscription.Token != token {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.subscription
	return &copy, nil
}

func (r *activationUserRepo) FindOneSubscribeByTokenForUpdate(ctx context.Context, token string) (*userEntity.Subscribe, error) {
	return r.FindOneSubscribeByToken(ctx, token)
}

func (r *activationUserRepo) UpdateSubscribe(_ context.Context, data *userEntity.Subscribe, _ ...*gorm.DB) error {
	copy := *data
	r.subscription = &copy
	return nil
}

func (r *activationUserRepo) ClearSubscribeCache(_ context.Context, _ ...*userEntity.Subscribe) error {
	return nil
}

type activationLogRepo struct {
	repository.LogRepo
	logs []*logEntity.SystemLog
}

func (r *activationLogRepo) Insert(_ context.Context, data *logEntity.SystemLog) error {
	r.logs = append(r.logs, data)
	return nil
}

type activationSubscribeRepo struct {
	repository.SubscribeRepo
	subscribe *subscribeEntity.Subscribe
}

func (r *activationSubscribeRepo) FindOne(_ context.Context, id int64) (*subscribeEntity.Subscribe, error) {
	if r.subscribe == nil || r.subscribe.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.subscribe
	return &copy, nil
}

func (r *activationSubscribeRepo) ClearCache(_ context.Context, _ ...int64) error {
	return nil
}

func TestActivateRechargeCommitsSettlementOnlyOnce(t *testing.T) {
	store := &activationStore{
		orders: &activationOrderRepo{order: &orderEntity.Order{
			OrderNo: "recharge-order", UserId: 7, Type: OrderTypeRecharge, Price: 1250, Status: OrderStatusPaid,
		}},
		users: &activationUserRepo{user: &userEntity.User{Id: 7, Balance: 500}},
		logs:  &activationLogRepo{},
		inbox: newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(&svc.ServiceContext{Store: store})
	payload, err := json.Marshal(types.ForthwithActivateOrderPayload{OrderNo: "recharge-order"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	task := asynq.NewTask(types.ForthwithActivateOrder, payload)

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("duplicate activation: %v", err)
	}
	if store.orders.order.Status != OrderStatusFinished {
		t.Fatalf("order status = %d, want finished", store.orders.order.Status)
	}
	if store.users.user.Balance != 1750 {
		t.Fatalf("balance = %d, want 1750", store.users.user.Balance)
	}
	if len(store.logs.logs) != 1 {
		t.Fatalf("recharge logs = %d, want 1", len(store.logs.logs))
	}
}

// TestActivateRechargeReplayAfterFulfillmentSkipsSecondCredit simulates a
// crash between the fulfillment and finalize stages: the balance credit
// committed but the order is still Paid, so the reconciler replays the task.
// The inbox marker must prevent a second credit.
func TestActivateRechargeReplayAfterFulfillmentSkipsSecondCredit(t *testing.T) {
	store := &activationStore{
		orders: &activationOrderRepo{order: &orderEntity.Order{
			OrderNo: "recharge-replay", UserId: 7, Type: OrderTypeRecharge, Price: 1250, Status: OrderStatusPaid,
		}},
		users: &activationUserRepo{user: &userEntity.User{Id: 7, Balance: 500}},
		logs:  &activationLogRepo{},
		inbox: newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(&svc.ServiceContext{Store: store})
	payload, err := json.Marshal(types.ForthwithActivateOrderPayload{OrderNo: "recharge-replay"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	task := asynq.NewTask(types.ForthwithActivateOrder, payload)

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	// Simulate the finalize stage having been lost: the order is Paid again.
	store.orders.order.Status = OrderStatusPaid

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("replayed activation: %v", err)
	}
	if store.users.user.Balance != 1750 {
		t.Fatalf("balance = %d, want 1750 (credited exactly once)", store.users.user.Balance)
	}
	if len(store.logs.logs) != 1 {
		t.Fatalf("balance logs = %d, want 1", len(store.logs.logs))
	}
	if store.orders.order.Status != OrderStatusFinished {
		t.Fatalf("order status = %d, want finished after replay", store.orders.order.Status)
	}
}

// TestActivateRenewalReplayExtendsSubscriptionOnce guards the most dangerous
// replay: extending a renewal twice would silently gift subscription time.
func TestActivateRenewalReplayExtendsSubscriptionOnce(t *testing.T) {
	expire := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	store := &activationStore{
		orders: &activationOrderRepo{order: &orderEntity.Order{
			OrderNo: "renewal-replay", UserId: 7, Type: OrderTypeRenewal, Status: OrderStatusPaid,
			SubscribeId: 9, SubscribeToken: "renewal-token", Quantity: 1,
		}},
		users: &activationUserRepo{
			user: &userEntity.User{Id: 7},
			subscription: &userEntity.Subscribe{
				Id: 11, UserId: 7, SubscribeId: 9, Token: "renewal-token",
				ExpireTime: expire, Status: userEntity.SubscribeStatusActive,
			},
		},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9, UnitTime: "Month"}},
		logs:       &activationLogRepo{},
		inbox:      newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(&svc.ServiceContext{Store: store})
	payload, err := json.Marshal(types.ForthwithActivateOrderPayload{OrderNo: "renewal-replay"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	task := asynq.NewTask(types.ForthwithActivateOrder, payload)

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	extendedOnce := store.users.subscription.ExpireTime
	if !extendedOnce.After(expire) {
		t.Fatalf("first activation must extend the subscription: %v -> %v", expire, extendedOnce)
	}

	// Simulate the finalize stage having been lost: the order is Paid again.
	store.orders.order.Status = OrderStatusPaid

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("replayed activation: %v", err)
	}
	if !store.users.subscription.ExpireTime.Equal(extendedOnce) {
		t.Fatalf("replay extended the subscription twice: %v -> %v", extendedOnce, store.users.subscription.ExpireTime)
	}
	if store.orders.order.Status != OrderStatusFinished {
		t.Fatalf("order status = %d, want finished after replay", store.orders.order.Status)
	}
}

func TestCreateUserSubscriptionTxEnforcesQuota(t *testing.T) {
	users := &activationUserRepo{quotaCount: 1}
	store := &activationStore{users: users}
	logic := NewActivateOrderLogic(&svc.ServiceContext{})

	_, err := logic.createUserSubscriptionTx(context.Background(), store, &orderEntity.Order{UserId: 7, SubscribeId: 9}, &subscribeEntity.Subscribe{Quota: 1})
	if err == nil {
		t.Fatal("activation created a subscription after quota was exhausted")
	}
	if users.quotaCountCalls != 1 {
		t.Fatalf("CountQuotaConsumingSubscriptions calls = %d, want 1", users.quotaCountCalls)
	}
}

func TestCreateUserSubscriptionTxEnforcesSingleModel(t *testing.T) {
	users := &activationUserRepo{blocking: true}
	store := &activationStore{users: users}
	logic := NewActivateOrderLogic(&svc.ServiceContext{Config: config.Config{Subscribe: config.SubscribeConfig{SingleModel: true}}})

	_, err := logic.createUserSubscriptionTx(context.Background(), store, &orderEntity.Order{UserId: 7, SubscribeId: 9}, &subscribeEntity.Subscribe{})
	if err == nil {
		t.Fatal("activation created a subscription despite a blocking subscription")
	}
	if users.hasBlockingCalls != 1 {
		t.Fatalf("HasBlockingSubscription calls = %d, want 1", users.hasBlockingCalls)
	}
}

func TestActivateResetTrafficTxClearsFinishedAt(t *testing.T) {
	logic, store := newResetTrafficTestLogic(t)

	result, err := logic.activateResetTrafficTx(context.Background(), store, &orderEntity.Order{
		OrderNo: "reset-order", UserId: 7, SubscribeToken: "subscription-token",
	})
	if err != nil {
		t.Fatalf("activate reset traffic: %v", err)
	}
	if store.users.subscription.FinishedAt != nil {
		t.Fatal("reset traffic left FinishedAt set")
	}
	if result.userSub.FinishedAt != nil {
		t.Fatal("activation result left FinishedAt set")
	}
	if store.users.subscription.Status != userEntity.SubscribeStatusActive {
		t.Fatalf("status = %d, want active", store.users.subscription.Status)
	}
}

func newResetTrafficTestLogic(t *testing.T) (*ActivateOrderLogic, *activationStore) {
	t.Helper()
	finishedAt := time.Now().Add(-time.Hour)
	store := &activationStore{
		users: &activationUserRepo{
			user: &userEntity.User{Id: 7},
			subscription: &userEntity.Subscribe{
				Id: 11, UserId: 7, SubscribeId: 9, Token: "subscription-token",
				Download: 100, Upload: 200, Status: userEntity.SubscribeStatusFinished, FinishedAt: &finishedAt,
			},
		},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9}},
		logs:       &activationLogRepo{},
	}
	return NewActivateOrderLogic(&svc.ServiceContext{Store: store}), store
}
