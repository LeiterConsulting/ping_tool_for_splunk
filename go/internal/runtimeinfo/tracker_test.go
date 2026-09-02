package runtimeinfo

import (
	"errors"
	"testing"
	"time"
)

func TestTrackerRecordsRuntimeTruth(t *testing.T) {
	tracker := New("monitor", "cfg-a", "end-a", 2)
	started := time.Now().Add(-250 * time.Millisecond)
	tracker.CycleStarted(3, "cycle-3", 4, started)
	running := tracker.Snapshot()
	if running.State != "running" || !running.CycleRunning || running.ActiveEndpoints != 4 {
		t.Fatalf("running snapshot = %#v", running)
	}

	completed := time.Now()
	tracker.CycleCompleted(3, completed, 250*time.Millisecond, completed.Add(time.Minute), 2, 1, 1)
	tracker.EndpointReloaded("end-b", 4, completed)
	tracker.EndpointReloadFailed(errors.New("invalid endpoint file"))
	finished := tracker.Snapshot()
	if finished.CycleRunning || finished.LastProductionSuccess != 2 || finished.LastProductionFailed != 1 {
		t.Fatalf("finished snapshot = %#v", finished)
	}
	if finished.EffectiveEndpointsRevision != "end-b" || finished.LastEndpointReloadError == "" {
		t.Fatalf("reload snapshot = %#v", finished)
	}
}

func TestTrackerRecordsControlledRestart(t *testing.T) {
	tracker := New("monitor", "cfg-a", "end-a", 2)
	tracker.RestartRequested()
	requested := tracker.Snapshot()
	if !requested.Restarting || requested.State != "restarting" {
		t.Fatalf("requested snapshot = %#v", requested)
	}

	restartedAt := time.Now()
	tracker.Restarted("cfg-b", "end-b", 3, restartedAt)
	restarted := tracker.Snapshot()
	if restarted.Restarting || restarted.RestartCount != 1 || restarted.EffectiveConfigRevision != "cfg-b" || restarted.ActiveEndpoints != 3 {
		t.Fatalf("restarted snapshot = %#v", restarted)
	}

	tracker.RestartRequested()
	tracker.RestartFailed(errors.New("config changed during restart"))
	failed := tracker.Snapshot()
	if failed.Restarting || failed.LastRestartError == "" || failed.EffectiveConfigRevision != "cfg-b" {
		t.Fatalf("failed restart snapshot = %#v", failed)
	}
}

func TestTrackerRecordsWorkerUtilizationAndPeak(t *testing.T) {
	tracker := New("monitor", "cfg-a", "end-a", 2)
	tracker.ConfigureMonitoringWorkers(8)
	tracker.MonitoringWorkerStarted()
	tracker.MonitoringWorkerStarted()
	tracker.MonitoringWorkerFinished()
	tracker.ConfigureDiscoveryProbeWorkers(32)
	tracker.DiscoveryProbeWorkerStarted()
	tracker.DiscoveryProbeWorkerFinished()
	tracker.ConfigureDiscoveryDNSWorkers(12)

	snapshot := tracker.Snapshot()
	if snapshot.MonitoringWorkers.Configured != 8 || snapshot.MonitoringWorkers.Active != 1 || snapshot.MonitoringWorkers.Peak != 2 {
		t.Fatalf("monitoring workers = %#v", snapshot.MonitoringWorkers)
	}
	if snapshot.DiscoveryProbeWorkers.Configured != 32 || snapshot.DiscoveryProbeWorkers.Active != 0 || snapshot.DiscoveryProbeWorkers.Peak != 1 {
		t.Fatalf("discovery probe workers = %#v", snapshot.DiscoveryProbeWorkers)
	}
	if snapshot.DiscoveryDNSWorkers.Configured != 12 || snapshot.DiscoveryDNSWorkers.Active != 0 {
		t.Fatalf("discovery DNS workers = %#v", snapshot.DiscoveryDNSWorkers)
	}
}
