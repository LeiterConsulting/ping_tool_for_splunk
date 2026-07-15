package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	Up       = "up"
	Degraded = "degraded"
	Down     = "down"
	Unknown  = "unknown"
)

type Policy struct {
	DownAfterFailures      int
	RecoveryAfterSuccesses int
}

type Observation struct {
	Valid      bool
	Sent       int
	Successful int
	ObservedAt time.Time
}

type Snapshot struct {
	State                string
	Reason               string
	PreviousState        string
	ChangedAt            time.Time
	ConsecutiveSuccesses int
	ConsecutiveFailures  int
}

type record struct {
	state                string
	changedAt            time.Time
	lastObservedAt       time.Time
	consecutiveSuccesses int
	consecutiveFailures  int
}

type checkpoint struct {
	Version int                         `json:"version"`
	SavedAt time.Time                   `json:"saved_at"`
	Records map[string]checkpointRecord `json:"records"`
}

type checkpointRecord struct {
	State                string    `json:"state"`
	ChangedAt            time.Time `json:"changed_at"`
	LastObservedAt       time.Time `json:"last_observed_at"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	ConsecutiveFailures  int       `json:"consecutive_failures"`
}

type Evaluator struct {
	mu      sync.Mutex
	policy  Policy
	records map[string]*record
}

func New(policy Policy) *Evaluator {
	if policy.DownAfterFailures < 1 {
		policy.DownAfterFailures = 3
	}
	if policy.RecoveryAfterSuccesses < 1 {
		policy.RecoveryAfterSuccesses = 2
	}
	return &Evaluator{policy: policy, records: make(map[string]*record)}
}

func Load(path string, policy Policy, maxAge time.Duration) (*Evaluator, error) {
	evaluator := New(policy)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return evaluator, nil
	}
	if err != nil {
		return evaluator, err
	}
	var saved checkpoint
	if err := json.Unmarshal(b, &saved); err != nil {
		return evaluator, fmt.Errorf("decode state checkpoint: %w", err)
	}
	if saved.Version != 1 {
		return evaluator, fmt.Errorf("unsupported state checkpoint version %d", saved.Version)
	}
	cutoff := time.Time{}
	if maxAge > 0 {
		cutoff = time.Now().UTC().Add(-maxAge)
	}
	for endpointID, item := range saved.Records {
		if endpointID == "" || !validState(item.State) {
			continue
		}
		if !cutoff.IsZero() && (item.LastObservedAt.IsZero() || item.LastObservedAt.Before(cutoff)) {
			continue
		}
		evaluator.records[endpointID] = &record{
			state: item.State, changedAt: item.ChangedAt, lastObservedAt: item.LastObservedAt,
			consecutiveSuccesses: item.ConsecutiveSuccesses, consecutiveFailures: item.ConsecutiveFailures,
		}
	}
	return evaluator, nil
}

func (e *Evaluator) Save(path string) error {
	e.mu.Lock()
	saved := checkpoint{Version: 1, SavedAt: time.Now().UTC(), Records: make(map[string]checkpointRecord, len(e.records))}
	for endpointID, rec := range e.records {
		saved.Records[endpointID] = checkpointRecord{
			State: rec.state, ChangedAt: rec.changedAt, LastObservedAt: rec.lastObservedAt,
			ConsecutiveSuccesses: rec.consecutiveSuccesses, ConsecutiveFailures: rec.consecutiveFailures,
		}
	}
	e.mu.Unlock()

	b, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (e *Evaluator) Evaluate(endpointID string, observation Observation) Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	at := observation.ObservedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	rec := e.records[endpointID]
	if rec == nil {
		rec = &record{state: Unknown, changedAt: at, lastObservedAt: at}
		e.records[endpointID] = rec
	}
	rec.lastObservedAt = at
	previous := rec.state

	if !observation.Valid || observation.Sent < 1 || observation.Successful < 0 || observation.Successful > observation.Sent {
		return Snapshot{
			State:                Unknown,
			Reason:               "measurement_invalid",
			PreviousState:        previous,
			ChangedAt:            rec.changedAt,
			ConsecutiveSuccesses: rec.consecutiveSuccesses,
			ConsecutiveFailures:  rec.consecutiveFailures,
		}
	}

	if observation.Successful == 0 {
		rec.consecutiveFailures++
		rec.consecutiveSuccesses = 0
		if previous == Down || rec.consecutiveFailures >= e.policy.DownAfterFailures {
			e.setState(rec, Down, at)
			return snapshot(rec, previous, "consecutive_full_loss")
		}
		e.setState(rec, Degraded, at)
		return snapshot(rec, previous, "down_pending")
	}

	rec.consecutiveSuccesses++
	rec.consecutiveFailures = 0
	if previous == Down && rec.consecutiveSuccesses < e.policy.RecoveryAfterSuccesses {
		return snapshot(rec, previous, "recovery_pending")
	}

	if observation.Successful < observation.Sent {
		e.setState(rec, Degraded, at)
		return snapshot(rec, previous, "partial_loss")
	}

	e.setState(rec, Up, at)
	return snapshot(rec, previous, "all_replies_received")
}

func validState(value string) bool {
	return value == Up || value == Degraded || value == Down || value == Unknown
}

func (e *Evaluator) setState(rec *record, next string, at time.Time) {
	if rec.state == next {
		return
	}
	rec.state = next
	rec.changedAt = at
}

func snapshot(rec *record, previous string, reason string) Snapshot {
	return Snapshot{
		State:                rec.state,
		Reason:               reason,
		PreviousState:        previous,
		ChangedAt:            rec.changedAt,
		ConsecutiveSuccesses: rec.consecutiveSuccesses,
		ConsecutiveFailures:  rec.consecutiveFailures,
	}
}
