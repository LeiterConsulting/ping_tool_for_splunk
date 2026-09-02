package discovery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pingpkg "github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
)

type stressPinger struct {
	active  atomic.Int64
	maximum atomic.Int64
	calls   atomic.Int64
	readyAt int64
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func newStressPinger(readyAt int) *stressPinger {
	return &stressPinger{
		readyAt: int64(readyAt),
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (p *stressPinger) Backend() string { return "stress_icmp" }

func (p *stressPinger) Ping(ctx context.Context, _ string, _ int, _ time.Duration) ([]pingpkg.PingResult, error) {
	p.calls.Add(1)
	current := p.active.Add(1)
	defer p.active.Add(-1)
	for {
		maximum := p.maximum.Load()
		if current <= maximum || p.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	if current >= p.readyAt {
		p.once.Do(func() { close(p.ready) })
	}
	select {
	case <-p.release:
		return []pingpkg.PingResult{{Backend: p.Backend(), Error: "timeout"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestStressSlash16SweepUsesExactBoundedWorkerCount(t *testing.T) {
	const (
		requestedWorkers = 4096
		effectiveWorkers = MaxProbeWorkerLimit
	)
	pinger := newStressPinger(effectiveWorkers)
	telemetry := runtimeinfo.New("monitor", "cfg", "endpoints", 1)
	resultCh := make(chan Result, 1)
	errCh := make(chan error, 1)
	started := time.Now()
	go func() {
		result, err := Run(context.Background(), Options{
			TargetNetwork:   "198.18.0.0/16",
			SubnetMask:      16,
			Concurrency:     requestedWorkers,
			Pinger:          pinger,
			Resolver:        fakeResolver{},
			WorkerTelemetry: telemetry,
		})
		resultCh <- result
		errCh <- err
	}()

	select {
	case <-pinger.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("discovery did not fill the bounded worker pool")
	}
	if maximum := pinger.maximum.Load(); maximum != effectiveWorkers {
		t.Fatalf("active probes = %d, want exactly %d bounded workers", maximum, effectiveWorkers)
	}
	activeSnapshot := telemetry.Snapshot().DiscoveryProbeWorkers
	if activeSnapshot.Configured != effectiveWorkers || activeSnapshot.Active != effectiveWorkers || activeSnapshot.Peak != effectiveWorkers {
		t.Fatalf("active worker telemetry = %#v", activeSnapshot)
	}
	close(pinger.release)

	var result Result
	select {
	case result = <-resultCh:
	case <-time.After(20 * time.Second):
		t.Fatal("full /16 discovery sweep did not complete")
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if calls := pinger.calls.Load(); calls != 65534 {
		t.Fatalf("probe calls = %d, want 65534", calls)
	}
	if result.Evidence.HostsConsidered != 65534 || result.Evidence.HostsProbed != 65534 ||
		result.Evidence.HostsNotObserved != 65534 || result.Evidence.HostsObserved != 0 || result.Evidence.HostsIndeterminate != 0 {
		t.Fatalf("/16 evidence accounting = %#v", result.Evidence)
	}
	if result.Evidence.RequestedConcurrency != requestedWorkers || result.Evidence.ProbeWorkers != effectiveWorkers || result.Evidence.DNSWorkers != 0 {
		t.Fatalf("/16 worker evidence = %#v", result.Evidence)
	}
	finishedSnapshot := telemetry.Snapshot()
	if finishedSnapshot.DiscoveryProbeWorkers.Active != 0 || finishedSnapshot.DiscoveryProbeWorkers.Peak != effectiveWorkers || finishedSnapshot.DiscoveryDNSWorkers.Configured != 0 {
		t.Fatalf("finished worker telemetry = %#v", finishedSnapshot)
	}
	if elapsed := time.Since(started); elapsed > 20*time.Second {
		t.Fatalf("in-memory /16 sweep took %s", elapsed)
	}
}

func TestStressCancellationDrainsMaximumWorkerPool(t *testing.T) {
	const (
		requestedWorkers = 1024
		effectiveWorkers = MaxProbeWorkerLimit
	)
	pinger := newStressPinger(effectiveWorkers)
	telemetry := runtimeinfo.New("monitor", "cfg", "endpoints", 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := Run(ctx, Options{
			TargetNetwork:   "198.18.0.0/16",
			SubnetMask:      16,
			Concurrency:     requestedWorkers,
			Pinger:          pinger,
			Resolver:        fakeResolver{},
			WorkerTelemetry: telemetry,
		})
		errCh <- err
	}()

	select {
	case <-pinger.ready:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("discovery did not start the configured maximum worker pool")
	}
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v, want context canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("discovery workers did not drain after cancellation")
	}
	if active := pinger.active.Load(); active != 0 {
		t.Fatalf("active probes after cancellation = %d, want 0", active)
	}
	if maximum := pinger.maximum.Load(); maximum != effectiveWorkers {
		t.Fatalf("maximum active probes = %d, want effective bound %d for requested concurrency %d", maximum, effectiveWorkers, requestedWorkers)
	}
	if workers := telemetry.Snapshot().DiscoveryProbeWorkers; workers.Active != 0 || workers.Peak != effectiveWorkers {
		t.Fatalf("worker telemetry after cancellation = %#v", workers)
	}
}
