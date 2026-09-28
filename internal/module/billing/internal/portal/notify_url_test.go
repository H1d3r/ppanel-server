package portal

import (
	"context"
	stderrors "errors"
	"net/url"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	order2 "github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func TestPaymentPublicBaseURL(t *testing.T) {
	tests := []struct {
		name        string
		domain      string
		requestHost string
		configHost  string
		siteHost    string
		want        string
	}{
		{"payment domain wins", "https://pay.example.test/", "request.example.test", "panel.example.test", "www.example.test", "https://pay.example.test"},
		{"configured base path", "https://pay.example.test/custom/", "", "panel.example.test", "", "https://pay.example.test/custom"},
		{"request host is never trusted", "", "attacker.example.test", "panel.example.test/", "", "https://panel.example.test"},
		{"config host fallback", "", "", "panel.example.test/", "", "https://panel.example.test"},
		{"bind address falls back to site host", "", "attacker.example.test", "0.0.0.0", "https://www.example.test/", "https://www.example.test"},
		{"bare site host", "", "", "", "www.example.test", "https://www.example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.requestHost != "" {
				ctx = context.WithValue(ctx, requestctx.CtxKeyRequestHost, tt.requestHost)
			}
			logic := NewPurchaseCheckoutLogic(ctx, CheckoutDependencies{
				Config: CheckoutConfig{Host: tt.configHost, SiteHost: tt.siteHost},
			})
			got, err := logic.paymentPublicBaseURL(&payment.Payment{Domain: tt.domain})
			if err != nil {
				t.Fatalf("paymentPublicBaseURL error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("paymentPublicBaseURL = %q, want %q", got, tt.want)
			}
		})
	}
}

// Without a payment Domain or a reachable configured host there is no
// trustworthy callback address; the request Host must not fill the gap.
func TestPaymentPublicBaseURLFailsWithoutConfiguredHost(t *testing.T) {
	for _, configHost := range []string{"", "0.0.0.0", "[::]:8080", "127.0.0.1:8080", "localhost"} {
		t.Run(configHost, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), requestctx.CtxKeyRequestHost, "attacker.example.test")
			logic := NewPurchaseCheckoutLogic(ctx, CheckoutDependencies{Config: CheckoutConfig{Host: configHost}})
			got, err := logic.paymentPublicBaseURL(&payment.Payment{Id: 3})
			if !stderrors.Is(err, errNotifyURLNotConfigured) {
				t.Fatalf("paymentPublicBaseURL = (%q, %v), want errNotifyURLNotConfigured", got, err)
			}
		})
	}
}

type notifyCheckoutStore struct {
	CheckoutStore
	order        *order2.Order
	payment      *payment.Payment
	expectations int
}

func (s *notifyCheckoutStore) FindOrderByOrderNo(_ context.Context, _ string) (*order2.Order, error) {
	copy := *s.order
	return &copy, nil
}

func (s *notifyCheckoutStore) FindPayment(_ context.Context, _ int64) (*payment.Payment, error) {
	copy := *s.payment
	return &copy, nil
}

func (s *notifyCheckoutStore) UpdatePaymentExpectation(_ context.Context, _ string, amount int64, currency string) (bool, error) {
	s.expectations++
	s.order.PaymentAmount, s.order.PaymentCurrency = amount, currency
	return true, nil
}

// epayNotifyCheckout drives a guest EPay checkout whose request carries an
// attacker-chosen Host header.
func epayNotifyCheckout(t *testing.T, config CheckoutConfig) (*notifyCheckoutStore, *dto.CheckoutOrderResponse, error) {
	t.Helper()
	enabled := true
	store := &notifyCheckoutStore{
		order: &order2.Order{
			OrderNo: "guest-epay", Status: 1, Amount: 1000, PaymentId: 3, Method: "EPay",
			GuestCheckoutTokenHash: order2.CheckoutTokenHash("guest-capability"),
		},
		payment: &payment.Payment{
			Id: 3, Platform: "EPay", Token: "notify-token", Enable: &enabled,
			Config: `{"pid":"1001","url":"https://gateway.example.test","key":"secret","type":"alipay"}`,
		},
	}
	config.CurrencyUnit = "CNY"
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyRequestHost, "attacker.example.test")
	logic := NewPurchaseCheckoutLogic(ctx, CheckoutDependencies{Store: store, Config: config})
	resp, err := logic.PurchaseCheckout(&dto.CheckoutOrderRequest{OrderNo: "guest-epay", CheckoutToken: "guest-capability"})
	return store, resp, err
}

func TestEPayCheckoutNotifyURLIgnoresRequestHost(t *testing.T) {
	_, resp, err := epayNotifyCheckout(t, CheckoutConfig{Host: "0.0.0.0", SiteHost: "www.example.test"})
	if err != nil {
		t.Fatalf("PurchaseCheckout: %v", err)
	}
	checkoutURL, err := url.Parse(resp.CheckoutUrl)
	if err != nil {
		t.Fatalf("parse checkout URL: %v", err)
	}
	if got, want := checkoutURL.Query().Get("notify_url"), "https://www.example.test/v1/notify/EPay/notify-token"; got != want {
		t.Fatalf("notify_url = %q, want %q", got, want)
	}
}

// A missing callback configuration fails the checkout with a clear error and
// before the payment expectation is recorded, so the order does not look as
// if it had been sent to the gateway.
func TestEPayCheckoutFailsWithoutConfiguredNotifyHost(t *testing.T) {
	store, _, err := epayNotifyCheckout(t, CheckoutConfig{Host: "0.0.0.0"})
	var codeErr *xerr.CodeError
	if !errors.As(errors.Cause(err), &codeErr) || codeErr.GetErrMsg() != "PAYMENT_NOTIFY_URL_NOT_CONFIGURED" {
		t.Fatalf("PurchaseCheckout error = %v, want the notify URL configuration error", err)
	}
	if store.expectations != 0 || store.order.PaymentCurrency != "" {
		t.Fatalf("payment expectation recorded despite the configuration error: %+v", store.order)
	}
}
