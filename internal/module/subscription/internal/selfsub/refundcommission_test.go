package selfsub

import (
	"errors"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
)

const (
	refundBuyer    int64 = 7
	refundReferrer int64 = 3
	refundPlan     int64 = 9
	refundOrder    int64 = 1
)

// paidSubscription is a balance-paid subscription (3000 from the balance,
// 1000 from the gift amount) whose refund is exactly 3000: it starts later,
// so the refundable share is the traffic left, three quarters. The order
// earned its buyer's referrer 800 commission.
func (f *fixture) paidSubscription(t *testing.T) *usersub.Subscribe {
	t.Helper()
	allow := true
	f.Plan(t, subscribe.Subscribe{Id: refundPlan, UnitTime: "Month", AllowDeduction: &allow})
	start := time.Now().Add(time.Hour)
	sub := f.Subscription(t, usersub.Subscribe{
		UserId: refundBuyer, OrderId: refundOrder, SubscribeId: refundPlan, StartTime: start, ExpireTime: start.AddDate(0, 1, 0),
		Traffic: 1000, Upload: 250, Status: usersub.SubscribeStatusActive,
	})
	f.orders[refundOrder] = &order.Details{Id: refundOrder, UserId: refundBuyer, OrderNo: "A", Method: "balance", Amount: 3000, GiftAmount: 1000, Commission: 800}
	f.buyers[refundBuyer] = &user.User{Id: refundBuyer, RefererId: refundReferrer}
	f.billing.wallets[refundBuyer] = wallet.Wallet{Balance: 500}
	f.billing.wallets[refundReferrer] = wallet.Wallet{Commission: 5000}
	return sub
}

// cancelledSubscription is a subscription whose cancellation committed with
// the given marker while its refund never did, paid by card through order
// details.
func (f *fixture) cancelledSubscription(t *testing.T, details *order.Details, marker string, referrer int64) *usersub.Subscribe {
	t.Helper()
	f.Plan(t, subscribe.Subscribe{Id: refundPlan})
	sub := f.Subscription(t, usersub.Subscribe{UserId: refundBuyer, OrderId: refundOrder, SubscribeId: refundPlan, Status: usersub.SubscribeStatusDeducted})
	details.Id, details.UserId, details.OrderNo, details.Method = refundOrder, refundBuyer, "A", "stripe"
	f.orders[refundOrder] = details
	f.buyers[refundBuyer] = &user.User{Id: refundBuyer, RefererId: referrer}
	f.billing.wallets[refundReferrer] = wallet.Wallet{Commission: 5000}
	f.cancelled(t, sub.Id, marker)
	return sub
}

func logContent[T interface{ Unmarshal([]byte) error }](t *testing.T, f *fixture, typ log.Type, content T) []log.SystemLog {
	t.Helper()
	rows := f.Logs(t, typ)
	if len(rows) == 1 {
		if err := content.Unmarshal([]byte(rows[0].Content)); err != nil {
			t.Fatal(err)
		}
	}
	return rows
}

// The quote is what the cancellation refunds: the gift share goes back to the
// gift amount, the rest to the balance, each movement logged, and the
// referrer loses the commission share the refund takes back. A repeated
// request pays nothing twice.
func TestUnsubscribeRefundsTheUnusedShareOnce(t *testing.T) {
	f := newFixture(t)
	sub := f.paidSubscription(t)

	quote, err := f.svc.PreUnsubscribe(as(refundBuyer), &dto.PreUnsubscribeRequest{Id: sub.Id})
	if err != nil || quote.DeductionAmount != 3000 {
		t.Fatalf("PreUnsubscribe = %+v, %v; want 3000", quote, err)
	}
	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); err != nil {
		t.Fatal(err)
	}

	if got := f.Load(t, sub.Id).Status; got != usersub.SubscribeStatusDeducted {
		t.Fatalf("status = %d, want Deducted", got)
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 2500 || got.GiftAmount != 1000 {
		t.Fatalf("buyer wallet = %+v, want 2000 back to the balance and 1000 to the gift amount", got)
	}
	// Three quarters of the 800 commission are taken back.
	if got := f.billing.wallet(refundReferrer); got.Commission != 4400 {
		t.Fatalf("referrer commission = %d, want 4400", got.Commission)
	}
	var balance log.Balance
	if rows := logContent(t, f, log.TypeBalance, &balance); len(rows) != 1 || rows[0].ObjectID != refundBuyer ||
		balance.Type != log.BalanceTypeRefund || balance.Amount != 2000 || balance.Balance != 2500 || balance.OrderNo != "A" {
		t.Fatalf("balance log = %+v %+v", rows, balance)
	}
	var gift log.Gift
	if rows := logContent(t, f, log.TypeGift, &gift); len(rows) != 1 || gift.Type != log.GiftTypeIncrease || gift.Amount != 1000 || gift.Balance != 1000 || gift.SubscribeId != sub.Id {
		t.Fatalf("gift log = %+v %+v", rows, gift)
	}
	var commission log.Commission
	if rows := logContent(t, f, log.TypeCommission, &commission); len(rows) != 1 || rows[0].ObjectID != refundReferrer ||
		commission.Type != log.CommissionTypeRefund || commission.Amount != -600 {
		t.Fatalf("commission log = %+v %+v", rows, commission)
	}

	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); !errors.Is(err, errNotCancelable) {
		t.Fatalf("repeated Unsubscribe = %v, want errNotCancelable", err)
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 2500 || got.GiftAmount != 1000 || len(f.Logs(t, log.TypeBalance)) != 1 {
		t.Fatalf("the repeated request moved money: %+v", got)
	}
}

