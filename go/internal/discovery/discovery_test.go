package discovery

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	pingpkg "github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
)

type fakePinger struct {
	mu      sync.Mutex
	results map[string]pingpkg.PingResult
	errors  map[string]error
	block   bool
}

func (p *fakePinger) Backend() string { return "fake_icmp" }

func (p *fakePinger) Ping(ctx context.Context, ip string, _ int, _ time.Duration) ([]pingpkg.PingResult, error) {
	if p.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.errors[ip]; err != nil {
		return nil, err
	}
	result, ok := p.results[ip]
	if !ok {
		result = pingpkg.PingResult{Backend: "fake_icmp", Error: "timeout"}
	}
	return []pingpkg.PingResult{result}, nil
}

type fakeResolver struct {
	ptr     map[string][]string
	forward map[string][]net.IPAddr
}

func (r fakeResolver) LookupAddr(_ context.Context, address string) ([]string, error) {
	values, ok := r.ptr[address]
	if !ok {
		return nil, errors.New("not found")
	}
	return values, nil
}

func (r fakeResolver) LookupIPAddr(_ context.Context, name string) ([]net.IPAddr, error) {
	values, ok := r.forward[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return values, nil
}

type fakeDetector struct {
	selection LocalNetwork
	err       error
}

func (d fakeDetector) Detect(context.Context) (LocalNetwork, error) {
	return d.selection, d.err
}

func TestRunProducesStructuredTruthfulEvidence(t *testing.T) {
	pinger := &fakePinger{results: map[string]pingpkg.PingResult{
		"192.0.2.1": {
			Success: true, LatencyMs: 0.75, LatencyMeasured: true, LatencySource: "windows_icmp_rtt",
			LatencyResolutionMs: 0.001, ProbeElapsedMs: 0.91, Backend: "windows_icmp", TTL: 128,
		},
		"192.0.2.2": {Error: "timeout", Backend: "windows_icmp"},
	}}
	resolver := fakeResolver{
		ptr: map[string][]string{"192.0.2.1": {"Srv01.Example.Test."}},
		forward: map[string][]net.IPAddr{
			"srv01.example.test": {{IP: net.ParseIP("192.0.2.1")}},
		},
	}
	var progress []Progress
	result, err := Run(context.Background(), Options{
		TargetNetwork: "192.0.2.0", SubnetMask: 30, Timeout: 125 * time.Millisecond,
		Concurrency: 2, Pinger: pinger, Resolver: resolver,
		Now:      func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) },
		Progress: func(update Progress) error { progress = append(progress, update); return nil },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Target != "192.0.2.0/30" || len(result.Items) != 1 {
		t.Fatalf("result target/items = %q/%d", result.Target, len(result.Items))
	}
	if result.Evidence.Engine != EngineName || result.Evidence.HostsConsidered != 2 || result.Evidence.HostsProbed != 2 ||
		result.Evidence.HostsObserved != 1 || result.Evidence.HostsNotObserved != 1 || result.Evidence.HostsIndeterminate != 0 ||
		result.Evidence.DNSForwardConfirmed != 1 || result.Evidence.DNSPtrOnly != 0 || result.Evidence.DNSUnresolved != 0 {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
	endpoint := result.Items[0]
	if endpoint.IP != "192.0.2.1" || endpoint.Hostname != "srv01" || endpoint.FQDN != "srv01.example.test" ||
		endpoint.DNSStatus != "forward_confirmed" || !endpoint.DNSForwardConfirmed {
		t.Fatalf("endpoint DNS evidence = %#v", endpoint)
	}
	if endpoint.DiscoveryLatencyMs == nil || *endpoint.DiscoveryLatencyMs != 0.75 ||
		endpoint.DiscoveryProbeBackend != "windows_icmp" || endpoint.DiscoveryLatencySource != "windows_icmp_rtt" {
		t.Fatalf("endpoint probe evidence = %#v", endpoint)
	}
	if len(progress) < 4 || progress[len(progress)-1].Stage != "complete" {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestRunPreservesCensoredLatencyWithoutInventingZero(t *testing.T) {
	pinger := &fakePinger{results: map[string]pingpkg.PingResult{
		"198.51.100.1": {
			Success: true, LatencyCensored: true, LatencyUpperBoundMs: 1,
			LatencySource: "windows_icmp_rtt", LatencyResolutionMs: 1, Backend: "windows_icmp",
		},
		"198.51.100.2": {Error: "timeout", Backend: "windows_icmp"},
	}}
	result, err := Run(context.Background(), Options{
		TargetNetwork: "198.51.100.0/30", SubnetMask: 30, Pinger: pinger, Resolver: fakeResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := result.Items[0]
	if endpoint.DiscoveryLatencyMs != nil || !endpoint.DiscoveryLatencyCensored ||
		endpoint.DiscoveryLatencyUpperBoundMs == nil || *endpoint.DiscoveryLatencyUpperBoundMs != 1 {
		t.Fatalf("censored latency = %#v", endpoint)
	}
}

func TestDefaultResolverUsesContextAwareGoDNS(t *testing.T) {
	resolver, ok := defaultResolver().(*net.Resolver)
	if !ok || !resolver.PreferGo || !resolver.StrictErrors {
		t.Fatalf("default resolver = %#v, want context-aware strict Go DNS", resolver)
	}
}

func TestRunFailsClosedOnIndeterminateProbeEvidence(t *testing.T) {
	pinger := &fakePinger{results: map[string]pingpkg.PingResult{
		"203.0.113.1": {Error: "backend_error", Backend: "windows_icmp"},
		"203.0.113.2": {Error: "timeout", Backend: "windows_icmp"},
	}}
	_, err := Run(context.Background(), Options{
		TargetNetwork: "203.0.113.0/30", SubnetMask: 30, Pinger: pinger, Resolver: fakeResolver{},
	})
	if err == nil || !strings.Contains(err.Error(), "indeterminate evidence") {
		t.Fatalf("Run() error = %v, want indeterminate evidence failure", err)
	}
}

func TestRunAcceptsCompleteScanWithNoObservedHosts(t *testing.T) {
	result, err := Run(context.Background(), Options{
		TargetNetwork: "192.0.2.0/30", SubnetMask: 30,
		Pinger: &fakePinger{results: map[string]pingpkg.PingResult{}}, Resolver: fakeResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 0 || result.Evidence.HostsNotObserved != 2 || result.Evidence.HostsIndeterminate != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunUsesDetectedAddressOnlyWhenTargetIsOmitted(t *testing.T) {
	detector := fakeDetector{selection: LocalNetwork{
		Address: netip.MustParseAddr("10.20.30.41"), Interface: "LAN", Description: "LAN (10.20.30.41)",
	}}
	result, err := Run(context.Background(), Options{
		SubnetMask: 30, Detector: detector,
		Pinger: &fakePinger{results: map[string]pingpkg.PingResult{}}, Resolver: fakeResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "10.20.30.40/30" {
		t.Fatalf("target = %q, want canonical detected network", result.Target)
	}
}

func TestRunHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, Options{
		TargetNetwork: "192.0.2.0/30", SubnetMask: 30,
		Pinger: &fakePinger{block: true}, Resolver: fakeResolver{},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
}

func TestResolveTargetExplicitBypassesDetector(t *testing.T) {
	prefix, _, err := resolveTarget(context.Background(), "10.0.0.19", 28, fakeDetector{err: errors.New("must not be called")})
	if err != nil {
		t.Fatal(err)
	}
	if prefix.String() != "10.0.0.16/28" {
		t.Fatalf("prefix = %s", prefix)
	}
}

func TestSelectLocalCandidateUsesPreferredRouteAddress(t *testing.T) {
	candidates := []localCandidate{
		{Address: netip.MustParseAddr("10.0.0.10"), Prefix: 24, Interface: "LAN", Index: 4},
		{Address: netip.MustParseAddr("192.168.1.10"), Prefix: 24, Interface: "Wi-Fi", Index: 7},
	}
	selected, err := selectLocalCandidate(candidates, netip.MustParseAddr("192.168.1.10"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Address.String() != "192.168.1.10" || selected.Interface != "Wi-Fi" || selected.Warning != "" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectLocalCandidateFailsClosedWhenAmbiguous(t *testing.T) {
	candidates := []localCandidate{
		{Address: netip.MustParseAddr("10.0.0.10"), Prefix: 24, Interface: "LAN A", Index: 4},
		{Address: netip.MustParseAddr("10.1.0.10"), Prefix: 24, Interface: "LAN B", Index: 5},
	}
	_, err := selectLocalCandidate(candidates, netip.Addr{}, errors.New("no route"))
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "LAN A") || !strings.Contains(err.Error(), "LAN B") {
		t.Fatalf("error = %v", err)
	}
}

func TestSelectLocalCandidateWarnsWhenOnlyInterfaceHasNoPreferredRoute(t *testing.T) {
	selected, err := selectLocalCandidate([]localCandidate{{
		Address: netip.MustParseAddr("172.20.0.9"), Prefix: 24, Interface: "Isolated LAN", Index: 12,
	}}, netip.Addr{}, errors.New("no route"))
	if err != nil {
		t.Fatal(err)
	}
	if selected.Address.String() != "172.20.0.9" || !strings.Contains(selected.Warning, "only active IPv4 interface") {
		t.Fatalf("selected = %#v", selected)
	}
}
