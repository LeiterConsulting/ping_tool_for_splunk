package engine

import (
	"context"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/schedule"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/state"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
	"github.com/google/uuid"
)

type Options struct {
	RunOnce         bool
	MaxCycles       int
	EndpointsPath   string
	CollectorID     string
	StatePath       string
	ReloadEndpoints func() ([]models.Endpoint, bool, error)
	Runtime         *runtimeinfo.Tracker
	Output          *output.Manager
}

type endpointJob struct {
	Endpoint models.Endpoint
	CycleID  string
}

type pingResult struct {
	Individual []models.PingEvent
	Summary    models.SummaryEvent
	Status     string // success|partial|failed
	IsDev      bool
}

type scheduleCapacity struct {
	ProbeBudget      time.Duration
	Interval         time.Duration
	DispatchDelay    time.Duration
	WorstCaseLoadPct float64
}

func ValidateSchedule(cfg config.Config, endpointCount int) error {
	return schedule.Analyze(cfg, endpointCount).Error()
}

func Run(ctx context.Context, cfg config.Config, endpoints []models.Endpoint, opts Options) error {
	collectorHost, _ := os.Hostname()
	if collectorHost == "" {
		collectorHost = "unknown"
	}
	collectorID := opts.CollectorID
	if collectorID == "" {
		collectorID = collectorHost
	}
	activeEndpoints := append([]models.Endpoint(nil), endpoints...)
	startupProbeEndpoints, _ := partitionEndpoints(activeEndpoints, time.Now())
	startupCapacity, err := calculateScheduleCapacity(cfg, len(startupProbeEndpoints), opts.RunOnce)
	if err != nil {
		return err
	}
	diagnostics.LogScheduleCapacity(len(startupProbeEndpoints), cfg.ParallelThreads, startupCapacity.ProbeBudget.Milliseconds(), startupCapacity.Interval.Milliseconds(), startupCapacity.DispatchDelay.Milliseconds(), startupCapacity.WorstCaseLoadPct)
	lastReloadWarn := time.Time{}
	lastReloadErr := ""

	// The output manager is single-threaded. A caller-owned manager allows the
	// monitor and discovery scheduler to share one durable file/HEC pipeline.
	out := opts.Output
	ownsOutput := false
	if out == nil {
		out, err = output.NewManager(cfg, collectorHost, collectorID)
		if err != nil {
			return err
		}
		ownsOutput = true
	}
	if ownsOutput {
		defer out.Close()
	}

	onFallback := func(ip string, from string, to string, reason string) {
		if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
			diagnostics.LogWarn("ping fallback", map[string]interface{}{
				"target_ip": ip,
				"from":      from,
				"to":        to,
				"reason":    reason,
			})
		}
	}
	pinger := ping.NewPinger(cfg.Ping.Mode, onFallback)
	statePolicy := state.Policy{
		DownAfterFailures:      cfg.Health.DownAfterFailures,
		RecoveryAfterSuccesses: cfg.Health.RecoveryAfterSuccesses,
	}
	evaluator := state.New(statePolicy)
	if opts.StatePath != "" {
		maxStateAge := time.Duration(cfg.CycleIntervalSeconds*cfg.Health.StaleAfterIntervals) * time.Second
		loadedEvaluator, stateErr := state.Load(opts.StatePath, statePolicy, maxStateAge)
		if stateErr != nil {
			diagnostics.LogWarn("state checkpoint load failed; starting with unknown state", map[string]interface{}{
				"path": opts.StatePath, "error": stateErr.Error(),
			})
		} else {
			evaluator = loadedEvaluator
		}
	}

	jobs := make(chan endpointJob, cfg.ParallelThreads)
	results := make(chan pingResult, cfg.ParallelThreads)
	opts.Runtime.ConfigureMonitoringWorkers(cfg.ParallelThreads)

	var wg sync.WaitGroup
	for i := 0; i < cfg.ParallelThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				opts.Runtime.MonitoringWorkerStarted()
				res := func() pingResult {
					defer opts.Runtime.MonitoringWorkerFinished()
					return runEndpoint(ctx, cfg, collectorHost, collectorID, job.CycleID, job.Endpoint, pinger, evaluator)
				}()
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	defer func() {
		close(jobs)
		wg.Wait()
	}()

	cycle := 0
	for {
		cycle++
		if opts.MaxCycles > 0 && cycle > opts.MaxCycles {
			break
		}
		if cycle > 1 && opts.ReloadEndpoints != nil {
			reloaded, changed, err := opts.ReloadEndpoints()
			if err != nil {
				opts.Runtime.EndpointReloadFailed(err)
				msg := err.Error()
				if msg != lastReloadErr || time.Since(lastReloadWarn) >= 30*time.Second {
					diagnostics.LogWarn("endpoints reload failed; using previous set", map[string]interface{}{
						"path":      opts.EndpointsPath,
						"error":     msg,
						"endpoints": len(activeEndpoints),
					})
					lastReloadWarn = time.Now()
					lastReloadErr = msg
				}
			} else if changed && len(reloaded) > 0 {
				reloadedProbeEndpoints, _ := partitionEndpoints(reloaded, time.Now())
				if _, capacityErr := calculateScheduleCapacity(cfg, len(reloadedProbeEndpoints), opts.RunOnce); capacityErr != nil {
					opts.Runtime.EndpointReloadFailed(capacityErr)
					diagnostics.LogWarn("endpoints reload rejected; using previous set", map[string]interface{}{
						"path": opts.EndpointsPath, "cycle": cycle, "error": capacityErr.Error(),
						"candidate_endpoints": len(reloaded), "current_endpoints": len(activeEndpoints),
					})
				} else {
					previous := len(activeEndpoints)
					activeEndpoints = append([]models.Endpoint(nil), reloaded...)
					lastReloadErr = ""
					endpointRevision, _ := revision.File(opts.EndpointsPath)
					opts.Runtime.EndpointReloaded(endpointRevision, len(activeEndpoints), time.Now())
					diagnostics.LogInfo("endpoints reloaded", map[string]interface{}{
						"path":               opts.EndpointsPath,
						"cycle":              cycle,
						"previous_endpoints": previous,
						"current_endpoints":  len(activeEndpoints),
					})
				}
			}
		}

		cycleStart := time.Now()
		cycleID := uuid.NewString()
		success := 0
		failed := 0
		partial := 0
		cycleEndpoints := append([]models.Endpoint(nil), activeEndpoints...)
		probeEndpoints, suppressedEndpoints := partitionEndpoints(cycleEndpoints, cycleStart)
		capacity, err := calculateScheduleCapacity(cfg, len(probeEndpoints), opts.RunOnce)
		if err != nil {
			return err
		}
		opts.Runtime.CycleStarted(cycle, cycleID, len(cycleEndpoints), cycleStart)

		for _, suppressed := range suppressedEndpoints {
			result := runSuppressedEndpoint(cfg, collectorHost, collectorID, cycleID, suppressed, cycleStart)
			if err := out.HandleResult(ctx, nil, result.Summary); err != nil {
				return err
			}
		}

		// Spread dispatches across the cycle. This avoids a synchronized burst of
		// ICMP and output work while keeping each endpoint close to the configured
		// observation interval.
		dispatchDelay := capacity.DispatchDelay
		go func(batch []models.Endpoint, batchCycleID string, delay time.Duration) {
			for index, ep := range batch {
				select {
				case jobs <- endpointJob{Endpoint: ep, CycleID: batchCycleID}:
				case <-ctx.Done():
					return
				}
				if delay > 0 && index < len(batch)-1 {
					timer := time.NewTimer(delay)
					select {
					case <-timer.C:
					case <-ctx.Done():
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}
						return
					}
				}
			}
		}(probeEndpoints, cycleID, dispatchDelay)

		// Collect exactly one result for each endpoint that was eligible to probe.
		for i := 0; i < len(probeEndpoints); i++ {
			select {
			case r := <-results:
				if !r.IsDev {
					if r.Status == "success" {
						success++
					} else if r.Status == "partial" {
						partial++
					} else {
						failed++
					}
				}
				if err := out.HandleResult(ctx, r.Individual, r.Summary); err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if err := out.FlushCycle(ctx); err != nil {
			return err
		}
		if opts.StatePath != "" {
			if err := evaluator.Save(opts.StatePath); err != nil {
				diagnostics.LogWarn("state checkpoint save failed", map[string]interface{}{
					"path": opts.StatePath, "error": err.Error(),
				})
			}
		}

		duration := time.Since(cycleStart)
		durationMs := duration.Milliseconds()
		overrun := maxDuration(0, duration-capacity.Interval)
		diagnostics.LogCycleSummary(cycle, success, failed, partial, durationMs, overrun.Milliseconds(), dispatchDelay.Milliseconds(), capacity.WorstCaseLoadPct)
		if overrun > 0 {
			diagnostics.LogWarn("monitoring cycle exceeded configured interval", map[string]interface{}{
				"cycle": cycle, "overrun_ms": overrun.Milliseconds(), "endpoints": len(cycleEndpoints),
				"worst_case_load_pct": math.Round(capacity.WorstCaseLoadPct*10) / 10,
			})
		}
		if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
			diagnostics.LogRuntimeSnapshot("cycle", runtime.NumGoroutine())
		}
		nextCycle := time.Time{}
		if !opts.RunOnce {
			nextCycle = cycleStart.Add(time.Duration(cfg.CycleIntervalSeconds) * time.Second)
		}
		opts.Runtime.CycleCompleted(cycle, time.Now(), duration, nextCycle, success, partial, failed)

		if opts.RunOnce {
			break
		}

		// Sleep remaining time in interval.
		interval := time.Duration(cfg.CycleIntervalSeconds) * time.Second
		sleep := interval - time.Since(cycleStart)
		if sleep < 0 {
			sleep = 0
		}
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
	}

	return nil
}