// A refund that fails after the cancellation committed rolls back whole —
// no money, no log, no marker — and the retry settles the amount the
// cancellation recorded, once.
func TestUnsubscribeResumesAFailedRefund(t *testing.T) {
	f := newFixture(t)
	sub := f.paidSubscription(t)
	f.billing.failSaves = 1

	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); !errors.Is(err, errWalletUnavailable) {
		t.Fatalf("Unsubscribe = %v, want the wallet failure", err)
	}
	if got := f.Load(t, sub.Id).Status; got != usersub.SubscribeStatusDeducted {
		t.Fatalf("status = %d, want the committed cancellation", got)
	}
	if result, ok := f.marker(t, unsubscribeCancelConsumer, sub.Id); !ok || result != "1|3000" {
		t.Fatalf("cancellation marker = %q, %v", result, ok)
	}
	if _, ok := f.marker(t, unsubscribeRefundConsumer, sub.Id); ok {
		t.Fatal("the failed refund left its marker")
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 500 || got.GiftAmount != 0 || len(f.Logs(t, log.TypeGift)) != 0 {
		t.Fatalf("the failed refund left money or logs behind: %+v", got)
	}

	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); err != nil {
		t.Fatalf("retry = %v", err)
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 2500 || got.GiftAmount != 1000 {
		t.Fatalf("buyer wallet after the retry = %+v", got)
	}
	if got := f.billing.wallet(refundReferrer); got.Commission != 4400 {
		t.Fatalf("referrer commission after the retry = %d", got.Commission)
	}
	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); !errors.Is(err, errNotCancelable) {
		t.Fatalf("second retry = %v, want errNotCancelable", err)
	}
}

// A refund takes back the commission its orders earned, in proportion, so a
// buy-and-refund loop on recycled balance cannot farm commission. Paid
// renewals count; a traffic reset is neither refunded nor part of the basis.
func TestSettleRefundReversesCommissionInProportion(t *testing.T) {
	f := newFixture(t)
	sub := f.cancelledSubscription(t, &order.Details{
		Amount: 6000, Commission: 1200,
		SubOrders: []*order.Order{
			{Type: order.TypeRenewal, Status: order.StatusPaid, Amount: 4000, Commission: 800},
			{Type: order.TypeResetTraffic, Status: order.StatusPaid, Amount: 500, Commission: 0},
		},
	}, "1|5000", refundReferrer)

	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); err != nil {
		t.Fatal(err)
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 5000 || got.GiftAmount != 0 {
		t.Fatalf("buyer wallet = %+v, want 5000 back to the balance", got)
	}
	// Half of the 10000 basis was refunded, so half of the 2000 commission
	// goes back.
	if got := f.billing.wallet(refundReferrer); got.Commission != 4000 {
		t.Fatalf("referrer commission = %d, want 4000", got.Commission)
	}
	var commission log.Commission
	if rows := logContent(t, f, log.TypeCommission, &commission); len(rows) != 1 || commission.Amount != -1000 || commission.Type != log.CommissionTypeRefund {
		t.Fatalf("commission reversal log = %+v %+v, want a refund entry of -1000", rows, commission)
	}
	if _, ok := f.marker(t, unsubscribeRefundConsumer, sub.Id); !ok {
		t.Fatal("the settled refund left no marker")
	}
}

// A cancellation recorded by the old formula could exceed what was paid; the
// settlement caps it at the basis.
func TestSettleRefundCapsRefundAtAmountPaid(t *testing.T) {
	f := newFixture(t)
	sub := f.cancelledSubscription(t, &order.Details{Amount: 36500, Commission: 7300}, "1|72900", refundReferrer)

	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); err != nil {
		t.Fatal(err)
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 36500 {
		t.Fatalf("buyer balance = %d, want the 36500 paid", got.Balance)
	}
	if got := f.billing.wallet(refundReferrer); got.Commission != 5000-7300 {
		t.Fatalf("referrer commission = %d, want the whole 7300 reversed", got.Commission)
	}
}

func TestSettleRefundWithoutReferrerKeepsCommissionsUntouched(t *testing.T) {
	f := newFixture(t)
	sub := f.cancelledSubscription(t, &order.Details{Amount: 6000, Commission: 1200}, "1|3000", 0)

	if err := f.svc.Unsubscribe(as(refundBuyer), &dto.UnsubscribeRequest{Id: sub.Id}); err != nil {
		t.Fatal(err)
	}
	if got := f.billing.wallet(refundBuyer); got.Balance != 3000 {
		t.Fatalf("buyer balance = %d, want 3000", got.Balance)
	}
	if got := f.billing.wallet(refundReferrer); got.Commission != 5000 {
		t.Fatalf("referrer commission = %d, want 5000", got.Commission)
	}
	if rows := f.Logs(t, log.TypeCommission); len(rows) != 0 {
		t.Fatalf("commission logs = %+v, want none", rows)
	}
}
