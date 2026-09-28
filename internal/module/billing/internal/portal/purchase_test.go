package portal

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	order2 "github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type guestAuthLookup struct {
	lookups []string
}

func (r *guestAuthLookup) FindUserAuthMethodByOpenID(_ context.Context, method, openID string) (*userEntity.AuthMethods, error) {
	r.lookups = append(r.lookups, method+"|"+openID)
	return &userEntity.AuthMethods{}, gorm.ErrRecordNotFound
}

type guestPurchaseOrders struct {
	repository.OrderRepo
	pending        int64
	pendingQueries []string
	inserted       []*order2.Order
}

func (r *guestPurchaseOrders) CountPendingGuestOrders(_ context.Context, authType, identifier string) (int64, error) {
	r.pendingQueries = append(r.pendingQueries, authType+"|"+identifier)
	return r.pending, nil
}

func (r *guestPurchaseOrders) Insert(_ context.Context, data *order2.Order, _ ...*gorm.DB) error {
	copy := *data
	r.inserted = append(r.inserted, &copy)
	return nil
}

type guestPurchaseLogs struct {
	repository.LogRepo
}

func (guestPurchaseLogs) Insert(context.Context, *logEntity.SystemLog) error { return nil }

type guestPurchaseStore struct {
	Store
	orders *guestPurchaseOrders
}

func (s *guestPurchaseStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(guestPurchaseTx{orders: s.orders})
}

type guestPurchaseTx struct {
	repository.BillingStore
	orders *guestPurchaseOrders
}

func (tx guestPurchaseTx) Order() repository.OrderRepo { return tx.orders }
func (tx guestPurchaseTx) Log() repository.LogRepo     { return guestPurchaseLogs{} }

type guestPlans struct {
	PlanReader
}

func (guestPlans) FindOne(_ context.Context, id int64) (*subscribe.Subscribe, error) {
	sell := true
	return &subscribe.Subscribe{Id: id, Sell: &sell, Inventory: -1, UnitPrice: 1000}, nil
}

type guestPayments struct {
	repository.PaymentRepo
}

func (guestPayments) FindOne(_ context.Context, id int64) (*payment.Payment, error) {
	enabled := true
	return &payment.Payment{Id: id, Platform: "EPay", Enable: &enabled}, nil
}

type guestInventory struct{}

func (guestInventory) Reserve(context.Context, string, int64) error { return nil }

type guestQueue struct{}

func (guestQueue) EnqueueDeferredClose(context.Context, string) error { return nil }

type guestPurchaseFixture struct {
	svc    *Service
	auths  *guestAuthLookup
	orders *guestPurchaseOrders
}

func newGuestPurchaseFixture(verification *GuestVerification, verify TurnstileVerifier) *guestPurchaseFixture {
	f := &guestPurchaseFixture{auths: &guestAuthLookup{}, orders: &guestPurchaseOrders{}}
	config := Config{}
	if verification != nil {
		policy := *verification
		config.GuestVerification = func() GuestVerification { return policy }
	}
	f.svc = NewService(Deps{
		Orders:          f.orders,
		Payments:        guestPayments{},
		UserAuths:       f.auths,
		Plans:           guestPlans{},
		Store:           &guestPurchaseStore{orders: f.orders},
		Inventory:       guestInventory{},
		Queue:           guestQueue{},
		Config:          config,
		VerifyTurnstile: verify,
	})
	return f
}

func guestPurchaseRequest(authType, identifier string) *dto.PortalPurchaseRequest {
	return &dto.PortalPurchaseRequest{
		AuthType: authType, Identifier: identifier, Password: "guest-password",
		Payment: 3, SubscribeId: 9, Quantity: 1,
	}
}

func errCode(err error) uint32 {
	var codeErr *xerr.CodeError
	if errors.As(errors.Cause(err), &codeErr) {
		return codeErr.GetErrCode()
	}
	return 0
}

// The paid order inserts the guest auth method as given and issues a session,
// so an OAuth or device identifier would pre-bind someone else's identity.
func TestPortalPurchaseRejectsNonPasswordGuestAuthTypes(t *testing.T) {
	for _, authType := range []string{"github", "telegram", "google", "apple", "facebook", "device", ""} {
		t.Run(authType, func(t *testing.T) {
			f := newGuestPurchaseFixture(nil, nil)
			_, err := f.svc.Purchase(context.Background(), guestPurchaseRequest(authType, "123456789"))
			if errCode(err) != xerr.InvalidParams {
				t.Fatalf("Purchase error = %v, want InvalidParams", err)
			}
			if len(f.auths.lookups) != 0 || len(f.orders.inserted) != 0 {
				t.Fatalf("rejected identity reached lookups %v or created %d orders", f.auths.lookups, len(f.orders.inserted))
			}
		})
	}
}

func TestPortalPurchaseRejectsMalformedGuestIdentifiers(t *testing.T) {
	tests := []struct{ authType, identifier string }{
		{"email", "not-an-email"},
		{"email", "Alice <alice@example.com>"},
		{"email", ""},
		{"mobile", "12345"},
		{"mobile", "not-a-number"},
		{"mobile", ""},
	}
	for _, tt := range tests {
		t.Run(tt.authType+"/"+tt.identifier, func(t *testing.T) {
			f := newGuestPurchaseFixture(nil, nil)
			_, err := f.svc.Purchase(context.Background(), guestPurchaseRequest(tt.authType, tt.identifier))
			if errCode(err) != xerr.InvalidParams {
				t.Fatalf("Purchase error = %v, want InvalidParams", err)
			}
			if len(f.auths.lookups) != 0 || len(f.orders.inserted) != 0 {
				t.Fatalf("malformed identity reached lookups %v or created %d orders", f.auths.lookups, len(f.orders.inserted))
			}
		})
	}
}

