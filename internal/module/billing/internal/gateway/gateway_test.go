package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

func TestNotifyURLPrefersThePaymentDomain(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		siteHost string
		want     string
	}{
		{"payment domain wins", "https://pay.example.test/", "www.example.test", "https://pay.example.test"},
		{"configured base path", "https://pay.example.test/custom/", "", "https://pay.example.test/custom"},
		{"site host fallback", "", "www.example.test/", "https://www.example.test"},
		{"site host with scheme and port", "", "http://panel.example.test:8080", "http://panel.example.test:8080"},
		{"first of several site hosts", "", "https://www.example.test/\nwww.example.org", "https://www.example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NotifyURL(&payment.Payment{Domain: tt.domain, Platform: "EPay", Token: "tok"}, tt.siteHost)
			if err != nil {
				t.Fatalf("NotifyURL error = %v", err)
			}
			if want := tt.want + "/v1/notify/EPay/tok"; got != want {
				t.Fatalf("NotifyURL = %q, want %q", got, want)
			}
		})
	}
}

// Without a payment domain or a reachable site host there is no trustworthy
// callback address; neither a listen address nor anything request-derived may
// fill the gap.
func TestNotifyURLFailsWithoutConfiguredHost(t *testing.T) {
	for _, host := range []string{"", "0.0.0.0", "[::]:8080", "127.0.0.1:8080", "localhost", "ftp://panel.example.test", "https://user@panel.example.test", "https://panel.example.test/?x=1"} {
		t.Run(host, func(t *testing.T) {
			got, err := NotifyURL(&payment.Payment{Id: 3, Platform: "EPay", Token: "tok"}, host)
			if xerr.CodeOf(err) != xerr.PaymentNotifyURLNotConfigured || got != "" {
				t.Fatalf("NotifyURL = (%q, %v), want PaymentNotifyURLNotConfigured", got, err)
			}
		})
	}
}

type methods struct {
	method *payment.Payment
	err    error
}

func (m methods) FindOne(context.Context, int64) (*payment.Payment, error) { return m.method, m.err }

// Every order flow reports a missing or unusable payment method the same way.
func TestLookupMethodMapsFailures(t *testing.T) {
	enabled, disabled := true, false
	ctx := context.Background()
	tests := []struct {
		name  string
		repo  methods
		code  uint32
		found bool
	}{
		{"missing", methods{err: gorm.ErrRecordNotFound}, xerr.PaymentMethodNotFound, false},
		{"lookup failure", methods{err: errors.New("connection reset")}, xerr.DatabaseQueryError, false},
		{"disabled", methods{method: &payment.Payment{Platform: "EPay", Enable: &disabled}}, xerr.PaymentMethodNotFound, false},
		{"unsupported platform", methods{method: &payment.Payment{Platform: "CryptoSaaS", Enable: &enabled}}, xerr.PaymentMethodNotFound, false},
		{"usable", methods{method: &payment.Payment{Platform: "EPay", Enable: &enabled}}, 0, true},
		{"balance is usable", methods{method: &payment.Payment{Platform: "balance", Enable: &enabled}}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, err := LookupMethod(ctx, tt.repo, 1)
			if tt.found {
				if err != nil || method == nil {
					t.Fatalf("LookupMethod = (%v, %v)", method, err)
				}
				return
			}
			if xerr.CodeOf(err) != tt.code {
				t.Fatalf("LookupMethod error = %v, want code %d", err, tt.code)
			}
		})
	}
}

type fixedRates struct {
	rate  float64
	err   error
	calls int
}

func (r *fixedRates) Rate(context.Context, string, string) (float64, error) {
	r.calls++
	return r.rate, r.err
}

func TestChargeForConvertsOnlyWhenTheGatewayCollectsAnotherCurrency(t *testing.T) {
	registry := NewRegistry()
	epay, err := registry.Open(&payment.Payment{Platform: "EPay", Config: `{"pid":"1","url":"https://pay.example","key":"k","type":"alipay"}`})
	if err != nil {
		t.Fatal(err)
	}
	stripe, err := registry.Open(&payment.Payment{Platform: "Stripe", Config: `{"secret_key":"sk","payment":"card"}`})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rates := &fixedRates{rate: 7.2419}

	if charge, err := ChargeFor(ctx, epay, 1990, "cny", rates); err != nil || charge != (Charge{Amount: 1990, Currency: "CNY"}) || rates.calls != 0 {
		t.Fatalf("CNY site with a CNY gateway = (%+v, %v), %d rate calls", charge, err, rates.calls)
	}
	// 1990 × 7.2419 = 14411.381 → 14411
	if charge, err := ChargeFor(ctx, epay, 1990, "USD", rates); err != nil || charge != (Charge{Amount: 14411, Currency: "CNY"}) {
		t.Fatalf("converted charge = (%+v, %v)", charge, err)
	}
	if charge, err := ChargeFor(ctx, stripe, 1990, "usd", rates); err != nil || charge != (Charge{Amount: 1990, Currency: "USD"}) {
		t.Fatalf("site-currency gateway = (%+v, %v)", charge, err)
	}
	// Stripe collects whole yen: the recorded expectation is rounded to what
	// it can charge, so the amount it reports back matches.
	if charge, err := ChargeFor(ctx, stripe, 100050, "JPY", rates); err != nil || charge != (Charge{Amount: 100100, Currency: "JPY"}) {
		t.Fatalf("zero-decimal Stripe charge = (%+v, %v), want 100100 JPY hundredths", charge, err)
	}
	if _, err := ChargeFor(ctx, epay, 1990, "USD", &fixedRates{err: errors.New("rate API down")}); err == nil {
		t.Fatal("a missing rate must refuse the charge")
	}
	if _, err := ChargeFor(ctx, epay, 1990, "USD", nil); err == nil {
		t.Fatal("no rate source must refuse the charge")
	}
}

