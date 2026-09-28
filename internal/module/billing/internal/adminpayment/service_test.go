package adminpayment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	paymentModel "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/gateway"
	"github.com/perfect-panel/server/pkg/xerr"
	stripeSDK "github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

// memoryPayments is an in-memory payment-method store.
type memoryPayments struct {
	rows      map[int64]*paymentModel.Payment
	nextID    int64
	insertErr error
	updated   *paymentModel.Payment
	deleted   []int64
}

var _ Payments = (*memoryPayments)(nil)

func newMemoryPayments(rows ...*paymentModel.Payment) *memoryPayments {
	m := &memoryPayments{rows: map[int64]*paymentModel.Payment{}, nextID: 100}
	for _, row := range rows {
		m.rows[row.Id] = row
	}
	return m
}

func (m *memoryPayments) Insert(_ context.Context, data *paymentModel.Payment) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.nextID++
	data.Id = m.nextID
	copy := *data
	m.rows[data.Id] = &copy
	return nil
}

func (m *memoryPayments) FindOne(_ context.Context, id int64) (*paymentModel.Payment, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *row
	return &copy, nil
}

func (m *memoryPayments) Update(_ context.Context, data *paymentModel.Payment) error {
	m.updated = data
	copy := *data
	m.rows[data.Id] = &copy
	return nil
}

func (m *memoryPayments) Delete(_ context.Context, id int64) error {
	m.deleted = append(m.deleted, id)
	delete(m.rows, id)
	return nil
}

func (m *memoryPayments) FindListByPage(context.Context, int, int, *paymentModel.Filter) (int64, []*paymentModel.Payment, error) {
	list := make([]*paymentModel.Payment, 0, len(m.rows))
	for _, row := range m.rows {
		list = append(list, row)
	}
	return int64(len(list)), list, nil
}

type pendingOrders struct {
	pending int64
	calls   int
}

var _ PendingOrders = (*pendingOrders)(nil)

func (r *pendingOrders) CountPendingByPaymentID(context.Context, int64) (int64, error) {
	r.calls++
	return r.pending, nil
}

func assertCode(t *testing.T, err error, want uint32) {
	t.Helper()
	if got := xerr.CodeOf(err); err == nil || got != want {
		t.Fatalf("error = %v (code %d), want code %d", err, got, want)
	}
}

func TestCryptomusConfigRequiresBothCredentials(t *testing.T) {
	tests := []struct {
		name   string
		config any
	}{
		{"empty object", map[string]any{}},
		{"missing merchant", map[string]any{"api_key": "test-key"}},
		{"missing key", map[string]any{"merchant_id": "merchant-1"}},
		{"blank merchant", map[string]any{"merchant_id": " \n", "api_key": "test-key"}},
		{"blank key", map[string]any{"merchant_id": "merchant-1", "api_key": " \t"}},
		{"wrong field type", map[string]any{"merchant_id": 123, "api_key": "test-key"}},
		{"null", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enable := true
			repo := newMemoryPayments(&paymentModel.Payment{
				Id: 1, Platform: "Cryptomus", Config: `{"merchant_id":"merchant-1","api_key":"test-key"}`, Enable: &enable,
			})
			orders := &pendingOrders{pending: 1}
			svc := NewService(Deps{Payments: repo, Orders: orders})
			_, err := svc.Create(context.Background(), &dto.CreatePaymentMethodRequest{
				Name: "Cryptomus", Platform: "Cryptomus", Config: test.config, Enable: &enable,
			})
			assertCode(t, err, xerr.InvalidPaymentConfig)
			if !strings.Contains(err.Error(), "INVALID_PAYMENT_CONFIG") {
				t.Fatalf("the message must keep the historical identifier: %v", err)
			}
			_, err = svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
				Id: 1, Name: "Cryptomus", Platform: "Cryptomus", Config: test.config, Enable: &enable,
			})
			assertCode(t, err, xerr.InvalidPaymentConfig)
			if repo.updated != nil || orders.calls != 0 || len(repo.rows) != 1 {
				t.Fatal("invalid config must not reach persistence or the pending-order check")
			}
		})
	}
}

func TestCryptomusConfigTrimsCredentials(t *testing.T) {
	enable := true
	repo := newMemoryPayments(&paymentModel.Payment{Id: 1, Platform: "Cryptomus", Config: `{"merchant_id":"old","api_key":"old"}`, Enable: &enable})
	svc := NewService(Deps{Payments: repo, Orders: &pendingOrders{}})
	if _, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id: 1, Name: "Cryptomus", Platform: "Cryptomus", Enable: &enable,
		Config: map[string]any{"merchant_id": " merchant-1 \n", "api_key": "\ttest-key "},
	}); err != nil {
		t.Fatal(err)
	}
	var config paymentModel.CryptomusConfig
	if err := json.Unmarshal([]byte(repo.updated.Config), &config); err != nil {
		t.Fatal(err)
	}
	if config.MerchantID != "merchant-1" || config.APIKey != "test-key" {
		t.Fatalf("credentials must be trimmed before persisting: %+v", config)
	}
}