func probeDispatchDelay(cfg config.Config, endpointCount int, runOnce bool) time.Duration {
	capacity, err := calculateScheduleCapacity(cfg, endpointCount, runOnce)
	if err != nil {
		return 0
	}
	return capacity.DispatchDelay
}

func calculateScheduleCapacity(cfg config.Config, endpointCount int, runOnce bool) (scheduleCapacity, error) {
	if endpointCount == 0 {
		interval := time.Duration(cfg.CycleIntervalSeconds) * time.Second
		return scheduleCapacity{Interval: interval}, nil
	}
	plan := schedule.Analyze(cfg, endpointCount)
	capacity := scheduleCapacity{
		ProbeBudget:      plan.ProbeBudgetDuration,
		Interval:         plan.IntervalDuration,
		DispatchDelay:    plan.DispatchDelayDuration,
		WorstCaseLoadPct: plan.AverageWorkerLoadPct,
	}
	if runOnce {
		capacity.DispatchDelay = 0
		return capacity, nil
	}
	if err := plan.Error(); err != nil {
		return capacity, err
	}
	return capacity, nil
}

func partitionEndpoints(endpoints []models.Endpoint, now time.Time) ([]models.Endpoint, []models.Endpoint) {
	probe := make([]models.Endpoint, 0, len(endpoints))
	suppressed := make([]models.Endpoint, 0)
	for _, endpoint := range endpoints {
		if _, isSuppressed := endpointSuppression(endpoint, now); isSuppressed {
			suppressed = append(suppressed, endpoint)
			continue
		}
		probe = append(probe, endpoint)
	}
	return probe, suppressed
}

