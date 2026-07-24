package edge

import (
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/model/entity/node"
	"github.com/perfect-panel/server/internal/model/entity/user"
)

func TestProxyFromProtocol(t *testing.T) {
	item := &node.Node{Id: 7, Name: "Tokyo", Address: "jp.example.com", Port: 443, Protocol: "vless", Tags: "asia, premium", Sort: 3}
	protocol := node.Protocol{
		Type:      "vless",
		Enable:    true,
		Security:  "tls",
		SNI:       "jp.example.com",
		Transport: "ws",
		Host:      "cdn.example.com",
		Path:      "/ws",
		Flow:      "xtls-rprx-vision",
	}

	proxy, supported, reason := proxyFromProtocol(item, protocol, "00000000-0000-4000-8000-000000000001")
	if !supported || reason != "" {
		t.Fatalf("expected proxy to be supported, got supported=%v reason=%q", supported, reason)
	}
	if proxy.UUID == "" || proxy.TLS == nil || proxy.Transport == nil {
		t.Fatalf("expected credentials, tls and transport, got %#v", proxy)
	}
	if proxy.Transport.Type != "ws" || proxy.Transport.Host != "cdn.example.com" {
		t.Fatalf("unexpected transport: %#v", proxy.Transport)
	}
}

func TestProxyFromProtocolRejectsUnsupportedWorkerFeatures(t *testing.T) {
	item := &node.Node{Name: "Reality", Address: "example.com", Port: 443}
	_, supported, reason := proxyFromProtocol(item, node.Protocol{Type: "vless", Enable: true, Security: "reality"}, "user-secret")
	if supported || reason == "" {
		t.Fatalf("expected reality node to be rejected, got supported=%v reason=%q", supported, reason)
	}

	_, supported, reason = proxyFromProtocol(item, node.Protocol{Type: "shadowsocks", Enable: true, Cipher: "2022-blake3-aes-128-gcm"}, "user-secret")
	if supported || reason == "" {
		t.Fatalf("expected shadowsocks 2022 node to be rejected, got supported=%v reason=%q", supported, reason)
	}

	_, supported, reason = proxyFromProtocol(item, node.Protocol{Type: "shadowsocks", Enable: true, Cipher: "aes-128-gcm", Security: "tls"}, "user-secret")
	if supported || reason == "" {
		t.Fatalf("expected Shadowsocks TLS node to be rejected, got supported=%v reason=%q", supported, reason)
	}
}

func TestSubscriptionState(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if state := subscriptionState(&user.Subscribe{Status: 1, Traffic: 100, Upload: 40, Download: 60}, now); state != "traffic_exhausted" {
		t.Fatalf("expected traffic exhaustion, got %q", state)
	}
	if state := subscriptionState(&user.Subscribe{Status: 5}, now); state != "suspended" {
		t.Fatalf("expected suspended, got %q", state)
	}
	if state := subscriptionState(&user.Subscribe{Status: 255}, now); state != "disabled" {
		t.Fatalf("expected unknown status to be disabled, got %q", state)
	}
}
