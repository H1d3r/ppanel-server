package selfsub

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription/internal/subtest"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"gorm.io/gorm"
)

// fixture is the subscription fixture with the billing side of a refund:
// buyers, orders and wallets in memory, the refund's system log and inbox
// markers in the fixture database.
type fixture struct {
	*subtest.Fixture
	svc     *Service
	buyers  buyers
	orders  orders
	billing *billing
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	logtest.Discard(t)
	f := &fixture{Fixture: subtest.New(t), buyers: buyers{}, orders: orders{}}
	f.billing = &billing{db: f.DB, orders: f.orders, wallets: map[int64]wallet.Wallet{}}
	f.svc = NewService(Deps{
		UserSubs:    f.Store.UserSubscription(),
		Plans:       f.Store.Subscribe(),
		Users:       f.buyers,
		Orders:      f.orders,
		Cache:       f.Store.UserCache(),
		Logs:        subtest.NewLogs(f.DB),
		Inbox:       subtest.NewInbox(f.DB),
		Store:       refundStore{Store: f.Store, billing: f.billing},
		SingleModel: func() bool { return false },
	})
	return f
}

// as returns a request context authenticated as the user.
func as(userID int64) context.Context {
	return user.NewContext(context.Background(), &user.User{Id: userID})
}

// marker returns the result of an inbox marker, or false without one.
func (f *fixture) marker(t *testing.T, consumer string, subID int64) (string, bool) {
	t.Helper()
	record, err := subtest.NewInbox(f.DB).Find(context.Background(), consumer, fmt.Sprint(subID))
	if err != nil {
		t.Fatal(err)
	}
	if record == nil {
		return "", false
	}
	return record.Result, true
}

// cancelled records a cancellation whose refund was never settled, as a
// crash between the two stages leaves it.
func (f *fixture) cancelled(t *testing.T, subID int64, result string) {
	t.Helper()
	if err := subtest.NewInbox(f.DB).Insert(context.Background(), unsubscribeCancelConsumer, fmt.Sprint(subID), result); err != nil {
		t.Fatal(err)
	}
}

// buyers is the identity read port.
type buyers map[int64]*user.User

var _ BuyerReader = buyers{}

func (b buyers) FindOne(_ context.Context, id int64) (*user.User, error) {
	if u, ok := b[id]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// orders is the billing read port.
type orders map[int64]*order.Details

var _ OrderReader = orders{}

func (o orders) FindOneDetails(_ context.Context, id int64) (*order.Details, error) {
	if details, ok := o[id]; ok {
		return details, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// refundStore is the package's Store over the fixture: the fixture's
// subscription transaction and the billing transaction below.
type refundStore struct {
	*subtest.Store
	billing *billing
}

var _ Store = refundStore{}

func (s refundStore) InRefundTx(ctx context.Context, fn func(RefundLedger) error) error {
	return s.billing.inTx(ctx, fn)
}

var errWalletUnavailable = errors.New("wallet unavailable")

// billing holds the wallets. A settlement runs on a copy of them and on a
// database transaction for its log and marker rows; a failed one leaves
// neither behind.
type billing struct {
	db      *gorm.DB
	orders  orders
	wallets map[int64]wallet.Wallet
	// locks counts the wallets settlements locked; failSaves fails that many
	// balance writes.
	locks     int
	failSaves int
}

func (b *billing) inTx(ctx context.Context, fn func(RefundLedger) error) error {
	var staged map[int64]wallet.Wallet
	err := b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		l := &ledger{billing: b, tx: tx, wallets: maps.Clone(b.wallets)}
		if err := fn(l); err != nil {
			return err
		}
		staged = l.wallets
		return nil
	})
	if err == nil {
		b.wallets = staged
	}
	return err
}

// wallet returns the user's committed wallet.
func (b *billing) wallet(userID int64) wallet.Wallet {
	w := b.wallets[userID]
	w.UserId = userID
	return w
}

// ledger is one settlement's view of the billing side: its own copy of the
// wallets and the database transaction.
type ledger struct {
	billing *billing
	tx      *gorm.DB
	wallets map[int64]wallet.Wallet
}

var _ RefundLedger = (*ledger)(nil)

func (l *ledger) LockWallet(_ context.Context, userID int64) (*wallet.Wallet, error) {
	l.billing.locks++
	w := l.wallets[userID]
	w.UserId = userID
	return &w, nil
}

func (l *ledger) SaveBalance(_ context.Context, w *wallet.Wallet) error {
	if l.billing.failSaves > 0 {
		l.billing.failSaves--
		return errWalletUnavailable
	}
	stored := l.wallets[w.UserId]
	stored.Balance, stored.GiftAmount = w.Balance, w.GiftAmount
	l.wallets[w.UserId] = stored
	return nil
}

func (l *ledger) SaveCommission(_ context.Context, w *wallet.Wallet) error {
	stored := l.wallets[w.UserId]
	stored.Commission = w.Commission
	l.wallets[w.UserId] = stored
	return nil
}

func (l *ledger) OrderDetails(ctx context.Context, orderID int64) (*order.Details, error) {
	return l.billing.orders.FindOneDetails(ctx, orderID)
}

func (l *ledger) AppendLog(ctx context.Context, entry *log.SystemLog) error {
	return subtest.NewLogs(l.tx).Insert(ctx, entry)
}

func (l *ledger) MarkRefunded(ctx context.Context, subKey string) error {
	return subtest.NewInbox(l.tx).Insert(ctx, unsubscribeRefundConsumer, subKey, "")
}