func endpointSuppression(endpoint models.Endpoint, now time.Time) (string, bool) {
	if endpoint.EffectiveDeviceMode() == models.DeviceModeMaintenance {
		return "maintenance_mode", true
	}
	if !endpoint.IsMonitoringEnabled() {
		return "monitoring_disabled", true
	}
	if value := strings.TrimSpace(endpoint.MaintenanceUntil); value != "" {
		until, err := time.Parse(time.RFC3339, value)
		if err == nil && now.Before(until) {
			return "scheduled_maintenance", true
		}
	}
	return "", false
}

func runSuppressedEndpoint(cfg config.Config, collectorHost string, collectorID string, cycleID string, endpoint models.Endpoint, observedAt time.Time) pingResult {
	reason, _ := endpointSuppression(endpoint, observedAt)
	stateName := "disabled"
	if reason == "scheduled_maintenance" || reason == "maintenance_mode" {
		stateName = "maintenance"
	}
	timestamp := util.FormatDotNetO(observedAt)
	endpointID := models.StableEndpointID(endpoint.EndpointID, endpoint.IP)
	summary := buildSummary(endpoint, summaryIdentity{
		EventID:     util.EventID(collectorHost, endpoint.IP, "monitoring_control", timestamp, -1),
		CollectorID: collectorID, EndpointID: endpointID, CycleID: cycleID,
		Timestamp: timestamp, RecordType: "monitoring_control", ProbeBackend: "suppressed",
	})
	summary.MeasurementValid = false
	summary.ObservationStatus = "suppressed"
	summary.State = stateName
	summary.StateReason = reason
	summary.StateConfidence = "confirmed"
	summary.CycleIntervalSeconds = cfg.CycleIntervalSeconds
	summary.StaleAfterIntervals = cfg.Health.StaleAfterIntervals
	summary.StaleAfterSeconds = cfg.CycleIntervalSeconds * cfg.Health.StaleAfterIntervals
	return pingResult{Summary: summary, Status: "suppressed", IsDev: endpoint.Dev}
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func runEndpoint(ctx context.Context, cfg config.Config, collectorHost string, collectorID string, cycleID string, ep models.Endpoint, pinger ping.Pinger, evaluator *state.Evaluator) pingResult {
	count := cfg.PingsPerCycle
	if count < 1 {
		count = 1
	}
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	recordTypeSummary := "summary"
	recordTypePing := "ping"
	if ep.Dev {
		recordTypeSummary = "summary_dev"
		recordTypePing = "ping_dev"
	}

	out := pingResult{IsDev: ep.Dev}
	endpointID := models.StableEndpointID(ep.EndpointID, ep.IP)

	pings, err := pinger.Ping(ctx, ep.IP, count, timeout)
	if err != nil {
		if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
			diagnostics.LogWarn("ping failed", map[string]interface{}{"target_ip": ep.IP, "hostname": ep.Hostname, "error": err.Error()})
		}
		completedAt := time.Now()
		sumTs := util.FormatDotNetO(completedAt)
		sumID := util.EventID(collectorHost, ep.IP, recordTypeSummary, sumTs, -1)
		stateSnapshot := evaluator.Evaluate(endpointID, state.Observation{Valid: false, ObservedAt: completedAt})
		out.Summary = buildSummary(ep, summaryIdentity{
			EventID: sumID, CollectorID: collectorID, EndpointID: endpointID, CycleID: cycleID,
			Timestamp: sumTs, RecordType: recordTypeSummary, ProbeBackend: pinger.Backend(),
		})
		out.Summary.MeasurementValid = false
		out.Summary.ObservationStatus = "probe_error"
		out.Summary.ObservationError = err.Error()
		applyState(&out.Summary, stateSnapshot, cfg)
		out.Status = "failed"
		return out
	}

	successCount := 0
	totalLatency := 0.0
	minLat := math.Inf(1)
	maxLat := 0.0
	latencySamples := 0
	latencyCensored := 0
	latencySource := ""
	latencyResolution := 0.0
	latencyUpperBound := 0.0
	measurementValid := len(pings) == count
	observationError := ""
	probeBackend := pinger.Backend()
	if len(pings) != count {
		observationError = "unexpected_probe_result_count"
	}

	if cfg.EmitIndividualPings {
		out.Individual = make([]models.PingEvent, 0, len(pings))
	}

	for i, pr := range pings {
		sequence := pr.Sequence
		if sequence < 1 {
			sequence = i + 1
		}
		observedAt := pr.Timestamp
		if observedAt.IsZero() {
			observedAt = time.Now()
		}
		sentAt := pr.SentAt
		if sentAt.IsZero() {
			sentAt = observedAt
		}
		ts := util.FormatDotNetO(observedAt)
		id := util.EventID(collectorHost, ep.IP, recordTypePing, ts, sequence)
		if pr.Backend != "" {
			if probeBackend == "" {
				probeBackend = pr.Backend
			} else if probeBackend != pr.Backend {
				probeBackend = "mixed"
			}
		}
		attemptValid := pr.Success || isValidNoReply(pr.Error)
		if !attemptValid {
			measurementValid = false
			if observationError == "" {
				observationError = pr.Error
			}
		}

		event := models.PingEvent{
			SchemaVersion: models.SchemaVersion,
			EventID:       id, CollectorID: collectorID, EndpointID: endpointID, CycleID: cycleID,
			Timestamp: ts, SentAt: util.FormatDotNetO(sentAt),
			TargetIP: ep.IP, Hostname: ep.Hostname, FQDN: ep.FQDN, Dev: ep.Dev,
			DeviceMode: ep.EffectiveDeviceMode(), MonitoringEnabled: ep.IsMonitoringEnabled(),
			AlertingEnabled: ep.IsAlertingEnabled(), AlertingReason: ep.AlertingReason,
			AssetID: ep.AssetID, DynamicAddress: ep.DynamicAddress,
			SubnetID: ep.SubnetID, SubnetName: ep.SubnetName, SubnetVLAN: ep.SubnetVLAN,
			SubnetLocation: ep.SubnetLocation, AddressingMode: ep.AddressingMode, RoutingDomain: ep.RoutingDomain,
			ClassificationSource: ep.ClassificationSource, Group: ep.Group,
			Description: ep.Description, EntityType: ep.EntityType, Device: ep.Device, Vendor: ep.Vendor,
			Notes: ep.AdditionalNotes, MeasurementValid: attemptValid, ProbeBackend: pr.Backend,
			PingNumber: sequence, PingsInCycle: count, RecordType: recordTypePing,
		}
		if pr.ProbeElapsedMs > 0 {
			elapsed := round3(pr.ProbeElapsedMs)
			event.ProbeElapsedMs = &elapsed
		}
		if pr.ICMPStatusCode != nil {
			statusCode := *pr.ICMPStatusCode
			event.ICMPStatusCode = &statusCode
		}
		if pr.Success {
			successCount++
			event.Status = "success"
			event.ObservationStatus = "reply"
			event.LatencySource = pr.LatencySource
			if pr.LatencyResolutionMs > 0 {
				resolution := pr.LatencyResolutionMs
				event.LatencyResolutionMs = &resolution
				if resolution > latencyResolution {
					latencyResolution = resolution
				}
			}
			if pr.LatencySource != "" {
				if latencySource == "" {
					latencySource = pr.LatencySource
				} else if latencySource != pr.LatencySource {
					latencySource = "mixed"
				}
			}
			if pr.LatencyMeasured {
				latencySamples++
				totalLatency += pr.LatencyMs
				if pr.LatencyMs < minLat {
					minLat = pr.LatencyMs
				}
				if pr.LatencyMs > maxLat {
					maxLat = pr.LatencyMs
				}
				latency := round3(pr.LatencyMs)
				event.LatencyMs = &latency
			}
			if pr.LatencyCensored {
				latencyCensored++
				event.LatencyCensored = true
				if pr.LatencyUpperBoundMs > 0 {
					upper := pr.LatencyUpperBoundMs
					event.LatencyUpperBoundMs = &upper
					if upper > latencyUpperBound {
						latencyUpperBound = upper
					}
				}
			}
			if pr.TTL >= 0 {
				ttl := pr.TTL
				event.TTL = &ttl
			}
			if !pr.ReceivedAt.IsZero() {
				event.ReceivedAt = util.FormatDotNetO(pr.ReceivedAt)
			}
			if cfg.EmitIndividualPings {
				out.Individual = append(out.Individual, event)
			}
			continue
		}

		if cfg.EmitIndividualPings {
			emsg := pr.Error
			event.Status = "failed"
			event.ObservationStatus = "no_reply"
			if !attemptValid {
				event.ObservationStatus = "probe_error"
			}
			event.ErrorMessage = &emsg
			out.Individual = append(out.Individual, event)
		}
	}

	completedAt := time.Now()
	stateSnapshot := evaluator.Evaluate(endpointID, state.Observation{
		Valid: measurementValid, Sent: count, Successful: successCount, ObservedAt: completedAt,
	})
	observationStatus := "reply"
	if !measurementValid {
		observationStatus = "probe_error"
	} else if successCount == 0 {
		observationStatus = "no_reply"
	} else if successCount < count {
		observationStatus = "partial_reply"
	}

	sumTs := util.FormatDotNetO(completedAt)
	sumID := util.EventID(collectorHost, ep.IP, recordTypeSummary, sumTs, -1)
	out.Summary = buildSummary(ep, summaryIdentity{
		EventID: sumID, CollectorID: collectorID, EndpointID: endpointID, CycleID: cycleID,
		Timestamp: sumTs, RecordType: recordTypeSummary, ProbeBackend: probeBackend,
	})
	out.Summary.MeasurementValid = measurementValid
	out.Summary.ObservationStatus = observationStatus
	out.Summary.ObservationError = observationError
	out.Summary.PingsSent = count
	out.Summary.PingsSuccessful = successCount
	out.Summary.PingsFailed = count - successCount
	applyState(&out.Summary, stateSnapshot, cfg)
	out.Summary.LatencySampleCount = latencySamples
	out.Summary.LatencyCensoredCount = latencyCensored
	out.Summary.LatencySource = latencySource
	if latencyResolution > 0 {
		resolution := latencyResolution
		out.Summary.LatencyResolutionMs = &resolution
	}
	if latencyUpperBound > 0 {
		upper := latencyUpperBound
		out.Summary.LatencyUpperBoundMs = &upper
	}

	if measurementValid {
		loss := round2((float64(count-successCount) / float64(count)) * 100)
		out.Summary.PacketLossPct = &loss
	}
	if measurementValid && latencySamples > 0 {
		avgLatency := round3(totalLatency / float64(latencySamples))
		minimumLatency := round3(minLat)
		maximumLatency := round3(maxLat)
		out.Summary.AvgLatencyMs = &avgLatency
		out.Summary.MinLatencyMs = &minimumLatency
		out.Summary.MaxLatencyMs = &maximumLatency
	}

	if !measurementValid {
		out.Status = "failed"
	} else if successCount == count {
		out.Status = "success"
	} else if successCount == 0 {
		out.Status = "failed"
	} else {
		out.Status = "partial"
	}
	return out
}

