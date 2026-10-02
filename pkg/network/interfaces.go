// Package network provides observation-only guest network helpers. Topologies
// and endpoint MAC addresses remain native Containerlab objects.
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
)

type Guest interface {
	Pipe(context.Context, io.Reader, ...string) ([]byte, error)
}

// InterfaceNames resolves caller-declared endpoint MACs in the same order.
// Never infer a guest's interface name from a wrapper's ethN: cloned network
// appliances can retain installer udev names. Missing/duplicate MACs fail closed.
func InterfaceNames(ctx context.Context, guest Guest, macs ...string) ([]string, error) {
	if len(macs) == 0 {
		return nil, fmt.Errorf("explicit endpoint MACs required")
	}
	want := make([]string, len(macs))
	seen := map[string]bool{}
	for i, value := range macs {
		mac, err := net.ParseMAC(value)
		if err != nil || len(mac) != 6 {
			return nil, fmt.Errorf("invalid Ethernet MAC %q", value)
		}
		want[i] = mac.String()
		if seen[want[i]] {
			return nil, fmt.Errorf("duplicate endpoint MAC %q", value)
		}
		seen[want[i]] = true
	}
	data, err := guest.Pipe(ctx, nil, "ip", "-j", "link", "show")
	if err != nil {
		return nil, err
	}
	return interfaceNames(data, want)
}

func interfaceNames(data []byte, macs []string) ([]string, error) {
	var links []struct {
		Name string `json:"ifname"`
		MAC  string `json:"address"`
	}
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, err
	}
	names := make([]string, len(macs))
	for i, mac := range macs {
		count := 0
		for _, link := range links {
			parsed, err := net.ParseMAC(link.MAC)
			if err == nil && parsed.String() == mac {
				count++
				names[i] = link.Name
			}
		}
		if count != 1 || names[i] == "" {
			return nil, fmt.Errorf("endpoint MAC %s has %d guest interfaces", mac, count)
		}
	}
	return names, nil
}
