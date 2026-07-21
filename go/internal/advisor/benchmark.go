package advisor

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	pingruntime "github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/ping"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/schedule"
)

type BenchmarkResult struct {
	GeneratedAt           time.Time      `json:"generated_at"`
	DurationMs            int64          `json:"duration_ms"`
	ConfigInventoryMs     int64          `json:"config_inventory_ms"`
	PlannerIterations     int            `json:"planner_iterations"`
	PlannerOpsPerSecond   float64        `json:"planner_ops_per_second"`
	FilesystemWrites      int            `json:"filesystem_writes"`
	FilesystemBytes       int64          `json:"filesystem_bytes"`
	FilesystemMBPerSecond float64        `json:"filesystem_mb_per_second"`
	Goroutines            int            `json:"goroutines"`
	LoopbackTarget        string         `json:"loopback_target"`
	PingBackend           string         `json:"ping_backend"`
	PingFallback          string         `json:"ping_fallback,omitempty"`
	LoopbackAttempts      int            `json:"loopback_attempts"`
	LoopbackSuccesses     int            `json:"loopback_successes"`
	LoopbackDurationMs    int64          `json:"loopback_duration_ms"`
	Schedule              *schedule.Plan `json:"schedule,omitempty"`
	Warnings              []string       `json:"warnings,omitempty"`
	NonSLA                bool           `json:"non_sla"`
}

func BenchmarkDeployment(ctx context.Context, opts AnalyzeOptions) (BenchmarkResult, error) {
	started := time.Now()
	loadStarted := time.Now()
	report := AnalyzeDeployment(ctx, opts)
	result := BenchmarkResult{GeneratedAt: time.Now().UTC(), ConfigInventoryMs: time.Since(loadStarted).Milliseconds(), Goroutines: runtime.NumGoroutine(), NonSLA: true, Schedule: report.Schedule}
	if report.LoadedConfig == nil {
		return result, fmt.Errorf("benchmark requires a loadable configuration")
	}

	const plannerIterations = 50000
	plannerStarted := time.Now()
	for index := 0; index < plannerIterations; index++ {
		_ = schedule.Analyze(*report.LoadedConfig, report.Inventory.SchedulableEndpoints)
	}
	plannerDuration := time.Since(plannerStarted)
	result.PlannerIterations = plannerIterations
	result.PlannerOpsPerSecond = float64(plannerIterations) / plannerDuration.Seconds()

	result.LoopbackTarget = "127.0.0.1"
	var fallback string
	pinger := pingruntime.NewPinger(report.LoadedConfig.Ping.Mode, func(_ string, from string, to string, reason string) {
		fallback = fmt.Sprintf("%s to %s: %s", from, to, reason)
	})
	loopbackStarted := time.Now()
	loopbackCtx, cancelLoopback := context.WithTimeout(ctx, 3*time.Second)
	loopbackResults, loopbackErr := pinger.Ping(loopbackCtx, result.LoopbackTarget, 3, 250*time.Millisecond)
	cancelLoopback()
	result.LoopbackDurationMs = time.Since(loopbackStarted).Milliseconds()
	result.PingBackend = pinger.Backend()
	result.PingFallback = fallback
	result.LoopbackAttempts = len(loopbackResults)
	for _, probe := range loopbackResults {
		if probe.Success {
			result.LoopbackSuccesses++
		}
	}
	if loopbackErr != nil {
		result.Warnings = append(result.Warnings, "configured ping backend loopback self-test failed: "+loopbackErr.Error())
	} else if result.LoopbackSuccesses != result.LoopbackAttempts || result.LoopbackAttempts == 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("configured ping backend returned %d/%d successful loopback replies", result.LoopbackSuccesses, result.LoopbackAttempts))
	}

	benchmarkDir, err := os.MkdirTemp(filepath.Dir(opts.ConfigPath), ".pingmonitor-benchmark-")
	if err != nil {
		result.Warnings = append(result.Warnings, "deployment filesystem benchmark skipped: "+err.Error())
	} else {
		defer os.RemoveAll(benchmarkDir)
		payload := make([]byte, 64*1024)
		if _, err := rand.Read(payload); err != nil {
			return result, err
		}
		const writes = 64
		writeStarted := time.Now()
		for index := 0; index < writes; index++ {
			if err := os.WriteFile(filepath.Join(benchmarkDir, fmt.Sprintf("block-%03d.tmp", index)), payload, 0o600); err != nil {
				result.Warnings = append(result.Warnings, "filesystem benchmark stopped: "+err.Error())
				break
			}
			result.FilesystemWrites++
			result.FilesystemBytes += int64(len(payload))
		}
		writeDuration := time.Since(writeStarted)
		if writeDuration > 0 {
			result.FilesystemMBPerSecond = float64(result.FilesystemBytes) / 1024 / 1024 / writeDuration.Seconds()
		}
	}
	result.DurationMs = time.Since(started).Milliseconds()
	return result, nil
}
