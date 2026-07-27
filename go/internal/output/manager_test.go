package output

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
)

func testSummary(cycleID string) models.SummaryEvent {
	zero := 0.0
	return models.SummaryEvent{
		SchemaVersion: models.SchemaVersion, CycleID: cycleID, EventID: "event-" + cycleID,
		CollectorID: "collector-id", EndpointID: "endpoint-1", Timestamp: util.FormatDotNetO(time.Now()),
		TargetIP: "127.0.0.1", Hostname: "loopback", RecordType: "summary",
		ProbeBackend: "test", MeasurementValid: true, ObservationStatus: "reply", State: "up",
		PingsSent: 1, PingsSuccessful: 1, PacketLossPct: &zero, AvgLatencyMs: &zero,
	}
}

func TestManagerDecouplesProbeResultFromSlowHEC(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.Defaults(t.TempDir())
	cfg.OutputMode = "hec"
	cfg.ParallelThreads = 1
	cfg.HEC.Enabled = true
	cfg.HEC.URL = server.URL
	cfg.HEC.Token = "test-token"
	cfg.HEC.BatchSize = 1
	cfg.HEC.MaxBufferEvents = 100
	cfg.HEC.MaxBufferBytes = "1MB"

	manager, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	start := time.Now()
	err = manager.HandleResult(context.Background(), nil, models.SummaryEvent{
		SchemaVersion: models.SchemaVersion,
		CycleID:       "cycle-slow-hec",
		EventID:       "event-slow-hec",
		Timestamp:     util.FormatDotNetO(time.Now()),
		RecordType:    "summary",
	})
	if err != nil {
		t.Fatalf("HandleResult() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("HandleResult() blocked for %s", elapsed)
	}

	flushStart := time.Now()
	if err := manager.FlushCycle(context.Background()); err != nil {
		t.Fatalf("FlushCycle() error = %v", err)
	}
	if elapsed := time.Since(flushStart); elapsed > 100*time.Millisecond {
		t.Fatalf("durable FlushCycle() waited for slow HEC for %s", elapsed)
	}
	waitForDeliveryState(t, manager, "healthy")
	if requests.Load() != 1 {
		t.Fatalf("HEC requests = %d, want 1", requests.Load())
	}
}

func TestManagerRetainsFailedCycleAndDrainsAfterRestart(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.OutputMode = "hec"
	cfg.HEC.Enabled = true
	cfg.HEC.URL = server.URL + "/services/collector/event"
	cfg.HEC.Token = "token"
	cfg.HEC.Retry.Enabled = false
	cfg.Delivery.SpoolPath = root + "/outbox"

	manager, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleResult(context.Background(), nil, testSummary("cycle-outage")); err != nil {
		t.Fatal(err)
	}
	if err := manager.FlushCycle(context.Background()); err != nil {
		t.Fatalf("durable network failure should not stop probes: %v", err)
	}
	waitForDeliveryState(t, manager, "impaired")
	status := manager.DeliveryStatus()
	if status.State != "impaired" || status.PendingEnvelopes != 1 {
		t.Fatalf("unexpected impaired status: %#v", status)
	}
	manager.Close()

	fail.Store(false)
	restarted, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.FlushCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForDeliveryState(t, restarted, "healthy")
	status = restarted.DeliveryStatus()
	if status.State != "healthy" || status.PendingEnvelopes != 0 {
		t.Fatalf("unexpected recovered status: %#v", status)
	}
	if requests.Load() != 2 {
		t.Fatalf("HEC requests = %d, want 2", requests.Load())
	}
}

func TestManagerCloseAttemptsPersistedDelivery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	cfg := config.Defaults(t.TempDir())
	cfg.OutputMode = "hec"
	cfg.HEC.Enabled = true
	cfg.HEC.URL = server.URL
	cfg.HEC.Token = "token"
	manager, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleResult(context.Background(), nil, testSummary("cycle-close")); err != nil {
		t.Fatal(err)
	}
	if err := manager.FlushCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager.Close()
	if requests.Load() != 1 {
		t.Fatalf("HEC requests during close = %d, want 1", requests.Load())
	}
}

