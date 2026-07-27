package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	filerevision "github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
)

func requestBodyWithRevision(t *testing.T, path string, raw string) *bytes.Reader {
	t.Helper()
	currentRevision, err := filerevision.File(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	payload["revision"] = currentRevision
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(encoded)
}

func TestStaticAssetsAreVersionedAndNotCached(t *testing.T) {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	handler := staticHandler(staticRoot, "v5.9.0")

	indexResponse := httptest.NewRecorder()
	handler.ServeHTTP(indexResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexResponse.Code != http.StatusOK {
		t.Fatalf("index status = %d", indexResponse.Code)
	}
	for _, expected := range []string{"/app.css?v=v5.9.0", "/app.js?v=v5.9.0"} {
		if !strings.Contains(indexResponse.Body.String(), expected) {
			t.Fatalf("index does not contain versioned asset %q", expected)
		}
	}
	if cacheControl := indexResponse.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("index Cache-Control = %q, want no-store", cacheControl)
	}

	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, httptest.NewRequest(http.MethodGet, "/app.js?v=v5.9.0", nil))
	if assetResponse.Code != http.StatusOK {
		t.Fatalf("asset status = %d", assetResponse.Code)
	}
	if cacheControl := assetResponse.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("asset Cache-Control = %q, want no-store", cacheControl)
	}
	if assetResponse.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("asset Pragma = %q, want no-cache", assetResponse.Header().Get("Pragma"))
	}
}

func TestEndpointsAPI(t *testing.T) {
	tempDir := t.TempDir()
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	configPath := filepath.Join(tempDir, "config.psd1")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	content := "ip,hostname,group,description,entitytype,device,vendor,additional_notes,dev\n" +
		"10.0.0.1,core-router,network,Core Router,network,router,Cisco,,false\n" +
		"10.0.0.25,lab-api,development,Lab API,service,vm,VMware,,true\n"
	if err := os.WriteFile(endpointsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/endpoints", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var payload endpointsResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.Summary.Total != 2 {
		t.Fatalf("summary.total = %d, want 2", payload.Summary.Total)
	}
	if payload.Summary.Production != 1 {
		t.Fatalf("summary.production = %d, want 1", payload.Summary.Production)
	}
	if payload.Summary.Dev != 1 {
		t.Fatalf("summary.dev = %d, want 1", payload.Summary.Dev)
	}
	if payload.Summary.Groups != 2 {
		t.Fatalf("summary.groups = %d, want 2", payload.Summary.Groups)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(payload.Items))
	}
	if payload.Revision == "" {
		t.Fatal("revision is blank")
	}
	if !payload.Items[1].Dev {
		t.Fatal("items[1].dev = false, want true")
	}
}

