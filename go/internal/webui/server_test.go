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
	"regexp"
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
	for _, expected := range []string{"/theme.js?v=v5.9.0", "/app.css?v=v5.9.0", "/discovery_csv.js?v=v5.9.0", "/app.js?v=v5.9.0"} {
		if !strings.Contains(indexResponse.Body.String(), expected) {
			t.Fatalf("index does not contain versioned asset %q", expected)
		}
	}
	if cacheControl := indexResponse.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("index Cache-Control = %q, want no-store", cacheControl)
	}

	for _, path := range []string{"/advisor", "/endpoints", "/discovery", "/settings/appearance", "/settings/discovery", "/settings/splunk", "/settings/diagnostics"} {
		deepLinkResponse := httptest.NewRecorder()
		handler.ServeHTTP(deepLinkResponse, httptest.NewRequest(http.MethodGet, path, nil))
		if deepLinkResponse.Code != http.StatusOK {
			t.Errorf("deep link %s status = %d, want 200", path, deepLinkResponse.Code)
			continue
		}
		if !strings.Contains(deepLinkResponse.Body.String(), `data-route="/endpoints"`) {
			t.Errorf("deep link %s did not serve the application shell", path)
		}
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

func TestStaticUIUsesRoutedPagesAndConsolidatedActions(t *testing.T) {
	indexBytes, err := fs.ReadFile(staticFiles, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	appBytes, err := fs.ReadFile(staticFiles, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	cssBytes, err := fs.ReadFile(staticFiles, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := string(indexBytes)
	appJS := string(appBytes)
	appCSS := string(cssBytes)

	for _, expected := range []string{
		`data-route="/advisor"`,
		`data-route="/endpoints"`,
		`data-route="/discovery"`,
		`data-route="/settings"`,
		`data-settings-route="/settings/appearance"`,
		`data-settings-route="/settings/discovery"`,
		`data-settings-route="/settings/splunk"`,
		`data-settings-route="/settings/diagnostics"`,
		`href="/settings/discovery"`,
		`id="endpoint-bulk-action"`,
		`class="endpoint-workspace"`,
		`id="endpoint-open-editor-button"`,
		`id="endpoint-editor-panel" open`,
		`id="discovery-bulk-action"`,
		`id="discovery-review-action"`,
		`class="discovery-workspace"`,
		`id="discovery-controls-panel" open`,
		`class="panel section-stack discovery-results-panel"`,
		`class="panel-caret"`,
		`id="theme-choice-grid"`,
		`data-theme-choice="signal-blue"`,
		`data-theme-choice="daylight"`,
		`id="appearance-save-button"`,
		`class="action-menu"`,
		`class="panel settings-card section-stack naming-rules-card"`,
	} {
		if !strings.Contains(indexHTML, expected) {
			t.Errorf("routed interface does not contain %q", expected)
		}
	}
	for _, obsolete := range []string{`href="#overview"`, `href="#inventory"`, `href="#discovery"`, `href="#settings"`} {
		if strings.Contains(indexHTML, obsolete) {
			t.Errorf("routed interface still contains anchor navigation %q", obsolete)
		}
	}
	for _, expected := range []string{
		`function renderRoute(pathname, historyMode = '')`,
		`history.pushState(null, '', route)`,
		`window.addEventListener('popstate'`,
		`renderRoute('/endpoints', 'push')`,
		`function openEndpointEditor(scrollIntoView = true)`,
		`async function loadUIPreferences(showSuccess = false)`,
		`putJson('/api/ui-preferences'`,
		`aria-label="Open Pair ${pairNumber} actions"`,
		`event.target === elements.classificationPreviewHostname`,
	} {
		if !strings.Contains(appJS, expected) {
			t.Errorf("route controller does not contain %q", expected)
		}
	}
	for _, expected := range []string{
		`@media (max-width: 1100px)`,
		`grid-template-columns: 76px minmax(0, 1fr)`,
		`@media (max-width: 640px)`,
		`width: calc(100vw - 58px)`,
		`@media (max-width: 480px)`,
		`grid-template-columns: repeat(2, minmax(0, 1fr))`,
		`.action-menu-popover`,
		`.naming-rules-card`,
		`.discovery-workspace`,
		`.discovery-controls-panel[open] .panel-caret`,
		`.endpoint-editor-panel[open] .panel-caret`,
		`.endpoint-field-grid`,
		`.info-grid > .panel`,
		`[data-color-scheme="signal-blue"]`,
		`[data-color-scheme="daylight"]`,
		`.theme-choice-grid`,
		`[data-density="compact"]`,
	} {
		if !strings.Contains(appCSS, expected) {
			t.Errorf("responsive interface does not contain %q", expected)
		}
	}
	discoveryControls := strings.Index(indexHTML, `id="discovery-controls-panel"`)
	discoveryResults := strings.Index(indexHTML, `class="panel section-stack discovery-results-panel"`)
	if discoveryControls < 0 || discoveryResults <= discoveryControls {
		t.Errorf("Discovery Results must remain a full-width panel after the collapsible Discovery Controls panel: controls=%d results=%d", discoveryControls, discoveryResults)
	}
	endpointTable := strings.Index(indexHTML, `class="panel section-stack endpoint-table-panel"`)
	endpointEditor := strings.Index(indexHTML, `id="endpoint-editor-panel"`)
	if endpointTable < 0 || endpointEditor <= endpointTable {
		t.Errorf("Endpoint Editor must remain a full-width panel after the Endpoint Inventory table: table=%d editor=%d", endpointTable, endpointEditor)
	}
	for _, obsolete := range []string{"scrollSectionIntoView", "updateActiveNavFromScroll", "sectionHashes"} {
		if strings.Contains(appJS, obsolete) {
			t.Errorf("route controller still contains obsolete scroll navigation %q", obsolete)
		}
	}

	idPattern := regexp.MustCompile(`\bid="([^"]+)"`)
	seenIDs := make(map[string]bool)
	for _, match := range idPattern.FindAllStringSubmatch(indexHTML, -1) {
		if seenIDs[match[1]] {
			t.Errorf("static interface contains duplicate id %q", match[1])
		}
		seenIDs[match[1]] = true
	}
}

func TestStaticNamingRuleEditorKeepsDynamicContextAccessible(t *testing.T) {
	appBytes, err := fs.ReadFile(staticFiles, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	appJS := string(appBytes)
	for _, expected := range []string{
		`aria-labelledby="${titleID}"`,
		`data-rule-title`,
		`aria-label="Move Pair ${pairNumber} up"`,
		`aria-label="Move Pair ${pairNumber} down"`,
		`aria-label="Remove Pair ${pairNumber}"`,
		`title.textContent = `,
	} {
		if !strings.Contains(appJS, expected) {
			t.Errorf("naming rule editor does not contain %q", expected)
		}
	}
}

func TestStaticUIProvidesContextHelpForEveryConfigurationField(t *testing.T) {
	indexBytes, err := fs.ReadFile(staticFiles, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	appBytes, err := fs.ReadFile(staticFiles, "static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := string(indexBytes)
	appJS := string(appBytes)
	fieldPattern := regexp.MustCompile(`id="(cfg-[^"]+)"`)
	matches := fieldPattern.FindAllStringSubmatch(indexHTML, -1)
	if len(matches) == 0 {
		t.Fatal("no configuration fields found")
	}
	for _, match := range matches {
		if !strings.Contains(appJS, fmt.Sprintf("'%s': helpTopic(", match[1])) {
			t.Errorf("configuration field %s has no contextual help topic", match[1])
		}
	}
	for _, requiredID := range []string{
		"endpoint-ip", "endpoint-hostname", "endpoint-fqdn", "endpoint-device-mode", "endpoint-alerting-enabled",
		"endpoint-alerting-reason", "endpoint-dynamic-address", "endpoint-asset-id", "endpoint-classification-source", "endpoint-monitoring-enabled",
		"endpoint-maintenance-until", "discovery-target-network", "discovery-subnet-mask",
		"discovery-timeout-ms", "discovery-throttle-limit", "discovery-merge-mode", "discovery-bulk-group",
		"discovery-bulk-entitytype", "discovery-bulk-device", "discovery-bulk-vendor",
		"discovery-review-note",
	} {
		if !strings.Contains(appJS, fmt.Sprintf("'%s': helpTopic(", requiredID)) {
			t.Errorf("operator field %s has no contextual help topic", requiredID)
		}
	}
	panelPattern := regexp.MustCompile(`<h3 class="panel-title">([^<]+)</h3>`)
	for _, match := range panelPattern.FindAllStringSubmatch(indexHTML, -1) {
		title := strings.TrimSpace(match[1])
		quoted := fmt.Sprintf("'%s': panelHelp(", title)
		identifier := fmt.Sprintf("%s: panelHelp(", title)
		if !strings.Contains(appJS, quoted) && !strings.Contains(appJS, identifier) {
			t.Errorf("interface panel %q has no contextual help topic", title)
		}
	}
	discoveryStart := strings.Index(indexHTML, `<section id="discovery"`)
	operationsStart := strings.Index(indexHTML, `id="discovery-operations-panel"`)
	settingsStart := strings.Index(indexHTML, `<section id="settings"`)
	if discoveryStart < 0 || operationsStart < discoveryStart || settingsStart < operationsStart {
		t.Fatalf("Discovery Operations must remain inside the Discovery section: discovery=%d operations=%d settings=%d", discoveryStart, operationsStart, settingsStart)
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
	if body := resp.Body.String(); !containsAll(body, "Ping Monitor", `data-route="/endpoints"`, "Collector Administration", "Monitoring Cycle", "Pending Delivery", "Cancel Discovery", "Delete this endpoint", "Needs Review", "Defer Review", "Return to Needs Review", "/review_workflow.js?v=test") {
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
	index, err := loadDiscoveryHistoryIndex(cfg.Discovery.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Scans) != 2 || index.Scans[0].ScanID != "scan-2" || index.Scans[1].ScanID != "scan-1" {
		t.Fatalf("history index = %#v", index.Scans)
	}
	detail, err := loadDiscoverySnapshot(cfg.Discovery.HistoryPath, index, "scan-2")
	if err != nil {
		t.Fatal(err)
	}
	newItems, missingItems, unresolvedItems := calculateDiscoveryItemChanges(first.Items, detail.Items)
	if len(newItems) != 1 || newItems[0].IP != "10.0.0.3" || len(missingItems) != 1 || missingItems[0].IP != "10.0.0.1" {
		t.Fatalf("item changes new=%#v missing=%#v", newItems, missingItems)
	}
	if len(unresolvedItems) != 0 {
		t.Fatalf("unresolved items = %#v, want none", unresolvedItems)
	}
}

func TestDiscoveryEventsPreserveTruthfulObservedAndMissingEvidence(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	cfg.Discovery.Subnets = []config.DiscoverySubnet{{
		ID: "lab", CIDR: "10.0.0.0/24", Name: "Lab", VLAN: "100", Location: "HQ",
		AddressingMode: "static", RoutingDomain: "corp",
	}}
	var emittedBatchID string
	var emitted []json.RawMessage
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg, CollectorID: "collector-1", CollectorHost: "host-1",
		EmitDiscoveryEvents: func(_ context.Context, batchID string, events []json.RawMessage) error {
			emittedBatchID = batchID
			emitted = append([]json.RawMessage(nil), events...)
			return nil
		},
	})
	request := discoveryRunRequest{
		TargetNetwork: "10.0.0.0", SubnetMask: 24, TimeoutMs: 700,
		ThrottleLimit: 40, ScheduleID: "weekly",
	}
	first := discoveryResponse{
		GeneratedAt: "2026-07-27T12:00:00Z", ScanID: "scan-1", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 2},
		Items: []models.Endpoint{
			{IP: "10.0.0.1", Hostname: "one", FQDN: "one.example.test", DNSStatus: "verified", DNSForwardConfirmed: true},
			{IP: "10.0.0.2", Hostname: "two"},
		},
	}
	if err := server.persistDiscoverySnapshot(request, &first); err != nil {
		t.Fatal(err)
	}
	second := discoveryResponse{
		GeneratedAt: "2026-07-28T12:00:00Z", ScanID: "scan-2", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 2},
		Items: []models.Endpoint{
			{IP: "10.0.0.2", Hostname: "two"},
			{IP: "10.0.0.3", Hostname: "three"},
		},
		DurationMs: 1234,
	}
	if err := server.persistDiscoverySnapshot(request, &second); err != nil {
		t.Fatal(err)
	}
	if err := server.emitDiscoveryEvents(context.Background(), request, second); err != nil {
		t.Fatal(err)
	}
	if emittedBatchID != "discovery-scan-2" {
		t.Fatalf("batch id = %q", emittedBatchID)
	}
	if len(emitted) != 4 {
		t.Fatalf("events = %d, want scan summary + 2 observations + 1 missing", len(emitted))
	}
	var scan models.DiscoveryScanEvent
	if err := json.Unmarshal(emitted[0], &scan); err != nil {
		t.Fatal(err)
	}
	if scan.RecordType != "discovery_scan_summary" || scan.NewEndpoints != 1 || scan.MissingEndpoints != 1 || scan.Unchanged != 1 || !scan.BaselineAvailable {
		t.Fatalf("scan event = %#v", scan)
	}
	if scan.SubnetID != "lab" || scan.SubnetName != "Lab" || scan.SubnetVLAN != "100" || scan.SubnetLocation != "HQ" || scan.RoutingDomain != "corp" {
		t.Fatalf("scan subnet metadata = %#v", scan)
	}
	byIP := make(map[string]models.DiscoveryEvent)
	for _, raw := range emitted[1:] {
		var event models.DiscoveryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		byIP[event.TargetIP] = event
		var fields map[string]interface{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, exists := fields["state"]; exists {
			t.Fatalf("discovery evidence incorrectly claimed monitoring state: %s", raw)
		}
		if event.SubnetID != "lab" {
			t.Fatalf("discovery event subnet id = %q", event.SubnetID)
		}
	}
	if event := byIP["10.0.0.1"]; event.DiscoveryStatus != "not_observed" || event.DiscoveryDelta != "missing" || event.DiscoveryObserved {
		t.Fatalf("missing evidence = %#v", event)
	}
	if event := byIP["10.0.0.2"]; event.DiscoveryStatus != "observed" || event.DiscoveryDelta != "unchanged" || !event.DiscoveryObserved {
		t.Fatalf("unchanged evidence = %#v", event)
	}
	if event := byIP["10.0.0.3"]; event.DiscoveryDelta != "new" || !event.DiscoveryObserved {
		t.Fatalf("new evidence = %#v", event)
	}
}

func TestDynamicDiscoveryUsesStableIdentityAndDoesNotInventNewAssets(t *testing.T) {
	previous := discoverySnapshot{ScanID: "scan-1", Items: []models.Endpoint{
		{IP: "10.0.0.10", FQDN: "laptop.example.test", DNSForwardConfirmed: true, DynamicAddress: true},
		{IP: "10.0.0.20", Hostname: "unresolved-a", DynamicAddress: true},
	}}
	current := []models.Endpoint{
		{IP: "10.0.0.11", FQDN: "laptop.example.test", DNSForwardConfirmed: true, DynamicAddress: true},
		{IP: "10.0.0.21", Hostname: "unresolved-b", DynamicAddress: true},
	}
	delta := calculateDiscoveryDelta(previous, current)
	if delta.New != 0 || delta.Missing != 0 || delta.Unchanged != 1 || delta.UnresolvedDynamic != 1 {
		t.Fatalf("dynamic delta = %#v", delta)
	}
	newItems, missingItems, unresolved := calculateDiscoveryItemChanges(previous.Items, current)
	if len(newItems) != 0 || len(missingItems) != 0 || len(unresolved) != 1 || unresolved[0].IP != "10.0.0.21" {
		t.Fatalf("dynamic changes new=%#v missing=%#v unresolved=%#v", newItems, missingItems, unresolved)
	}
}

func TestDiscoveryEnrichmentAppliesSubnetAndRegexRules(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.Subnets = []config.DiscoverySubnet{
		{ID: "broad", CIDR: "10.20.0.0/16", Name: "Broad", AddressingMode: "static"},
		{ID: "servers", CIDR: "10.20.30.0/24", Name: "Server VLAN", VLAN: "230", Location: "NYC", AddressingMode: "dhcp", RoutingDomain: "corp"},
	}
	cfg.Classification.Rules = []config.ClassificationRule{{
		ID: "server", Enabled: true, Source: "hostname", Pattern: `^(?P<site>[a-z]{3})-srv-`,
		Assignments: map[string]string{"group": "${site} servers", "entitytype": "server", "device": "vm"},
	}}
	server := newAPIServer(Options{ConfigPath: filepath.Join(root, "config.psd1"), RootDir: root, EffectiveConfig: &cfg})
	items, err := server.enrichDiscoveryEndpoints("10.20.30.0/24", []models.Endpoint{{
		IP: "10.20.30.10", Hostname: "nyc-srv-001", Group: "default",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].DynamicAddress || items[0].Group != "nyc servers" ||
		items[0].EntityType != "server" || items[0].Device != "vm" || items[0].ClassificationSource != "rule:server" ||
		items[0].DiscoveryReviewState != models.DiscoveryReviewNeedsReview ||
		items[0].SubnetID != "servers" || items[0].SubnetName != "Server VLAN" || items[0].SubnetVLAN != "230" ||
		items[0].SubnetLocation != "NYC" || items[0].AddressingMode != "dhcp" || items[0].RoutingDomain != "corp" {
		t.Fatalf("enriched item = %#v", items)
	}
}

func TestClassificationPreviewHandlerReturnsChangesWithoutMutation(t *testing.T) {
	server := newAPIServer(Options{RootDir: t.TempDir()})
	body := `{
		"rules":[{"id":"network","enabled":true,"source":"hostname","pattern":"^(?P<site>[a-z]{3})-(?P<role>sw|fw)-","assignments":{"group":"${site}","entitytype":"network","device":"${role}"}}],
		"items":[{"ip":"10.0.0.1","hostname":"nyc-sw-01","group":"default"}]
	}`
	response := httptest.NewRecorder()
	server.handleClassificationPreview(response, httptest.NewRequest(http.MethodPost, "/api/classification/preview", strings.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", response.Code, response.Body.String())
	}
	var payload classificationPreviewResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 1 || payload.Results[0].Endpoint.Group != "nyc" ||
		payload.Results[0].Endpoint.Device != "sw" || payload.Results[0].Endpoint.EntityType != "network" ||
		payload.Results[0].Endpoint.ClassificationSource != "rule:network" {
		t.Fatalf("preview payload = %#v", payload)
	}
}

func TestDiscoveryReviewRegistryPersistsDecisionAndMetadataAcrossAddressChange(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg,
	})
	body := `{
		"state":"deferred",
		"note":"Awaiting CMDB owner",
		"items":[{
			"ip":"10.20.30.10","hostname":"laptop-01","fqdn":"laptop-01.example.test",
			"dns_forward_confirmed":true,"dynamic_address":true,"asset_id":"asset-42",
			"group":"Workstations","entitytype":"Endpoint","device":"Laptop","vendor":"Example"
		}]
	}`
	response := httptest.NewRecorder()
	server.handleDiscoveryReviews(response, httptest.NewRequest(http.MethodPost, "/api/discovery/reviews", strings.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("review status = %d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.Discovery.HistoryPath, "reviews.json")); err != nil {
		t.Fatalf("review registry was not persisted: %v", err)
	}

	restarted := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg,
	})
	items, err := restarted.reconcileDiscoveryEndpoints([]models.Endpoint{{
		IP: "10.20.30.99", Hostname: "laptop-01", FQDN: "laptop-01.example.test",
		DNSForwardConfirmed: true, DynamicAddress: true, Group: "default",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].DiscoveryReviewState != models.DiscoveryReviewDeferred ||
		items[0].DiscoveryReviewNote != "Awaiting CMDB owner" || items[0].AssetID != "asset-42" ||
		items[0].Group != "Workstations" || items[0].IP != "10.20.30.99" {
		t.Fatalf("reconciled review = %#v", items)
	}
}

func TestDiscoveryInventoryReconciliationCarriesAssetIdentityAcrossDHCPChange(t *testing.T) {
	root := t.TempDir()
	endpointsPath := filepath.Join(root, "endpoints.csv")
	known := models.Endpoint{
		IP: "10.20.30.10", Hostname: "laptop-01", FQDN: "laptop-01.example.test",
		DNSForwardConfirmed: true, DynamicAddress: true, AssetID: "asset-42", Group: "Managed",
		DeviceMode: models.DeviceModeProduction, AlertingEnabled: models.Bool(false),
	}
	if err := config.SaveEndpoints(endpointsPath, []models.Endpoint{known}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: endpointsPath,
		RootDir: root, EffectiveConfig: &cfg,
	})
	items, err := server.reconcileDiscoveryEndpoints([]models.Endpoint{{
		IP: "10.20.30.99", Hostname: "laptop-01", FQDN: "laptop-01.example.test",
		DNSForwardConfirmed: true, DynamicAddress: true, Group: "default",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AssetID != "asset-42" || items[0].IP != "10.20.30.99" ||
		items[0].DiscoveryReviewState != models.DiscoveryReviewApproved || items[0].IsAlertingEnabled() {
		t.Fatalf("inventory reconciliation = %#v", items)
	}

	previous := discoverySnapshot{ScanID: "scan-1", Items: []models.Endpoint{{
		IP: "10.20.30.10", Hostname: "laptop-01", FQDN: "laptop-01.example.test",
		DNSForwardConfirmed: true, DynamicAddress: true,
	}}}
	delta := calculateDiscoveryDelta(previous, items)
	if delta.New != 0 || delta.Missing != 0 || delta.Unchanged != 1 || delta.UnresolvedDynamic != 0 {
		t.Fatalf("identity transition delta = %#v", delta)
	}
}

func TestDiscoveryReviewHandlerRejectsApprovalWithoutEndpointSave(t *testing.T) {
	server := newAPIServer(Options{RootDir: t.TempDir()})
	response := httptest.NewRecorder()
	server.handleDiscoveryReviews(response, httptest.NewRequest(http.MethodPost, "/api/discovery/reviews", strings.NewReader(`{
		"state":"approved","items":[{"ip":"10.0.0.1","hostname":"one"}]
	}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "saving the device") {
		t.Fatalf("approval response = %d %s", response.Code, response.Body.String())
	}
}

func TestClassificationPreviewHandlerRejectsInvalidRegex(t *testing.T) {
	server := newAPIServer(Options{RootDir: t.TempDir()})
	body := `{"rules":[{"id":"bad","enabled":true,"source":"hostname","pattern":"[","assignments":{"group":"x"}}],"items":[{"hostname":"test"}]}`
	response := httptest.NewRecorder()
	server.handleClassificationPreview(response, httptest.NewRequest(http.MethodPost, "/api/classification/preview", strings.NewReader(body)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "pattern") {
		t.Fatalf("preview status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestPersistDiscoverySnapshotPrunesByConfiguredScanCount(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	cfg.Discovery.RetentionScans = 1
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg,
	})
	request := discoveryRunRequest{TargetNetwork: "10.0.0.0", SubnetMask: 24}
	first := discoveryResponse{
		GeneratedAt: "2026-07-27T12:00:00Z", ScanID: "scan-1", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 1}, Items: []models.Endpoint{{IP: "10.0.0.1", Hostname: "one"}},
	}
	second := discoveryResponse{
		GeneratedAt: "2026-07-28T12:00:00Z", ScanID: "scan-2", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 1}, Items: []models.Endpoint{{IP: "10.0.0.2", Hostname: "two"}},
	}
	if err := server.persistDiscoverySnapshot(request, &first); err != nil {
		t.Fatal(err)
	}
	if err := server.persistDiscoverySnapshot(request, &second); err != nil {
		t.Fatal(err)
	}
	scans, err := filepath.Glob(filepath.Join(root, "history", "scans", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 1 {
		t.Fatalf("history files = %d, want 1", len(scans))
	}
	index, err := loadDiscoveryHistoryIndex(cfg.Discovery.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Scans) != 1 || index.Scans[0].ScanID != "scan-2" {
		t.Fatalf("pruned history index = %#v", index.Scans)
	}
}

func TestDiscoveryHistoryRetainsBoundedCustomerScaleSnapshots(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	cfg.Discovery.RetentionScans = 12
	cfg.Discovery.RetentionDays = 0
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg,
	})
	items := make([]models.Endpoint, 2048)
	for index := range items {
		items[index] = models.Endpoint{
			IP:        fmt.Sprintf("10.%d.%d.%d", index/65536, (index/256)%256, index%256),
			Hostname:  fmt.Sprintf("asset-%04d", index),
			FQDN:      fmt.Sprintf("asset-%04d.example.test", index),
			DNSStatus: "verified", DNSForwardConfirmed: true,
		}
	}
	request := discoveryRunRequest{TargetNetwork: "10.0.0.0", SubnetMask: 8}
	start := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	for scan := 0; scan < 36; scan++ {
		response := discoveryResponse{
			GeneratedAt: start.Add(time.Duration(scan) * time.Hour).Format(time.RFC3339),
			ScanID:      fmt.Sprintf("scale-scan-%02d", scan), Target: "10.0.0.0/8",
			Summary: endpointSummary{Total: len(items)}, Items: items,
		}
		if err := server.persistDiscoverySnapshot(request, &response); err != nil {
			t.Fatalf("persist scan %d: %v", scan, err)
		}
	}
	index, err := loadDiscoveryHistoryIndex(cfg.Discovery.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Scans) != cfg.Discovery.RetentionScans {
		t.Fatalf("history index scans = %d, want %d", len(index.Scans), cfg.Discovery.RetentionScans)
	}
	if index.Scans[0].ScanID != "scale-scan-35" || index.Scans[len(index.Scans)-1].ScanID != "scale-scan-24" {
		t.Fatalf("retained range = %s..%s", index.Scans[0].ScanID, index.Scans[len(index.Scans)-1].ScanID)
	}
	files, err := filepath.Glob(filepath.Join(cfg.Discovery.HistoryPath, "scans", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != cfg.Discovery.RetentionScans {
		t.Fatalf("snapshot files = %d, want %d", len(files), cfg.Discovery.RetentionScans)
	}
}

func TestDiscoveryScheduleStatusesReportDueAndLastError(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.Schedules = []config.DiscoverySchedule{{
		ID: "weekly", Enabled: true, Targets: []string{"10.0.0.0/24"},
		Frequency: "weekly", Day: "sunday", Time: "02:00", Timezone: "UTC",
	}}
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	statuses := discoveryScheduleStatuses(cfg, root, now)
	if len(statuses) != 1 || !statuses[0].Due || statuses[0].NextRunAt == "" {
		t.Fatalf("schedule statuses = %#v", statuses)
	}
}

func TestDiscoveryHistoryHandlerListsAndLoadsRetainedScan(t *testing.T) {
	root := t.TempDir()
	cfg := config.Defaults(root)
	cfg.Discovery.HistoryPath = filepath.Join(root, "history")
	cfg.Discovery.RetentionDays = 90
	server := newAPIServer(Options{
		ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv"),
		RootDir: root, EffectiveConfig: &cfg,
	})
	request := discoveryRunRequest{TargetNetwork: "10.0.0.0", SubnetMask: 24, ScheduleID: "weekly"}
	response := discoveryResponse{
		GeneratedAt: "2026-07-27T12:00:00Z", ScanID: "scan-history", Target: "10.0.0.0/24",
		Summary: endpointSummary{Total: 1, Production: 1, Groups: 1},
		Items:   []models.Endpoint{{IP: "10.0.0.1", Hostname: "one"}},
	}
	if err := server.persistDiscoverySnapshot(request, &response); err != nil {
		t.Fatal(err)
	}

	listRecorder := httptest.NewRecorder()
	server.handleDiscoveryHistory(listRecorder, httptest.NewRequest(http.MethodGet, "/api/discovery/history?limit=10", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var list discoveryHistoryResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.TotalScans != 1 || len(list.Scans) != 1 || list.Scans[0].ScheduleID != "weekly" || list.RetentionDays != 90 {
		t.Fatalf("history list = %#v", list)
	}
	if list.Scans[0].FileName != "" {
		t.Fatalf("history API leaked internal file name %q", list.Scans[0].FileName)
	}

	detailRecorder := httptest.NewRecorder()
	server.handleDiscoveryHistory(detailRecorder, httptest.NewRequest(http.MethodGet, "/api/discovery/history?scan_id=scan-history", nil))
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detail discoveryHistoryDetail
	if err := json.Unmarshal(detailRecorder.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.BaselineAvailable || len(detail.NewItems) != 1 || detail.NewItems[0].IP != "10.0.0.1" {
		t.Fatalf("history detail = %#v", detail)
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
