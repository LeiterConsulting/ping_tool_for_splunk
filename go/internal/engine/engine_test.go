package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	pingpkg "github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/state"
)

type fakePinger struct {
	results []pingpkg.PingResult
	err     error
	backend string
}

func (p fakePinger) Ping(context.Context, string, int, time.Duration) ([]pingpkg.PingResult, error) {
	return p.results, p.err
}

func (p fakePinger) Backend() string { return p.backend }

func TestRunEndpointProbeErrorIsUnknownAndDoesNotEmitHealthyNumbers(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	evaluator := state.New(state.Policy{})
	result := runEndpoint(context.Background(), cfg, "collector-host", "collector-id", "cycle-id", testEndpoint(), fakePinger{
		err: errors.New("socket permission denied"), backend: "udp_icmp",
	}, evaluator)

	summary := result.Summary
	if summary.MeasurementValid {
		t.Fatal("MeasurementValid = true, want false")
	}
	if summary.State != state.Unknown || summary.ObservationStatus != "probe_error" {
		t.Fatalf("state/status = %q/%q, want unknown/probe_error", summary.State, summary.ObservationStatus)
	}
	if summary.PingsSent != 0 || summary.PingsFailed != 0 {
		t.Fatalf("sent/failed = %d/%d, want 0/0 for an engine error", summary.PingsSent, summary.PingsFailed)
	}
	if summary.PacketLossPct != nil || summary.AvgLatencyMs != nil {
		t.Fatalf("invalid measurement emitted loss/latency: %#v", summary)
	}
}

func TestRunEndpointFullLossIsValidAndTransitionsDownAfterThreshold(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 4
	evaluator := state.New(state.Policy{DownAfterFailures: 3, RecoveryAfterSuccesses: 2})
	pinger := fakePinger{backend: "native", results: failedResults(4, "timeout")}

	var result pingResult
	for cycle := 1; cycle <= 3; cycle++ {
		result = runEndpoint(context.Background(), cfg, "collector-host", "collector-id", "cycle", testEndpoint(), pinger, evaluator)
	}
	summary := result.Summary
	if !summary.MeasurementValid || summary.PacketLossPct == nil || *summary.PacketLossPct != 100 {
		t.Fatalf("full-loss summary = %#v", summary)
	}
	if summary.PingsFailed != 4 || summary.State != state.Down || summary.ConsecutiveFailures != 3 {
		t.Fatalf("full-loss state/counts = %#v", summary)
	}
	if summary.AvgLatencyMs != nil || summary.MinLatencyMs != nil || summary.MaxLatencyMs != nil {
		t.Fatal("full-loss summary must omit latency")
	}
}

func TestRunEndpointPreservesSubMillisecondLatency(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 2
	cfg.EmitIndividualPings = true
	now := time.Now()
	pinger := fakePinger{backend: "native", results: []pingpkg.PingResult{
		{Sequence: 1, SentAt: now, ReceivedAt: now, Timestamp: now, Success: true, LatencyMs: 0.1234, LatencyMeasured: true, LatencySource: "test", LatencyResolutionMs: 0.0001, TTL: 64, Backend: "native"},
		{Sequence: 2, SentAt: now, ReceivedAt: now, Timestamp: now, Success: true, LatencyMs: 0.4567, LatencyMeasured: true, LatencySource: "test", LatencyResolutionMs: 0.0001, TTL: 64, Backend: "native"},
	}}
	result := runEndpoint(context.Background(), cfg, "collector-host", "collector-id", "cycle", testEndpoint(), pinger, state.New(state.Policy{}))

	if result.Summary.AvgLatencyMs == nil || *result.Summary.AvgLatencyMs != 0.29 {
		t.Fatalf("AvgLatencyMs = %#v, want 0.29", result.Summary.AvgLatencyMs)
	}
	if len(result.Individual) != 2 || result.Individual[0].LatencyMs == nil || *result.Individual[0].LatencyMs != 0.123 {
		t.Fatalf("individual latency was not precision-preserving: %#v", result.Individual)
	}
}

