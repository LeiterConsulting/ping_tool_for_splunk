package runtimeinfo

import (
	"sync"
	"time"
)

type Snapshot struct {
	Mode                       string `json:"mode"`
	State                      string `json:"state"`
	StartedAt                  string `json:"started_at"`
	UptimeSeconds              int64  `json:"uptime_seconds"`
	CycleRunning               bool   `json:"cycle_running"`
	CurrentCycle               int    `json:"current_cycle"`
	CurrentCycleID             string `json:"current_cycle_id,omitempty"`
	CurrentCycleStartedAt      string `json:"current_cycle_started_at,omitempty"`
	LastCycleCompletedAt       string `json:"last_cycle_completed_at,omitempty"`
	LastCycleDurationMs        int64  `json:"last_cycle_duration_ms"`
	NextCycleAt                string `json:"next_cycle_at,omitempty"`
	ActiveEndpoints            int    `json:"active_endpoints"`
	LastProductionSuccess      int    `json:"last_production_success"`
	LastProductionPartial      int    `json:"last_production_partial"`
	LastProductionFailed       int    `json:"last_production_failed"`
	LastEndpointReloadAt       string `json:"last_endpoint_reload_at,omitempty"`
	LastEndpointReloadError    string `json:"last_endpoint_reload_error,omitempty"`
	EffectiveConfigRevision    string `json:"effective_config_revision,omitempty"`
	EffectiveEndpointsRevision string `json:"effective_endpoints_revision,omitempty"`
	FatalError                 string `json:"fatal_error,omitempty"`
	Restarting                 bool   `json:"restarting"`
	RestartCount               int    `json:"restart_count"`
	LastRestartAt              string `json:"last_restart_at,omitempty"`
	LastRestartError           string `json:"last_restart_error,omitempty"`
}

type Tracker struct {
	mu      sync.RWMutex
	started time.Time
	snap    Snapshot
}

func New(mode string, configRevision string, endpointsRevision string, endpointCount int) *Tracker {
	now := time.Now().UTC()
	state := "starting"
	if mode == "ui_only" {
		state = "ui_only"
	}
	return &Tracker{
		started: now,
		snap: Snapshot{
			Mode: mode, State: state, StartedAt: now.Format(time.RFC3339Nano),
			ActiveEndpoints: endpointCount, EffectiveConfigRevision: configRevision,
			EffectiveEndpointsRevision: endpointsRevision,
		},
	}
}

func (t *Tracker) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{Mode: "unknown", State: "unknown"}
	}
	t.mu.RLock()
	snapshot := t.snap
	started := t.started
	t.mu.RUnlock()
	snapshot.UptimeSeconds = max(0, int64(time.Since(started).Seconds()))
	return snapshot
}

func (t *Tracker) CycleStarted(number int, cycleID string, endpointCount int, started time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.State = "running"
	t.snap.CycleRunning = true
	t.snap.CurrentCycle = number
	t.snap.CurrentCycleID = cycleID
	t.snap.CurrentCycleStartedAt = started.UTC().Format(time.RFC3339Nano)
	t.snap.NextCycleAt = ""
	t.snap.ActiveEndpoints = endpointCount
	t.snap.FatalError = ""
}

func (t *Tracker) CycleCompleted(number int, completed time.Time, duration time.Duration, next time.Time, success int, partial int, failed int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.State = "running"
	t.snap.CycleRunning = false
	t.snap.CurrentCycle = number
	t.snap.CurrentCycleID = ""
	t.snap.CurrentCycleStartedAt = ""
	t.snap.LastCycleCompletedAt = completed.UTC().Format(time.RFC3339Nano)
	t.snap.LastCycleDurationMs = duration.Milliseconds()
	if !next.IsZero() {
		t.snap.NextCycleAt = next.UTC().Format(time.RFC3339Nano)
	} else {
		t.snap.NextCycleAt = ""
	}
	t.snap.LastProductionSuccess = success
	t.snap.LastProductionPartial = partial
	t.snap.LastProductionFailed = failed
}

func (t *Tracker) EndpointReloaded(revision string, endpointCount int, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.ActiveEndpoints = endpointCount
	t.snap.EffectiveEndpointsRevision = revision
	t.snap.LastEndpointReloadAt = at.UTC().Format(time.RFC3339Nano)
	t.snap.LastEndpointReloadError = ""
}

func (t *Tracker) EndpointReloadFailed(err error) {
	if t == nil || err == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.LastEndpointReloadError = err.Error()
}

func (t *Tracker) Failed(err error) {
	if t == nil || err == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.State = "failed"
	t.snap.CycleRunning = false
	t.snap.FatalError = err.Error()
}

func (t *Tracker) RestartRequested() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.State = "restarting"
	t.snap.Restarting = true
	t.snap.LastRestartError = ""
}

func (t *Tracker) Restarted(configRevision string, endpointsRevision string, endpointCount int, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.State = "starting"
	t.snap.Restarting = false
	t.snap.RestartCount++
	t.snap.LastRestartAt = at.UTC().Format(time.RFC3339Nano)
	t.snap.LastRestartError = ""
	t.snap.FatalError = ""
	t.snap.CycleRunning = false
	t.snap.CurrentCycleID = ""
	t.snap.CurrentCycleStartedAt = ""
	t.snap.NextCycleAt = ""
	t.snap.ActiveEndpoints = endpointCount
	t.snap.EffectiveConfigRevision = configRevision
	t.snap.EffectiveEndpointsRevision = endpointsRevision
}

func (t *Tracker) RestartFailed(err error) {
	if t == nil || err == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap.State = "starting"
	t.snap.Restarting = false
	t.snap.LastRestartError = err.Error()
}
