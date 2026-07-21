package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/webui"
)

func TestRunMonitorLoopAppliesControlledRestartInProcess(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	cfg := config.Defaults(root)
	cfg.PingsPerCycle = 1
	cfg.TimeoutMs = 100
	cfg.CycleIntervalSeconds = 2
	cfg.ParallelThreads = 1
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	cfg.LogPath = filepath.Join(root, "ping.log")
	if _, err := config.SaveConfig(context.Background(), configPath, root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n127.0.0.1,loopback\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial, err := loadRuntimeDeployment(context.Background(), configPath, endpointsPath, root, "")
	if err != nil {
		t.Fatal(err)
	}
	tracker := runtimeinfo.New("monitor", initial.ConfigRevision, initial.EndpointsRevision, 1)
	store := newEffectiveConfigStore(initial.Config)
	restarts := make(chan webui.RestartRequest, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runMonitorLoop(ctx, initial, configPath, endpointsPath, root, "", "test-collector", false, 0, tracker, store, restarts)
	}()
	waitForRuntime(t, tracker, 5*time.Second, func(snapshot runtimeinfo.Snapshot) bool { return snapshot.State == "running" })

	cfg.EmitIndividualPings = !cfg.EmitIndividualPings
	if _, err := config.SaveConfig(context.Background(), configPath, root, cfg); err != nil {
		cancel()
		t.Fatal(err)
	}
	configRevision, _ := revision.File(configPath)
	endpointsRevision, _ := revision.File(endpointsPath)
	tracker.RestartRequested()
	restarts <- webui.RestartRequest{ConfigRevision: configRevision, EndpointsRevision: endpointsRevision}
	waitForRuntime(t, tracker, 5*time.Second, func(snapshot runtimeinfo.Snapshot) bool {
		return snapshot.RestartCount == 1 && snapshot.State == "running" && snapshot.EffectiveConfigRevision == configRevision
	})
	active, ok := store.Get()
	if !ok || active.EmitIndividualPings != cfg.EmitIndividualPings {
		t.Fatalf("effective config was not replaced: %#v", active)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runMonitorLoop() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runMonitorLoop() did not stop after cancellation")
	}
}

func waitForRuntime(t *testing.T, tracker *runtimeinfo.Tracker, timeout time.Duration, condition func(runtimeinfo.Snapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition(tracker.Snapshot()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("runtime condition was not reached")
}