func TestRunEndpointBackendFailureResultIsInvalid(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 1
	pinger := fakePinger{backend: "exec", results: failedResults(1, "backend_unavailable")}
	result := runEndpoint(context.Background(), cfg, "collector-host", "collector-id", "cycle", testEndpoint(), pinger, state.New(state.Policy{}))
	if result.Summary.MeasurementValid || result.Summary.State != state.Unknown || result.Summary.PacketLossPct != nil {
		t.Fatalf("backend failure summary = %#v", result.Summary)
	}
}

func TestRunEndpointOmitsAllSignalValuesWhenAnyAttemptIsInvalid(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 2
	now := time.Now()
	pinger := fakePinger{backend: "native", results: []pingpkg.PingResult{
		{Sequence: 1, SentAt: now, ReceivedAt: now, Timestamp: now, Success: true, LatencyMs: 1.25, LatencyMeasured: true, LatencySource: "test", LatencyResolutionMs: 0.01, TTL: 64, Backend: "native"},
		{Sequence: 2, SentAt: now, Timestamp: now, Error: "backend_error", TTL: -1, Backend: "native"},
	}}
	result := runEndpoint(context.Background(), cfg, "collector-host", "collector-id", "cycle", testEndpoint(), pinger, state.New(state.Policy{}))
	if result.Summary.MeasurementValid || result.Summary.PacketLossPct != nil || result.Summary.AvgLatencyMs != nil || result.Summary.MinLatencyMs != nil || result.Summary.MaxLatencyMs != nil {
		t.Fatalf("invalid mixed result emitted signal values: %#v", result.Summary)
	}
}

func TestRunEndpointDoesNotInventLatencyForCensoredReply(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 1
	cfg.EmitIndividualPings = true
	now := time.Now()
	pinger := fakePinger{backend: "windows_icmp", results: []pingpkg.PingResult{{
		Sequence: 1, SentAt: now, ReceivedAt: now, Timestamp: now, Success: true,
		LatencySource: "windows_icmp_rtt", LatencyResolutionMs: 1,
		LatencyCensored: true, LatencyUpperBoundMs: 1, TTL: 128, Backend: "windows_icmp",
	}}}
	result := runEndpoint(context.Background(), cfg, "collector-host", "collector-id", "cycle", testEndpoint(), pinger, state.New(state.Policy{}))
	if result.Summary.AvgLatencyMs != nil || result.Summary.LatencySampleCount != 0 || result.Summary.LatencyCensoredCount != 1 {
		t.Fatalf("censored summary invented an exact latency: %#v", result.Summary)
	}
	if len(result.Individual) != 1 || result.Individual[0].LatencyMs != nil || !result.Individual[0].LatencyCensored || result.Individual[0].LatencyUpperBoundMs == nil {
		t.Fatalf("censored ping event is not truthful: %#v", result.Individual)
	}
}

func TestProbeDispatchDelaySpreadsNormalCycle(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	delay := probeDispatchDelay(cfg, 78, false)
	if delay < 720*time.Millisecond || delay > 722*time.Millisecond {
		t.Fatalf("probeDispatchDelay() = %s, want roughly 720.8ms", delay)
	}
	if got := probeDispatchDelay(cfg, 78, true); got != 0 {
		t.Fatalf("run-once probeDispatchDelay() = %s, want no delay", got)
	}
}

func TestScheduleCapacityRejectsImpossibleCadence(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.CycleIntervalSeconds = 10
	cfg.TimeoutMs = 1000
	cfg.PingsPerCycle = 4
	cfg.ParallelThreads = 1
	if _, err := calculateScheduleCapacity(cfg, 3, false); err == nil {
		t.Fatal("calculateScheduleCapacity() succeeded for an impossible worst-case cadence")
	}
}

func TestScheduleCapacityRejectsSingleEndpointLongerThanInterval(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.CycleIntervalSeconds = 2
	cfg.TimeoutMs = 1000
	cfg.PingsPerCycle = 4
	if _, err := calculateScheduleCapacity(cfg, 1, false); err == nil {
		t.Fatal("calculateScheduleCapacity() accepted a single probe budget longer than its interval")
	}
}

