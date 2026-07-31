package node

import (
	"strings"
	"testing"
)

func TestNormalizeProtocolForStorageRejectsUnsupportedRuntimeProtocol(t *testing.T) {
	for _, protocolType := range []string{"http", "socks"} {
		if _, err := NormalizeProtocolForStorage(Protocol{Type: protocolType}); err == nil {
			t.Fatalf("NormalizeProtocolForStorage(%q) expected error", protocolType)
		}
	}
}

func TestNormalizeProtocolForStorageClearsNoopFrontendValues(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:           "vless",
		Security:       "none",
		CertMode:       "none",
		Flow:           "none",
		Obfs:           "none",
		Multiplex:      "none",
		Encryption:     "none",
		EncryptionMode: "native",
		EncryptionRtt:  "0rtt",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Security != "" || protocol.CertMode != "" || protocol.Flow != "" ||
		protocol.Obfs != "" || protocol.Multiplex != "" || protocol.Encryption != "" ||
		protocol.EncryptionMode != "" || protocol.EncryptionRtt != "" {
		t.Fatalf("noop fields were not cleared: %#v", protocol)
	}
}

func TestNormalizeProtocolForStorageRejectsEnabledIncompleteTLS(t *testing.T) {
	_, err := NormalizeProtocolForStorage(Protocol{
		Type:     "hysteria",
		Port:     443,
		Enable:   true,
		Security: "tls",
	})
	if err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected incomplete TLS error")
	}
}

func TestNormalizeProtocolForStorageAllowsDisabledIncompleteTLS(t *testing.T) {
	if _, err := NormalizeProtocolForStorage(Protocol{Type: "hysteria", Security: "tls"}); err != nil {
		t.Fatalf("NormalizeProtocolForStorage() disabled protocol error = %v", err)
	}
}

func TestNormalizeProtocolForStoragePreservesMieruTransport(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:      "mieru",
		Port:      443,
		Enable:    true,
		Transport: "udp",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Transport != "udp" {
		t.Fatalf("Transport = %q, want udp", protocol.Transport)
	}
}

func TestNormalizeProtocolForStoragePreservesMieruMultiplexForSubscriptions(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:      "mieru",
		Port:      443,
		Enable:    true,
		Transport: "tcp",
		Multiplex: "MULTIPLEXING_HIGH",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Multiplex != "MULTIPLEXING_HIGH" {
		t.Fatalf("Multiplex = %q, want MULTIPLEXING_HIGH", protocol.Multiplex)
	}
}

func TestNormalizeProtocolForStorageDefaultsMieruMultiplexLow(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:      "mieru",
		Port:      443,
		Enable:    true,
		Transport: "tcp",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Multiplex != "MULTIPLEXING_LOW" {
		t.Fatalf("Multiplex = %q, want MULTIPLEXING_LOW", protocol.Multiplex)
	}
}

func TestNormalizeProtocolForStorageMapsLegacyMieruMultiplex(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:      "mieru",
		Port:      443,
		Enable:    true,
		Transport: "tcp",
		Multiplex: "middle",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Multiplex != "MULTIPLEXING_MIDDLE" {
		t.Fatalf("Multiplex = %q, want MULTIPLEXING_MIDDLE", protocol.Multiplex)
	}
}

func TestSanitizeProtocolsForNodeDistributionClearsMieruMultiplex(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:      "mieru",
		Port:      443,
		Enable:    true,
		Transport: "tcp",
		Multiplex: "MULTIPLEXING_LOW",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	if protocols[0].Multiplex != "" {
		t.Fatalf("node distribution Multiplex = %q, want empty", protocols[0].Multiplex)
	}
}

func TestNormalizeProtocolForStorageRejectsInvalidMieruTransport(t *testing.T) {
	_, err := NormalizeProtocolForStorage(Protocol{
		Type:      "mieru",
		Port:      443,
		Enable:    true,
		Transport: "quic",
	})
	if err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected invalid Mieru transport error")
	}
}

