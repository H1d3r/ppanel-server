package activation

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	inboxEntity "github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type commissionOrders struct {
	repository.OrderRepo
	orders     map[string]*order.Order
	commission map[string]int64
}

func (r *commissionOrders) FindOneByOrderNo(_ context.Context, orderNo string) (*order.Order, error) {
	return r.orders[orderNo], nil
}

func (r *commissionOrders) SetCommission(_ context.Context, orderNo string, amount int64, _ ...*gorm.DB) error {
	r.commission[orderNo] = amount
	return nil
}

func (r *commissionOrders) HasCommissionedOrder(_ context.Context, userID int64, exceptOrderNo string) (bool, error) {
	for orderNo, amount := range r.commission {
		if orderNo != exceptOrderNo && amount > 0 && r.orders[orderNo].UserId == userID {
			return true, nil
		}
	}
	return false, nil
}

type commissionWallets struct {
	repository.WalletRepo
	wallets map[int64]*walletEntity.Wallet
}

func (r *commissionWallets) FindOneForUpdate(_ context.Context, userID int64) (*walletEntity.Wallet, error) {
	return r.wallets[userID], nil
}

func (r *commissionWallets) UpdateCommission(_ context.Context, data *walletEntity.Wallet, _ ...*gorm.DB) error {
	r.wallets[data.UserId].Commission = data.Commission
	return nil
}

type commissionLogs struct {
	repository.LogRepo
	entries []*log.SystemLog
}

func (r *commissionLogs) Insert(_ context.Context, data *log.SystemLog) error {
	r.entries = append(r.entries, data)
	return nil
}

type commissionInbox struct {
	repository.InboxRepo
	marks map[string]bool
}

func (r *commissionInbox) Find(_ context.Context, consumer, key string) (*inboxEntity.Record, error) {
	if r.marks[consumer+"|"+key] {
		return &inboxEntity.Record{Consumer: consumer, EventKey: key}, nil
	}
	return nil, nil
}

func (r *commissionInbox) Insert(_ context.Context, consumer, key, _ string) error {
	r.marks[consumer+"|"+key] = true
	return nil
}

type commissionStore struct {
	repository.BillingStore
	orders  *commissionOrders
	wallets *commissionWallets
	logs    *commissionLogs
	inbox   *commissionInbox
}

func (s *commissionStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s)
}
func (s *commissionStore) Order() repository.OrderRepo   { return s.orders }
func (s *commissionStore) Wallet() repository.WalletRepo { return s.wallets }
func (s *commissionStore) Log() repository.LogRepo       { return s.logs }
func (s *commissionStore) Inbox() repository.InboxRepo   { return s.inbox }

type commissionProfiles map[int64]*user.User

func (p commissionProfiles) FindOne(_ context.Context, id int64) (*user.User, error) {
	return p[id], nil
}

const (
	commissionBuyer    int64 = 7
	commissionReferrer int64 = 3
)

func newCommissionService(onlyFirst bool, orders ...*order.Order) (*Service, *commissionStore) {
	byNo := make(map[string]*order.Order, len(orders))
	for _, o := range orders {
		byNo[o.OrderNo] = o
	}
	store := &commissionStore{
		orders:  &commissionOrders{orders: byNo, commission: map[string]int64{}},
		wallets: &commissionWallets{wallets: map[int64]*walletEntity.Wallet{commissionReferrer: {UserId: commissionReferrer}}},
		logs:    &commissionLogs{},
		inbox:   &commissionInbox{marks: map[string]bool{}},
	}
	svc := NewService(Deps{
		Orders: store.orders,
		Store:  store,
		Profiles: commissionProfiles{
			commissionBuyer:    {Id: commissionBuyer, RefererId: commissionReferrer},
			commissionReferrer: {Id: commissionReferrer},
		},
		InvitePolicy: func() (uint8, bool) { return 20, onlyFirst },
	})
	return svc, store
}

// The paid commission is kept on the order: a refund reverses it from there
// because inbox markers and logs are purged over time.
func TestSettleOrderCommissionRecordsAmountOnOrder(t *testing.T) {
	svc, store := newCommissionService(false,
		&order.Order{OrderNo: "A", UserId: commissionBuyer, Type: OrderTypeSubscribe, Amount: 10000})

	if err := svc.SettleOrderCommission(context.Background(), "A", commissionBuyer); err != nil {
		t.Fatalf("SettleOrderCommission() error = %v", err)
	}

	if got := store.wallets.wallets[commissionReferrer].Commission; got != 2000 {
		t.Fatalf("referrer commission = %d, want 2000", got)
	}
	if got := store.orders.commission["A"]; got != 2000 {
		t.Fatalf("order commission = %d, want 2000", got)
	}
}

// IsNew is fixed when an order is created, so orders opened before the first
// one was paid all carry it. First-purchase-only commission must still be
// paid once.
func TestSettleOrderCommissionFirstPurchaseOnlyPaysOnce(t *testing.T) {
	svc, store := newCommissionService(true,
		&order.Order{OrderNo: "A", UserId: commissionBuyer, Type: OrderTypeSubscribe, Amount: 10000, IsNew: true},
		&order.Order{OrderNo: "B", UserId: commissionBuyer, Type: OrderTypeSubscribe, Amount: 10000, IsNew: true})

	for _, orderNo := range []string{"A", "B"} {
		if err := svc.SettleOrderCommission(context.Background(), orderNo, commissionBuyer); err != nil {
			t.Fatalf("SettleOrderCommission(%s) error = %v", orderNo, err)
		}
	}

	if got := store.wallets.wallets[commissionReferrer].Commission; got != 2000 {
		t.Fatalf("referrer commission = %d, want one first-purchase commission of 2000", got)
	}
	if got := store.orders.commission["B"]; got != 0 {
		t.Fatalf("second order commission = %d, want 0", got)
	}
}
