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