func TestScheduleCapacityAccountsForEveryTimeout(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	capacity, err := calculateScheduleCapacity(cfg, 78, false)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.ProbeBudget != 4500*time.Millisecond {
		t.Fatalf("ProbeBudget = %s, want 4.5s", capacity.ProbeBudget)
	}
	if capacity.WorstCaseLoadPct < 58 || capacity.WorstCaseLoadPct > 59 {
		t.Fatalf("WorstCaseLoadPct = %.2f, want about 58.5", capacity.WorstCaseLoadPct)
	}
}

func TestProbeDispatchDelayDisablesWhenProbeBudgetConsumesCycle(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.CycleIntervalSeconds = 1
	cfg.TimeoutMs = 1000
	if got := probeDispatchDelay(cfg, 10, false); got != 0 {
		t.Fatalf("probeDispatchDelay() = %s, want no delay", got)
	}
}

func TestEndpointSuppressionIsIndependentFromDevClassification(t *testing.T) {
	now := time.Now().UTC()
	legacy := models.Endpoint{IP: "192.0.2.1", Hostname: "legacy", Dev: true}
	if reason, suppressed := endpointSuppression(legacy, now); suppressed {
		t.Fatalf("legacy dev endpoint suppressed with reason %q", reason)
	}
	disabled := legacy
	disabled.MonitoringEnabled = models.Bool(false)
	if reason, suppressed := endpointSuppression(disabled, now); !suppressed || reason != "monitoring_disabled" {
		t.Fatalf("disabled endpoint reason/suppressed = %q/%t", reason, suppressed)
	}
	maintenance := legacy
	maintenance.MaintenanceUntil = now.Add(time.Hour).Format(time.RFC3339)
	maintenance.MaintenanceReason = "change window"
	if reason, suppressed := endpointSuppression(maintenance, now); !suppressed || reason != "scheduled_maintenance" {
		t.Fatalf("maintenance endpoint reason/suppressed = %q/%t", reason, suppressed)
	}
	result := runSuppressedEndpoint(config.Defaults(t.TempDir()), "collector", "collector-id", "cycle", maintenance, now)
	if result.Summary.RecordType != "monitoring_control" || result.Summary.State != "maintenance" ||
		result.Summary.ObservationStatus != "suppressed" || result.Summary.MeasurementValid {
		t.Fatalf("suppression summary = %#v", result.Summary)
	}
	indefinite := models.Endpoint{IP: "192.0.2.2", Hostname: "maintenance", DeviceMode: models.DeviceModeMaintenance}
	if reason, suppressed := endpointSuppression(indefinite, now); !suppressed || reason != "maintenance_mode" {
		t.Fatalf("indefinite maintenance reason/suppressed = %q/%t", reason, suppressed)
	}
	alertingDisabled := models.Endpoint{IP: "192.0.2.3", Hostname: "quiet", AlertingEnabled: models.Bool(false)}
	if reason, suppressed := endpointSuppression(alertingDisabled, now); suppressed {
		t.Fatalf("alerting-disabled endpoint was suppressed with reason %q", reason)
	}
	quietSummary := buildSummary(alertingDisabled, summaryIdentity{})
	if quietSummary.AlertingEnabled || quietSummary.DeviceMode != models.DeviceModeProduction {
		t.Fatalf("alerting policy was not propagated: %#v", quietSummary)
	}
}

func failedResults(count int, reason string) []pingpkg.PingResult {
	results := make([]pingpkg.PingResult, 0, count)
	now := time.Now()
	for index := 0; index < count; index++ {
		results = append(results, pingpkg.PingResult{
			Sequence: index + 1, SentAt: now, Timestamp: now, Success: false, TTL: -1, Error: reason, Backend: "native",
		})
	}
	return results
}

func testEndpoint() models.Endpoint {
	return models.Endpoint{IP: "192.0.2.10", Hostname: "test-endpoint", Group: "test"}
}
