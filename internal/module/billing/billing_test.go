package billing_test

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type fakeOrderRepo struct {
	repository.OrderRepo
	order        *orderEntity.Order
	details      *orderEntity.Details
	pendingCount int64
	inserted     *orderEntity.Order
	markedPaid   bool
	closed       bool
}

func (f *fakeOrderRepo) FindOneDetailsByOrderNo(_ context.Context, orderNo string) (*orderEntity.Details, error) {
	if f.details == nil || f.details.OrderNo != orderNo {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *f.details
	return &copy, nil
}

func (f *fakeOrderRepo) FindOne(_ context.Context, id int64) (*orderEntity.Order, error) {
	if f.order == nil || f.order.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *f.order
	return &copy, nil
}

func (f *fakeOrderRepo) FindOneByOrderNo(_ context.Context, orderNo string) (*orderEntity.Order, error) {
	if f.order == nil || f.order.OrderNo != orderNo {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *f.order
	return &copy, nil
}

func (f *fakeOrderRepo) FindOneByOrderNoForUpdate(_ context.Context, orderNo string) (*orderEntity.Order, error) {
	if f.order == nil || f.order.OrderNo != orderNo {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *f.order
	return &copy, nil
}

func (f *fakeOrderRepo) Insert(_ context.Context, data *orderEntity.Order, _ ...*gorm.DB) error {
	f.inserted = data
	return nil
}

func (f *fakeOrderRepo) Update(_ context.Context, data *orderEntity.Order, _ ...*gorm.DB) error {
	f.order = data
	return nil
}

func (f *fakeOrderRepo) MarkOrderPaid(_ context.Context, orderNo, tradeNo string, _ ...*gorm.DB) (bool, error) {
	if f.order.OrderNo != orderNo || f.order.Status != 1 {
		return false, nil
	}
	f.order.Status = 2
	f.order.TradeNo = tradeNo
	f.markedPaid = true
	return true, nil
}

func (f *fakeOrderRepo) UpdateOrderStatusFrom(_ context.Context, orderNo string, from, to uint8, _ ...*gorm.DB) (bool, error) {
	if f.order.OrderNo != orderNo || f.order.Status != from {
		return false, nil
	}
	f.order.Status = to
	f.closed = true
	return true, nil
}

func (f *fakeOrderRepo) CountPendingByPaymentID(_ context.Context, _ int64) (int64, error) {
	return f.pendingCount, nil
}

type fakePaymentRepo struct {
	repository.PaymentRepo
	method  *paymentEntity.Payment
	deleted []int64
}

func (f *fakePaymentRepo) FindOne(_ context.Context, id int64) (*paymentEntity.Payment, error) {
	if f.method == nil || f.method.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *f.method
	return &copy, nil
}

func (f *fakePaymentRepo) Delete(_ context.Context, id int64, _ ...*gorm.DB) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeBillingTx struct {
	orders   *fakeOrderRepo
	payments *fakePaymentRepo
}

func (f fakeBillingTx) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(billingStoreView{orders: f.orders, payments: f.payments})
}

// billingStoreView satisfies repository.BillingStore for the fakes.
type billingStoreView struct {
	repository.BillingStore
	orders   *fakeOrderRepo
	payments *fakePaymentRepo
	coupons  *fakeReleaseCouponRepo
	wallets  *fakeWalletRepo
	logs     *fakeLogRepo
}

func (v billingStoreView) Order() repository.OrderRepo     { return v.orders }
func (v billingStoreView) Payment() repository.PaymentRepo { return v.payments }
func (v billingStoreView) Coupon() repository.CouponRepo   { return v.coupons }
func (v billingStoreView) Wallet() repository.WalletRepo   { return v.wallets }
func (v billingStoreView) Log() repository.LogRepo         { return v.logs }

// fakeCloseStore serves the checkout close flow the admin close runs through.
type fakeCloseStore struct {
	billing.Store
	view billingStoreView
}

func (s fakeCloseStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s.view)
}

type fakeReleaseCouponRepo struct {
	repository.CouponRepo
	released []string
}

func (f *fakeReleaseCouponRepo) ReleaseUsage(_ context.Context, code string, _ ...*gorm.DB) error {
	f.released = append(f.released, code)
	return nil
}

type fakeWalletRepo struct {
	repository.WalletRepo
	wallet *walletEntity.Wallet
}

func (f *fakeWalletRepo) FindOneForUpdate(_ context.Context, userID int64) (*walletEntity.Wallet, error) {
	if f.wallet == nil || f.wallet.UserId != userID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *f.wallet
	return &copy, nil
}

func (f *fakeWalletRepo) UpdateBalanceFields(_ context.Context, data *walletEntity.Wallet, _ ...*gorm.DB) error {
	f.wallet.Balance, f.wallet.GiftAmount = data.Balance, data.GiftAmount
	return nil
}

type fakeLogRepo struct {
	repository.LogRepo
}

func (fakeLogRepo) Insert(context.Context, *logEntity.SystemLog) error { return nil }

type fakeInventory struct {
	restored []string
}

func (f *fakeInventory) Reserve(context.Context, string, int64) error { return nil }

func (f *fakeInventory) Restore(_ context.Context, orderNo string, _ int64) error {
	f.restored = append(f.restored, orderNo)
	return nil
}

type fakeActivationQueue struct {
	enqueued []string
}

func (f *fakeActivationQueue) EnqueueActivation(_ context.Context, orderNo string) error {
	f.enqueued = append(f.enqueued, orderNo)
	return nil
}

func (f *fakeActivationQueue) EnqueueDeferredClose(_ context.Context, _ string) error { return nil }

type billingFakes struct {
	orders   *fakeOrderRepo
	payments *fakePaymentRepo
	queue    *fakeActivationQueue
}

func newBillingService(orders *fakeOrderRepo, payments *fakePaymentRepo) (billing.Service, *billingFakes) {
	fakes := &billingFakes{orders: orders, payments: payments, queue: &fakeActivationQueue{}}
	svc := billing.New(billing.Deps{
		Orders:   orders,
		Payments: payments,
		Tx:       fakeBillingTx{orders: orders, payments: payments},
		Store:    fakeCloseStore{view: billingStoreView{orders: orders, payments: payments}},
		Queue:    fakes.queue,
		Host:     "panel.example.com",
	})
	return svc, fakes
}

func newBillingServiceWithCoupons(coupons *fakeCouponRepo) billing.Service {
	return billing.New(billing.Deps{
		Orders:   &fakeOrderRepo{},
		Payments: &fakePaymentRepo{},
		Coupons:  coupons,
		Queue:    &fakeActivationQueue{},
	})
}

func TestUpdateOrderStatusRejectsInvalidTransitions(t *testing.T) {
	orders := &fakeOrderRepo{order: &orderEntity.Order{Id: 1, OrderNo: "o-1", Status: 1}}
	svc, fakes := newBillingService(orders, &fakePaymentRepo{})

	for _, req := range []*dto.UpdateOrderStatusRequest{
		{Id: 1, Status: 5, TradeNo: "t"},               // arbitrary terminal state
		{Id: 1, Status: 2},                             // paid without trade number
		{Id: 1, Status: 3, TradeNo: "t"},               // close with payment fields
		{Id: 1, Status: 3, PaymentId: 9},               // close with payment fields
		{Id: 1, Status: 1, TradeNo: "t", PaymentId: 0}, // no-op transition
	} {
		if err := svc.UpdateOrderStatus(context.Background(), req); err == nil {
			t.Fatalf("transition %+v must be rejected", req)
		}
	}
	if orders.order.Status != 1 || len(fakes.queue.enqueued) != 0 {
		t.Fatalf("rejected transitions must not mutate state: %+v", orders.order)
	}
}

func TestUpdateOrderStatusMarksPaidAndEnqueuesActivation(t *testing.T) {
	orders := &fakeOrderRepo{order: &orderEntity.Order{Id: 1, OrderNo: "o-2", Status: 1}}
	svc, fakes := newBillingService(orders, &fakePaymentRepo{})

	if err := svc.UpdateOrderStatus(context.Background(), &dto.UpdateOrderStatusRequest{Id: 1, Status: 2, TradeNo: "trade-1"}); err != nil {
		t.Fatalf("UpdateOrderStatus: %v", err)
	}
	if !orders.markedPaid || orders.order.TradeNo != "trade-1" {
		t.Fatalf("order not marked paid: %+v", orders.order)
	}
	if len(fakes.queue.enqueued) != 1 || fakes.queue.enqueued[0] != "o-2" {
		t.Fatalf("activation not enqueued: %v", fakes.queue.enqueued)
	}
}

func TestUpdateOrderStatusCloseDoesNotEnqueue(t *testing.T) {
	orders := &fakeOrderRepo{order: &orderEntity.Order{Id: 1, OrderNo: "o-3", Status: 1}}
	svc, fakes := newBillingService(orders, &fakePaymentRepo{})

	if err := svc.UpdateOrderStatus(context.Background(), &dto.UpdateOrderStatusRequest{Id: 1, Status: 3}); err != nil {
		t.Fatalf("UpdateOrderStatus: %v", err)
	}
	if !orders.closed {
		t.Fatalf("order not closed: %+v", orders.order)
	}
	if len(fakes.queue.enqueued) != 0 {
		t.Fatal("closing must not enqueue activation")
	}
}

// An administrator's close runs the shared close flow. The bare status update
// it replaced kept the coupon use, the gift deduction and the plan stock.
func TestUpdateOrderStatusCloseReleasesReservations(t *testing.T) {
	orders := &fakeOrderRepo{order: &orderEntity.Order{
		Id: 1, OrderNo: "o-4", Status: 1, Type: 1, UserId: 7, SubscribeId: 9,
		GiftAmount: 300, Coupon: "SPRING", CouponReserved: true,
	}}
	payments := &fakePaymentRepo{}
	coupons := &fakeReleaseCouponRepo{}
	wallets := &fakeWalletRepo{wallet: &walletEntity.Wallet{UserId: 7, GiftAmount: 100}}
	inventory := &fakeInventory{}
	svc := billing.New(billing.Deps{
		Orders:   orders,
		Payments: payments,
		Tx:       fakeBillingTx{orders: orders, payments: payments},
		Store: fakeCloseStore{view: billingStoreView{
			orders: orders, payments: payments, coupons: coupons, wallets: wallets, logs: &fakeLogRepo{},
		}},
		Inventory: inventory,
		Queue:     &fakeActivationQueue{},
	})
	// The administrator is not the order's owner.
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &userEntity.User{Id: 99})

	if err := svc.UpdateOrderStatus(ctx, &dto.UpdateOrderStatusRequest{Id: 1, Status: 3}); err != nil {
		t.Fatalf("UpdateOrderStatus: %v", err)
	}
	if orders.order.Status != 3 {
		t.Fatalf("status = %d, want closed", orders.order.Status)
	}
	if len(coupons.released) != 1 || coupons.released[0] != "SPRING" {
		t.Fatalf("released coupons = %v, want [SPRING]", coupons.released)
	}
	if wallets.wallet.GiftAmount != 400 {
		t.Fatalf("gift amount = %d, want the 300 deduction refunded", wallets.wallet.GiftAmount)
	}
	if len(inventory.restored) != 1 || inventory.restored[0] != "o-4" {
		t.Fatalf("restored inventory = %v, want [o-4]", inventory.restored)
	}

	if err := svc.UpdateOrderStatus(ctx, &dto.UpdateOrderStatusRequest{Id: 1, Status: 3}); err == nil {
		t.Fatal("closing an order that is no longer pending must be rejected")
	}
	if len(coupons.released) != 1 {
		t.Fatal("a repeated close must not release the coupon again")
	}
}