func TestNormalizeProtocolForStorageNormalizesShadowsocksPlugin(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:          "shadowsocks",
		Port:          443,
		Enable:        true,
		Cipher:        "aes-128-gcm",
		Plugin:        "simple-obfs",
		PluginOptions: map[string]any{"obfs": "tls", "host": " edge.example "},
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Plugin != "obfs" {
		t.Fatalf("Plugin = %q, want obfs", protocol.Plugin)
	}
	options, ok := protocol.PluginOptions.(map[string]any)
	if !ok || options["mode"] != "tls" || options["host"] != "edge.example" {
		t.Fatalf("PluginOptions = %#v, want normalized mode and host", protocol.PluginOptions)
	}
}

func TestSanitizeProtocolsForNodeDistributionClearsShadowsocksObfsHost(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:          "shadowsocks",
		Port:          443,
		Enable:        true,
		Cipher:        "aes-128-gcm",
		Plugin:        "obfs",
		PluginOptions: map[string]any{"mode": "http", "host": "edge.example"},
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	options, ok := protocols[0].PluginOptions.(map[string]any)
	if !ok || options["mode"] != "http" {
		t.Fatalf("PluginOptions = %#v, want mode http", protocols[0].PluginOptions)
	}
	if _, exists := options["host"]; exists {
		t.Fatalf("node distribution PluginOptions = %#v, want host removed", options)
	}
}

func TestNormalizeProtocolForStorageRejectsShadowsocksPluginTLSWithoutCertificate(t *testing.T) {
	_, err := NormalizeProtocolForStorage(Protocol{
		Type:          "shadowsocks",
		Port:          443,
		Enable:        true,
		Cipher:        "aes-128-gcm",
		Plugin:        "v2ray-plugin",
		PluginOptions: map[string]any{"mode": "websocket", "tls": true},
	})
	if err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected plugin TLS certificate error")
	}
}

func TestSanitizeProtocolsForNodeDistributionPreservesShadowsocksPluginTLSCertificate(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:          "shadowsocks",
		Port:          443,
		Enable:        true,
		Cipher:        "aes-128-gcm",
		Plugin:        "v2ray-plugin",
		PluginOptions: map[string]any{"mode": "websocket", "tls": true},
		Security:      "tls",
		SNI:           "edge.example",
		CertMode:      "self",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	protocol := protocols[0]
	if protocol.Security != "tls" || protocol.SNI != "edge.example" || protocol.CertMode != "self" {
		t.Fatalf("plugin TLS certificate fields were not preserved: %#v", protocol)
	}
}

func TestNormalizeProtocolForStorageAcceptsSnellV6(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:      "snell",
		Port:      443,
		Enable:    true,
		Version:   6,
		ServerKey: "123456789012",
		Mode:      "UNSHAPED",
		Transport: "TCP",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Mode != "unshaped" || protocol.Transport != "tcp" {
		t.Fatalf("Snell fields were not normalized: %#v", protocol)
	}
	if protocol.ServerKey != "" {
		t.Fatalf("ServerKey = %q, want cleared", protocol.ServerKey)
	}
}

func TestNormalizeProtocolForStorageAcceptsSnellWithoutServerKey(t *testing.T) {
	for _, version := range []int{5, 6} {
		protocol, err := NormalizeProtocolForStorage(Protocol{
			Type: "snell", Port: 443, Enable: true, Version: version,
		})
		if err != nil {
			t.Fatalf("NormalizeProtocolForStorage() v%d error = %v", version, err)
		}
		if protocol.ServerKey != "" {
			t.Fatalf("ServerKey = %q, want empty", protocol.ServerKey)
		}
	}
}

func TestNormalizeProtocolForStorageRejectsInvalidSnell(t *testing.T) {
	_, err := NormalizeProtocolForStorage(Protocol{
		Type:    "snell",
		Port:    443,
		Enable:  true,
		Version: 6,
		Obfs:    "http",
	})
	if err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected invalid Snell error")
	}
}