// The seeded balance method (config is an empty string) must be toggleable:
// it has no platform config to validate and no callback, so the update must
// neither fail with INVALID_PAYMENT_CONFIG nor hit the pending-order guard.
func TestUpdateBalancePaymentMethodTogglesEnable(t *testing.T) {
	enable := true
	repo := newMemoryPayments(&paymentModel.Payment{Id: -1, Name: "Balance", Platform: "balance", Enable: new(bool)})
	orders := &pendingOrders{pending: 1}
	svc := NewService(Deps{Payments: repo, Orders: orders})

	if _, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id: -1, Name: "Balance", Platform: "balance", Config: map[string]any{}, Enable: &enable,
	}); err != nil {
		t.Fatalf("Update error = %v, want success", err)
	}
	if repo.updated == nil || repo.updated.Enable == nil || !*repo.updated.Enable {
		t.Fatal("Enable was not toggled on")
	}
	if repo.updated.Config != "" {
		t.Fatalf("Config = %q, want stored config preserved", repo.updated.Config)
	}
	if orders.calls != 0 {
		t.Fatalf("CountPendingByPaymentID calls = %d, want 0 for balance", orders.calls)
	}
}

// The storefront depends on the seeded balance method (id -1); deleting it
// breaks every balance purchase with a record-not-found at PreCreateOrder.
func TestDeleteBalancePaymentMethodIsRejected(t *testing.T) {
	repo := newMemoryPayments(&paymentModel.Payment{Id: -1, Name: "Balance", Platform: "balance", Enable: new(bool)})
	svc := NewService(Deps{Payments: repo, Orders: &pendingOrders{}})

	err := svc.Delete(context.Background(), &dto.DeletePaymentMethodRequest{Id: -1})
	assertCode(t, err, xerr.PaymentMethodInternal)
	if !strings.Contains(err.Error(), "cannot be deleted") || len(repo.deleted) != 0 {
		t.Fatalf("Delete error = %v, deleted %v", err, repo.deleted)
	}
}

func TestPaymentMethodGuardsReportDeclaredCodes(t *testing.T) {
	enable := true
	epay := &paymentModel.Payment{Id: 2, Name: "EPay", Platform: "EPay", Config: `{"pid":"1","url":"https://pay.example","key":"k","type":"alipay"}`, Enable: &enable}
	ctx := context.Background()

	svc := NewService(Deps{Payments: newMemoryPayments(epay), Orders: &pendingOrders{pending: 2}})
	_, err := svc.Create(ctx, &dto.CreatePaymentMethodRequest{Name: "x", Platform: "Nope", Enable: &enable})
	assertCode(t, err, xerr.UnsupportedPaymentPlatform)
	_, err = svc.Create(ctx, &dto.CreatePaymentMethodRequest{Name: "x", Platform: "EPay", FeeMode: 9, Enable: &enable})
	assertCode(t, err, xerr.InvalidPaymentFee)
	_, err = svc.Update(ctx, &dto.UpdatePaymentMethodRequest{Id: 2, Name: "EPay", Platform: "Stripe", Config: map[string]any{}, Enable: &enable})
	assertCode(t, err, xerr.PaymentPlatformImmutable)
	_, err = svc.Update(ctx, &dto.UpdatePaymentMethodRequest{Id: 2, Name: "EPay", Platform: "EPay", Domain: "https://new.example", Enable: &enable,
		Config: map[string]any{"pid": "1", "url": "https://pay.example", "key": "k", "type": "alipay"}})
	assertCode(t, err, xerr.PaymentMethodHasPendingOrders)
	assertCode(t, svc.Delete(ctx, &dto.DeletePaymentMethodRequest{Id: 2}), xerr.PaymentMethodHasPendingOrders)
	_, err = svc.Update(ctx, &dto.UpdatePaymentMethodRequest{Id: 99, Name: "EPay", Platform: "EPay", Config: map[string]any{}, Enable: &enable})
	assertCode(t, err, xerr.DatabaseQueryError)
}

func TestUpdateGatewayPaymentMethodStillValidatesConfig(t *testing.T) {
	enable := true
	repo := newMemoryPayments(&paymentModel.Payment{Id: 1, Name: "EPay", Platform: "EPay", Enable: new(bool)})
	svc := NewService(Deps{Payments: repo, Orders: &pendingOrders{}})

	_, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id: 1, Name: "EPay", Platform: "EPay", Config: "not-a-config", Enable: &enable,
	})
	assertCode(t, err, xerr.InvalidPaymentConfig)
}