func TestRegistryDescribesThePlatforms(t *testing.T) {
	registry := NewRegistry()
	for platform, want := range map[string]CallbackStyle{
		"EPay":      {UniqueParams: true, TextReply: true, TextFailure: true},
		"AlipayF2F": {TextReply: true},
		"Stripe":    {Body: true},
		"Cryptomus": {Body: true, TextReply: true, TextFailure: true},
	} {
		style, ok := registry.CallbackStyle(platform)
		if !ok || style != want || !registry.Handles(platform) {
			t.Fatalf("%s: style = (%+v, %t)", platform, style, ok)
		}
	}
	for _, platform := range []string{"balance", "CryptoSaaS", ""} {
		if _, ok := registry.CallbackStyle(platform); ok || registry.Handles(platform) {
			t.Fatalf("%q must not be served by a gateway", platform)
		}
		if _, err := registry.Open(&payment.Payment{Platform: platform}); !errors.Is(err, ErrNotGateway) {
			t.Fatalf("Open(%q) = %v, want ErrNotGateway", platform, err)
		}
	}
	if !registry.CloseWithoutCheckout("Cryptomus").RequireStableCheckout || registry.CloseWithoutCheckout("EPay").RequireStableCheckout {
		t.Fatal("only Cryptomus guards a close before its checkout started")
	}
}

func TestRegistryValidatesConfigurations(t *testing.T) {
	registry := NewRegistry()
	valid := map[string]any{
		"EPay":      map[string]any{"pid": "1", "url": "https://pay.example", "key": "k", "type": "alipay"},
		"Stripe":    map[string]any{"secret_key": "sk", "public_key": "pk", "webhook_secret": "wh", "payment": "card"},
		"AlipayF2F": map[string]any{"app_id": "app", "private_key": "pk", "public_key": "pub", "sandbox": "true"},
		"Cryptomus": map[string]any{"merchant_id": " m ", "api_key": " k "},
	}
	for platform, config := range valid {
		if normalized, err := registry.NormalizeConfig(platform, config); err != nil || normalized == "" {
			t.Fatalf("%s: NormalizeConfig = (%q, %v)", platform, normalized, err)
		}
	}
	if normalized, _ := registry.NormalizeConfig("Cryptomus", valid["Cryptomus"]); normalized != `{"merchant_id":"m","api_key":"k"}` {
		t.Fatalf("Cryptomus credentials are not trimmed: %s", normalized)
	}
	if _, err := registry.NormalizeConfig("balance", map[string]any{}); err == nil {
		t.Fatal("the balance method has no gateway configuration")
	}
	if _, err := registry.NormalizeConfig("EPay", "not an object"); err == nil {
		t.Fatal("a malformed configuration must be rejected")
	}
	for platform, config := range map[string]string{
		"EPay":      `{"pid":"","url":"https://pay.example","key":"k"}`,
		"Cryptomus": `{"merchant_id":"m"}`,
		"AlipayF2F": `{"app_id":"app","private_key":"not a key"}`,
	} {
		if _, err := registry.Open(&payment.Payment{Platform: platform, Config: config}); err == nil {
			t.Fatalf("%s: an unusable configuration opened a gateway", platform)
		}
	}
}

// A Cryptomus checkout that never started closes, but only if it is still
// not started when the close commits; the other gateways need no guard.
func TestCryptomusReconcileWithoutCheckoutRequiresStableCheckout(t *testing.T) {
	gw, err := NewRegistry().Open(&payment.Payment{Platform: "Cryptomus", Config: `{"merchant_id":"m","api_key":"k"}`})
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := gw.Reconcile(context.Background(), CloseRequest{Order: &order.Order{OrderNo: "o"}})
	if err != nil || !verdict.RequireStableCheckout || verdict.TradeNo != "" {
		t.Fatalf("Reconcile = (%+v, %v)", verdict, err)
	}
}