func TestNormalizeProtocolForStorageAcceptsShadowsocksR(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:        "ssr",
		Port:        8389,
		Enable:      true,
		Transport:   "TCP,UDP",
		Cipher:      "AES-256-CFB",
		ServerKey:   "secret",
		SSRProtocol: "AUTH_AES128_MD5",
		Obfs:        "TLS1.2_TICKET_AUTH",
		ObfsParam:   "example.com",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Type != "shadowsocksr" || protocol.Transport != "tcp,udp" ||
		protocol.Cipher != "aes-256-cfb" || protocol.SSRProtocol != "auth_aes128_md5" ||
		protocol.Obfs != "tls1.2_ticket_auth" {
		t.Fatalf("SSR fields were not normalized: %#v", protocol)
	}
}

func TestNormalizeProtocolForStorageRejectsInvalidShadowsocksR(t *testing.T) {
	_, err := NormalizeProtocolForStorage(Protocol{
		Type:        "shadowsocksr",
		Port:        8389,
		Enable:      true,
		Transport:   "quic",
		Cipher:      "aes-256-cfb",
		ServerKey:   "secret",
		SSRProtocol: "auth_aes128_md5",
	})
	if err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected invalid SSR error")
	}
}

func TestSanitizeProtocolsForNodeDistributionCleansVlessNoneSecurity(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:           "vless",
		Port:           443,
		Enable:         true,
		Security:       "none",
		SNI:            "unused.example",
		CertMode:       "none",
		AllowInsecure:  true,
		Fingerprint:    "chrome",
		Encryption:     "none",
		EncryptionMode: "native",
		EncryptionRtt:  "0rtt",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	protocol := protocols[0]
	if protocol.Security != "" || protocol.SNI != "" || protocol.CertMode != "" ||
		protocol.AllowInsecure || protocol.Fingerprint != "" || protocol.EncryptionMode != "" ||
		protocol.EncryptionRtt != "" {
		t.Fatalf("vless runtime fields were not sanitized: %#v", protocol)
	}
}

func TestSanitizeProtocolsForNodeDistributionFiltersIncompleteTLS(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:     "hysteria2",
		Port:     443,
		Enable:   true,
		Security: "tls",
	}})
	if len(protocols) != 0 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 0", len(protocols))
	}
}

func TestSanitizeProtocolsForNodeDistributionCleansHysteria2Alias(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:                 "hysteria2",
		Port:                 443,
		Enable:               true,
		Security:             "tls",
		SNI:                  "node.example",
		CertMode:             "self",
		Fingerprint:          "chrome",
		HopPorts:             "20000-30000",
		CongestionController: "bbr",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	protocol := protocols[0]
	if protocol.Type != "hysteria" || protocol.Fingerprint != "" || protocol.HopPorts != "" ||
		protocol.CongestionController != "" {
		t.Fatalf("hysteria runtime fields were not sanitized: %#v", protocol)
	}
}

func TestNormalizeProtocolForStorageKeepsCertPinForSelfSignedTLS(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:          "vmess",
		Port:          443,
		Enable:        true,
		Security:      "tls",
		SNI:           "node.example",
		CertMode:      "self",
		CertPinSHA256: strings.ToUpper(testCertPin),
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.CertPinSHA256 != testCertPin {
		t.Fatalf("CertPinSHA256 = %q, want lowercased %q", protocol.CertPinSHA256, testCertPin)
	}
}

func TestNormalizeProtocolForStorageClearsCertPinWhenNotSelfSigned(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
	}{
		{"cert_mode dns", Protocol{
			Type: "vmess", Port: 443, Enable: true, Security: "tls",
			SNI: "node.example", CertMode: "dns", CertDNSProvider: "cloudflare",
			CertPinSHA256: testCertPin,
		}},
		{"security reality", Protocol{
			Type: "vless", Port: 443, Enable: true, Security: "reality",
			SNI: "node.example", RealityPrivateKey: "key", RealityShortId: "0123abcd",
			CertPinSHA256: testCertPin,
		}},
		{"invalid fingerprint", Protocol{
			Type: "vmess", Port: 443, Enable: true, Security: "tls",
			SNI: "node.example", CertMode: "self",
			CertPinSHA256: "not-a-fingerprint",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			protocol, err := NormalizeProtocolForStorage(tc.protocol)
			if err != nil {
				t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
			}
			if protocol.CertPinSHA256 != "" {
				t.Fatalf("CertPinSHA256 = %q, want empty", protocol.CertPinSHA256)
			}
		})
	}
}

