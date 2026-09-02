//go:build windows

package webui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/discovery"
)

func TestDiscoverySchedulerExecutesDueNativeScanOnce(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "discovery-history")
	cfg.Discovery.Schedules = []config.DiscoverySchedule{{
		ID:          "loopback-weekly",
		Enabled:     true,
		Targets:     []string{"127.0.0.0/30"},
		Frequency:   "weekly",
		Day:         "monday",
		Time:        "00:00",
		Timezone:    "UTC",
		TimeoutMs:   250,
		Concurrency: 2,
	}}
	server := newAPIServer(Options{
		ConfigPath:      filepath.Join(root, "config.psd1"),
		EndpointsPath:   filepath.Join(root, "endpoints.csv"),
		RootDir:         root,
		EffectiveConfig: &cfg,
	})
	states := make(map[string]discoveryScheduleRunState)

	server.evaluateDiscoverySchedules(context.Background(), states)
	state := states["loopback-weekly"]
	if state.LastRunAt.IsZero() || state.LastAttemptAt.IsZero() || state.LastError != "" {
		t.Fatalf("first scheduled run state = %#v", state)
	}

	index, err := loadDiscoveryHistoryIndex(cfg.Discovery.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Scans) != 1 {
		t.Fatalf("retained scans after first evaluation = %d, want 1", len(index.Scans))
	}
	if index.Scans[0].ScheduleID != "loopback-weekly" {
		t.Fatalf("schedule id = %q", index.Scans[0].ScheduleID)
	}
	if index.Scans[0].Evidence.Engine != discovery.EngineName {
		t.Fatalf("discovery engine = %q, want %q", index.Scans[0].Evidence.Engine, discovery.EngineName)
	}

	server.evaluateDiscoverySchedules(context.Background(), states)
	index, err = loadDiscoveryHistoryIndex(cfg.Discovery.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Scans) != 1 {
		t.Fatalf("retained scans after duplicate evaluation = %d, want 1", len(index.Scans))
	}
}
