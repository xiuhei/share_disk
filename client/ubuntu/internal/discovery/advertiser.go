// Package discovery publishes an explicitly enabled LAN Agent using standard
// DNS-SD/mDNS. Discovery only supplies a candidate address; API authentication
// remains mandatory.
package discovery

import (
	"fmt"
	"net"
	"strings"
	"unicode"

	"github.com/libp2p/zeroconf/v2"
)

const (
	Service = "_sharedisk._tcp"
	Domain  = "local."
)

// Advertiser owns one DNS-SD registration.
type Advertiser struct {
	server *zeroconf.Server
}

// Start advertises the Agent on all suitable multicast interfaces.
func Start(deviceID string, port int, agentVersion string) (*Advertiser, error) {
	return startOnInterfaces(deviceID, port, agentVersion, nil)
}

// startOnInterfaces exists so integration tests can prove advertisement and
// browsing on the same physical interface. A nil slice preserves the
// production behavior of advertising on every suitable multicast interface.
func startOnInterfaces(deviceID string, port int, agentVersion string, ifaces []net.Interface) (*Advertiser, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid discovery port: %d", port)
	}
	instance := instanceName(deviceID)
	if instance == "" {
		return nil, fmt.Errorf("device id is required for discovery")
	}
	server, err := zeroconf.Register(instance, Service, Domain, port, []string{
		"api=lan-v1",
		"scheme=http",
		"version=" + sanitizeTXT(agentVersion),
		"device=" + sanitizeTXT(deviceID),
	}, ifaces)
	if err != nil {
		return nil, fmt.Errorf("register DNS-SD service: %w", err)
	}
	return &Advertiser{server: server}, nil
}

// Close withdraws the DNS-SD registration and releases multicast sockets.
func (a *Advertiser) Close() {
	if a != nil && a.server != nil {
		a.server.Shutdown()
	}
}

func instanceName(deviceID string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(deviceID) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() >= 12 {
			break
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "Share Disk " + b.String()
}

func sanitizeTXT(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '=' {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(value) > 180 {
		value = value[:180]
	}
	if value == "" {
		return "unknown"
	}
	return value
}