func TestNormalizeProtocolForStorageAllowsTrojanReality(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type:              "trojan",
		Port:              443,
		Enable:            true,
		Security:          "reality",
		SNI:               "node.example",
		RealityPrivateKey: "key",
		RealityPublicKey:  "public",
		RealityShortId:    "0123abcd",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Security != "reality" || protocol.RealityPrivateKey != "key" ||
		protocol.RealityPublicKey != "public" || protocol.RealityShortId != "0123abcd" {
		t.Fatalf("reality fields were not preserved: %#v", protocol)
	}
	if protocol.CertMode != "" {
		t.Fatalf("CertMode = %q, want empty for reality", protocol.CertMode)
	}
}

func TestNormalizeProtocolForStorageRejectsTrojanWithoutSecurity(t *testing.T) {
	if _, err := NormalizeProtocolForStorage(Protocol{
		Type:   "trojan",
		Port:   443,
		Enable: true,
	}); err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected trojan security error")
	}
}

func TestSanitizeProtocolsForNodeDistributionKeepsShadowsocksMultiplex(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type:      "shadowsocks",
		Port:      443,
		Enable:    true,
		Cipher:    "chacha20-ietf-poly1305",
		Multiplex: "smux",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	if protocols[0].Multiplex != "smux" {
		t.Fatalf("node distribution Multiplex = %q, want smux", protocols[0].Multiplex)
	}
}

func TestSanitizeProtocolsForNodeDistributionClearsMultiplexWhereUnsupported(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
	}{
		{"snell", Protocol{
			Type: "snell", Port: 443, Enable: true, ServerKey: "0123456789abcdef",
			Multiplex: "smux",
		}},
		{"hysteria", Protocol{
			Type: "hysteria", Port: 443, Enable: true, Security: "tls",
			SNI: "node.example", CertMode: "self", Multiplex: "smux",
		}},
		{"naive", Protocol{
			Type: "naive", Port: 443, Enable: true, Security: "tls",
			SNI: "node.example", CertMode: "self", Multiplex: "smux",
		}},
		{"shadowsocksr", Protocol{
			Type: "shadowsocksr", Port: 443, Enable: true, Cipher: "aes-256-cfb",
			ServerKey: "key", SSRProtocol: "auth_aes128_md5", Multiplex: "smux",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			protocols := SanitizeProtocolsForNodeDistribution([]Protocol{tc.protocol})
			if len(protocols) != 1 {
				t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
			}
			if protocols[0].Multiplex != "" {
				t.Fatalf("node distribution Multiplex = %q, want empty", protocols[0].Multiplex)
			}
		})
	}
}

func TestSanitizeProtocolsForNodeDistributionKeepsAnytlsPaddingScheme(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type: "anytls", Port: 443, Enable: true, Security: "tls",
		SNI: "node.example", CertMode: "self", PaddingScheme: "stop=8\n0=30-30",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	if protocols[0].PaddingScheme != "stop=8\n0=30-30" {
		t.Fatalf("node distribution PaddingScheme = %q, want preserved", protocols[0].PaddingScheme)
	}
}

func TestSanitizeProtocolsForNodeDistributionKeepsSnellWithoutServerKey(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type: "snell", Port: 443, Enable: true, Version: 6,
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	if protocols[0].ServerKey != "" {
		t.Fatalf("node distribution ServerKey = %q, want empty", protocols[0].ServerKey)
	}
}

func TestSanitizeProtocolsForNodeDistributionClearsStaleSnellServerKey(t *testing.T) {
	protocols := SanitizeProtocolsForNodeDistribution([]Protocol{{
		Type: "snell", Port: 443, Enable: true, Version: 6, ServerKey: "short",
	}})
	if len(protocols) != 1 {
		t.Fatalf("SanitizeProtocolsForNodeDistribution() len = %d, want 1", len(protocols))
	}
	if protocols[0].ServerKey != "" {
		t.Fatalf("node distribution ServerKey = %q, want empty", protocols[0].ServerKey)
	}
}

