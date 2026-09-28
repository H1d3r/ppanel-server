package gateway

import (
	"net"
	"net/url"
	"strings"

	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/pkg/xerr"
	pkgerrors "github.com/pkg/errors"
)

// NotifyHosts are the operator-configured hosts a callback URL may be built
// on when a payment method has no domain of its own.
type NotifyHosts struct {
	// Host is the configured server host; it defaults to the 0.0.0.0 listen
	// address, which no gateway can reach.
	Host string
	// SiteHost is the configured public site host.
	SiteHost string
}

// NotifyURL builds the callback URL of a payment method; it carries the
// method's secret token in its path. The base is the method's Domain, else a
// reachable configured Host, else the site host. The request Host header is
// client-controlled and never used: deriving the callback from it would let
// a caller send the notification, token included, to a server of its choice.
func NotifyURL(method *paymentEntity.Payment, hosts NotifyHosts) (string, error) {
	base, err := notifyBaseURL(method, hosts)
	if err != nil {
		return "", err
	}
	return base + "/v1/notify/" + method.Platform + "/" + method.Token, nil
}

func notifyBaseURL(method *paymentEntity.Payment, hosts NotifyHosts) (string, error) {
	if base := strings.TrimSuffix(strings.TrimSpace(method.Domain), "/"); base != "" {
		return base, nil
	}
	for _, host := range []string{hosts.Host, hosts.SiteHost} {
		if base, ok := publicBaseURL(host); ok {
			return base, nil
		}
	}
	return "", pkgerrors.Wrapf(xerr.NewErrCode(xerr.PaymentNotifyURLNotConfigured),
		"payment method %d has no domain and no site host is configured", method.Id)
}

// publicBaseURL turns a configured host, either a bare host[:port] or a full
// URL, into a callback base URL. Wildcard and loopback listen addresses are
// unreachable for a gateway and count as not configured.
func publicBaseURL(host string) (string, bool) {
	raw := strings.TrimSpace(host)
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "localhost" {
		return "", false
	}
	if ip := net.ParseIP(hostname); ip != nil && (ip.IsUnspecified() || ip.IsLoopback()) {
		return "", false
	}
	return strings.TrimSuffix(parsed.Scheme+"://"+parsed.Host+parsed.EscapedPath(), "/"), true
}
