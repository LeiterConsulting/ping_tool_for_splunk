package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluatorRequiresConsecutiveFailuresAndRecovery(t *testing.T) {
	evaluator := New(Policy{DownAfterFailures: 3, RecoveryAfterSuccesses: 2})
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)

	assertState(t, evaluator.Evaluate("endpoint", observation(base, 4, 4)), Up, "all_replies_received")
	assertState(t, evaluator.Evaluate("endpoint", observation(base.Add(time.Minute), 4, 0)), Degraded, "down_pending")
	assertState(t, evaluator.Evaluate("endpoint", observation(base.Add(2*time.Minute), 4, 0)), Degraded, "down_pending")
	down := evaluator.Evaluate("endpoint", observation(base.Add(3*time.Minute), 4, 0))
	assertState(t, down, Down, "consecutive_full_loss")
	if down.ConsecutiveFailures != 3 {
		t.Fatalf("ConsecutiveFailures = %d, want 3", down.ConsecutiveFailures)
	}

	firstRecovery := evaluator.Evaluate("endpoint", observation(base.Add(4*time.Minute), 4, 4))
	assertState(t, firstRecovery, Down, "recovery_pending")
	assertState(t, evaluator.Evaluate("endpoint", observation(base.Add(5*time.Minute), 4, 4)), Up, "all_replies_received")
}

func TestCheckpointPreservesHysteresisAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	policy := Policy{DownAfterFailures: 3, RecoveryAfterSuccesses: 2}
	now := time.Now().UTC()
	evaluator := New(policy)
	evaluator.Evaluate("endpoint", observation(now, 4, 0))
	evaluator.Evaluate("endpoint", observation(now.Add(time.Second), 4, 0))
	if err := evaluator.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := Load(path, policy, time.Hour)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	snapshot := reloaded.Evaluate("endpoint", observation(now.Add(2*time.Second), 4, 0))
	if snapshot.State != Down || snapshot.ConsecutiveFailures != 3 {
		t.Fatalf("reloaded snapshot = %#v, want third failure to transition down", snapshot)
	}
}

func TestCheckpointDropsStaleRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	evaluator := New(Policy{})
	evaluator.Evaluate("endpoint", observation(time.Now().UTC().Add(-time.Hour), 4, 0))
	if err := evaluator.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reloaded, err := Load(path, Policy{}, time.Minute)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	snapshot := reloaded.Evaluate("endpoint", observation(time.Now().UTC(), 4, 0))
	if snapshot.ConsecutiveFailures != 1 {
		t.Fatalf("stale record was restored: %#v", snapshot)
	}
}

func TestEvaluatorMarksPartialLossDegraded(t *testing.T) {
	evaluator := New(Policy{})
	snapshot := evaluator.Evaluate("endpoint", observation(time.Now(), 4, 3))
	assertState(t, snapshot, Degraded, "partial_loss")
}

func TestEvaluatorReportsInvalidMeasurementAsUnknownWithoutDiscardingState(t *testing.T) {
	evaluator := New(Policy{})
	now := time.Now()
	assertState(t, evaluator.Evaluate("endpoint", observation(now, 4, 4)), Up, "all_replies_received")
	invalid := evaluator.Evaluate("endpoint", Observation{Valid: false, ObservedAt: now.Add(time.Minute)})
	assertState(t, invalid, Unknown, "measurement_invalid")
	if invalid.PreviousState != Up {
		t.Fatalf("PreviousState = %q, want %q", invalid.PreviousState, Up)
	}
	assertState(t, evaluator.Evaluate("endpoint", observation(now.Add(2*time.Minute), 4, 4)), Up, "all_replies_received")
}

func observation(at time.Time, sent int, successful int) Observation {
	return Observation{Valid: true, Sent: sent, Successful: successful, ObservedAt: at}
}

func assertState(t *testing.T, snapshot Snapshot, wantState string, wantReason string) {
	t.Helper()
	if snapshot.State != wantState || snapshot.Reason != wantReason {
		t.Fatalf("snapshot = state %q reason %q, want state %q reason %q", snapshot.State, snapshot.Reason, wantState, wantReason)
	}
}
