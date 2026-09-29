package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	stripeSDK "github.com/stripe/stripe-go/v81"
)

// Registry opens the gateway of a payment method from its configuration. It
// is the only place that knows which platform maps to which protocol.
type Registry struct {
	httpClient       *http.Client
	stripeBackends   *stripeSDK.Backends
	cryptomusBaseURL string
}

// Option configures a Registry.
type Option func(*Registry)

// WithHTTPClient sends every gateway request through client. Tests inject a
// client that answers locally; nil keeps each protocol's default client.
func WithHTTPClient(client *http.Client) Option {
	return func(r *Registry) { r.httpClient = client }
}

// WithStripeBackends points the Stripe clients at backends; nil keeps the
// production API.
func WithStripeBackends(backends *stripeSDK.Backends) Option {
	return func(r *Registry) { r.stripeBackends = backends }
}

// WithCryptomusBaseURL points the Cryptomus clients at another API host.
// Production never sets it: the invoice query is the authoritative payment
// confirmation, so it must not be redirectable through the database
// configuration of a payment method.
func WithCryptomusBaseURL(baseURL string) Option {
	return func(r *Registry) { r.cryptomusBaseURL = baseURL }
}

// NewRegistry builds a registry of the supported gateways.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// platformSpec describes one supported gateway platform.
type platformSpec struct {
	open func(r *Registry, method *paymentEntity.Payment) (Gateway, error)
	// normalize validates an administrator's configuration and returns the
	// canonical form stored on the payment method.
	normalize func(raw []byte) (string, error)
	style     CallbackStyle
	// stableCheckoutClose requires a close to find the checkout unchanged
	// even when none had started: the gateway records the expectation
	// before it creates the payment, so a checkout may be in flight.
	stableCheckoutClose bool
}

var platforms = map[payment.Platform]platformSpec{
	payment.EPay:      {open: openEPay, normalize: normalizeEPay, style: CallbackStyle{UniqueParams: true, TextReply: true, TextFailure: true}},
	payment.AlipayF2F: {open: openAlipay, normalize: normalizeAlipay, style: CallbackStyle{TextReply: true}},
	payment.Stripe:    {open: openStripe, normalize: normalizeStripe, style: CallbackStyle{Body: true, StatusFailure: true}},
	payment.Cryptomus: {open: openCryptomus, normalize: normalizeCryptomus, style: CallbackStyle{Body: true, TextReply: true, TextFailure: true}, stableCheckoutClose: true},
}

// Handles reports whether orders of platform are paid through a gateway.
func (r *Registry) Handles(platform string) bool {
	_, ok := platforms[payment.ParsePlatform(platform)]
	return ok
}

// Collects reports whether billing itself collected the payments of method:
// the wallet balance or a gateway of this registry. A method a provider
// settles on its own, such as an app store, is not billing's to refund.
func Collects(method string) bool {
	platform := payment.ParsePlatform(method)
	if platform == payment.Balance {
		return true
	}
	_, ok := platforms[platform]
	return ok
}

// CloseWithoutCheckout is the verdict on closing an order of platform whose
// checkout never started: no gateway holds a payment for it, so it closes
// without consulting the gateway.
func (r *Registry) CloseWithoutCheckout(platform string) Reconciliation {
	return Reconciliation{RequireStableCheckout: platforms[payment.ParsePlatform(platform)].stableCheckoutClose}
}

// ErrNotGateway reports a payment method that is not served by a gateway:
// the internal balance method or an unsupported platform.
var ErrNotGateway = errors.New("payment method is not served by a gateway")

// Open returns the gateway of the payment method.
func (r *Registry) Open(method *paymentEntity.Payment) (Gateway, error) {
	if method == nil {
		return nil, ErrNotGateway
	}
	spec, ok := platforms[payment.ParsePlatform(method.Platform)]
	if !ok {
		return nil, ErrNotGateway
	}
	return spec.open(r, method)
}

// CallbackStyle returns how the platform's callbacks are delivered.
func (r *Registry) CallbackStyle(platform string) (CallbackStyle, bool) {
	spec, ok := platforms[payment.ParsePlatform(platform)]
	return spec.style, ok
}

// NormalizeConfig validates an administrator's configuration of a platform
// and returns the canonical form to store.
func (r *Registry) NormalizeConfig(platform string, config any) (string, error) {
	spec, ok := platforms[payment.ParsePlatform(platform)]
	if !ok {
		return "", ErrNotGateway
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return spec.normalize(raw)
}
