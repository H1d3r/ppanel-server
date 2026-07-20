package server

import (
	"testing"

	"github.com/perfect-panel/server/internal/model/entity/node"
)

func TestEnsureGeneratedProtocolKeyGeneratesForEmptySnellAndSSR(t *testing.T) {
	for _, protocolType := range []string{"snell", "shadowsocksr", "ssr"} {
		t.Run(protocolType, func(t *testing.T) {
			protocol := node.Protocol{Type: protocolType}
			ensureGeneratedProtocolKey(&protocol, nil)
			if len(protocol.ServerKey) != generatedServerKeyLength {
				t.Fatalf("ServerKey length = %d, want %d", len(protocol.ServerKey), generatedServerKeyLength)
			}
		})
	}
}

func TestEnsureGeneratedProtocolKeyPreservesExistingKey(t *testing.T) {
	protocol := node.Protocol{Type: "snell"}
	ensureGeneratedProtocolKey(&protocol, map[string]string{"snell": "existing-psk"})
	if protocol.ServerKey != "existing-psk" {
		t.Fatalf("ServerKey = %q, want existing-psk", protocol.ServerKey)
	}
}

func TestEnsureGeneratedProtocolKeyKeepsProvidedKey(t *testing.T) {
	protocol := node.Protocol{Type: "shadowsocksr", ServerKey: "provided"}
	ensureGeneratedProtocolKey(&protocol, map[string]string{"shadowsocksr": "existing"})
	if protocol.ServerKey != "provided" {
		t.Fatalf("ServerKey = %q, want provided", protocol.ServerKey)
	}
}

func TestProtocolKeyLookupNormalizesAliases(t *testing.T) {
	keys := protocolKeyLookup([]node.Protocol{{Type: "ssr", ServerKey: "secret"}})
	if keys["shadowsocksr"] != "secret" {
		t.Fatalf("SSR key = %q, want secret", keys["shadowsocksr"])
	}
}
