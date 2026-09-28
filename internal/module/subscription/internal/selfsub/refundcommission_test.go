package selfsub

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	usermodel "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"gorm.io/gorm"
)

type refundWallets struct {
	repository.WalletRepo
	wallets map[int64]*walletEntity.Wallet
}

func (r *refundWallets) FindOneForUpdate(_ context.Context, userID int64) (*walletEntity.Wallet, error) {
	w := *r.wallets[userID]
	return &w, nil
}

func (r *refundWallets) UpdateBalanceFields(_ context.Context, data *walletEntity.Wallet, _ ...*gorm.DB) error {
	r.wallets[data.UserId].Balance = data.Balance
	r.wallets[data.UserId].GiftAmount = data.GiftAmount
	return nil
}

func (r *refundWallets) UpdateCommission(_ context.Context, data *walletEntity.Wallet, _ ...*gorm.DB) error {
	r.wallets[data.UserId].Commission = data.Commission
	return nil
}

type refundOrders struct {
	repository.OrderRepo
	details *order.Details
}

func (r *refundOrders) FindOneDetails(_ context.Context, _ int64) (*order.Details, error) {
	return r.details, nil
}

type refundLogs struct {
	repository.LogRepo
	entries []*log.SystemLog
}

func (r *refundLogs) Insert(_ context.Context, data *log.SystemLog) error {
	r.entries = append(r.entries, data)
	return nil
}

type refundStore struct {
	repository.Store
	wallets *refundWallets
	orders  *refundOrders
	logs    *refundLogs
	inbox   *fakeInboxRepo
}

func (s *refundStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s)
}
func (s *refundStore) Wallet() repository.WalletRepo { return s.wallets }
func (s *refundStore) Order() repository.OrderRepo   { return s.orders }
func (s *refundStore) Log() repository.LogRepo       { return s.logs }
func (s *refundStore) Inbox() repository.InboxRepo   { return s.inbox }

type refundUsers struct {
	repository.UserRepo
	users map[int64]*usermodel.User
}

func (r *refundUsers) FindOne(_ context.Context, id int64) (*usermodel.User, error) {
	return r.users[id], nil
}

const (
	refundBuyer    int64 = 7
	refundReferrer int64 = 3
)

func newRefundLogic(details *order.Details, cancellation string, refererID int64) (*UnsubscribeLogic, *refundStore) {
	store := &refundStore{
		wallets: &refundWallets{wallets: map[int64]*walletEntity.Wallet{
			refundBuyer:    {UserId: refundBuyer},
			refundReferrer: {UserId: refundReferrer, Commission: 5000},
		}},
		orders: &refundOrders{details: details},
		logs:   &refundLogs{},
		inbox:  newFakeInboxRepo(),
	}
	_ = store.inbox.Insert(context.Background(), unsubscribeCancelConsumer, "9", cancellation)
	logic := newUnsubscribeLogic(context.Background(), Deps{
		Store: store,
		Inbox: store.inbox,
		Users: &refundUsers{users: map[int64]*usermodel.User{
			refundBuyer: {Id: refundBuyer, RefererId: refererID},
		}},
	})
	return logic, store
}

// A refund takes back the commission its orders earned, in proportion, so a
// buy-and-refund loop on recycled balance cannot farm commission.
func TestSettleRefundReversesCommissionInProportion(t *testing.T) {
	logtest.Discard(t)
	details := &order.Details{
		Id: 1, UserId: refundBuyer, OrderNo: "A", Method: "stripe", Amount: 6000, Commission: 1200,
		SubOrders: []*order.Order{
			{Type: orderTypeRenewal, Status: 2, Amount: 4000, Commission: 800},
			// A traffic reset is neither refunded nor part of the basis.
			{Type: 3, Status: 2, Amount: 500, Commission: 0},
		},
	}
	logic, store := newRefundLogic(details, "1|5000", refundReferrer)

	if err := logic.settleRefundOnce(refundBuyer, 9, "9"); err != nil {
		t.Fatalf("settleRefundOnce() error = %v", err)
	}

	if got := store.wallets.wallets[refundBuyer].Balance; got != 5000 {
		t.Fatalf("buyer balance = %d, want 5000", got)
	}
	// Half of the 10000 basis was refunded, so half of the 2000 commission
	// goes back.
	if got := store.wallets.wallets[refundReferrer].Commission; got != 4000 {
		t.Fatalf("referrer commission = %d, want 4000", got)
	}
	var reversal *log.Commission
	for _, entry := range store.logs.entries {
		if entry.Type == log.TypeCommission.Uint8() && entry.ObjectID == refundReferrer {
			var c log.Commission
			if err := c.Unmarshal([]byte(entry.Content)); err != nil {
				t.Fatal(err)
			}
			reversal = &c
		}
	}
	if reversal == nil || reversal.Type != log.CommissionTypeRefund || reversal.Amount != -1000 {
		t.Fatalf("commission reversal log = %+v, want a refund entry of -1000", reversal)
	}
}

// A cancellation recorded by the old formula could exceed what was paid; the
// settlement caps it at the basis.
func TestSettleRefundCapsRefundAtAmountPaid(t *testing.T) {
	logtest.Discard(t)
	details := &order.Details{Id: 1, UserId: refundBuyer, OrderNo: "A", Method: "stripe", Amount: 36500, Commission: 7300}
	logic, store := newRefundLogic(details, "1|72900", refundReferrer)

	if err := logic.settleRefundOnce(refundBuyer, 9, "9"); err != nil {
		t.Fatalf("settleRefundOnce() error = %v", err)
	}

	if got := store.wallets.wallets[refundBuyer].Balance; got != 36500 {
		t.Fatalf("buyer balance = %d, want the 36500 paid", got)
	}
	if got := store.wallets.wallets[refundReferrer].Commission; got != 5000-7300 {
		t.Fatalf("referrer commission = %d, want the whole 7300 reversed", got)
	}
}

func TestSettleRefundWithoutReferrerKeepsCommissionsUntouched(t *testing.T) {
	logtest.Discard(t)
	details := &order.Details{Id: 1, UserId: refundBuyer, OrderNo: "A", Method: "stripe", Amount: 6000, Commission: 1200}
	logic, store := newRefundLogic(details, "1|3000", 0)

	if err := logic.settleRefundOnce(refundBuyer, 9, "9"); err != nil {
		t.Fatalf("settleRefundOnce() error = %v", err)
	}

	if got := store.wallets.wallets[refundReferrer].Commission; got != 5000 {
		t.Fatalf("referrer commission = %d, want 5000", got)
	}
}
