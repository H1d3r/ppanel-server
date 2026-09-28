package billing_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
)

// wallet seeds a wallet with every amount set.
func (f *facade) wallet(t *testing.T, userID, balance, gift, commission int64) {
	t.Helper()
	if err := f.h.DB.Create(&wallet.Wallet{UserId: userID, Balance: balance, GiftAmount: gift, Commission: commission}).Error; err != nil {
		t.Fatal(err)
	}
}

// commissionLogs decodes the commission ledger of userID.
func (f *facade) commissionLogs(t *testing.T, userID int64) []logEntity.Commission {
	t.Helper()
	var entries []logEntity.Commission
	for _, row := range f.h.Logs(logEntity.TypeCommission, userID) {
		var entry logEntity.Commission
		if err := entry.Unmarshal([]byte(row.Content)); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}

const refundSubscription int64 = 21

// refundSettled reports whether the refund of refundSubscription is settled.
func (f *facade) refundSettled(t *testing.T) bool {
	t.Helper()
	settled, err := f.svc.UnsubscribeRefundSettled(context.Background(), refundSubscription)
	if err != nil {
		t.Fatal(err)
	}
	return settled
}

// A cancelled subscription's refund returns the gift share of a balance-paid
// order to the gift amount first and the rest to the balance, each movement
// logged, and the referrer loses the commission share the refund takes back.
// The refund is settled once: a second settlement fails and moves nothing.
func TestSettleUnsubscribeRefundReturnsTheGiftShareFirst(t *testing.T) {
	f := newFacade(t)
	referrer := f.h.User()
	buyer := f.h.User(func(u *user.User) { u.RefererId = referrer.Id })
	f.wallet(t, buyer.Id, 500, 0, 0)
	f.wallet(t, referrer.Id, 0, 0, 5000)
	o := f.h.Order(&order.Order{OrderNo: "A", UserId: buyer.Id, Type: order.TypeSubscribe, Status: order.StatusFinished, Method: "balance", Amount: 3000, GiftAmount: 1000, Commission: 800})
	ctx := context.Background()

	if f.refundSettled(t) {
		t.Fatal("an unsettled refund reads as settled")
	}
	if err := f.svc.SettleUnsubscribeRefund(ctx, buyer.Id, refundSubscription, o.Id, 3000); err != nil {
		t.Fatalf("SettleUnsubscribeRefund: %v", err)
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 2500 || got.GiftAmount != 1000 {
		t.Fatalf("buyer wallet = %+v, want 2000 back to the balance and 1000 to the gift amount", got)
	}
	// Three quarters of the 4000 paid were refunded: three quarters of the
	// 800 commission are taken back.
	if got := f.h.ReloadWallet(referrer.Id); got.Commission != 4400 {
		t.Fatalf("referrer commission = %d, want 4400", got.Commission)
	}
	if logs := f.h.BalanceLogs(buyer.Id); len(logs) != 1 || logs[0].Type != logEntity.BalanceTypeRefund || logs[0].Amount != 2000 || logs[0].Balance != 2500 || logs[0].OrderNo != "A" || logs[0].Timestamp == 0 {
		t.Fatalf("balance logs = %+v, want the refunded balance share, timestamped", logs)
	}
	if logs := f.h.GiftLogs(buyer.Id); len(logs) != 1 || logs[0].Type != logEntity.GiftTypeIncrease || logs[0].Amount != 1000 || logs[0].Balance != 1000 || logs[0].SubscribeId != refundSubscription || logs[0].Timestamp == 0 {
		t.Fatalf("gift logs = %+v, want the refunded gift share, timestamped", logs)
	}
	if logs := f.commissionLogs(t, referrer.Id); len(logs) != 1 || logs[0].Type != logEntity.CommissionTypeRefund || logs[0].Amount != -600 || logs[0].OrderNo != "A" {
		t.Fatalf("commission logs = %+v", logs)
	}
	if !f.refundSettled(t) {
		t.Fatal("the settled refund reads as unsettled")
	}

	if err := f.svc.SettleUnsubscribeRefund(ctx, buyer.Id, refundSubscription, o.Id, 3000); err == nil {
		t.Fatal("a second settlement succeeded")
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 2500 || got.GiftAmount != 1000 || len(f.h.BalanceLogs(buyer.Id)) != 1 {
		t.Fatalf("the second settlement moved money: %+v", got)
	}
	if got := f.h.ReloadWallet(referrer.Id); got.Commission != 4400 {
		t.Fatalf("the second settlement took commission again: %d", got.Commission)
	}
}

// A refund that fails rolls back whole: no money, no log, no marker. The
// retry settles it once.
func TestSettleUnsubscribeRefundRollsBackAFailure(t *testing.T) {
	f := newFacade(t)
	buyer := f.h.User()
	f.wallet(t, buyer.Id, 500, 0, 0)
	o := f.h.Order(&order.Order{OrderNo: "A", UserId: buyer.Id, Type: order.TypeSubscribe, Status: order.StatusFinished, Method: "balance", Amount: 3000, GiftAmount: 1000})
	if err := f.h.DB.Exec(`CREATE TRIGGER fail_wallet BEFORE UPDATE ON user_wallet BEGIN SELECT RAISE(ABORT, 'wallet unavailable'); END`).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := f.svc.SettleUnsubscribeRefund(ctx, buyer.Id, refundSubscription, o.Id, 3000); err == nil {
		t.Fatal("the failed refund was reported as settled")
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 500 || got.GiftAmount != 0 || len(f.h.GiftLogs(buyer.Id)) != 0 || f.refundSettled(t) {
		t.Fatalf("the failed refund left money, logs or its marker behind: %+v", got)
	}

	if err := f.h.DB.Exec(`DROP TRIGGER fail_wallet`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SettleUnsubscribeRefund(ctx, buyer.Id, refundSubscription, o.Id, 3000); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 2500 || got.GiftAmount != 1000 || !f.refundSettled(t) {
		t.Fatalf("buyer wallet after the retry = %+v", got)
	}
}

// A refund takes back the commission its orders earned, in proportion, so a
// buy-and-refund loop on recycled balance cannot farm commission. Paid
// renewals count; a traffic reset is neither refunded nor part of the basis.
// An order paid by card refunds to the balance.
func TestSettleUnsubscribeRefundReversesCommissionInProportion(t *testing.T) {
	f := newFacade(t)
	referrer := f.h.User()
	buyer := f.h.User(func(u *user.User) { u.RefererId = referrer.Id })
	f.wallet(t, referrer.Id, 0, 0, 5000)
	o := f.h.Order(&order.Order{OrderNo: "A", UserId: buyer.Id, Type: order.TypeSubscribe, Status: order.StatusFinished, Method: "stripe", Amount: 6000, Commission: 1200})
	f.h.Order(&order.Order{ParentId: o.Id, OrderNo: "A-renewal", UserId: buyer.Id, Type: order.TypeRenewal, Status: order.StatusPaid, Method: "stripe", Amount: 4000, Commission: 800})
	f.h.Order(&order.Order{ParentId: o.Id, OrderNo: "A-reset", UserId: buyer.Id, Type: order.TypeResetTraffic, Status: order.StatusPaid, Method: "stripe", Amount: 500})

	if err := f.svc.SettleUnsubscribeRefund(context.Background(), buyer.Id, refundSubscription, o.Id, 5000); err != nil {
		t.Fatal(err)
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 5000 || got.GiftAmount != 0 {
		t.Fatalf("buyer wallet = %+v, want 5000 back to the balance", got)
	}
	// Half of the 10000 basis was refunded, so half of the 2000 commission
	// goes back.
	if got := f.h.ReloadWallet(referrer.Id); got.Commission != 4000 {
		t.Fatalf("referrer commission = %d, want 4000", got.Commission)
	}
	if logs := f.commissionLogs(t, referrer.Id); len(logs) != 1 || logs[0].Amount != -1000 || logs[0].Type != logEntity.CommissionTypeRefund {
		t.Fatalf("commission reversal logs = %+v, want a refund entry of -1000", logs)
	}
	if !f.refundSettled(t) {
		t.Fatal("the settled refund left no marker")
	}
}

// A cancellation recorded by the old formula could exceed what was paid; the
// settlement caps it at the basis and takes back the whole commission.
func TestSettleUnsubscribeRefundCapsTheRefundAtTheAmountPaid(t *testing.T) {
	f := newFacade(t)
	referrer := f.h.User()
	buyer := f.h.User(func(u *user.User) { u.RefererId = referrer.Id })
	f.wallet(t, referrer.Id, 0, 0, 5000)
	o := f.h.Order(&order.Order{OrderNo: "A", UserId: buyer.Id, Type: order.TypeSubscribe, Status: order.StatusFinished, Method: "stripe", Amount: 36500, Commission: 7300})

	if err := f.svc.SettleUnsubscribeRefund(context.Background(), buyer.Id, refundSubscription, o.Id, 72900); err != nil {
		t.Fatal(err)
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 36500 {
		t.Fatalf("buyer balance = %d, want the 36500 paid", got.Balance)
	}
	if got := f.h.ReloadWallet(referrer.Id); got.Commission != 5000-7300 {
		t.Fatalf("referrer commission = %d, want the whole 7300 reversed", got.Commission)
	}
}

func TestSettleUnsubscribeRefundWithoutReferrerKeepsCommissionsUntouched(t *testing.T) {
	f := newFacade(t)
	bystander := f.h.User()
	buyer := f.h.User()
	f.wallet(t, bystander.Id, 0, 0, 5000)
	o := f.h.Order(&order.Order{OrderNo: "A", UserId: buyer.Id, Type: order.TypeSubscribe, Status: order.StatusFinished, Method: "stripe", Amount: 6000, Commission: 1200})

	if err := f.svc.SettleUnsubscribeRefund(context.Background(), buyer.Id, refundSubscription, o.Id, 3000); err != nil {
		t.Fatal(err)
	}
	if got := f.h.ReloadWallet(buyer.Id); got.Balance != 3000 {
		t.Fatalf("buyer balance = %d, want 3000", got.Balance)
	}
	if got := f.h.ReloadWallet(bystander.Id); got.Commission != 5000 {
		t.Fatalf("commission = %d, want 5000", got.Commission)
	}
	if logs := f.commissionLogs(t, bystander.Id); len(logs) != 0 {
		t.Fatalf("commission logs = %+v, want none", logs)
	}
}

// A subscription an administrator created has no order: its settlement moves
// no money and only records that the refund stage ran.
func TestSettleUnsubscribeRefundWithoutAnOrderOnlyMarks(t *testing.T) {
	f := newFacade(t)
	owner := f.h.User()

	if err := f.svc.SettleUnsubscribeRefund(context.Background(), owner.Id, refundSubscription, 0, 0); err != nil {
		t.Fatal(err)
	}
	if !f.refundSettled(t) {
		t.Fatal("the settled refund stage left no marker")
	}
	var wallets int64
	if err := f.h.DB.Model(&wallet.Wallet{}).Count(&wallets).Error; err != nil {
		t.Fatal(err)
	}
	if wallets != 0 || len(f.h.BalanceLogs(owner.Id)) != 0 || len(f.h.GiftLogs(owner.Id)) != 0 {
		t.Fatalf("money moved for a subscription without an order: %d wallets", wallets)
	}
}

// A quota task's gift is credited to the gift balance once per (task,
// subscription), with its gift log dated at the task run.
func TestCreditQuotaGiftCreditsOnce(t *testing.T) {
	f := newFacade(t)
	owner := f.h.User()
	f.wallet(t, owner.Id, 70, 50, 0)
	ctx := context.Background()
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	if credited, err := f.svc.QuotaGiftCredited(ctx, 7, 9); err != nil || credited {
		t.Fatalf("QuotaGiftCredited = %v, %v; want not credited", credited, err)
	}
	if err := f.svc.CreditQuotaGift(ctx, 7, 9, owner.Id, 199, at); err != nil {
		t.Fatalf("CreditQuotaGift: %v", err)
	}
	if got := f.h.ReloadWallet(owner.Id); got.GiftAmount != 249 || got.Balance != 70 {
		t.Fatalf("wallet = %+v, want the gift amount credited", got)
	}
	rows := f.h.Logs(logEntity.TypeGift, owner.Id)
	gifts := f.h.GiftLogs(owner.Id)
	if len(rows) != 1 || rows[0].Date != "2026-03-04" || gifts[0].Type != logEntity.GiftTypeIncrease || gifts[0].SubscribeId != 9 ||
		gifts[0].Amount != 199 || gifts[0].Balance != 249 || gifts[0].Remark != "Quota task gift" || gifts[0].Timestamp != at.UnixMilli() {
		t.Fatalf("gift logs = %+v %+v", rows, gifts)
	}
	if credited, err := f.svc.QuotaGiftCredited(ctx, 7, 9); err != nil || !credited {
		t.Fatalf("QuotaGiftCredited = %v, %v; want credited", credited, err)
	}

	if err := f.svc.CreditQuotaGift(ctx, 7, 9, owner.Id, 199, at); err != nil {
		t.Fatalf("replayed CreditQuotaGift: %v", err)
	}
	if got := f.h.ReloadWallet(owner.Id); got.GiftAmount != 249 || len(f.h.GiftLogs(owner.Id)) != 1 {
		t.Fatalf("the replay credited again: %+v", got)
	}
	if credited, err := f.svc.QuotaGiftCredited(ctx, 8, 9); err != nil || credited {
		t.Fatalf("another task's gift = %v, %v; want not credited", credited, err)
	}
}

// A gift of nothing only records that the stage ran; an overflowing one is
// refused without a trace.
func TestCreditQuotaGiftMarksZeroAndRefusesOverflow(t *testing.T) {
	f := newFacade(t)
	owner := f.h.User()
	ctx := context.Background()

	if err := f.svc.CreditQuotaGift(ctx, 7, 9, owner.Id, 0, time.Now()); err != nil {
		t.Fatalf("zero gift: %v", err)
	}
	if credited, err := f.svc.QuotaGiftCredited(ctx, 7, 9); err != nil || !credited {
		t.Fatalf("zero gift credited = %v, %v; want the marker", credited, err)
	}
	var wallets int64
	if err := f.h.DB.Model(&wallet.Wallet{}).Count(&wallets).Error; err != nil {
		t.Fatal(err)
	}
	if wallets != 0 || len(f.h.GiftLogs(owner.Id)) != 0 {
		t.Fatalf("a zero gift touched the wallet: %d wallets", wallets)
	}

	f.wallet(t, owner.Id, 0, math.MaxInt64-10, 0)
	if err := f.svc.CreditQuotaGift(ctx, 8, 9, owner.Id, 11, time.Now()); err == nil {
		t.Fatal("an overflowing gift was credited")
	}
	if credited, err := f.svc.QuotaGiftCredited(ctx, 8, 9); err != nil || credited {
		t.Fatalf("the refused gift credited = %v, %v; want no marker", credited, err)
	}
	if got := f.h.ReloadWallet(owner.Id); got.GiftAmount != math.MaxInt64-10 || len(f.h.GiftLogs(owner.Id)) != 0 {
		t.Fatalf("the refused gift moved money: %+v", got)
	}
}

// An administrator's wallet edit sets the amounts under the wallet lock and
// audits each one that changed; an unchanged wallet is left alone.
func TestAdjustWalletAuditsEachChange(t *testing.T) {
	f := newFacade(t)
	owner := f.h.User()
	f.wallet(t, owner.Id, 100, 50, 10)
	ctx := context.Background()

	if err := f.svc.AdjustWallet(ctx, wallet.Wallet{UserId: owner.Id, Balance: 100, GiftAmount: 50, Commission: 10}); err != nil {
		t.Fatalf("unchanged AdjustWallet: %v", err)
	}
	for _, typ := range []logEntity.Type{logEntity.TypeBalance, logEntity.TypeGift, logEntity.TypeCommission} {
		if rows := f.h.Logs(typ, owner.Id); len(rows) != 0 {
			t.Fatalf("an unchanged wallet was audited: %+v", rows)
		}
	}

	if err := f.svc.AdjustWallet(ctx, wallet.Wallet{UserId: owner.Id, Balance: 300, GiftAmount: 20, Commission: 10}); err != nil {
		t.Fatalf("AdjustWallet: %v", err)
	}
	if got := f.h.ReloadWallet(owner.Id); got.Balance != 300 || got.GiftAmount != 20 || got.Commission != 10 {
		t.Fatalf("wallet = %+v", got)
	}
	if logs := f.h.BalanceLogs(owner.Id); len(logs) != 1 || logs[0].Type != logEntity.BalanceTypeAdjust || logs[0].Amount != 200 || logs[0].Balance != 300 {
		t.Fatalf("balance logs = %+v", logs)
	}
	if logs := f.h.GiftLogs(owner.Id); len(logs) != 1 || logs[0].Type != logEntity.GiftTypeReduce || logs[0].Amount != -30 || logs[0].Balance != 20 || logs[0].Remark != "Admin adjustment" {
		t.Fatalf("gift logs = %+v", logs)
	}
	if logs := f.commissionLogs(t, owner.Id); len(logs) != 0 {
		t.Fatalf("commission logs = %+v, want none for an unchanged commission", logs)
	}

	if err := f.svc.AdjustWallet(ctx, wallet.Wallet{UserId: owner.Id, Balance: 300, GiftAmount: 20, Commission: 4}); err != nil {
		t.Fatalf("AdjustWallet: %v", err)
	}
	if logs := f.commissionLogs(t, owner.Id); len(logs) != 1 || logs[0].Type != logEntity.CommissionTypeAdjust || logs[0].Amount != -6 {
		t.Fatalf("commission logs = %+v", logs)
	}
}

// The opening wallet of an account an administrator created carries the
// amounts entered, and the wallet reads show them.
func TestOpenWalletSetsTheOpeningAmounts(t *testing.T) {
	f := newFacade(t)
	owner, other := f.h.User(), f.h.User()
	ctx := context.Background()

	if w, err := f.svc.FindWallet(ctx, owner.Id); err != nil || w != nil {
		t.Fatalf("FindWallet before the opening = %+v, %v; want none", w, err)
	}
	if err := f.svc.OpenWallet(ctx, wallet.Wallet{UserId: owner.Id, Balance: 500, GiftAmount: 200, Commission: 50}); err != nil {
		t.Fatalf("OpenWallet: %v", err)
	}
	if w, err := f.svc.FindWallet(ctx, owner.Id); err != nil || w == nil || w.Balance != 500 || w.GiftAmount != 200 || w.Commission != 50 {
		t.Fatalf("FindWallet = %+v, %v", w, err)
	}
	found, err := f.svc.FindWallets(ctx, []int64{owner.Id, other.Id})
	if err != nil || len(found) != 1 || found[owner.Id] == nil || found[owner.Id].Balance != 500 {
		t.Fatalf("FindWallets = %+v, %v; want only the opened wallet", found, err)
	}
}

// The subscription module reads orders through the facade: by id, by number
// and with the renewals a refund pays back.
func TestOrderReadsForTheSubscriptionModule(t *testing.T) {
	f := newFacade(t)
	buyer := f.h.User()
	o := f.h.Order(&order.Order{OrderNo: "A", UserId: buyer.Id, Type: order.TypeSubscribe, Status: order.StatusFinished, Amount: 1000})
	f.h.Order(&order.Order{ParentId: o.Id, OrderNo: "A-renewal", UserId: buyer.Id, Type: order.TypeRenewal, Status: order.StatusPaid, Amount: 800})
	ctx := context.Background()

	if got, err := f.svc.FindOrder(ctx, o.Id); err != nil || got.OrderNo != "A" {
		t.Fatalf("FindOrder = %+v, %v", got, err)
	}
	if got, err := f.svc.FindOrderByNo(ctx, "A-renewal"); err != nil || got.ParentId != o.Id {
		t.Fatalf("FindOrderByNo = %+v, %v", got, err)
	}
	details, err := f.svc.FindOrderDetails(ctx, o.Id)
	if err != nil || len(details.SubOrders) != 1 || details.RefundBasis() != 1800 {
		t.Fatalf("FindOrderDetails = %+v, %v; want the renewal in a basis of 1800", details, err)
	}
}
