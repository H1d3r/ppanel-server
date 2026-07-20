package server

import (
	"strings"

	"github.com/perfect-panel/server/internal/model/entity/node"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/uuidx"
)

const generatedServerKeyLength = 32

func protocolKeyLookup(protocols []node.Protocol) map[string]string {
	keys := make(map[string]string, len(protocols))
	for _, protocol := range protocols {
		protocolType := generatedKeyProtocolType(protocol.Type)
		if protocolType == "" || strings.TrimSpace(protocol.ServerKey) == "" {
			continue
		}
		keys[protocolType] = protocol.ServerKey
	}
	return keys
}

func ensureGeneratedProtocolKey(protocol *node.Protocol, existing map[string]string) {
	protocolType := generatedKeyProtocolType(protocol.Type)
	if protocolType == "" || strings.TrimSpace(protocol.ServerKey) != "" {
		return
	}
	if key := strings.TrimSpace(existing[protocolType]); key != "" {
		protocol.ServerKey = key
		return
	}
	protocol.ServerKey = tool.GenerateCipher(uuidx.NewUUID().String(), generatedServerKeyLength)
}

func generatedKeyProtocolType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ssr", "shadowsocks-r", "shadowsocksr":
		return "shadowsocksr"
	case "snell":
		return "snell"
	default:
		return ""
	}
}
