package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

type logLine map[string]interface{}

func LogStartup(configSource string, cfg config.Config, endpointCount int) {
	ll := logLine{
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"level":      "info",
		"msg":        "pingmonitor starting",
		"version":    "v5.0.0-dev",
		"config_src": configSource,
		"endpoints":  endpointCount,
		"output":     cfg.OutputMode,
		"metrics":    map[string]interface{}{"enabled": cfg.Metrics.Enabled, "mode": cfg.Metrics.Mode},
	}
	write(ll)
}

func LogRuntimeSnapshot(phase string, goroutines int) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	ll := logLine{
		"ts":            time.Now().UTC().Format(time.RFC3339Nano),
		"level":         "info",
		"msg":           "runtime snapshot",
		"phase":         phase,
		"goroutines":    goroutines,
		"heap_alloc_mb": float64(ms.HeapAlloc) / (1024 * 1024),
		"sys_mb":        float64(ms.Sys) / (1024 * 1024),
		"num_gc":        ms.NumGC,
	}
	write(ll)
}

func LogCycleSummary(cycle int, success int, failed int, partial int, durationMs int64) {
	ll := logLine{
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"level":       "info",
		"msg":         "cycle complete",
		"cycle":       cycle,
		"success":     success,
		"failed":      failed,
		"partial":     partial,
		"duration_ms": durationMs,
	}
	write(ll)
}

func LogWarn(msg string, fields map[string]interface{}) {
	ll := logLine{"ts": time.Now().UTC().Format(time.RFC3339Nano), "level": "warn", "msg": msg}
	for k, v := range fields {
		ll[k] = v
	}
	write(ll)
}

func LogError(msg string, err error, fields map[string]interface{}) {
	ll := logLine{"ts": time.Now().UTC().Format(time.RFC3339Nano), "level": "error", "msg": msg, "error": err.Error()}
	for k, v := range fields {
		ll[k] = v
	}
	write(ll)
}

func write(ll logLine) {
	b, err := json.Marshal(ll)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log marshal failed:", err)
		return
	}
	fmt.Fprintln(os.Stdout, string(b))
}
