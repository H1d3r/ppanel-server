package selfsub

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
)

// A subscription an administrator created has no order: cancelling it runs
// both stages but moves no money, and drops the cached subscription and
// plan.
func TestUnsubscribe_AdminCreatedSubscription_SkipsRefund(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	const owner int64 = 100
	f.Plan(t, subscribe.Subscribe{Id: 300})
	sub := f.Subscription(t, usersub.Subscribe{UserId: owner, SubscribeId: 300, ExpireTime: time.Now().Add(24 * time.Hour), Status: usersub.SubscribeStatusActive, Token: "admin-token"})
	if _, err := f.Store.UserSubscription().FindOneSubscribeByToken(ctx, "admin-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.Subscribe().FindOne(ctx, 300); err != nil {
		t.Fatal(err)
	}

	quote, err := f.svc.PreUnsubscribe(as(owner), &dto.PreUnsubscribeRequest{Id: sub.Id})
	if err != nil || quote.DeductionAmount != 0 {
		t.Fatalf("PreUnsubscribe = %+v, %v; want nothing to refund", quote, err)
	}
	if err := f.svc.Unsubscribe(as(owner), &dto.UnsubscribeRequest{Id: sub.Id}); err != nil {
		t.Fatalf("Unsubscribe() error = %v, want nil", err)
	}

	if got := f.Load(t, sub.Id); got.Status != usersub.SubscribeStatusDeducted {
		t.Fatalf("subscription status = %d, want Deducted", got.Status)
	}
	if result, ok := f.marker(t, unsubscribeCancelConsumer, sub.Id); !ok || result != "0|0" {
		t.Fatalf("cancellation marker = %q, %v; want no order and no refund", result, ok)
	}
	if _, ok := f.marker(t, unsubscribeRefundConsumer, sub.Id); !ok {
		t.Fatal("the settled refund stage left no marker")
	}
	if f.billing.locks != 0 || len(f.billing.wallets) != 0 {
		t.Fatalf("wallets touched: %d locks, %+v", f.billing.locks, f.billing.wallets)
	}
	for _, typ := range []log.Type{log.TypeBalance, log.TypeGift, log.TypeCommission} {
		if rows := f.Logs(t, typ); len(rows) != 0 {
			t.Fatalf("money movement logged: %+v", rows)
		}
	}
	if f.Cached("cache:user:subscribe:token:admin-token") || f.Cached("cache:subscribe:id:300") {
		t.Fatal("the cancelled subscription or its plan stays cached")
	}

	// A repeated request finds nothing left to cancel.
	if err := f.svc.Unsubscribe(as(owner), &dto.UnsubscribeRequest{Id: sub.Id}); !errors.Is(err, errNotCancelable) {
		t.Fatalf("repeated Unsubscribe = %v, want errNotCancelable", err)
	}
}