type summaryIdentity struct {
	EventID      string
	CollectorID  string
	EndpointID   string
	CycleID      string
	Timestamp    string
	RecordType   string
	ProbeBackend string
}

func buildSummary(ep models.Endpoint, identity summaryIdentity) models.SummaryEvent {
	return models.SummaryEvent{
		SchemaVersion: models.SchemaVersion,
		EventID:       identity.EventID, CollectorID: identity.CollectorID, EndpointID: identity.EndpointID,
		CycleID: identity.CycleID, Timestamp: identity.Timestamp, ProbeBackend: identity.ProbeBackend,
		TargetIP: ep.IP, Hostname: ep.Hostname, FQDN: ep.FQDN, Dev: ep.Dev,
		DeviceMode: ep.EffectiveDeviceMode(), MonitoringEnabled: ep.IsMonitoringEnabled(),
		AlertingEnabled: ep.IsAlertingEnabled(), AlertingReason: ep.AlertingReason,
		AssetID: ep.AssetID, DynamicAddress: ep.DynamicAddress,
		SubnetID: ep.SubnetID, SubnetName: ep.SubnetName, SubnetVLAN: ep.SubnetVLAN,
		SubnetLocation: ep.SubnetLocation, AddressingMode: ep.AddressingMode, RoutingDomain: ep.RoutingDomain,
		ClassificationSource: ep.ClassificationSource, MaintenanceUntil: ep.MaintenanceUntil,
		MaintenanceReason: ep.MaintenanceReason, Group: ep.Group,
		Description: ep.Description, EntityType: ep.EntityType, Device: ep.Device, Vendor: ep.Vendor,
		Notes: ep.AdditionalNotes, RecordType: identity.RecordType,
	}
}