func TestDeletePaymentMethodGuardsPendingOrders(t *testing.T) {
	orders := &fakeOrderRepo{pendingCount: 2}
	payments := &fakePaymentRepo{method: &paymentEntity.Payment{Id: 5}}
	svc, _ := newBillingService(orders, payments)

	if err := svc.DeletePaymentMethod(context.Background(), &dto.DeletePaymentMethodRequest{Id: 5}); err == nil {
		t.Fatal("deleting a payment method with pending orders must be rejected")
	}
	if len(payments.deleted) != 0 {
		t.Fatal("payment method must not be deleted")
	}

	orders.pendingCount = 0
	if err := svc.DeletePaymentMethod(context.Background(), &dto.DeletePaymentMethodRequest{Id: 5}); err != nil {
		t.Fatalf("DeletePaymentMethod: %v", err)
	}
	if len(payments.deleted) != 1 {
		t.Fatal("payment method deletion missing")
	}
}

func TestCreatePaymentMethodValidatesFeeAndPlatform(t *testing.T) {
	svc, _ := newBillingService(&fakeOrderRepo{}, &fakePaymentRepo{})

	if _, err := svc.CreatePaymentMethod(context.Background(), &dto.CreatePaymentMethodRequest{Platform: "Nope"}); err == nil {
		t.Fatal("unsupported platform must be rejected")
	}
	if _, err := svc.CreatePaymentMethod(context.Background(), &dto.CreatePaymentMethodRequest{Platform: "EPay", FeeMode: 9}); err == nil {
		t.Fatal("invalid fee mode must be rejected")
	}
}
