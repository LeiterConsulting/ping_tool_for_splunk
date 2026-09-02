package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	pingpkg "github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
	"github.com/google/uuid"
)

const (
	EngineName          = "native_go"
	MaxProbeWorkerLimit = 256
	MaxDNSWorkerLimit   = 128
)

type WorkerTelemetry interface {
	ConfigureDiscoveryProbeWorkers(int)
	DiscoveryProbeWorkerStarted()
	DiscoveryProbeWorkerFinished()
	ConfigureDiscoveryDNSWorkers(int)
	DiscoveryDNSWorkerStarted()
	DiscoveryDNSWorkerFinished()
}

type Pinger interface {
	Ping(context.Context, string, int, time.Duration) ([]pingpkg.PingResult, error)
	Backend() string
}

type Resolver interface {
	LookupAddr(context.Context, string) ([]string, error)
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type LocalNetworkDetector interface {
	Detect(context.Context) (LocalNetwork, error)
}

type LocalNetwork struct {
	Address     netip.Addr
	Interface   string
	Description string
	Warning     string
}

type Options struct {
	TargetNetwork     string
	SubnetMask        int
	Timeout           time.Duration
	Concurrency       int
	PingMode          string
	Pinger            Pinger
	Resolver          Resolver
	Detector          LocalNetworkDetector
	DNSLookupTimeout  time.Duration
	OnBackendFallback func(ip, from, to, reason string)
	Progress          func(Progress) error
	WorkerTelemetry   WorkerTelemetry
	Now               func() time.Time
}

type Progress struct {
	Stage              string `json:"stage"`
	Message            string `json:"message"`
	LogLine            string `json:"log_line,omitempty"`
	Target             string `json:"target,omitempty"`
	HostsConsidered    int    `json:"hosts_considered"`
	HostsProbed        int    `json:"hosts_probed"`
	HostsObserved      int    `json:"hosts_observed"`
	HostsNotObserved   int    `json:"hosts_not_observed"`
	HostsIndeterminate int    `json:"hosts_indeterminate"`
}

type Evidence struct {
	Engine               string   `json:"engine"`
	ProbeBackends        []string `json:"probe_backends,omitempty"`
	LatencySources       []string `json:"latency_sources,omitempty"`
	RequestedConcurrency int      `json:"requested_concurrency"`
	ProbeWorkers         int      `json:"probe_workers"`
	DNSWorkers           int      `json:"dns_workers"`
	HostsConsidered      int      `json:"hosts_considered"`
	HostsProbed          int      `json:"hosts_probed"`
	HostsObserved        int      `json:"hosts_observed"`
	HostsNotObserved     int      `json:"hosts_not_observed"`
	HostsIndeterminate   int      `json:"hosts_indeterminate"`
	DNSForwardConfirmed  int      `json:"dns_forward_confirmed"`
	DNSPtrOnly           int      `json:"dns_ptr_only"`
	DNSUnresolved        int      `json:"dns_unresolved"`
}

type Result struct {
	GeneratedAt time.Time
	ScanID      string
	Target      string
	Items       []models.Endpoint
	Logs        []string
	Duration    time.Duration
	Evidence    Evidence
}

type probeOutcome struct {
	IP     netip.Addr
	Result pingpkg.PingResult
	State  string
	Reason string
}

type observedHost struct {
	IP     netip.Addr
	Result pingpkg.PingResult
}

type resolvedHost struct {
	Host observedHost
	DNS  dnsEvidence
}

type dnsEvidence struct {
	Hostname         string
	FQDN             string
	Status           string
	ForwardConfirmed bool
}

func Run(ctx context.Context, options Options) (Result, error) {
	started := time.Now()
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Timeout <= 0 {
		options.Timeout = 500 * time.Millisecond
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 50
	}
	if options.DNSLookupTimeout <= 0 {
		options.DNSLookupTimeout = 2 * time.Second
	}
	if options.Pinger == nil {
		options.Pinger = pingpkg.NewPinger(options.PingMode, options.OnBackendFallback)
	}
	if options.Resolver == nil {
		// Windows' native resolver calls cannot always be interrupted after the
		// lookup context expires. A high-response subnet scan can therefore leave
		// blocked OS threads and handles behind. The Go resolver uses the system's
		// configured DNS servers while preserving context cancellation.
		options.Resolver = defaultResolver()
	}
	if options.Detector == nil {
		options.Detector = systemLocalNetworkDetector{}
	}

	result := Result{
		ScanID:   uuid.NewString(),
		Evidence: Evidence{Engine: EngineName},
	}
	emit := func(progress Progress) error {
		if strings.TrimSpace(progress.LogLine) != "" {
			result.Logs = append(result.Logs, strings.TrimSpace(progress.LogLine))
		}
		if options.Progress != nil {
			return options.Progress(progress)
		}
		return nil
	}

	target, selection, err := resolveTarget(ctx, options.TargetNetwork, options.SubnetMask, options.Detector)
	if err != nil {
		return Result{}, err
	}
	result.Target = target.String()
	hostCount := usableHostCount(target)
	result.Evidence.HostsConsidered = hostCount

	if err := emit(Progress{
		Stage: "preparing", Message: fmt.Sprintf("Preparing native discovery for %s across %d host%s.", result.Target, hostCount, plural(hostCount)),
		LogLine: fmt.Sprintf("Discovery engine: %s", EngineName), Target: result.Target, HostsConsidered: hostCount,
	}); err != nil {
		return Result{}, err
	}
	if selection.Description != "" {
		if err := emit(Progress{
			Stage: "preparing", Message: fmt.Sprintf("Selected %s for local discovery.", selection.Description),
			LogLine: "Local network selection: " + selection.Description, Target: result.Target, HostsConsidered: hostCount,
		}); err != nil {
			return Result{}, err
		}
	}
	if selection.Warning != "" {
		if err := emit(Progress{
			Stage: "preparing", Message: selection.Warning, LogLine: "Warning: " + selection.Warning,
			Target: result.Target, HostsConsidered: hostCount,
		}); err != nil {
			return Result{}, err
		}
	}
	if err := emit(Progress{
		Stage: "scanning", Message: fmt.Sprintf("Scanning %d host%s in %s.", hostCount, plural(hostCount), result.Target),
		LogLine: fmt.Sprintf("Probe sweep: %d hosts with %d workers (requested %d) and %s timeout", hostCount, effectiveProbeWorkers(options.Concurrency, hostCount), options.Concurrency, options.Timeout),
		Target:  result.Target, HostsConsidered: hostCount,
	}); err != nil {
		return Result{}, err
	}

	observed, evidence, err := probeNetwork(ctx, target, options, emit)
	result.Evidence = evidence
	if err != nil {
		return Result{}, err
	}
	if evidence.HostsIndeterminate > 0 {
		return Result{}, fmt.Errorf("discovery evidence is incomplete: %d of %d probes were indeterminate; refusing to calculate missing-device changes", evidence.HostsIndeterminate, evidence.HostsConsidered)
	}
	result.Evidence.DNSWorkers = effectiveDNSWorkers(options.Concurrency, len(observed))
	if err := emit(Progress{
		Stage: "resolving", Message: fmt.Sprintf("Observed %d host%s. Resolving DNS evidence.", evidence.HostsObserved, plural(evidence.HostsObserved)),
		LogLine: fmt.Sprintf("Probe sweep complete: observed=%d not_observed=%d indeterminate=%d", evidence.HostsObserved, evidence.HostsNotObserved, evidence.HostsIndeterminate),
		Target:  result.Target, HostsConsidered: hostCount, HostsProbed: evidence.HostsProbed,
		HostsObserved: evidence.HostsObserved, HostsNotObserved: evidence.HostsNotObserved, HostsIndeterminate: evidence.HostsIndeterminate,
	}); err != nil {
		return Result{}, err
	}

	resolved, err := resolveHosts(ctx, observed, options)
	if err != nil {
		return Result{}, err
	}
	result.GeneratedAt = options.Now().UTC()
	result.Items = make([]models.Endpoint, 0, len(resolved))
	for _, host := range resolved {
		switch host.DNS.Status {
		case "forward_confirmed":
			result.Evidence.DNSForwardConfirmed++
		case "ptr_only":
			result.Evidence.DNSPtrOnly++
		default:
			result.Evidence.DNSUnresolved++
		}
		result.Items = append(result.Items, endpointFromHost(host, result.ScanID, result.Target, result.GeneratedAt))
	}
	sort.Slice(result.Items, func(i, j int) bool {
		left, _ := netip.ParseAddr(result.Items[i].IP)
		right, _ := netip.ParseAddr(result.Items[j].IP)
		return left.Less(right)
	})
	result.Duration = time.Since(started)
	if err := emit(Progress{
		Stage: "summarizing", Message: fmt.Sprintf("DNS evidence: %d forward-confirmed, %d PTR-only, %d unresolved.", result.Evidence.DNSForwardConfirmed, result.Evidence.DNSPtrOnly, result.Evidence.DNSUnresolved),
		LogLine: fmt.Sprintf("DNS evidence: forward_confirmed=%d ptr_only=%d unresolved=%d", result.Evidence.DNSForwardConfirmed, result.Evidence.DNSPtrOnly, result.Evidence.DNSUnresolved),
		Target:  result.Target, HostsConsidered: hostCount, HostsProbed: evidence.HostsProbed,
		HostsObserved: evidence.HostsObserved, HostsNotObserved: evidence.HostsNotObserved,
	}); err != nil {
		return Result{}, err
	}
	if err := emit(Progress{
		Stage: "complete", Message: fmt.Sprintf("Discovery completed with %d observed endpoint%s.", len(result.Items), plural(len(result.Items))),
		LogLine: fmt.Sprintf("Discovery complete: target=%s observed=%d duration=%s", result.Target, len(result.Items), result.Duration.Round(time.Millisecond)),
		Target:  result.Target, HostsConsidered: hostCount, HostsProbed: evidence.HostsProbed,
		HostsObserved: evidence.HostsObserved, HostsNotObserved: evidence.HostsNotObserved,
	}); err != nil {
		return Result{}, err
	}
	return result, nil
}

func defaultResolver() Resolver {
	return &net.Resolver{PreferGo: true, StrictErrors: true}
}

func resolveTarget(ctx context.Context, rawTarget string, mask int, detector LocalNetworkDetector) (netip.Prefix, LocalNetwork, error) {
	if mask < 16 || mask > 30 {
		return netip.Prefix{}, LocalNetwork{}, errors.New("discovery subnet mask must be between /16 and /30")
	}
	trimmed := strings.TrimSpace(rawTarget)
	if trimmed != "" {
		if strings.Contains(trimmed, "/") {
			prefix, err := netip.ParsePrefix(trimmed)
			if err != nil || !prefix.Addr().Is4() {
				return netip.Prefix{}, LocalNetwork{}, errors.New("discovery target must be a valid IPv4 address or CIDR range")
			}
			if prefix.Bits() < 16 || prefix.Bits() > 30 {
				return netip.Prefix{}, LocalNetwork{}, errors.New("discovery target CIDR mask must be between /16 and /30")
			}
			return prefix.Masked(), LocalNetwork{}, nil
		}
		address, err := netip.ParseAddr(trimmed)
		if err != nil || !address.Is4() {
			return netip.Prefix{}, LocalNetwork{}, errors.New("discovery target must be a valid IPv4 address or CIDR range")
		}
		return netip.PrefixFrom(address, mask).Masked(), LocalNetwork{}, nil
	}

	selection, err := detector.Detect(ctx)
	if err != nil {
		return netip.Prefix{}, LocalNetwork{}, err
	}
	if !selection.Address.IsValid() || !selection.Address.Is4() {
		return netip.Prefix{}, LocalNetwork{}, errors.New("local discovery selected an invalid IPv4 address")
	}
	return netip.PrefixFrom(selection.Address, mask).Masked(), selection, nil
}

func usableHostCount(prefix netip.Prefix) int {
	return (1 << (32 - prefix.Bits())) - 2
}

func probeNetwork(ctx context.Context, target netip.Prefix, options Options, emit func(Progress) error) ([]observedHost, Evidence, error) {
	evidence := Evidence{
		Engine: EngineName, HostsConsidered: usableHostCount(target),
		RequestedConcurrency: options.Concurrency,
	}
	if evidence.HostsConsidered <= 0 {
		return nil, evidence, errors.New("discovery target contains no usable host addresses")
	}
	workerCount := effectiveProbeWorkers(options.Concurrency, evidence.HostsConsidered)
	evidence.ProbeWorkers = workerCount
	if options.WorkerTelemetry != nil {
		options.WorkerTelemetry.ConfigureDiscoveryProbeWorkers(workerCount)
	}
	jobs := make(chan netip.Addr, workerCount)
	outcomes := make(chan probeOutcome, workerCount)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for address := range jobs {
				if options.WorkerTelemetry != nil {
					options.WorkerTelemetry.DiscoveryProbeWorkerStarted()
				}
				outcome := func() probeOutcome {
					if options.WorkerTelemetry != nil {
						defer options.WorkerTelemetry.DiscoveryProbeWorkerFinished()
					}
					return probeAddress(runCtx, options.Pinger, address, options.Timeout)
				}()
				select {
				case outcomes <- outcome:
				case <-runCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		address := target.Addr().Next()
		for index := 0; index < evidence.HostsConsidered; index++ {
			select {
			case jobs <- address:
				address = address.Next()
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(outcomes)
	}()

	backendSet := make(map[string]struct{})
	latencySet := make(map[string]struct{})
	reasonCounts := make(map[string]int)
	observed := make([]observedHost, 0)
	nextProgress := max(1, evidence.HostsConsidered/100)
	progressAt := nextProgress
	var callbackErr error
	for outcome := range outcomes {
		evidence.HostsProbed++
		backend := strings.TrimSpace(outcome.Result.Backend)
		if backend == "" {
			backend = strings.TrimSpace(options.Pinger.Backend())
		}
		if backend != "" {
			backendSet[backend] = struct{}{}
		}
		if source := strings.TrimSpace(outcome.Result.LatencySource); source != "" {
			latencySet[source] = struct{}{}
		}
		switch outcome.State {
		case "observed":
			evidence.HostsObserved++
			observed = append(observed, observedHost{IP: outcome.IP, Result: outcome.Result})
		case "not_observed":
			evidence.HostsNotObserved++
		default:
			evidence.HostsIndeterminate++
			reasonCounts[firstNonEmpty(outcome.Reason, "unknown_probe_error")]++
		}
		if callbackErr == nil && (evidence.HostsProbed >= progressAt || evidence.HostsProbed == evidence.HostsConsidered) {
			callbackErr = emit(Progress{
				Stage: "scanning", Message: fmt.Sprintf("Probed %d of %d hosts; observed %d.", evidence.HostsProbed, evidence.HostsConsidered, evidence.HostsObserved),
				Target: target.String(), HostsConsidered: evidence.HostsConsidered, HostsProbed: evidence.HostsProbed,
				HostsObserved: evidence.HostsObserved, HostsNotObserved: evidence.HostsNotObserved, HostsIndeterminate: evidence.HostsIndeterminate,
			})
			if callbackErr != nil {
				cancel()
			}
			progressAt += nextProgress
		}
	}
	evidence.ProbeBackends = sortedKeys(backendSet)
	evidence.LatencySources = sortedKeys(latencySet)
	if callbackErr != nil {
		return nil, evidence, callbackErr
	}
	if err := ctx.Err(); err != nil {
		return nil, evidence, err
	}
	if evidence.HostsProbed != evidence.HostsConsidered {
		return nil, evidence, fmt.Errorf("discovery probe sweep stopped after %d of %d hosts", evidence.HostsProbed, evidence.HostsConsidered)
	}
	if evidence.HostsIndeterminate > 0 {
		return observed, evidence, fmt.Errorf("discovery probe backend produced indeterminate evidence for %d host%s (%s)", evidence.HostsIndeterminate, plural(evidence.HostsIndeterminate), formatCounts(reasonCounts))
	}
	return observed, evidence, nil
}

func probeAddress(ctx context.Context, pinger Pinger, address netip.Addr, timeout time.Duration) probeOutcome {
	results, err := pinger.Ping(ctx, address.String(), 1, timeout)
	if err != nil {
		return probeOutcome{IP: address, State: "indeterminate", Reason: "probe_error: " + err.Error()}
	}
	if len(results) == 0 {
		return probeOutcome{IP: address, State: "indeterminate", Reason: "empty_probe_result"}
	}
	result := results[0]
	if result.Success {
		return probeOutcome{IP: address, Result: result, State: "observed"}
	}
	reason := strings.ToLower(strings.TrimSpace(result.Error))
	switch reason {
	case "timeout", "unreachable", "ttl_expired", "packet_too_big":
		return probeOutcome{IP: address, Result: result, State: "not_observed", Reason: reason}
	default:
		return probeOutcome{IP: address, Result: result, State: "indeterminate", Reason: firstNonEmpty(reason, "unknown_probe_result")}
	}
}

func resolveHosts(ctx context.Context, hosts []observedHost, options Options) ([]resolvedHost, error) {
	if len(hosts) == 0 {
		if options.WorkerTelemetry != nil {
			options.WorkerTelemetry.ConfigureDiscoveryDNSWorkers(0)
		}
		return []resolvedHost{}, nil
	}
	// ICMP and DNS have different safe fan-out characteristics. Keep DNS below
	// a fixed ceiling even when an operator intentionally configures thousands
	// of concurrent ICMP probes for a large subnet.
	workerCount := effectiveDNSWorkers(options.Concurrency, len(hosts))
	if options.WorkerTelemetry != nil {
		options.WorkerTelemetry.ConfigureDiscoveryDNSWorkers(workerCount)
	}
	jobs := make(chan observedHost, workerCount)
	results := make(chan resolvedHost, workerCount)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for host := range jobs {
				if options.WorkerTelemetry != nil {
					options.WorkerTelemetry.DiscoveryDNSWorkerStarted()
				}
				lookupCtx, lookupCancel := context.WithTimeout(runCtx, options.DNSLookupTimeout)
				dns := func() dnsEvidence {
					defer lookupCancel()
					if options.WorkerTelemetry != nil {
						defer options.WorkerTelemetry.DiscoveryDNSWorkerFinished()
					}
					return resolveDNS(lookupCtx, options.Resolver, host.IP)
				}()
				select {
				case results <- resolvedHost{Host: host, DNS: dns}:
				case <-runCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, host := range hosts {
			select {
			case jobs <- host:
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	resolved := make([]resolvedHost, 0, len(hosts))
	for result := range results {
		resolved = append(resolved, result)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(resolved) != len(hosts) {
		return nil, fmt.Errorf("DNS resolution stopped after %d of %d observed hosts", len(resolved), len(hosts))
	}
	return resolved, nil
}

func effectiveProbeWorkers(requested, hosts int) int {
	return min(max(1, requested), hosts, MaxProbeWorkerLimit)
}

func effectiveDNSWorkers(requested, hosts int) int {
	if hosts <= 0 {
		return 0
	}
	return min(max(1, requested), hosts, MaxDNSWorkerLimit)
}

func resolveDNS(ctx context.Context, resolver Resolver, address netip.Addr) dnsEvidence {
	evidence := dnsEvidence{Status: "unresolved"}
	names, err := resolver.LookupAddr(ctx, address.String())
	if err != nil || len(names) == 0 {
		return evidence
	}
	normalized := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
		if name != "" {
			normalized = append(normalized, name)
		}
	}
	if len(normalized) == 0 {
		return evidence
	}
	sort.Strings(normalized)
	evidence.FQDN = normalized[0]
	evidence.Hostname = strings.SplitN(evidence.FQDN, ".", 2)[0]
	evidence.Status = "ptr_only"
	forward, err := resolver.LookupIPAddr(ctx, evidence.FQDN)
	if err != nil {
		return evidence
	}
	for _, candidate := range forward {
		parsed, ok := netip.AddrFromSlice(candidate.IP)
		if ok && parsed.Unmap() == address {
			evidence.ForwardConfirmed = true
			evidence.Status = "forward_confirmed"
			break
		}
	}
	return evidence
}

func endpointFromHost(host resolvedHost, scanID, source string, generatedAt time.Time) models.Endpoint {
	hostname := host.DNS.Hostname
	if hostname == "" {
		hostname = "host-" + strings.ReplaceAll(host.Host.IP.String(), ".", "-")
	}
	group, entityType, device, vendor, description := classifyHostname(hostname)
	latency := optionalFloat(host.Host.Result.LatencyMs, host.Host.Result.LatencyMeasured)
	resolution := optionalFloat(host.Host.Result.LatencyResolutionMs, host.Host.Result.LatencyResolutionMs > 0)
	upperBound := optionalFloat(host.Host.Result.LatencyUpperBoundMs, host.Host.Result.LatencyCensored && host.Host.Result.LatencyUpperBoundMs > 0)
	elapsed := optionalFloat(host.Host.Result.ProbeElapsedMs, host.Host.Result.ProbeElapsedMs > 0)
	return models.Endpoint{
		IP: host.Host.IP.String(), Hostname: hostname, FQDN: host.DNS.FQDN,
		Dev: group == "development", MonitoringEnabled: models.Bool(true), AlertingEnabled: models.Bool(true),
		DNSStatus: host.DNS.Status, DNSForwardConfirmed: host.DNS.ForwardConfirmed,
		DiscoveredAt: generatedAt.Format(time.RFC3339Nano), DiscoveryScanID: scanID, DiscoverySource: source,
		DiscoveryLatencyMs: latency, DiscoveryProbeBackend: host.Host.Result.Backend,
		DiscoveryLatencySource: host.Host.Result.LatencySource, DiscoveryLatencyResolutionMs: resolution,
		DiscoveryLatencyCensored: host.Host.Result.LatencyCensored, DiscoveryLatencyUpperBoundMs: upperBound,
		DiscoveryProbeElapsedMs: elapsed, Group: group, Description: description,
		EntityType: entityType, Device: device, Vendor: vendor, ClassificationSource: "native:heuristic",
	}
}

func optionalFloat(value float64, present bool) *float64 {
	if !present {
		return nil
	}
	copy := value
	return &copy
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func formatCounts(values map[string]int) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