func applyState(summary *models.SummaryEvent, snapshot state.Snapshot, cfg config.Config) {
	summary.State = snapshot.State
	summary.StateReason = snapshot.Reason
	summary.PreviousState = snapshot.PreviousState
	summary.StateChangedAt = util.FormatDotNetO(snapshot.ChangedAt)
	summary.ConsecutiveSuccesses = snapshot.ConsecutiveSuccesses
	summary.ConsecutiveFailures = snapshot.ConsecutiveFailures
	summary.DownAfterFailures = cfg.Health.DownAfterFailures
	summary.RecoveryAfterSuccesses = cfg.Health.RecoveryAfterSuccesses
	summary.CycleIntervalSeconds = cfg.CycleIntervalSeconds
	summary.StaleAfterIntervals = cfg.Health.StaleAfterIntervals
	summary.StaleAfterSeconds = cfg.CycleIntervalSeconds * cfg.Health.StaleAfterIntervals
	switch snapshot.Reason {
	case "down_pending", "recovery_pending":
		summary.StateConfidence = "pending"
	case "measurement_invalid":
		summary.StateConfidence = "unknown"
	default:
		summary.StateConfidence = "confirmed"
	}
}

func isValidNoReply(reason string) bool {
	switch reason {
	case "timeout", "unreachable", "no_reply", "ttl_expired", "packet_too_big":
		return true
	default:
		return false
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
