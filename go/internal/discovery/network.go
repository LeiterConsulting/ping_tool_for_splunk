package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

type systemLocalNetworkDetector struct{}

type localCandidate struct {
	Address   netip.Addr
	Prefix    int
	Interface string
	Index     int
}

func (systemLocalNetworkDetector) Detect(ctx context.Context) (LocalNetwork, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return LocalNetwork{}, fmt.Errorf("enumerate local network interfaces: %w", err)
	}
	candidates := make([]localCandidate, 0)
	seen := make(map[netip.Addr]struct{})
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, raw := range addresses {
			prefix, parseErr := netip.ParsePrefix(raw.String())
			if parseErr != nil {
				continue
			}
			address := prefix.Addr().Unmap()
			if !address.Is4() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() {
				continue
			}
			if _, exists := seen[address]; exists {
				continue
			}
			seen[address] = struct{}{}
			candidates = append(candidates, localCandidate{Address: address, Prefix: prefix.Bits(), Interface: iface.Name, Index: iface.Index})
		}
	}
	if len(candidates) == 0 {
		return LocalNetwork{}, errors.New("no usable active IPv4 interface was found; specify target_network explicitly")
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Index != candidates[j].Index {
			return candidates[i].Index < candidates[j].Index
		}
		return candidates[i].Address.Less(candidates[j].Address)
	})

	preferred, routeErr := preferredOutboundIPv4(ctx)
	return selectLocalCandidate(candidates, preferred, routeErr)
}

func selectLocalCandidate(candidates []localCandidate, preferred netip.Addr, routeErr error) (LocalNetwork, error) {
	if len(candidates) == 0 {
		return LocalNetwork{}, errors.New("no usable active IPv4 interface was found; specify target_network explicitly")
	}
	if routeErr == nil {
		for _, candidate := range candidates {
			if candidate.Address == preferred {
				return selectionFromCandidate(candidate, ""), nil
			}
		}
	}
	if len(candidates) == 1 {
		warning := "No preferred outbound IPv4 route could be matched; using the only active IPv4 interface. Specify target_network to select a different routed subnet explicitly."
		return selectionFromCandidate(candidates[0], warning), nil
	}
	descriptions := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		descriptions = append(descriptions, candidateDescription(candidate))
	}
	return LocalNetwork{}, fmt.Errorf("local subnet selection is ambiguous across active IPv4 interfaces: %s; specify target_network explicitly", strings.Join(descriptions, "; "))
}

func preferredOutboundIPv4(ctx context.Context) (netip.Addr, error) {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "udp4", "1.1.1.1:53")
	if err != nil {
		return netip.Addr{}, err
	}
	defer connection.Close()
	udpAddress, ok := connection.LocalAddr().(*net.UDPAddr)
	if !ok {
		return netip.Addr{}, fmt.Errorf("unexpected local route address %T", connection.LocalAddr())
	}
	address, ok := netip.AddrFromSlice(udpAddress.IP)
	if !ok || !address.Unmap().Is4() {
		return netip.Addr{}, errors.New("preferred outbound route did not select IPv4")
	}
	return address.Unmap(), nil
}

func selectionFromCandidate(candidate localCandidate, warning string) LocalNetwork {
	return LocalNetwork{
		Address: candidate.Address, Interface: candidate.Interface,
		Description: candidateDescription(candidate), Warning: warning,
	}
}

func candidateDescription(candidate localCandidate) string {
	return fmt.Sprintf("%s (index %d, %s/%d)", candidate.Interface, candidate.Index, candidate.Address, candidate.Prefix)
}