// The existence check and the persisted guest identity use the same
// canonical form the account flows store, so variants cannot bypass the
// check or create a second account for one address.
func TestPortalPurchaseCanonicalizesGuestIdentity(t *testing.T) {
	tests := []struct {
		name, authType, identifier string
		wantType, wantIdentifier   string
	}{
		{"email", "email", "  Alice@Example.COM ", "email", "alice@example.com"},
		{"auth type case", " Email ", "alice@example.com", "email", "alice@example.com"},
		{"mobile with spacing", "mobile", "+86 155 0250 5555", "mobile", "+8615502505555"},
		{"mobile without plus", "mobile", "8615502505555", "mobile", "+8615502505555"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGuestPurchaseFixture(nil, nil)
			if _, err := f.svc.Purchase(context.Background(), guestPurchaseRequest(tt.authType, tt.identifier)); err != nil {
				t.Fatalf("Purchase: %v", err)
			}
			want := tt.wantType + "|" + tt.wantIdentifier
			if len(f.auths.lookups) != 1 || f.auths.lookups[0] != want {
				t.Fatalf("existence lookups = %v, want [%s]", f.auths.lookups, want)
			}
			if len(f.orders.pendingQueries) != 1 || f.orders.pendingQueries[0] != want {
				t.Fatalf("pending order queries = %v, want [%s]", f.orders.pendingQueries, want)
			}
			if len(f.orders.inserted) != 1 {
				t.Fatalf("inserted orders = %d, want 1", len(f.orders.inserted))
			}
			if got := f.orders.inserted[0]; got.GuestAuthType != tt.wantType || got.GuestIdentifier != tt.wantIdentifier {
				t.Fatalf("persisted guest identity = (%q, %q), want (%q, %q)", got.GuestAuthType, got.GuestIdentifier, tt.wantType, tt.wantIdentifier)
			}
		})
	}
}

func TestPortalPurchaseVerifiesTurnstileWhenEnabled(t *testing.T) {
	type call struct{ secret, token, ip string }
	tests := []struct {
		name      string
		policy    GuestVerification
		token     string
		verified  bool
		wantErr   bool
		wantCalls int
	}{
		{name: "disabled skips the challenge", policy: GuestVerification{Enabled: false, Secret: "secret"}},
		{name: "missing token", policy: GuestVerification{Enabled: true, Secret: "secret"}, wantErr: true},
		{name: "missing secret fails closed", policy: GuestVerification{Enabled: true}, token: "token", wantErr: true},
		{name: "rejected token", policy: GuestVerification{Enabled: true, Secret: "secret"}, token: "token", wantErr: true, wantCalls: 1},
		{name: "verified token", policy: GuestVerification{Enabled: true, Secret: "secret"}, token: "token", verified: true, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			f := newGuestPurchaseFixture(&tt.policy, func(_ context.Context, secret, token, ip string) (bool, error) {
				calls = append(calls, call{secret, token, ip})
				return tt.verified, nil
			})
			ctx := requestmeta.With(context.Background(), requestmeta.New("203.0.113.7", "test-agent"))
			req := guestPurchaseRequest("email", "guest@example.com")
			req.TurnstileToken = tt.token

			_, err := f.svc.Purchase(ctx, req)
			if tt.wantErr {
				if errCode(err) != xerr.TooManyRequests {
					t.Fatalf("Purchase error = %v, want TooManyRequests", err)
				}
				if len(f.auths.lookups) != 0 || len(f.orders.inserted) != 0 {
					t.Fatal("an unverified guest purchase must not look up accounts or create orders")
				}
			} else if err != nil {
				t.Fatalf("Purchase: %v", err)
			}
			if len(calls) != tt.wantCalls {
				t.Fatalf("verifier calls = %d, want %d", len(calls), tt.wantCalls)
			}
			if len(calls) == 1 && calls[0] != (call{"secret", "token", "203.0.113.7"}) {
				t.Fatalf("verifier call = %+v", calls[0])
			}
		})
	}
}

func TestPortalPurchaseCapsPendingGuestOrders(t *testing.T) {
	f := newGuestPurchaseFixture(nil, nil)
	f.orders.pending = maxPendingGuestOrders
	_, err := f.svc.Purchase(context.Background(), guestPurchaseRequest("email", "guest@example.com"))
	if errCode(err) != xerr.TooManyRequests {
		t.Fatalf("Purchase error = %v, want TooManyRequests", err)
	}
	if len(f.orders.inserted) != 0 {
		t.Fatal("capped guest identity created another pending order")
	}

	f.orders.pending = maxPendingGuestOrders - 1
	if _, err := f.svc.Purchase(context.Background(), guestPurchaseRequest("email", "guest@example.com")); err != nil {
		t.Fatalf("Purchase below the cap: %v", err)
	}
	if len(f.orders.inserted) != 1 {
		t.Fatalf("inserted orders = %d, want 1", len(f.orders.inserted))
	}
}