// fakeStripeWebhooks is the Stripe webhook endpoint API.
type fakeStripeWebhooks struct {
	mu        sync.Mutex
	endpoints map[string]string
	keys      []string
}

func (f *fakeStripeWebhooks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = r.ParseForm()
	f.keys = append(f.keys, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/webhook_endpoints":
		f.endpoints["we_1"] = r.PostForm.Get("url")
		_, _ = w.Write([]byte(`{"id":"we_1","object":"webhook_endpoint","secret":"whsec_created"}`))
	case r.Method == http.MethodDelete && r.URL.Path == "/v1/webhook_endpoints/we_1":
		delete(f.endpoints, "we_1")
		_, _ = w.Write([]byte(`{"id":"we_1","object":"webhook_endpoint","deleted":true}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func stripeRegistry(t *testing.T) (*gateway.Registry, *fakeStripeWebhooks) {
	t.Helper()
	fake := &fakeStripeWebhooks{endpoints: map[string]string{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	backends := stripeSDK.NewBackendsWithConfig(&stripeSDK.BackendConfig{
		URL: stripeSDK.String(server.URL), HTTPClient: server.Client(), MaxNetworkRetries: stripeSDK.Int64(0),
		LeveledLogger: &stripeSDK.LeveledLogger{Level: stripeSDK.LevelNull},
	})
	return gateway.NewRegistry(gateway.WithStripeBackends(backends)), fake
}

// The Stripe webhook endpoint is registered at the notify URL checkout uses
// and its secret is stored with the method; the call happens outside any
// transaction.
func TestCreateStripeMethodRegistersItsWebhookEndpoint(t *testing.T) {
	registry, fake := stripeRegistry(t)
	repo := newMemoryPayments()
	enable := true
	svc := NewService(Deps{Payments: repo, Orders: &pendingOrders{}, Gateways: registry,
		NotifyHosts: func() gateway.NotifyHosts {
			return gateway.NotifyHosts{Host: "0.0.0.0", SiteHost: "panel.example.test"}
		}})

	resp, err := svc.Create(context.Background(), &dto.CreatePaymentMethodRequest{
		Name: "Stripe", Platform: "Stripe", Enable: &enable,
		Config: map[string]any{"secret_key": "sk_test_1", "public_key": "pk_test_1", "payment": "card"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored := repo.rows[resp.Id]
	var config paymentModel.StripeConfig
	if err := config.Unmarshal([]byte(stored.Config)); err != nil || config.WebhookSecret != "whsec_created" {
		t.Fatalf("stored config = %q (%v), want the endpoint secret", stored.Config, err)
	}
	if got, want := fake.endpoints["we_1"], "https://panel.example.test/v1/notify/Stripe/"+stored.Token; got != want {
		t.Fatalf("webhook URL = %q, want %q", got, want)
	}
	if fake.keys[0] != "sk_test_1" {
		t.Fatalf("webhook created with key %q", fake.keys[0])
	}
}

// An endpoint whose payment method could not be saved is removed again, so
// no orphaned endpoint keeps receiving events.
func TestCreateStripeMethodRemovesEndpointWhenSaveFails(t *testing.T) {
	registry, fake := stripeRegistry(t)
	repo := newMemoryPayments()
	repo.insertErr = errors.New("database unavailable")
	enable := true
	svc := NewService(Deps{Payments: repo, Orders: &pendingOrders{}, Gateways: registry,
		NotifyHosts: func() gateway.NotifyHosts { return gateway.NotifyHosts{SiteHost: "panel.example.test"} }})

	_, err := svc.Create(context.Background(), &dto.CreatePaymentMethodRequest{
		Name: "Stripe", Platform: "Stripe", Enable: &enable,
		Config: map[string]any{"secret_key": "sk_test_1", "payment": "card"},
	})
	assertCode(t, err, xerr.DatabaseInsertError)
	if len(fake.endpoints) != 0 {
		t.Fatalf("orphaned endpoints = %v", fake.endpoints)
	}
}

// Without a domain or site host there is no callback to register.
func TestCreateStripeMethodRequiresANotifyURL(t *testing.T) {
	registry, fake := stripeRegistry(t)
	enable := true
	svc := NewService(Deps{Payments: newMemoryPayments(), Orders: &pendingOrders{}, Gateways: registry,
		NotifyHosts: func() gateway.NotifyHosts { return gateway.NotifyHosts{Host: "0.0.0.0"} }})
	_, err := svc.Create(context.Background(), &dto.CreatePaymentMethodRequest{
		Name: "Stripe", Platform: "Stripe", Enable: &enable,
		Config: map[string]any{"secret_key": "sk_test_1", "payment": "card"},
	})
	assertCode(t, err, xerr.PaymentNotifyURLNotConfigured)
	if len(fake.keys) != 0 {
		t.Fatal("no Stripe call may happen without a callback URL")
	}
}
