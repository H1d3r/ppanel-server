package fulfillment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/module/subscription/internal/period"
)

// rotateToken gives the subscription new credentials, as the owner's reset,
// an administrator's reset or the rotation of every token does between a
// checkout and its payment.
func (f *periodFixture) rotateToken(t *testing.T, id int64, token string) {
	t.Helper()
	if err := f.store.db.Model(&usersub.Subscribe{}).Where("id = ?", id).Updates(map[string]any{"token": token, "uuid": token + "-uuid"}).Error; err != nil {
		t.Fatal(err)
	}
}

// A renewal or reset order names its subscription by id, so a token rotated
// between checkout and payment does not orphan the paid order: the order is
// fulfilled and its replay rebuilds the notice.
func TestFulfillmentSurvivesATokenRotatedBetweenCheckoutAndPayment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		orderType uint8
		notify    string
	}{
		{"renewal", order.TypeRenewal, NotifyRenewal},
		{"traffic reset", order.TypeResetTraffic, NotifyResetTraffic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPeriodFixture(t)
			ctx := context.Background()
			now := time.Now()
			expire := now.Add(10 * 24 * time.Hour).Truncate(time.Millisecond)
			sub := &usersub.Subscribe{
				UserId: 7, SubscribeId: 1, StartTime: now.Add(-20 * 24 * time.Hour), ExpireTime: expire,
				Traffic: 100, Upload: 30, Download: 20, Token: "checkout-token", UUID: "checkout-uuid",
				Status: usersub.SubscribeStatusActive,
			}
			if err := f.store.db.Create(sub).Error; err != nil {
				t.Fatal(err)
			}
			f.orders.rows[9] = &order.Order{
				Id: 9, OrderNo: "rotated-9", UserId: 7, SubscribeId: 1, Type: tc.orderType, Status: order.StatusPaid, Quantity: 1,
				SubscribeToken: "checkout-token", UserSubscribeId: sub.Id,
			}
			f.rotateToken(t, sub.Id, "rotated-token")

			outcome, err := f.service.FulfillPaidOrder(ctx, "rotated-9")
			if err != nil {
				t.Fatalf("FulfillPaidOrder after the rotation: %v", err)
			}
			if outcome.UserID != 7 || outcome.NotifyKind != tc.notify || !outcome.HasSub {
				t.Fatalf("outcome = %+v", outcome)
			}
			got := f.sub(t, sub.Id)
			if got.Token != "rotated-token" {
				t.Fatalf("the fulfillment touched the credentials: %+v", got)
			}
			switch tc.orderType {
			case order.TypeRenewal:
				want, err := period.App().TermEnd(period.UnitMonth, 1, expire)
				if err != nil {
					t.Fatal(err)
				}
				if !got.ExpireTime.Equal(want) || got.Upload != 30 {
					t.Fatalf("renewal did not extend the rotated subscription: %+v", got)
				}
			case order.TypeResetTraffic:
				if got.Upload != 0 || got.Download != 0 || !got.ExpireTime.Equal(expire) {
					t.Fatalf("reset did not clear the rotated subscription: %+v", got)
				}
			}

			// The delivery is replayed once the fulfillment committed: the
			// notice is rebuilt through the same id and nothing is applied
			// again.
			replay, err := f.service.FulfillPaidOrder(ctx, "rotated-9")
			if err != nil {
				t.Fatalf("replay after the rotation: %v", err)
			}
			if replay.NotifyKind != tc.notify || replay.UserID != 7 || !replay.ExpireAt.Equal(got.ExpireTime) {
				t.Fatalf("replayed outcome = %+v, want the committed fulfillment's", replay)
			}
			if again := f.sub(t, sub.Id); !again.ExpireTime.Equal(got.ExpireTime) || again.Upload != got.Upload {
				t.Fatalf("the replay applied the order again: %+v", again)
			}
		})
	}
}

// An order created before orders carried the subscription id is still
// fulfilled through the token it saw at checkout; one that names neither is
// refused instead of resolving an empty token to some row.
func TestFulfillmentResolvesLegacyOrdersByToken(t *testing.T) {
	f := newPeriodFixture(t)
	ctx := context.Background()
	now := time.Now()
	sub := &usersub.Subscribe{
		UserId: 7, SubscribeId: 1, StartTime: now.Add(-24 * time.Hour), ExpireTime: now.Add(24 * time.Hour),
		Traffic: 100, Upload: 30, Token: "legacy-token", UUID: "legacy-uuid", Status: usersub.SubscribeStatusActive,
	}
	if err := f.store.db.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	// A row of an older version that stored no token must never be what an
	// order without a subscription resolves to.
	if err := f.store.db.Exec("INSERT INTO user_subscribe (user_id, subscribe_id, start_time, expire_time, token, uuid, status) VALUES (7, 1, ?, ?, '', 'blank-uuid', 1)", now.Add(-time.Hour), now.Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	f.orders.rows[4] = &order.Order{Id: 4, OrderNo: "legacy-4", UserId: 7, SubscribeId: 1, Type: order.TypeResetTraffic, Status: order.StatusPaid, SubscribeToken: "legacy-token"}
	f.orders.rows[5] = &order.Order{Id: 5, OrderNo: "blank-5", UserId: 7, SubscribeId: 1, Type: order.TypeRenewal, Status: order.StatusPaid, Quantity: 1}

	if _, err := f.service.FulfillPaidOrder(ctx, "legacy-4"); err != nil {
		t.Fatalf("legacy order by token: %v", err)
	}
	if got := f.sub(t, sub.Id); got.Upload != 0 {
		t.Fatalf("legacy reset order was not applied: %+v", got)
	}
	if _, err := f.service.FulfillPaidOrder(ctx, "blank-5"); !errors.Is(err, errOrderSubscription) {
		t.Fatalf("order naming no subscription = %v, want errOrderSubscription", err)
	}
	var blank usersub.Subscribe
	if err := f.store.db.Where("uuid = ?", "blank-uuid").First(&blank).Error; err != nil {
		t.Fatal(err)
	}
	if blank.ExpireTime.After(now.Add(2 * time.Hour)) {
		t.Fatalf("the token-less row was renewed by an order naming no subscription: %+v", blank)
	}
}