func TestAdvisorAPIAnalyzeAndApplySafeFixes(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	cfg := config.Defaults(tempDir)
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	cfg.HEC.Token = "advisor-test-hec-secret"
	cfg.Metrics.Token = "advisor-test-metrics-secret"
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	content := "ip,hostname,group,description,entitytype,device,vendor,additional_notes,dev\n" +
		" 10.0.0.1 ,host-a,default,,,,,,false\n" +
		"10.0.0.1,host-a,default,,,,,,false\n"
	if err := os.WriteFile(endpointsPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "v5.9.0"})
	if err != nil {
		t.Fatal(err)
	}

	analyzeResponse := httptest.NewRecorder()
	handler.ServeHTTP(analyzeResponse, httptest.NewRequest(http.MethodGet, "/api/advisor?profile=standard", nil))
	if analyzeResponse.Code != http.StatusOK {
		t.Fatalf("analyze status = %d: %s", analyzeResponse.Code, analyzeResponse.Body.String())
	}
	if strings.Contains(analyzeResponse.Body.String(), "advisor-test-hec-secret") || strings.Contains(analyzeResponse.Body.String(), "advisor-test-metrics-secret") {
		t.Fatal("advisor response exposed a write-only output token")
	}
	var report struct {
		Summary struct {
			SafeFixes int `json:"safe_fixes"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(analyzeResponse.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.SafeFixes == 0 {
		t.Fatal("advisor did not offer safe inventory cleanup")
	}

	configRevision, _ := filerevision.File(configPath)
	endpointsRevision, _ := filerevision.File(endpointsPath)
	requestBody, _ := json.Marshal(advisorApplyRequest{
		Profile: "standard", ApplySafe: true,
		ConfigRevision: configRevision, EndpointsRevision: endpointsRevision,
	})
	applyResponse := httptest.NewRecorder()
	handler.ServeHTTP(applyResponse, httptest.NewRequest(http.MethodPost, "/api/advisor/apply", bytes.NewReader(requestBody)))
	if applyResponse.Code != http.StatusOK {
		t.Fatalf("apply status = %d: %s", applyResponse.Code, applyResponse.Body.String())
	}
	loaded, err := config.LoadEndpoints(endpointsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].IP != "10.0.0.1" {
		t.Fatalf("safe apply produced endpoints = %#v", loaded)
	}
}

func TestAdvisorAPIRejectsStaleRevision(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	cfg := config.Defaults(tempDir)
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir})
	if err != nil {
		t.Fatal(err)
	}
	requestBody, _ := json.Marshal(advisorApplyRequest{
		Profile: "standard", ApplyProfile: true,
		ConfigRevision: "stale", EndpointsRevision: "stale",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/advisor/apply", bytes.NewReader(requestBody)))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", response.Code, response.Body.String())
	}
}

func TestRuntimeRestartAPIRequiresConfirmationAndQueuesValidatedRestart(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	cfg := config.Defaults(tempDir)
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n127.0.0.1,loopback\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldConfigRevision, _ := filerevision.File(configPath)
	endpointsRevision, _ := filerevision.File(endpointsPath)
	tracker := runtimeinfo.New("monitor", oldConfigRevision, endpointsRevision, 1)
	cfg.ParallelThreads = 2
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatal(err)
	}
	newConfigRevision, _ := filerevision.File(configPath)
	queued := make(chan RestartRequest, 1)
	handler, err := newHandler(Options{
		ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir,
		Runtime: tracker, RequestRestart: func(request RestartRequest) error { queued <- request; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	unconfirmedBody, _ := json.Marshal(runtimeRestartRequest{
		ConfigRevision: newConfigRevision, EndpointsRevision: endpointsRevision,
	})
	unconfirmed := httptest.NewRecorder()
	handler.ServeHTTP(unconfirmed, httptest.NewRequest(http.MethodPost, "/api/runtime/restart", bytes.NewReader(unconfirmedBody)))
	if unconfirmed.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed status = %d, want 400", unconfirmed.Code)
	}

	confirmedBody, _ := json.Marshal(runtimeRestartRequest{
		ConfigRevision: newConfigRevision, EndpointsRevision: endpointsRevision, Confirmed: true,
	})
	confirmed := httptest.NewRecorder()
	handler.ServeHTTP(confirmed, httptest.NewRequest(http.MethodPost, "/api/runtime/restart", bytes.NewReader(confirmedBody)))
	if confirmed.Code != http.StatusAccepted {
		t.Fatalf("confirmed status = %d, want 202: %s", confirmed.Code, confirmed.Body.String())
	}
	select {
	case request := <-queued:
		if request.ConfigRevision != newConfigRevision || request.EndpointsRevision != endpointsRevision {
			t.Fatalf("queued request = %#v", request)
		}
	default:
		t.Fatal("restart request was not queued")
	}
	if !tracker.Snapshot().Restarting {
		t.Fatal("runtime tracker did not enter restarting state")
	}
}

func TestRuntimeRestartAPIBlocksInvalidDeployment(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	cfg := config.Defaults(tempDir)
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatal(err)
	}
	oldRevision, _ := filerevision.File(configPath)
	cfg.ParallelThreads = 2
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.1,broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configRevision, _ := filerevision.File(configPath)
	endpointsRevision, _ := filerevision.File(endpointsPath)
	called := false
	handler, err := newHandler(Options{
		ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir,
		Runtime:        runtimeinfo.New("monitor", oldRevision, endpointsRevision, 1),
		RequestRestart: func(RestartRequest) error { called = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(runtimeRestartRequest{
		ConfigRevision: configRevision, EndpointsRevision: endpointsRevision, Confirmed: true,
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/runtime/restart", bytes.NewReader(body)))
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("status = %d, callback called = %t: %s", response.Code, called, response.Body.String())
	}
}

func TestEndpointsAPI_PutRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	configPath := filepath.Join(tempDir, "config.psd1")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	body := requestBodyWithRevision(t, endpointsPath, `{"items":[{"ip":"10.0.0.1","hostname":"core-router","group":"network","description":"Core Router","entitytype":"network","device":"router","vendor":"Cisco","additional_notes":"Primary","dev":false},{"ip":"10.0.0.25","hostname":"qa-api","group":"development","description":"QA API","entitytype":"service","device":"vm","vendor":"VMware","additional_notes":"Excluded","dev":true}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/endpoints", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}
	loaded, err := config.LoadEndpoints(endpointsPath)
	if err != nil {
		t.Fatalf("LoadEndpoints() error = %v", err)
	}
	if len(loaded) != 2 || !loaded[1].Dev {
		t.Fatalf("unexpected endpoints after PUT: %#v", loaded)
	}
}

func TestEndpointsAPI_RejectsStaleRevision(t *testing.T) {
	tempDir := t.TempDir()
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	configPath := filepath.Join(tempDir, "config.psd1")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleRevision, err := filerevision.File(endpointsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.2,host-b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(endpointsWriteRequest{Revision: staleRevision})
	req := httptest.NewRequest(http.MethodPut, "/api/endpoints", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "reload from disk") {
		t.Fatalf("stale write status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestConfigAPI_PutRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	body := requestBodyWithRevision(t, configPath, `{"config":{"pings_per_cycle":9,"cycle_interval_seconds":45,"timeout_ms":1500,"parallel_threads":12,"output_mode":"both","log_path":"./logs/ui.log","log_rotation_size_mb":99,"emit_individual_pings":false,"ping":{"mode":"exec"},"diagnostics":{"enabled":true,"handle_probe_mode":"metrics_only"},"debug":{"emit_memory_stats":true},"hec":{"enabled":true,"url":"https://hec.example.com:8088","token":"secret-token","index":"main","sourcetype":"ping_monitor","verify_ssl":true,"ssl_protocol":"Default","batch_size":120,"drop_on_failure":false,"max_buffer_events":9000,"max_buffer_bytes":"9MB","retry":{"enabled":true,"max_attempts":5,"base_delay_ms":500,"jitter_pct":25,"backoff":"fixed"},"retry_count":1,"retry_delay_ms":500,"dead_letter_path":"./logs/hec.ndjson","dead_letter_rotation_size_mb":15},"metrics":{"enabled":true,"mode":"dual","index":"metrics","hec_url":"https://metrics.example.com:8088","token":"metric-token","verify_ssl":true,"ssl_protocol":"Default","compat_mode":false,"sourcetype":"ping_monitor:metrics","event_name":"metric","use_metrics_index":true,"batch_size":250,"max_buffer_events":10000,"max_buffer_bytes":"10MB"}}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "secret-token") || strings.Contains(resp.Body.String(), "metric-token") {
		t.Fatalf("config response exposed a write-only secret: %s", resp.Body.String())
	}
	var response configResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if response.Config.HEC.Token != "" || response.Config.Metrics.Token != "" || !response.Secrets.HECTokenConfigured || !response.Secrets.MetricsTokenConfigured {
		t.Fatalf("unexpected redacted config response: %#v", response)
	}
	loaded, _, err := config.LoadEditable(context.Background(), configPath, tempDir)
	if err != nil {
		t.Fatalf("LoadEditable() error = %v", err)
	}
	if loaded.PingsPerCycle != 9 || loaded.OutputMode != "both" || loaded.Ping.Mode != "exec" {
		t.Fatalf("unexpected config after PUT: %#v", loaded)
	}
	if !loaded.Diagnostics.Enabled || !loaded.Debug.EmitMemoryStats || !loaded.HEC.Enabled || loaded.HEC.Token != "secret-token" || !loaded.Metrics.Enabled || !loaded.Metrics.UseMetricsIndex {
		t.Fatalf("unexpected nested config after PUT: %#v", loaded)
	}
	loaded.HEC.Token = ""
	loaded.Metrics.Token = ""
	preserveBody, err := json.Marshal(configWriteRequest{Config: loaded, Revision: response.Revision})
	if err != nil {
		t.Fatal(err)
	}
	preserveReq := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(preserveBody))
	preserveReq.Header.Set("Content-Type", "application/json")
	preserveResp := httptest.NewRecorder()
	handler.ServeHTTP(preserveResp, preserveReq)
	if preserveResp.Code != http.StatusOK {
		t.Fatalf("blank-token PUT status = %d (%s)", preserveResp.Code, preserveResp.Body.String())
	}
	preserved, _, err := config.LoadEditable(context.Background(), configPath, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.HEC.Token != "secret-token" || preserved.Metrics.Token != "metric-token" {
		t.Fatalf("blank token did not preserve write-only secrets: %#v", preserved)
	}
}

func TestStatusAPI_ReportsRuntimeAndRestartTruth(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configRevision, _ := filerevision.File(configPath)
	endpointsRevision, _ := filerevision.File(endpointsPath)
	tracker := runtimeinfo.New("monitor", configRevision, endpointsRevision, 1)
	started := time.Now().Add(-100 * time.Millisecond)
	tracker.CycleStarted(2, "cycle-2", 1, started)
	tracker.CycleCompleted(2, time.Now(), 100*time.Millisecond, time.Now().Add(time.Minute), 1, 0, 0)

	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, append(contents, []byte("\n# pending restart\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test", Runtime: tracker})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", resp.Code, resp.Body.String())
	}
	var payload statusResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.ConfigRestartRequired || payload.Runtime.CurrentCycle != 2 || payload.Runtime.LastProductionSuccess != 1 {
		t.Fatalf("status truth = %#v", payload)
	}
}

func TestConfigAPI_RejectsStaleRevision(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleRevision, _ := filerevision.File(configPath)
	contents, _ := os.ReadFile(configPath)
	if err := os.WriteFile(configPath, append(contents, []byte("\n# external edit\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(configWriteRequest{Config: config.Defaults(tempDir), Revision: staleRevision})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "reload from disk") {
		t.Fatalf("stale config status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestStatusAPI_ResolvesDiscoveryScriptAdjacentToConfig(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	discoveryPath := filepath.Join(tempDir, "DiscoverEndpoints.ps1")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(discoveryPath, []byte("# discovery placeholder\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: rootDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}

	var payload statusResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.DiscoveryAvailable {
		t.Fatal("discovery_available = false, want true")
	}
	if payload.DiscoveryScriptPath != discoveryPath {
		t.Fatalf("discovery_script_path = %q, want %q", payload.DiscoveryScriptPath, discoveryPath)
	}
}

func TestStatusAPI_UsesEmbeddedDiscoveryFallback(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}

	var payload statusResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.DiscoveryAvailable {
		t.Fatal("discovery_available = false, want true")
	}
	if payload.DiscoveryScriptPath != embeddedDiscoveryScriptPath {
		t.Fatalf("discovery_script_path = %q, want %q", payload.DiscoveryScriptPath, embeddedDiscoveryScriptPath)
	}
}

func TestNormalizeDiscoveryTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		subnetMask int
		wantTarget string
		wantMask   int
		wantErr    string
	}{
		{name: "blank uses current mask", target: "", subnetMask: 24, wantTarget: "", wantMask: 24},
		{name: "ipv4 keeps supplied mask", target: "192.168.10.44", subnetMask: 23, wantTarget: "192.168.10.44", wantMask: 23},
		{name: "cidr overrides mask", target: "10.20.30.0/26", subnetMask: 24, wantTarget: "10.20.30.0", wantMask: 26},
		{name: "invalid host rejected", target: "bad-target", subnetMask: 24, wantErr: "target_network must be a valid IPv4 address or CIDR range"},
		{name: "invalid cidr mask rejected", target: "10.20.30.0/31", subnetMask: 24, wantErr: "target_network CIDR mask must be between /16 and /30"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotTarget, gotMask, err := normalizeDiscoveryTarget(test.target, test.subnetMask)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("normalizeDiscoveryTarget() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDiscoveryTarget() error = %v", err)
			}
			if gotTarget != test.wantTarget || gotMask != test.wantMask {
				t.Fatalf("normalizeDiscoveryTarget() = (%q, %d), want (%q, %d)", gotTarget, gotMask, test.wantTarget, test.wantMask)
			}
		})
	}
}

func TestNormalizeDiscoveryRunRequest(t *testing.T) {
	request := discoveryRunRequest{TargetNetwork: "10.20.30.0/26", TimeoutMs: 0, ThrottleLimit: 0}
	if err := normalizeDiscoveryRunRequest(&request); err != nil {
		t.Fatalf("normalizeDiscoveryRunRequest() error = %v", err)
	}
	if request.TargetNetwork != "10.20.30.0" || request.SubnetMask != 26 {
		t.Fatalf("unexpected normalized request: %#v", request)
	}
	if request.TimeoutMs != 500 || request.ThrottleLimit != 50 {
		t.Fatalf("expected default timeout/throttle values, got %#v", request)
	}
}

func TestHandleDiscoveryStream_InvalidTarget(t *testing.T) {
	handler, err := newHandler(Options{ConfigPath: "config.psd1", EndpointsPath: "endpoints.csv", RootDir: ".", Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	body := bytes.NewBufferString(`{"target_network":"bad-target","subnet_mask":24,"timeout_ms":250,"throttle_limit":16}`)
	req := httptest.NewRequest(http.MethodPost, "/api/discovery/stream", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "target_network must be a valid IPv4 address or CIDR range") {
		t.Fatalf("unexpected response body: %s", resp.Body.String())
	}
}

func TestUpdateDiscoveryProgressState(t *testing.T) {
	progress := newDiscoveryProgressState(discoveryRunRequest{TargetNetwork: "192.168.1.184", SubnetMask: 30})
	updateDiscoveryProgressState(&progress, "Hosts to scan: 2")
	if !strings.Contains(progress.SummaryText, "2 hosts") {
		t.Fatalf("summary after hosts line = %q, want host count", progress.SummaryText)
	}
	updateDiscoveryProgressState(&progress, "Scanning network (this may take a moment)...")
	if !strings.Contains(progress.SummaryText, "Scanning 2 hosts") {
		t.Fatalf("summary after scan line = %q, want scanning status", progress.SummaryText)
	}
	updateDiscoveryProgressState(&progress, "Found 2 active hosts out of 2 scanned")
	if !strings.Contains(progress.SummaryText, "Found 2 active hosts") {
		t.Fatalf("summary after active-host line = %q, want active host count", progress.SummaryText)
	}
}

func TestSanitizeDiscoveryLog(t *testing.T) {
	raw := "\x1b[32;1mHosts to scan:\x1b[0m 2\r\n\r\nFound 2 active hosts out of 2 scanned\n"
	cleaned := sanitizeDiscoveryLog(raw)
	if strings.Contains(cleaned, "\x1b") {
		t.Fatalf("sanitizeDiscoveryLog() left ANSI escape codes in %q", cleaned)
	}
	if !containsAll(cleaned, "Hosts to scan:", "Found 2 active hosts out of 2 scanned") {
		t.Fatalf("sanitizeDiscoveryLog() missing expected content: %q", cleaned)
	}
}

func TestMaterializeDiscoveryScript_EmbeddedFallback(t *testing.T) {
	scriptPath, cleanup, err := materializeDiscoveryScript(embeddedDiscoveryScriptPath)
	if err != nil {
		t.Fatalf("materializeDiscoveryScript() error = %v", err)
	}
	defer cleanup()

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("embedded discovery script materialized as empty file")
	}
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(content, embeddedDiscoveryScript) {
		t.Fatal("materialized discovery script does not match embedded asset")
	}
}

func TestOutputTestAPI_HECProbe(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Splunk test-token" {
			t.Fatalf("Authorization header = %q, want %q", auth, "Splunk test-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	defer probeServer.Close()

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	cfg := config.Defaults(tempDir)
	cfg.OutputMode = "hec"
	cfg.HEC.Enabled = true
	cfg.HEC.URL = probeServer.URL
	cfg.HEC.Token = "test-token"
	requestBody, err := json.Marshal(outputTestRequest{Target: "hec", Config: cfg})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/output/test", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}
	var payload outputTestResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.Success || payload.StatusCode != http.StatusOK {
		t.Fatalf("unexpected output test payload: %#v", payload)
	}
}

func TestOutputTestAPI_MetricsProbe(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, config.Defaults(tempDir)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Splunk metrics-token" {
			t.Fatalf("Authorization header = %q, want %q", auth, "Splunk metrics-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	defer probeServer.Close()

	handler, err := newHandler(Options{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	cfg := config.Defaults(tempDir)
	cfg.Metrics.Enabled = true
	cfg.Metrics.HECURL = probeServer.URL
	cfg.Metrics.Token = "metrics-token"
	requestBody, err := json.Marshal(outputTestRequest{Target: "metrics", Config: cfg})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/output/test", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", resp.Code, http.StatusOK, resp.Body.String())
	}
	var payload outputTestResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !payload.Success || payload.StatusCode != http.StatusOK {
		t.Fatalf("unexpected output test payload: %#v", payload)
	}
	if !strings.Contains(payload.Message, fmt.Sprintf("HTTP %d", http.StatusOK)) {
		t.Fatalf("message = %q, want HTTP status text", payload.Message)
	}
}

func TestStaticShellServesIndex(t *testing.T) {
	handler, err := newHandler(Options{ConfigPath: "config.psd1", EndpointsPath: "endpoints.csv", RootDir: ".", Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if body := resp.Body.String(); !containsAll(body, "Ping Monitor", "Endpoint Inventory", "Collector Administration", "Monitoring Cycle", "Pending Delivery", "Cancel Discovery", "Delete This Endpoint") {
		t.Fatalf("body missing expected shell markers: %q", body)
	}
	for header, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := resp.Header().Get(header); !strings.Contains(got, want) {
			t.Errorf("%s = %q, want content %q", header, got, want)
		}
	}
}

func TestStatusUsesEffectiveConfigWithoutReparsingDisk(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.psd1")
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	cfg := config.Defaults(tempDir)
	cfg.OutputMode = "hec"
	cfg.HEC.Enabled = true
	cfg.Delivery.SpoolPath = "./outbox"
	if _, err := config.SaveConfig(context.Background(), configPath, tempDir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n10.0.0.1,host-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spoolDir := filepath.Join(tempDir, "outbox")
	if err := os.MkdirAll(spoolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spoolDir, "status.json"), []byte(`{"state":"healthy","confirmation_mode":"hec_accepted_only"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configRevision, err := filerevision.File(configPath)
	if err != nil {
		t.Fatal(err)
	}
	endpointsRevision, err := filerevision.File(endpointsPath)
	if err != nil {
		t.Fatal(err)
	}
	tracker := runtimeinfo.New("monitor", configRevision, endpointsRevision, 1)
	handler, err := newHandler(Options{
		ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: tempDir,
		Version: "test", Runtime: tracker, EffectiveConfig: &cfg,
	})
	if err != nil {
		t.Fatal(err)
	}

	// A draft disk edit may be invalid while the monitor continues with its
	// startup config. Status must remain cheap and truthful instead of reparsing
	// that PSD1 on every browser poll.
	if err := os.WriteFile(configPath, []byte("@{ invalid ="), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var payload statusResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Delivery == nil || payload.Delivery.State != "healthy" {
		t.Fatalf("delivery = %#v, want cached healthy status", payload.Delivery)
	}
	if !payload.ConfigRestartRequired {
		t.Fatal("config_restart_required = false after disk revision changed")
	}
}

func TestPersistDiscoverySnapshotRetainsHistoryAndCalculatesDelta(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg,
	})
	request := discoveryRunRequest{TargetNetwork: "10.0.0.0", SubnetMask: 24, TimeoutMs: 500, ThrottleLimit: 25}
	first := discoveryResponse{
		GeneratedAt: "2026-07-27T12:00:00Z", ScanID: "scan-1", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 2},
		Items:   []models.Endpoint{{IP: "10.0.0.1", Hostname: "one"}, {IP: "10.0.0.2", Hostname: "two"}},
	}
	if err := server.persistDiscoverySnapshot(request, &first); err != nil {
		t.Fatal(err)
	}
	if first.Delta.New != 2 || first.Delta.Missing != 0 {
		t.Fatalf("first delta = %+v", first.Delta)
	}
	second := discoveryResponse{
		GeneratedAt: "2026-07-28T12:00:00Z", ScanID: "scan-2", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 2},
		Items:   []models.Endpoint{{IP: "10.0.0.2", Hostname: "two"}, {IP: "10.0.0.3", Hostname: "three"}},
	}
	if err := server.persistDiscoverySnapshot(request, &second); err != nil {
		t.Fatal(err)
	}
	if second.Delta.PreviousScanID != "scan-1" || second.Delta.New != 1 || second.Delta.Missing != 1 || second.Delta.Unchanged != 1 {
		t.Fatalf("second delta = %+v", second.Delta)
	}
	scans, err := filepath.Glob(filepath.Join(root, "history", "scans", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 2 {
		t.Fatalf("history files = %d, want 2", len(scans))
	}
}

func TestPreviousScheduleOccurrenceSupportsCatchUpAndTimezone(t *testing.T) {
	scheduleConfig := config.DiscoverySchedule{
		ID: "weekly", Frequency: "weekly", Day: "sunday", Time: "02:00", Timezone: "America/New_York",
	}
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	occurrence, err := previousScheduleOccurrence(scheduleConfig, now)
	if err != nil {
		t.Fatal(err)
	}
	if occurrence.Weekday() != time.Sunday || occurrence.Hour() != 2 || !occurrence.Before(now) {
		t.Fatalf("occurrence = %s", occurrence)
	}
}

func containsAll(body string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			return false
		}
	}
	return true
}
