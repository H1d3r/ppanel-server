package checkout

import (
	"context"
	"strings"
	"testing"
	"time"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// Coupon start/expire times are stored as Unix milliseconds. Comparing them
// against a seconds clock made every coupon with a start time permanently
// "not active" (seconds are always smaller than millisecond timestamps).
func TestEnsureCouponEnabledUsesMillisecondTimestamps(t *testing.T) {
	enabled := true
	now := timeutil.Now().UnixMilli()
	hour := time.Hour.Milliseconds()

	tests := []struct {
		name    string
		start   int64
		expire  int64
		wantErr string
	}{
		{name: "inside window is accepted", start: now - hour, expire: now + 365*24*hour, wantErr: ""},
		{name: "not yet started is rejected", start: now + hour, expire: now + 2*hour, wantErr: "not active"},
		{name: "expired is rejected", start: now - 2*hour, expire: now - hour, wantErr: "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureCouponEnabled(&coupon.Coupon{
				Enable:     &enabled,
				StartTime:  tt.start,
				ExpireTime: tt.expire,
			})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ensureCouponEnabled error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ensureCouponEnabled error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// couponLimitOrders counts the user's coupon uses. The wallet lock bumps the
// count, standing in for a concurrent order that committed while this
// request waited for the lock.
type couponLimitOrders struct {
	repository.OrderRepo
	usage    int64
	inserted int
}

func (r *couponLimitOrders) CountUserCouponUsage(context.Context, int64, string) (int64, error) {
	return r.usage, nil
}

func (r *couponLimitOrders) IsUserEligibleForNewOrder(context.Context, int64) (bool, error) {
	return true, nil
}

func (r *couponLimitOrders) Insert(context.Context, *orderEntity.Order, ...*gorm.DB) error {
	r.inserted++
	return nil
}

type couponLimitWallets struct {
	repository.WalletRepo
	orders *couponLimitOrders
}

func (w couponLimitWallets) FindOneForUpdate(_ context.Context, userID int64) (*walletEntity.Wallet, error) {
	w.orders.usage++
	return &walletEntity.Wallet{UserId: userID}, nil
}

type couponLimitCoupons struct {
	repository.CouponRepo
	coupon   *coupon.Coupon
	reserved int
}

func (r *couponLimitCoupons) FindOneByCode(context.Context, string) (*coupon.Coupon, error) {
	copy := *r.coupon
	return &copy, nil
}

func (r *couponLimitCoupons) ReserveUsage(context.Context, string, int64, ...*gorm.DB) (bool, error) {
	r.reserved++
	return true, nil
}

type couponLimitLogs struct {
	repository.LogRepo
}

func (couponLimitLogs) Insert(context.Context, *logEntity.SystemLog) error { return nil }

type couponLimitStore struct {
	Store
	tx couponLimitTx
}

func (s couponLimitStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s.tx)
}

type couponLimitTx struct {
	repository.BillingStore
	orders  *couponLimitOrders
	coupons *couponLimitCoupons
}

func (tx couponLimitTx) Order() repository.OrderRepo   { return tx.orders }
func (tx couponLimitTx) Coupon() repository.CouponRepo { return tx.coupons }
func (tx couponLimitTx) Wallet() repository.WalletRepo { return couponLimitWallets{orders: tx.orders} }
func (tx couponLimitTx) Log() repository.LogRepo       { return couponLimitLogs{} }

type couponLimitPayments struct {
	repository.PaymentRepo
}

func (couponLimitPayments) FindOne(_ context.Context, id int64) (*paymentEntity.Payment, error) {
	enabled := true
	return &paymentEntity.Payment{Id: id, Platform: "EPay", Enable: &enabled}, nil
}

type couponLimitUserSubs struct {
	UserSubscriptionReader
}

func (couponLimitUserSubs) FindOneUserSubscribe(_ context.Context, id int64) (*usersub.SubscribeDetails, error) {
	return &usersub.SubscribeDetails{Id: id, UserId: 42, SubscribeId: 10}, nil
}

type couponLimitInventory struct{}

func (couponLimitInventory) Reserve(context.Context, string, int64) error { return nil }
func (couponLimitInventory) Restore(context.Context, string, int64) error { return nil }

func newCouponLimitService() (*Service, *couponLimitOrders, *couponLimitCoupons) {
	enabled := true
	now := timeutil.Now().UnixMilli()
	orders := &couponLimitOrders{}
	coupons := &couponLimitCoupons{coupon: &coupon.Coupon{
		Code: "ONCE", Type: 2, Discount: 100, UserLimit: 1, Enable: &enabled,
		StartTime: now - time.Hour.Milliseconds(), ExpireTime: now + time.Hour.Milliseconds(),
	}}
	svc := NewService(Deps{
		Orders:      orders,
		Coupons:     coupons,
		Payments:    couponLimitPayments{},
		Plans:       policyPlans{subscribe: &subscribe.Subscribe{Id: 10, Sell: boolPtr(true), Inventory: -1, UnitPrice: 1000}},
		UserSubs:    couponLimitUserSubs{},
		Store:       couponLimitStore{tx: couponLimitTx{orders: orders, coupons: coupons}},
		Inventory:   couponLimitInventory{},
		Queue:       &closeQueue{},
		SingleModel: func() bool { return false },
	})
	return svc, orders, coupons
}

// Concurrent orders all pass the per-user count taken before the order
// transaction; the count must be repeated under the user's wallet lock.
func TestCheckoutRechecksCouponUserLimitUnderWalletLock(t *testing.T) {
	tests := []struct {
		name   string
		create func(*Service) error
	}{
		{name: "purchase", create: func(svc *Service) error {
			_, err := svc.Purchase(ownerContext(42), &dto.PurchaseOrderRequest{SubscribeId: 10, Quantity: 1, Payment: 3, Coupon: "ONCE"})
			return err
		}},
		{name: "renewal", create: func(svc *Service) error {
			_, err := svc.Renewal(ownerContext(42), &dto.RenewalOrderRequest{UserSubscribeID: 22, Quantity: 1, Payment: 3, Coupon: "ONCE"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, orders, coupons := newCouponLimitService()
			err := tt.create(svc)
			var codeErr *xerr.CodeError
			if !errors.As(errors.Cause(err), &codeErr) || codeErr.GetErrCode() != xerr.CouponInsufficientUsage {
				t.Fatalf("error = %v, want CouponInsufficientUsage", err)
			}
			if orders.inserted != 0 || coupons.reserved != 0 {
				t.Fatalf("over-limit order created: inserted=%d reserved=%d", orders.inserted, coupons.reserved)
			}

			// Below the limit the same flow creates the order.
			svc, orders, coupons = newCouponLimitService()
			coupons.coupon.UserLimit = 2
			if err := tt.create(svc); err != nil {
				t.Fatalf("order below the per-user limit: %v", err)
			}
			if orders.inserted != 1 || coupons.reserved != 1 {
				t.Fatalf("order below the limit: inserted=%d reserved=%d", orders.inserted, coupons.reserved)
			}
		})
	}
}
