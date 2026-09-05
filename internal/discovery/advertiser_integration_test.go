//go:build integration

package discovery

import (
	"context"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/zeroconf/v2"
)

func TestAdvertiserCanBeBrowsed(t *testing.T) {
	if os.Getenv("SHARE_DISK_TEST_MDNS") != "1" {
		t.Skip("set SHARE_DISK_TEST_MDNS=1 on a multicast-capable host")
	}
	var selected []net.Interface
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr == nil && len(addrs) > 0 {
			selected = []net.Interface{iface}
			break
		}
	}
	if len(selected) == 0 {
		t.Fatal("no active multicast interface")
	}
	addrs, err := selected[0].Addrs()
	if err != nil {
		t.Fatalf("list addresses for %s: %v", selected[0].Name, err)
	}
	t.Logf("using multicast interface %s with addresses %v", selected[0].Name, addrs)
	advertiser, err := startOnInterfaces("device-mdns-test", 19090, "test-version", selected)
	if err != nil {
		t.Fatal(err)
	}
	defer advertiser.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entries := make(chan *zeroconf.ServiceEntry, 8)
	errCh := make(chan error, 1)
	go func() { errCh <- zeroconf.Browse(ctx, Service, Domain, entries, zeroconf.SelectIfaces(selected)) }()
	for {
		select {
		case entry := <-entries:
			if entry != nil {
				t.Logf("received service instance=%q port=%d host=%q ipv4=%v ipv6=%v text=%v", entry.Instance, entry.Port, entry.HostName, entry.AddrIPv4, entry.AddrIPv6, entry.Text)
			}
			if entry != nil && entry.Port == 19090 && strings.ReplaceAll(entry.Instance, `\ `, " ") == "Share Disk device-mdns-" {
				for _, required := range []string{"api=lan-v1", "scheme=http", "version=test-version", "device=device-mdns-test"} {
					if !slices.Contains(entry.Text, required) {
						t.Fatalf("discovery TXT record is missing %q: %v", required, entry.Text)
					}
				}
				if len(entry.AddrIPv4) == 0 && len(entry.AddrIPv6) == 0 {
					t.Fatal("discovered service has no address")
				}
				return
			}
		case err := <-errCh:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("advertised Share Disk service was not discovered")
		}
	}
}