func TestManagerPersistsPerSinkProgress(t *testing.T) {
	var eventRequests atomic.Int32
	eventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		eventRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer eventServer.Close()
	var metricsFail atomic.Bool
	metricsFail.Store(true)
	var metricRequests atomic.Int32
	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricRequests.Add(1)
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if metricsFail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer metricsServer.Close()

	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.OutputMode = "hec"
	cfg.HEC.Enabled = true
	cfg.HEC.URL = eventServer.URL + "/services/collector/event"
	cfg.HEC.Token = "event-token"
	cfg.Metrics.Enabled = true
	cfg.Metrics.HECURL = metricsServer.URL + "/services/collector"
	cfg.Metrics.Token = "metric-token"
	cfg.Delivery.SpoolPath = root + "/outbox"

	manager, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleResult(context.Background(), nil, testSummary("cycle-dual")); err != nil {
		t.Fatal(err)
	}
	if err := manager.FlushCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForDeliveryState(t, manager, "impaired")
	manager.Close()
	if eventRequests.Load() != 1 {
		t.Fatalf("event requests before restart = %d, want 1", eventRequests.Load())
	}

	metricsFail.Store(false)
	restarted, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.FlushCycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForDeliveryState(t, restarted, "healthy")
	if eventRequests.Load() != 1 {
		t.Fatalf("event sink was resent after durable progress: %d", eventRequests.Load())
	}
	if restarted.DeliveryStatus().PendingEnvelopes != 0 {
		t.Fatalf("outbox did not drain: %#v", restarted.DeliveryStatus())
	}
}

func TestManagerPersistsDiscoveryEventsThroughFileAndHEC(t *testing.T) {
	var requests atomic.Int32
	var received atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		received.Store(string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.OutputMode = "both"
	cfg.LogPath = filepath.Join(root, "ping_results.log")
	cfg.HEC.Enabled = true
	cfg.HEC.URL = server.URL + "/services/collector/event"
	cfg.HEC.Token = "token"
	cfg.Metrics.Enabled = false
	cfg.Delivery.SpoolPath = filepath.Join(root, "outbox")

	manager, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	event := json.RawMessage(`{"schema_version":4,"event_id":"discovery-event-1","cycle_id":"discovery-scan-1","timestamp":"2026-07-27T12:00:00Z","record_type":"discovery_observation","target_ip":"192.0.2.1","discovery_observed":true}`)
	if err := manager.HandleEvents(context.Background(), "discovery-scan-1", []json.RawMessage{event}); err != nil {
		t.Fatalf("HandleEvents() error = %v", err)
	}
	waitForDeliveryState(t, manager, "healthy")

	logBody, err := os.ReadFile(cfg.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logBody), `"record_type":"discovery_observation"`) {
		t.Fatalf("file output does not contain discovery event: %s", logBody)
	}
	if requests.Load() != 1 {
		t.Fatalf("HEC requests = %d, want 1", requests.Load())
	}
	hecBody, _ := received.Load().(string)
	if !strings.Contains(hecBody, `"record_type":"discovery_observation"`) {
		t.Fatalf("HEC payload does not contain discovery event: %s", hecBody)
	}
}

func TestManagerRejectsInvalidExternalEvent(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	manager, err := NewManager(cfg, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.HandleEvents(context.Background(), "batch", []json.RawMessage{json.RawMessage(`not-json`)}); err == nil {
		t.Fatal("HandleEvents() accepted invalid JSON")
	}
}

func waitForDeliveryState(t *testing.T, manager *Manager, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if manager.DeliveryStatus().State == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("delivery state = %#v, want %q", manager.DeliveryStatus(), want)
}

func TestDeliveryConfirmationModeIsExplicit(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	if got := deliveryConfirmationMode(cfg, true, false); got != "hec_accepted_only" {
		t.Fatalf("mode = %q", got)
	}
	cfg.HEC.UseACK = true
	if got := deliveryConfirmationMode(cfg, true, false); got != "indexed_acknowledged" {
		t.Fatalf("mode = %q", got)
	}
	cfg.Metrics.UseACK = false
	if got := deliveryConfirmationMode(cfg, true, true); got != "mixed" {
		t.Fatalf("mode = %q", got)
	}
}
