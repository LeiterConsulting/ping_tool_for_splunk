package engine

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
)

type Options struct {
	RunOnce         bool
	MaxCycles       int
	EndpointsPath   string
	ReloadEndpoints func() ([]models.Endpoint, bool, error)
}

type pingResult struct {
	Individual []models.PingEvent
	Summary    models.SummaryEvent
	Status     string // success|partial|failed
}

func Run(ctx context.Context, cfg config.Config, endpoints []models.Endpoint, opts Options) error {
	collectorHost, _ := os.Hostname()
	if collectorHost == "" {
		collectorHost = "unknown"
	}
	activeEndpoints := append([]models.Endpoint(nil), endpoints...)
	lastReloadWarn := time.Time{}
	lastReloadErr := ""

	// Output manager is single-threaded: avoids locking and prevents buffer races.
	out, err := output.NewManager(cfg, collectorHost)
	if err != nil {
		return err
	}
	defer out.Close()

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

	jobs := make(chan models.Endpoint, cfg.ParallelThreads)
	results := make(chan pingResult, cfg.ParallelThreads)

	var wg sync.WaitGroup
	for i := 0; i < cfg.ParallelThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ep := range jobs {
				res := runEndpoint(ctx, cfg, collectorHost, ep, pinger)
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	cycle := 0
	for {
		cycle++
		if opts.MaxCycles > 0 && cycle > opts.MaxCycles {
			break
		}
		if cycle > 1 && opts.ReloadEndpoints != nil {
			reloaded, changed, err := opts.ReloadEndpoints()
			if err != nil {
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
				previous := len(activeEndpoints)
				activeEndpoints = append([]models.Endpoint(nil), reloaded...)
				lastReloadErr = ""
				diagnostics.LogInfo("endpoints reloaded", map[string]interface{}{
					"path":               opts.EndpointsPath,
					"cycle":              cycle,
					"previous_endpoints": previous,
					"current_endpoints":  len(activeEndpoints),
				})
			}
		}

		cycleStart := time.Now()
		success := 0
		failed := 0
		partial := 0
		cycleEndpoints := append([]models.Endpoint(nil), activeEndpoints...)

		// Feed jobs for this cycle.
		go func(batch []models.Endpoint) {
			for _, ep := range batch {
				select {
				case jobs <- ep:
				case <-ctx.Done():
					return
				}
			}
			// sentinel: close results by letting collector count
		}(cycleEndpoints)

		// Collect exactly len(endpoints) results.
		for i := 0; i < len(cycleEndpoints); i++ {
			select {
			case r := <-results:
				if r.Status == "success" {
					success++
				} else if r.Status == "partial" {
					partial++
				} else {
					failed++
				}
				if err := out.HandleResult(ctx, r.Individual, r.Summary); err != nil {
					return err
				}
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return ctx.Err()
			}
		}

		if err := out.FlushCycle(ctx); err != nil {
			return err
		}

		durationMs := time.Since(cycleStart).Milliseconds()
		diagnostics.LogCycleSummary(cycle, success, failed, partial, durationMs)
		if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
			diagnostics.LogRuntimeSnapshot("cycle", runtime.NumGoroutine())
		}

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

	close(jobs)
	wg.Wait()
	return nil
}

func runEndpoint(ctx context.Context, cfg config.Config, collectorHost string, ep models.Endpoint, pinger ping.Pinger) pingResult {
	count := cfg.PingsPerCycle
	if count < 1 {
		count = 1
	}
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond

	out := pingResult{}

	pings, err := pinger.Ping(ctx, ep.IP, count, timeout)
	if err != nil {
		if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
			diagnostics.LogWarn("ping failed", map[string]interface{}{"target_ip": ep.IP, "hostname": ep.Hostname, "error": err.Error()})
		}
		// Treat as total failure.
		sumTs := util.FormatDotNetO(time.Now())
		sumID := util.EventID(collectorHost, ep.IP, "summary", sumTs, -1)
		out.Summary = buildSummary(ep, sumID, sumTs, count, 0, 0, 0, 0)
		out.Status = "failed"
		return out
	}

	successCount := 0
	totalLatency := 0
	minLat := int(^uint(0) >> 1)
	maxLat := 0

	if cfg.EmitIndividualPings {
		out.Individual = make([]models.PingEvent, 0, len(pings))
	}

	for i, pr := range pings {
		ts := util.FormatDotNetO(pr.Timestamp)
		id := util.EventID(collectorHost, ep.IP, "ping", ts, i+1)
		if pr.Success {
			successCount++
			totalLatency += pr.LatencyMs
			if pr.LatencyMs < minLat {
				minLat = pr.LatencyMs
			}
			if pr.LatencyMs > maxLat {
				maxLat = pr.LatencyMs
			}
			if cfg.EmitIndividualPings {
				ev := models.PingEvent{
					EventID:      id,
					Timestamp:    ts,
					TargetIP:     ep.IP,
					Hostname:     ep.Hostname,
					Group:        ep.Group,
					Description:  ep.Description,
					EntityType:   ep.EntityType,
					Device:       ep.Device,
					Vendor:       ep.Vendor,
					Notes:        ep.AdditionalNotes,
					Status:       "success",
					LatencyMs:    pr.LatencyMs,
					TTL:          pr.TTL,
					PingNumber:   i + 1,
					PingsInCycle: count,
					RecordType:   "ping",
				}
				out.Individual = append(out.Individual, ev)
			}
			continue
		}

		if cfg.EmitIndividualPings {
			emsg := pr.Error
			ev := models.PingEvent{
				EventID:      id,
				Timestamp:    ts,
				TargetIP:     ep.IP,
				Hostname:     ep.Hostname,
				Group:        ep.Group,
				Description:  ep.Description,
				EntityType:   ep.EntityType,
				Device:       ep.Device,
				Vendor:       ep.Vendor,
				Notes:        ep.AdditionalNotes,
				Status:       "failed",
				LatencyMs:    -1,
				TTL:          -1,
				PingNumber:   i + 1,
				PingsInCycle: count,
				ErrorMessage: &emsg,
				RecordType:   "ping",
			}
			out.Individual = append(out.Individual, ev)
		}
	}

	pktLoss := round2((float64(count-successCount) / float64(count)) * 100)
	avgLat := -1.0
	minOut := -1
	maxOut := -1
	if successCount > 0 {
		avgLat = round2(float64(totalLatency) / float64(successCount))
		minOut = minLat
		maxOut = maxLat
	}

	sumTs := util.FormatDotNetO(time.Now())
	sumID := util.EventID(collectorHost, ep.IP, "summary", sumTs, -1)
	out.Summary = buildSummary(ep, sumID, sumTs, count, successCount, count-successCount, pktLoss, avgLat)
	out.Summary.MinLatencyMs = minOut
	out.Summary.MaxLatencyMs = maxOut

	if successCount == count {
		out.Status = "success"
	} else if successCount == 0 {
		out.Status = "failed"
	} else {
		out.Status = "partial"
	}
	return out
}

func buildSummary(ep models.Endpoint, id, ts string, sent, success, failed int, pktLoss float64, avgLat float64) models.SummaryEvent {
	return models.SummaryEvent{
		EventID:         id,
		Timestamp:       ts,
		TargetIP:        ep.IP,
		Hostname:        ep.Hostname,
		Group:           ep.Group,
		Description:     ep.Description,
		EntityType:      ep.EntityType,
		Device:          ep.Device,
		Vendor:          ep.Vendor,
		Notes:           ep.AdditionalNotes,
		RecordType:      "summary",
		PingsSent:       sent,
		PingsSuccessful: success,
		PingsFailed:     failed,
		PacketLossPct:   pktLoss,
		AvgLatencyMs:    avgLat,
		MinLatencyMs:    -1,
		MaxLatencyMs:    -1,
	}
}

func round2(v float64) float64 {
	// emulate PowerShell Round(...,2)
	if v == 0 {
		return 0
	}
	return float64(int(v*100+0.5)) / 100
}
