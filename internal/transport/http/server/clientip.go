package httpserver

import (
	"fmt"
	"net"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// remoteIPHeaders are the headers a trusted reverse proxy names the client
// in, in the order they are consulted.
var remoteIPHeaders = []string{"X-Forwarded-For", "X-Real-IP"}

// ParseTrustedProxies parses the configured reverse proxies, each an IP
// address or a CIDR, into the networks whose X-Forwarded-For and X-Real-IP
// headers are believed. It reports every entry that is neither, so a typo
// does not silently widen or narrow the trust.
func ParseTrustedProxies(entries []string) ([]*net.IPNet, error) {
	var networks []*net.IPNet
	var invalid []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			networks = append(networks, network)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			invalid = append(invalid, entry)
			continue
		}
		bits := 8 * net.IPv6len
		if ip.To4() != nil {
			ip = ip.To4()
			bits = 8 * net.IPv4len
		}
		networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	if len(invalid) > 0 {
		return networks, fmt.Errorf("trusted proxies %q are neither IP addresses nor CIDRs", invalid)
	}
	return networks, nil
}

// clientIPFunc resolves the client address of a request: the connection's
// remote address, unless that address is one of the trusted proxies, in
// which case the rightmost address of X-Forwarded-For (or X-Real-IP) that is
// not itself a trusted proxy names the client. With no trusted proxies no
// header is believed at all, so a client cannot choose the address the rate
// limits, audit logs and device records see.
func clientIPFunc(trusted []*net.IPNet) app.ClientIP {
	return app.ClientIPWithOption(app.ClientIPOptions{RemoteIPHeaders: remoteIPHeaders, TrustedCIDRs: trusted})
}