func TestNormalizeProtocolForStorageAcceptsMultiUserShadowsocksrProtocols(t *testing.T) {
	for _, ssrProtocol := range []string{
		"auth_aes128_md5", "auth_aes128_sha1", "auth_chain_a", "auth_chain_b",
		"auth_chain_c", "auth_chain_d", "auth_chain_e", "auth_chain_f",
	} {
		t.Run(ssrProtocol, func(t *testing.T) {
			protocol, err := NormalizeProtocolForStorage(Protocol{
				Type: "shadowsocksr", Port: 443, Enable: true, Cipher: "aes-256-cfb",
				ServerKey: "key", SSRProtocol: ssrProtocol, Obfs: "plain",
			})
			if err != nil {
				t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
			}
			if protocol.SSRProtocol != ssrProtocol {
				t.Fatalf("SSRProtocol = %q, want %q", protocol.SSRProtocol, ssrProtocol)
			}
		})
	}
}

func TestNormalizeProtocolForStorageRejectsSingleUserShadowsocksrProtocols(t *testing.T) {
	for _, ssrProtocol := range []string{
		"origin", "verify_simple", "verify_sha1", "auth_simple", "auth_sha1",
		"auth_sha1_v2", "auth_sha1_v4",
	} {
		t.Run(ssrProtocol, func(t *testing.T) {
			if _, err := NormalizeProtocolForStorage(Protocol{
				Type: "shadowsocksr", Port: 443, Enable: true, Cipher: "aes-256-cfb",
				ServerKey: "key", SSRProtocol: ssrProtocol, Obfs: "plain",
			}); err == nil {
				t.Fatalf("NormalizeProtocolForStorage(%q) expected single-user protocol error", ssrProtocol)
			}
		})
	}
}

func TestNormalizeProtocolForStorageAcceptsShadowsocksrTLS10Obfs(t *testing.T) {
	protocol, err := NormalizeProtocolForStorage(Protocol{
		Type: "shadowsocksr", Port: 443, Enable: true, Cipher: "aes-256-cfb",
		ServerKey: "key", SSRProtocol: "auth_chain_f", Obfs: "tls1.0_session_auth",
	})
	if err != nil {
		t.Fatalf("NormalizeProtocolForStorage() error = %v", err)
	}
	if protocol.Obfs != "tls1.0_session_auth" {
		t.Fatalf("Obfs = %q, want tls1.0_session_auth", protocol.Obfs)
	}
}

func TestNormalizeProtocolForStorageRejectsShadowsocksrUDPOnlyNonPlainObfs(t *testing.T) {
	if _, err := NormalizeProtocolForStorage(Protocol{
		Type: "shadowsocksr", Port: 443, Enable: true, Cipher: "aes-256-cfb",
		ServerKey: "key", SSRProtocol: "auth_chain_a", Obfs: "http_simple",
		Transport: "udp",
	}); err == nil {
		t.Fatal("NormalizeProtocolForStorage() expected udp-only obfs error")
	}
}

func TestNormalizeProtocolForStorageAllowsShadowsocksrTCPUDPNonPlainObfs(t *testing.T) {
	for _, transport := range []string{"", "both", "tcp", "tcp+udp"} {
		protocol, err := NormalizeProtocolForStorage(Protocol{
			Type: "shadowsocksr", Port: 443, Enable: true, Cipher: "aes-256-cfb",
			ServerKey: "key", SSRProtocol: "auth_chain_a", Obfs: "http_simple",
			Transport: transport,
		})
		if err != nil {
			t.Fatalf("NormalizeProtocolForStorage(%q) error = %v", transport, err)
		}
		if protocol.Obfs != "http_simple" {
			t.Fatalf("Obfs = %q, want http_simple", protocol.Obfs)
		}
	}
}
