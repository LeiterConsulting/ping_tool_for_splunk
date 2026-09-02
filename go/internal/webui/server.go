package webui

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/advisor"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/classification"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/httpcfg"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/outbox"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed assets/DiscoverEndpoints.ps1
var embeddedDiscoveryScript []byte

const embeddedDiscoveryScriptPath = "embedded:DiscoverEndpoints.ps1"

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

type Options struct {
	ListenAddr              string
	ConfigPath              string
	EndpointsPath           string
	RootDir                 string
	DiscoveryScriptPath     string
	Version                 string
	CollectorID             string
	CollectorHost           string
	Runtime                 *runtimeinfo.Tracker
	EffectiveConfig         *config.Config
	EffectiveConfigProvider func() (config.Config, bool)
	RequestRestart          func(RestartRequest) error
	EmitDiscoveryEvents     func(context.Context, string, []json.RawMessage) error
}

type RestartRequest struct {
	ConfigRevision    string `json:"config_revision"`
	EndpointsRevision string `json:"endpoints_revision"`
}

type statusResponse struct {
	Product                string               `json:"product"`
	Version                string               `json:"version"`
	CollectorID            string               `json:"collector_id"`
	ConfigPath             string               `json:"config_path"`
	ConfigFormat           string               `json:"config_format"`
	EndpointsPath          string               `json:"endpoints_path"`
	DiscoveryAvailable     bool                 `json:"discovery_available"`
	DiscoveryScriptPath    string               `json:"discovery_script_path,omitempty"`
	Mode                   string               `json:"mode"`
	Delivery               *outbox.Status       `json:"delivery,omitempty"`
	ResultLog              *resultLogStatus     `json:"result_log,omitempty"`
	Runtime                runtimeinfo.Snapshot `json:"runtime"`
	ConfigRevision         string               `json:"config_revision,omitempty"`
	EndpointsRevision      string               `json:"endpoints_revision,omitempty"`
	ConfigRestartRequired  bool                 `json:"config_restart_required"`
	EndpointsReloadPending bool                 `json:"endpoints_reload_pending"`
}

type resultLogStatus struct {
	Enabled           bool   `json:"enabled"`
	Path              string `json:"path"`
	CurrentBytes      int64  `json:"current_bytes"`
	MaxBytes          int64  `json:"max_bytes"`
	ArchiveCount      int    `json:"archive_count"`
	RetentionFiles    int    `json:"retention_files"`
	RetentionDays     int    `json:"retention_days"`
	CompressRotated   bool   `json:"compress_rotated"`
	ThresholdExceeded bool   `json:"threshold_exceeded"`
}

type endpointSummary struct {
	Total      int `json:"total"`
	Production int `json:"production"`
	Dev        int `json:"dev"`
	Groups     int `json:"groups"`
}

type endpointsResponse struct {
	GeneratedAt   string            `json:"generated_at"`
	EndpointsPath string            `json:"endpoints_path"`
	Summary       endpointSummary   `json:"summary"`
	Items         []models.Endpoint `json:"items"`
	Revision      string            `json:"revision"`
	ReloadPending bool              `json:"reload_pending"`
}

type endpointsWriteRequest struct {
	Items    []models.Endpoint `json:"items"`
	Revision string            `json:"revision"`
}

type configResponse struct {
	GeneratedAt     string        `json:"generated_at"`
	ConfigPath      string        `json:"config_path"`
	ConfigFormat    string        `json:"config_format"`
	Config          config.Config `json:"config"`
	Secrets         secretStatus  `json:"secrets"`
	Revision        string        `json:"revision"`
	RestartRequired bool          `json:"restart_required"`
}

type configWriteRequest struct {
	Config            config.Config `json:"config"`
	ClearHECToken     bool          `json:"clear_hec_token,omitempty"`
	ClearMetricsToken bool          `json:"clear_metrics_token,omitempty"`
	Revision          string        `json:"revision"`
}

type classificationPreviewRequest struct {
	Rules []config.ClassificationRule `json:"rules"`
	Items []models.Endpoint           `json:"items"`
}

type classificationPreviewResponse struct {
	Results []classification.Result `json:"results"`
}

type secretStatus struct {
	HECTokenConfigured     bool `json:"hec_token_configured"`
	MetricsTokenConfigured bool `json:"metrics_token_configured"`
}

type discoveryRunRequest struct {
	TargetNetwork string `json:"target_network"`
	SubnetMask    int    `json:"subnet_mask"`
	TimeoutMs     int    `json:"timeout_ms"`
	ThrottleLimit int    `json:"throttle_limit"`
	ScheduleID    string `json:"schedule_id,omitempty"`
}

type discoveryResponse struct {
	GeneratedAt string            `json:"generated_at"`
	ScanID      string            `json:"scan_id"`
	Target      string            `json:"target"`
	Summary     endpointSummary   `json:"summary"`
	Delta       discoveryDelta    `json:"delta"`
	Items       []models.Endpoint `json:"items"`
	Logs        string            `json:"logs,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
	changes     discoveryItemChanges
}

type discoveryStreamEvent struct {
	Type        string            `json:"type"`
	SummaryText string            `json:"summary_text,omitempty"`
	LogLine     string            `json:"log_line,omitempty"`
	GeneratedAt string            `json:"generated_at,omitempty"`
	ScanID      string            `json:"scan_id,omitempty"`
	Target      string            `json:"target,omitempty"`
	Summary     *endpointSummary  `json:"summary,omitempty"`
	Delta       *discoveryDelta   `json:"delta,omitempty"`
	Items       []models.Endpoint `json:"items,omitempty"`
	Logs        string            `json:"logs,omitempty"`
	DurationMs  int64             `json:"duration_ms,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type discoveryProgressState struct {
	Target      string
	HostsToScan int
	ActiveHosts int
	Stage       string
	SummaryText string
}

type discoveryScanResult struct {
	Logs     string
	Progress discoveryProgressState
	Err      error
}

type discoveryDelta struct {
	PreviousScanID    string `json:"previous_scan_id,omitempty"`
	New               int    `json:"new"`
	Missing           int    `json:"missing"`
	Unchanged         int    `json:"unchanged"`
	UnresolvedDynamic int    `json:"unresolved_dynamic"`
}

type discoveryItemChanges struct {
	NewItems               []models.Endpoint
	MissingItems           []models.Endpoint
	UnresolvedDynamicItems []models.Endpoint
}

type discoverySnapshot struct {
	SchemaVersion  int                 `json:"schema_version"`
	ScanID         string              `json:"scan_id"`
	GeneratedAt    string              `json:"generated_at"`
	Target         string              `json:"target"`
	SubnetID       string              `json:"subnet_id,omitempty"`
	SubnetName     string              `json:"subnet_name,omitempty"`
	SubnetVLAN     string              `json:"subnet_vlan,omitempty"`
	SubnetLocation string              `json:"subnet_location,omitempty"`
	AddressingMode string              `json:"addressing_mode,omitempty"`
	RoutingDomain  string              `json:"routing_domain,omitempty"`
	Request        discoveryRunRequest `json:"request"`
	Summary        endpointSummary     `json:"summary"`
	Delta          discoveryDelta      `json:"delta"`
	Items          []models.Endpoint   `json:"items"`
	DurationMs     int64               `json:"duration_ms,omitempty"`
}

type discoveryScanSummary struct {
	ScanID         string          `json:"scan_id"`
	GeneratedAt    string          `json:"generated_at"`
	Target         string          `json:"target"`
	SubnetID       string          `json:"subnet_id,omitempty"`
	SubnetName     string          `json:"subnet_name,omitempty"`
	SubnetVLAN     string          `json:"subnet_vlan,omitempty"`
	SubnetLocation string          `json:"subnet_location,omitempty"`
	AddressingMode string          `json:"addressing_mode,omitempty"`
	RoutingDomain  string          `json:"routing_domain,omitempty"`
	ScheduleID     string          `json:"schedule_id,omitempty"`
	Summary        endpointSummary `json:"summary"`
	Delta          discoveryDelta  `json:"delta"`
	DurationMs     int64           `json:"duration_ms,omitempty"`
	FileName       string          `json:"file_name,omitempty"`
}

type discoveryHistoryIndex struct {
	SchemaVersion int                    `json:"schema_version"`
	Scans         []discoveryScanSummary `json:"scans"`
}

type discoveryScheduleStatus struct {
	ID            string   `json:"id"`
	Enabled       bool     `json:"enabled"`
	Targets       []string `json:"targets"`
	Frequency     string   `json:"frequency"`
	Day           string   `json:"day"`
	Time          string   `json:"time"`
	Timezone      string   `json:"timezone"`
	LastRunAt     string   `json:"last_run_at,omitempty"`
	LastAttemptAt string   `json:"last_attempt_at,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
	NextRunAt     string   `json:"next_run_at,omitempty"`
	Due           bool     `json:"due"`
}

type discoveryHistoryResponse struct {
	GeneratedAt    string                    `json:"generated_at"`
	HistoryPath    string                    `json:"history_path"`
	RetentionScans int                       `json:"retention_scans"`
	RetentionDays  int                       `json:"retention_days"`
	TotalScans     int                       `json:"total_scans"`
	Scans          []discoveryScanSummary    `json:"scans"`
	Schedules      []discoveryScheduleStatus `json:"schedules"`
}

type discoveryHistoryDetail struct {
	discoverySnapshot
	NewItems               []models.Endpoint `json:"new_items"`
	MissingItems           []models.Endpoint `json:"missing_items"`
	UnresolvedDynamicItems []models.Endpoint `json:"unresolved_dynamic_items"`
	BaselineAvailable      bool              `json:"baseline_available"`
}

type outputTestRequest struct {
	Target string        `json:"target"`
	Config config.Config `json:"config"`
}

type outputTestResponse struct {
	Target       string   `json:"target"`
	URL          string   `json:"url"`
	Success      bool     `json:"success"`
	StatusCode   int      `json:"status_code,omitempty"`
	DurationMs   int64    `json:"duration_ms"`
	Message      string   `json:"message"`
	ResponseBody string   `json:"response_body,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

type advisorApplyRequest struct {
	Profile           string `json:"profile"`
	ApplySafe         bool   `json:"apply_safe"`
	ApplyProfile      bool   `json:"apply_profile"`
	ConfigRevision    string `json:"config_revision"`
	EndpointsRevision string `json:"endpoints_revision"`
}

type runtimeRestartRequest struct {
	ConfigRevision    string `json:"config_revision"`
	EndpointsRevision string `json:"endpoints_revision"`
	Confirmed         bool   `json:"confirmed"`
}

type apiServer struct {
	opts            Options
	writeMu         sync.Mutex
	benchmarkMu     sync.Mutex
	statusConfigMu  sync.RWMutex
	statusConfig    config.Config
	hasStatusConfig bool
	discoveryMu     sync.Mutex
}

func Start(ctx context.Context, opts Options) error {
	handler, api, err := newHandlerAndServer(opts)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	diagnostics.LogInfo("web ui listening", map[string]interface{}{
		"listen_addr":    listener.Addr().String(),
		"config_path":    opts.ConfigPath,
		"endpoints_path": opts.EndpointsPath,
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.LogError("web ui shutdown failed", err, nil)
		}
	}()

	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.LogError("web ui serve failed", err, map[string]interface{}{
				"listen_addr": listener.Addr().String(),
			})
		}
	}()

	go api.runDiscoveryScheduler(ctx)

	return nil
}

// StartDiscoveryScheduler runs configured discovery schedules when the admin UI
// listener is disabled. Callers must use either Start or StartDiscoveryScheduler
// for a deployment, not both, so manual and scheduled scans share one lock.
func StartDiscoveryScheduler(ctx context.Context, opts Options) {
	server := newAPIServer(opts)
	go server.runDiscoveryScheduler(ctx)
}

func newHandler(opts Options) (http.Handler, error) {
	handler, _, err := newHandlerAndServer(opts)
	return handler, err
}

func newHandlerAndServer(opts Options) (http.Handler, *apiServer, error) {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, nil, err
	}
	server := newAPIServer(opts)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/status", server.handleStatus)
	mux.HandleFunc("/api/ui-preferences", server.handleUIPreferences)
	mux.HandleFunc("/api/endpoints", server.handleEndpoints)
	mux.HandleFunc("/api/config", server.handleConfig)
	mux.HandleFunc("/api/classification/preview", server.handleClassificationPreview)
	mux.HandleFunc("/api/advisor", server.handleAdvisor)
	mux.HandleFunc("/api/advisor/profiles", server.handleAdvisorProfiles)
	mux.HandleFunc("/api/advisor/apply", server.handleAdvisorApply)
	mux.HandleFunc("/api/advisor/benchmark", server.handleAdvisorBenchmark)
	mux.HandleFunc("/api/runtime/restart", server.handleRuntimeRestart)
	mux.HandleFunc("/api/discovery/run", server.handleDiscoveryRun)
	mux.HandleFunc("/api/discovery/stream", server.handleDiscoveryStream)
	mux.HandleFunc("/api/discovery/history", server.handleDiscoveryHistory)
	mux.HandleFunc("/api/discovery/reviews", server.handleDiscoveryReviews)
	mux.HandleFunc("/api/output/test", server.handleOutputTest)
	mux.Handle("/", staticHandler(staticRoot, opts.Version))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(w, r)
	})
	return handler, server, nil
}

func newAPIServer(opts Options) *apiServer {
	if opts.ConfigPath == "" {
		opts.ConfigPath = "config.psd1"
	}
	if opts.EndpointsPath == "" {
		opts.EndpointsPath = "endpoints.csv"
	}
	if opts.RootDir == "" {
		opts.RootDir = "."
	}
	if strings.TrimSpace(opts.CollectorHost) == "" {
		opts.CollectorHost, _ = os.Hostname()
		if strings.TrimSpace(opts.CollectorHost) == "" {
			opts.CollectorHost = "unknown"
		}
	}
	opts.DiscoveryScriptPath = resolveDiscoveryScriptPath(opts)
	server := &apiServer{opts: opts}
	if opts.EffectiveConfig != nil {
		server.statusConfig = *opts.EffectiveConfig
		server.hasStatusConfig = true
	}
	return server
}

func resolveDiscoveryScriptPath(opts Options) string {
	if opts.DiscoveryScriptPath != "" {
		return cleanPathForStatus(opts.DiscoveryScriptPath)
	}

	// The embedded script is versioned with the binary and is the authoritative
	// default. Automatically preferring an adjacent script lets a stale file
	// silently shadow a collector hotfix after an executable-only upgrade.
	if hasEmbeddedDiscoveryScript() {
		return embeddedDiscoveryScriptPath
	}

	// This fallback only applies to builds where the embedded asset is absent.
	candidates := []string{
		filepath.Join(opts.RootDir, "DiscoverEndpoints.ps1"),
		filepath.Join(filepath.Dir(opts.ConfigPath), "DiscoverEndpoints.ps1"),
		filepath.Join(filepath.Dir(opts.EndpointsPath), "DiscoverEndpoints.ps1"),
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		if discoveryScriptAvailable(cleaned) {
			return cleanPathForStatus(cleaned)
		}
	}

	return cleanPathForStatus(candidates[0])
}

func hasEmbeddedDiscoveryScript() bool {
	return len(bytes.TrimSpace(embeddedDiscoveryScript)) > 0
}

func discoveryScriptAvailable(path string) bool {
	if path == embeddedDiscoveryScriptPath {
		return hasEmbeddedDiscoveryScript()
	}
	_, err := os.Stat(path)
	return err == nil
}

func materializeDiscoveryScript(path string) (string, func(), error) {
	if path != embeddedDiscoveryScriptPath {
		return path, func() {}, nil
	}
	if !hasEmbeddedDiscoveryScript() {
		return "", nil, errors.New("embedded discovery script is not available")
	}
	tempFile, err := os.CreateTemp("", "pingmonitor_discovery_*.ps1")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.Remove(tempFile.Name())
	}
	if _, err := tempFile.Write(embeddedDiscoveryScript); err != nil {
		_ = tempFile.Close()
		cleanup()
		return "", nil, err
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return tempFile.Name(), cleanup, nil
}

func cleanPathForStatus(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return filepath.Clean(path)
}

func inspectResultLog(cfg config.Config) *resultLogStatus {
	enabled := cfg.OutputMode == "file" || cfg.OutputMode == "both"
	status := &resultLogStatus{
		Enabled: enabled, Path: cfg.LogPath,
		MaxBytes:       int64(cfg.LogRotationSizeMB) * 1024 * 1024,
		RetentionFiles: cfg.LogRetentionFiles, RetentionDays: cfg.LogRetentionDays,
		CompressRotated: cfg.LogCompressRotated,
	}
	if !enabled {
		return status
	}
	if info, err := os.Stat(cfg.LogPath); err == nil {
		status.CurrentBytes = info.Size()
	}
	base := strings.TrimSuffix(cfg.LogPath, filepath.Ext(cfg.LogPath))
	archives, _ := filepath.Glob(base + "_*.log*")
	status.ArchiveCount = len(archives)
	status.ThresholdExceeded = status.MaxBytes > 0 && status.CurrentBytes > status.MaxBytes
	return status
}

func (s *apiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info, err := config.ResolveConfigSource(s.opts.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	configRevision, _ := revision.File(info.Path)
	endpointsRevision, _ := revision.File(s.opts.EndpointsPath)
	runtimeSnapshot := s.opts.Runtime.Snapshot()
	configRestartRequired := runtimeSnapshot.Mode == "monitor" && runtimeSnapshot.EffectiveConfigRevision != "" && configRevision != runtimeSnapshot.EffectiveConfigRevision
	endpointsReloadPending := runtimeSnapshot.Mode == "monitor" && runtimeSnapshot.EffectiveEndpointsRevision != "" && endpointsRevision != runtimeSnapshot.EffectiveEndpointsRevision
	var delivery *outbox.Status
	var resultLog *resultLogStatus
	if cfg, ok := s.effectiveStatusConfig(); ok &&
		(((cfg.OutputMode == "hec" || cfg.OutputMode == "both") && cfg.HEC.Enabled) || cfg.Metrics.Enabled) {
		spoolPath := cfg.Delivery.SpoolPath
		if !filepath.IsAbs(spoolPath) {
			spoolPath = filepath.Join(filepath.Dir(info.Path), spoolPath)
		}
		status, statusErr := outbox.ReadStatus(spoolPath)
		if statusErr == nil {
			delivery = &status
		} else if errors.Is(statusErr, os.ErrNotExist) {
			delivery = &outbox.Status{State: "not_started"}
		}
	}
	if cfg, ok := s.effectiveStatusConfig(); ok {
		resultLog = inspectResultLog(cfg)
	}
	writeJSON(w, http.StatusOK, statusResponse{
		Product:                "Ping Monitor",
		Version:                s.opts.Version,
		CollectorID:            s.opts.CollectorID,
		ConfigPath:             info.Path,
		ConfigFormat:           info.Format,
		EndpointsPath:          filepath.Clean(s.opts.EndpointsPath),
		DiscoveryAvailable:     discoveryScriptAvailable(s.opts.DiscoveryScriptPath),
		DiscoveryScriptPath:    s.opts.DiscoveryScriptPath,
		Mode:                   "editable",
		Delivery:               delivery,
		ResultLog:              resultLog,
		Runtime:                runtimeSnapshot,
		ConfigRevision:         configRevision,
		EndpointsRevision:      endpointsRevision,
		ConfigRestartRequired:  configRestartRequired,
		EndpointsReloadPending: endpointsReloadPending,
	})
}

func (s *apiServer) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		endpoints, err := config.LoadEditableEndpoints(s.opts.EndpointsPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		currentRevision, _ := revision.File(s.opts.EndpointsPath)
		runtimeSnapshot := s.opts.Runtime.Snapshot()
		writeJSON(w, http.StatusOK, endpointsResponse{
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			EndpointsPath: filepath.Clean(s.opts.EndpointsPath),
			Summary:       summarizeEndpoints(endpoints),
			Items:         endpoints,
			Revision:      currentRevision,
			ReloadPending: runtimeSnapshot.Mode == "monitor" && runtimeSnapshot.EffectiveEndpointsRevision != "" && currentRevision != runtimeSnapshot.EffectiveEndpointsRevision,
		})
	case http.MethodPut:
		var request endpointsWriteRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		currentRevision, err := revision.File(s.opts.EndpointsPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if request.Revision == "" || request.Revision != currentRevision {
			writeRevisionConflict(w, "endpoint file changed after this draft was loaded", currentRevision)
			return
		}
		if err := config.SaveEndpoints(s.opts.EndpointsPath, request.Items); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		newRevision, _ := revision.File(s.opts.EndpointsPath)
		runtimeSnapshot := s.opts.Runtime.Snapshot()
		endpoints, err := config.LoadEditableEndpoints(s.opts.EndpointsPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		diagnostics.LogInfo("web ui saved endpoints", map[string]interface{}{
			"endpoints_path": s.opts.EndpointsPath,
			"count":          len(endpoints),
		})
		writeJSON(w, http.StatusOK, endpointsResponse{
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			EndpointsPath: filepath.Clean(s.opts.EndpointsPath),
			Summary:       summarizeEndpoints(endpoints),
			Items:         endpoints,
			Revision:      newRevision,
			ReloadPending: runtimeSnapshot.Mode == "monitor" && runtimeSnapshot.EffectiveEndpointsRevision != "" && newRevision != runtimeSnapshot.EffectiveEndpointsRevision,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, info, err := config.LoadEditable(r.Context(), s.opts.ConfigPath, s.opts.RootDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		currentRevision, _ := revision.File(info.Path)
		writeJSON(w, http.StatusOK, redactedConfigResponse(cfg, info, currentRevision, configRestartRequired(s.opts.Runtime, currentRevision)))
	case http.MethodPut:
		var request configWriteRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		currentInfo, err := config.ResolveConfigSource(s.opts.ConfigPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		currentRevision, err := revision.File(currentInfo.Path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if request.Revision == "" || request.Revision != currentRevision {
			writeRevisionConflict(w, "config file changed after this form was loaded", currentRevision)
			return
		}
		existing, _, err := config.LoadEditable(r.Context(), s.opts.ConfigPath, s.opts.RootDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		mergeWriteOnlySecrets(&request.Config, existing, request.ClearHECToken, request.ClearMetricsToken)
		info, err := config.SaveConfig(r.Context(), s.opts.ConfigPath, s.opts.RootDir, request.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		newRevision, _ := revision.File(info.Path)
		cfg, _, err := config.LoadEditable(r.Context(), s.opts.ConfigPath, s.opts.RootDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		diagnostics.LogInfo("web ui saved config", map[string]interface{}{
			"config_path": info.Path,
			"format":      info.Format,
		})
		if s.opts.Runtime.Snapshot().Mode != "monitor" {
			s.setEffectiveStatusConfig(cfg)
		}
		writeJSON(w, http.StatusOK, redactedConfigResponse(cfg, info, newRevision, configRestartRequired(s.opts.Runtime, newRevision)))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleClassificationPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request classificationPreviewRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(request.Items) > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "classification preview is limited to 1000 items"})
		return
	}
	classifier, err := classification.Compile(request.Rules)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	results := make([]classification.Result, len(request.Items))
	for index, endpoint := range request.Items {
		results[index] = classifier.Apply(endpoint)
	}
	writeJSON(w, http.StatusOK, classificationPreviewResponse{Results: results})
}

func (s *apiServer) effectiveStatusConfig() (config.Config, bool) {
	if s.opts.EffectiveConfigProvider != nil {
		return s.opts.EffectiveConfigProvider()
	}
	s.statusConfigMu.RLock()
	defer s.statusConfigMu.RUnlock()
	return s.statusConfig, s.hasStatusConfig
}

func (s *apiServer) handleRuntimeRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request runtimeRestartRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !request.Confirmed {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "restart requires explicit confirmation"})
		return
	}
	if s.opts.Runtime.Snapshot().Mode != "monitor" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "collector restart is unavailable in configuration-only mode"})
		return
	}
	if s.opts.RequestRestart == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "controlled restart is unavailable in this runtime"})
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.opts.Runtime.Snapshot().Restarting {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "collector restart is already in progress"})
		return
	}
	configInfo, err := config.ResolveConfigSource(s.opts.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	configRevision, configErr := revision.File(configInfo.Path)
	endpointsRevision, endpointsErr := revision.File(s.opts.EndpointsPath)
	if configErr != nil || endpointsErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not verify deployment revisions"})
		return
	}
	if request.ConfigRevision == "" || request.ConfigRevision != configRevision {
		writeRevisionConflict(w, "config file changed after the restart prompt was opened", configRevision)
		return
	}
	if request.EndpointsRevision == "" || request.EndpointsRevision != endpointsRevision {
		writeRevisionConflict(w, "endpoint file changed after the restart prompt was opened", endpointsRevision)
		return
	}
	runtimeSnapshot := s.opts.Runtime.Snapshot()
	if runtimeSnapshot.EffectiveConfigRevision == configRevision {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the saved configuration is already active"})
		return
	}
	report := advisor.AnalyzeDeployment(r.Context(), s.advisorOptions("current"))
	if !report.Summary.ReadyToRun {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "restart blocked by configuration advisor findings", "report": report})
		return
	}
	queued := RestartRequest{ConfigRevision: configRevision, EndpointsRevision: endpointsRevision}
	s.opts.Runtime.RestartRequested()
	if err := s.opts.RequestRestart(queued); err != nil {
		s.opts.Runtime.RestartFailed(err)
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	diagnostics.LogInfo("web ui requested controlled collector restart", map[string]interface{}{
		"config_revision": configRevision, "endpoints_revision": endpointsRevision,
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "message": "controlled collector restart queued"})
}

func (s *apiServer) setEffectiveStatusConfig(cfg config.Config) {
	s.statusConfigMu.Lock()
	defer s.statusConfigMu.Unlock()
	s.statusConfig = cfg
	s.hasStatusConfig = true
}

func (s *apiServer) advisorOptions(profile string) advisor.AnalyzeOptions {
	return advisor.AnalyzeOptions{
		ConfigPath: s.opts.ConfigPath, EndpointsPath: s.opts.EndpointsPath,
		RootDir: s.opts.RootDir, Profile: profile, ProductVersion: s.opts.Version,
	}
}

func (s *apiServer) handleAdvisor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	report := advisor.AnalyzeDeployment(r.Context(), s.advisorOptions(r.URL.Query().Get("profile")))
	writeJSON(w, http.StatusOK, report)
}

func (s *apiServer) handleAdvisorProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, advisor.Profiles())
}

func (s *apiServer) handleAdvisorApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request advisorApplyRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !request.ApplySafe && !request.ApplyProfile {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "select at least one fix to apply"})
		return
	}
	if _, err := advisor.GetProfile(request.Profile); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	configInfo, err := config.ResolveConfigSource(s.opts.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	configRevision, configErr := revision.File(configInfo.Path)
	endpointsRevision, endpointsErr := revision.File(s.opts.EndpointsPath)
	if configErr != nil || endpointsErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not verify deployment revisions"})
		return
	}
	if request.ConfigRevision == "" || request.ConfigRevision != configRevision {
		writeRevisionConflict(w, "config file changed after advisor analysis", configRevision)
		return
	}
	if request.EndpointsRevision == "" || request.EndpointsRevision != endpointsRevision {
		writeRevisionConflict(w, "endpoint file changed after advisor analysis", endpointsRevision)
		return
	}

	result, err := advisor.Apply(r.Context(), advisor.ApplyOptions{
		AnalyzeOptions: s.advisorOptions(request.Profile),
		ApplySafe:      request.ApplySafe, ApplyProfile: request.ApplyProfile,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if result.ConfigChanged && s.opts.Runtime.Snapshot().Mode != "monitor" && result.Report.LoadedConfig != nil {
		s.setEffectiveStatusConfig(*result.Report.LoadedConfig)
	}
	diagnostics.LogInfo("configuration advisor applied fixes", map[string]interface{}{
		"profile": request.Profile, "fixes": result.AppliedFixes,
		"config_changed": result.ConfigChanged, "endpoints_changed": result.EndpointsChanged,
	})
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) handleAdvisorBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.benchmarkMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an advisor benchmark is already running"})
		return
	}
	defer s.benchmarkMu.Unlock()
	result, err := advisor.BenchmarkDeployment(r.Context(), s.advisorOptions(r.URL.Query().Get("profile")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) handleDiscoveryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request discoveryRunRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.ScheduleID = ""
	if err := normalizeDiscoveryRunRequest(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.runDiscovery(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) handleDiscoveryStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request discoveryRunRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.ScheduleID = ""
	if err := normalizeDiscoveryRunRequest(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	writeEvent := func(event discoveryStreamEvent) error {
		if err := encoder.Encode(event); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := s.streamDiscoveryRun(r.Context(), request, writeEvent); err != nil {
		return
	}
}

func (s *apiServer) handleDiscoveryHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := s.effectiveStatusConfig()
	if !ok {
		var err error
		cfg, _, err = config.LoadEditable(r.Context(), s.opts.ConfigPath, s.opts.RootDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	historyPath := discoveryHistoryPath(cfg, s.opts.ConfigPath)
	index, err := loadDiscoveryHistoryIndex(historyPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if scanID := strings.TrimSpace(r.URL.Query().Get("scan_id")); scanID != "" {
		snapshot, err := loadDiscoverySnapshot(historyPath, index, scanID)
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "discovery scan was not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		reconciled, reconcileErr := s.reconcileDiscoveryEndpoints(snapshot.Items)
		if reconcileErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": reconcileErr.Error()})
			return
		}
		snapshot.Items = reconciled
		detail := discoveryHistoryDetail{discoverySnapshot: snapshot}
		if snapshot.Delta.PreviousScanID == "" {
			detail.BaselineAvailable = true
			detail.NewItems, detail.MissingItems, detail.UnresolvedDynamicItems = calculateDiscoveryItemChanges(nil, snapshot.Items)
		} else if previous, previousErr := loadDiscoverySnapshot(historyPath, index, snapshot.Delta.PreviousScanID); previousErr == nil {
			if previous.Items, previousErr = s.reconcileDiscoveryEndpoints(previous.Items); previousErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": previousErr.Error()})
				return
			}
			detail.BaselineAvailable = true
			detail.NewItems, detail.MissingItems, detail.UnresolvedDynamicItems = calculateDiscoveryItemChanges(previous.Items, snapshot.Items)
		}
		writeJSON(w, http.StatusOK, detail)
		return
	}

	limit := 50
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if parsed, parseErr := strconv.Atoi(rawLimit); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}
	scans := append([]discoveryScanSummary(nil), index.Scans...)
	total := len(scans)
	if len(scans) > limit {
		scans = scans[:limit]
	}
	for index := range scans {
		scans[index].FileName = ""
	}
	writeJSON(w, http.StatusOK, discoveryHistoryResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		HistoryPath: historyPath, RetentionScans: cfg.Discovery.RetentionScans,
		RetentionDays: cfg.Discovery.RetentionDays, TotalScans: total, Scans: scans,
		Schedules: discoveryScheduleStatuses(cfg, historyPath, time.Now()),
	})
}

func (s *apiServer) handleOutputTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request outputTestRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	stored, _, err := config.LoadEditable(r.Context(), s.opts.ConfigPath, s.opts.RootDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	mergeWriteOnlySecrets(&request.Config, stored, false, false)

	var (
		result   outputTestResponse
		probeErr error
	)
	switch strings.ToLower(strings.TrimSpace(request.Target)) {
	case "hec":
		result, probeErr = probeHECOutput(r.Context(), request.Config)
	case "metrics":
		result, probeErr = probeMetricsOutput(r.Context(), request.Config)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target must be hec or metrics"})
		return
	}
	if probeErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": probeErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func redactedConfigResponse(cfg config.Config, info config.SourceInfo, currentRevision string, restartRequired bool) configResponse {
	secrets := secretStatus{
		HECTokenConfigured:     strings.TrimSpace(cfg.HEC.Token) != "",
		MetricsTokenConfigured: strings.TrimSpace(cfg.Metrics.Token) != "",
	}
	cfg.HEC.Token = ""
	cfg.Metrics.Token = ""
	return configResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), ConfigPath: info.Path,
		ConfigFormat: info.Format, Config: cfg, Secrets: secrets,
		Revision: currentRevision, RestartRequired: restartRequired,
	}
}

func configRestartRequired(tracker *runtimeinfo.Tracker, currentRevision string) bool {
	snapshot := tracker.Snapshot()
	return snapshot.Mode == "monitor" && snapshot.EffectiveConfigRevision != "" && currentRevision != snapshot.EffectiveConfigRevision
}

func writeRevisionConflict(w http.ResponseWriter, message string, currentRevision string) {
	writeJSON(w, http.StatusConflict, map[string]string{
		"error":            message + "; reload from disk before saving",
		"current_revision": currentRevision,
	})
}

func mergeWriteOnlySecrets(candidate *config.Config, existing config.Config, clearHEC bool, clearMetrics bool) {
	if clearHEC {
		candidate.HEC.Token = ""
	} else if strings.TrimSpace(candidate.HEC.Token) == "" {
		candidate.HEC.Token = existing.HEC.Token
	}
	if clearMetrics {
		candidate.Metrics.Token = ""
	} else if strings.TrimSpace(candidate.Metrics.Token) == "" {
		candidate.Metrics.Token = existing.Metrics.Token
	}
}

func (s *apiServer) runDiscovery(ctx context.Context, request discoveryRunRequest) (discoveryResponse, error) {
	if !s.discoveryMu.TryLock() {
		return discoveryResponse{}, errors.New("another discovery run is already active")
	}
	defer s.discoveryMu.Unlock()
	cmd, tempPath, cleanup, err := s.prepareDiscoveryCommand(ctx, request)
	if err != nil {
		return discoveryResponse{}, err
	}
	defer cleanup()
	start := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return discoveryResponse{}, fmt.Errorf("discovery failed: %w\n%s", err, sanitizeDiscoveryLog(string(out)))
	}
	endpoints, err := config.LoadEditableEndpoints(tempPath)
	if err != nil {
		return discoveryResponse{}, err
	}
	endpoints, err = s.enrichDiscoveryEndpoints(discoveryTargetLabel(request), endpoints)
	if err != nil {
		return discoveryResponse{}, err
	}
	response := discoveryResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ScanID:      discoveryScanID(endpoints),
		Target:      discoveryTargetLabel(request),
		Summary:     summarizeEndpoints(endpoints),
		Items:       endpoints,
		Logs:        sanitizeDiscoveryLog(string(out)),
		DurationMs:  time.Since(start).Milliseconds(),
	}
	if err := s.persistDiscoverySnapshot(request, &response); err != nil {
		return discoveryResponse{}, fmt.Errorf("persist discovery history: %w", err)
	}
	if err := s.emitDiscoveryEvents(ctx, request, response); err != nil {
		return discoveryResponse{}, fmt.Errorf("discovery completed and history was retained locally, but configured event output could not accept the evidence: %w", err)
	}
	return response, nil
}

func (s *apiServer) streamDiscoveryRun(ctx context.Context, request discoveryRunRequest, emit func(discoveryStreamEvent) error) error {
	if !s.discoveryMu.TryLock() {
		err := errors.New("another discovery run is already active")
		_ = emit(discoveryStreamEvent{Type: "error", Error: err.Error()})
		return err
	}
	defer s.discoveryMu.Unlock()
	cmd, tempPath, cleanup, err := s.prepareDiscoveryCommand(ctx, request)
	if err != nil {
		_ = emit(discoveryStreamEvent{Type: "error", Error: err.Error()})
		return err
	}
	defer cleanup()

	progress := newDiscoveryProgressState(request)
	if err := emit(discoveryStreamEvent{Type: "started", SummaryText: progress.SummaryText}); err != nil {
		return err
	}

	pipeReader, pipeWriter := io.Pipe()
	cmd.Stdout = pipeWriter
	cmd.Stderr = pipeWriter
	start := time.Now()

	scanDone := make(chan discoveryScanResult, 1)
	go func() {
		var builder strings.Builder
		scanner := bufio.NewScanner(pipeReader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		current := progress
		for scanner.Scan() {
			line := sanitizeDiscoveryLine(scanner.Text())
			if line == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(line)
			updateDiscoveryProgressState(&current, line)
			if err := emit(discoveryStreamEvent{Type: "progress", SummaryText: current.SummaryText, LogLine: line}); err != nil {
				scanDone <- discoveryScanResult{Logs: strings.TrimSpace(builder.String()), Progress: current, Err: err}
				return
			}
		}
		err := scanner.Err()
		if errors.Is(err, io.ErrClosedPipe) {
			err = nil
		}
		scanDone <- discoveryScanResult{Logs: strings.TrimSpace(builder.String()), Progress: current, Err: err}
	}()

	if err := cmd.Start(); err != nil {
		_ = pipeWriter.Close()
		result := <-scanDone
		streamErr := fmt.Errorf("discovery start failed: %w", err)
		_ = emit(discoveryStreamEvent{Type: "error", SummaryText: result.Progress.SummaryText, Logs: result.Logs, Error: streamErr.Error()})
		return streamErr
	}

	waitErr := cmd.Wait()
	_ = pipeWriter.Close()
	result := <-scanDone
	_ = pipeReader.Close()
	if result.Err != nil {
		return result.Err
	}
	if waitErr != nil {
		streamErr := fmt.Errorf("discovery failed: %w", waitErr)
		_ = emit(discoveryStreamEvent{Type: "error", SummaryText: result.Progress.SummaryText, Logs: result.Logs, Error: streamErr.Error()})
		return streamErr
	}

	endpoints, err := config.LoadEditableEndpoints(tempPath)
	if err != nil {
		streamErr := fmt.Errorf("discovery results load failed: %w", err)
		_ = emit(discoveryStreamEvent{Type: "error", SummaryText: result.Progress.SummaryText, Logs: result.Logs, Error: streamErr.Error()})
		return streamErr
	}
	endpoints, err = s.enrichDiscoveryEndpoints(discoveryTargetLabel(request), endpoints)
	if err != nil {
		streamErr := fmt.Errorf("discovery classification failed: %w", err)
		_ = emit(discoveryStreamEvent{Type: "error", SummaryText: result.Progress.SummaryText, Logs: result.Logs, Error: streamErr.Error()})
		return streamErr
	}
	summary := summarizeEndpoints(endpoints)
	response := discoveryResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ScanID:      discoveryScanID(endpoints),
		Target:      discoveryTargetLabel(request),
		Summary:     summary,
		Items:       endpoints,
		Logs:        result.Logs,
		DurationMs:  time.Since(start).Milliseconds(),
	}
	if err := s.persistDiscoverySnapshot(request, &response); err != nil {
		streamErr := fmt.Errorf("persist discovery history: %w", err)
		_ = emit(discoveryStreamEvent{Type: "error", SummaryText: result.Progress.SummaryText, Logs: result.Logs, Error: streamErr.Error()})
		return streamErr
	}
	if err := s.emitDiscoveryEvents(ctx, request, response); err != nil {
		streamErr := fmt.Errorf("discovery completed and history was retained locally, but configured event output could not accept the evidence: %w", err)
		_ = emit(discoveryStreamEvent{Type: "error", SummaryText: result.Progress.SummaryText, Logs: result.Logs, Error: streamErr.Error()})
		return streamErr
	}
	return emit(discoveryStreamEvent{
		Type:        "complete",
		GeneratedAt: response.GeneratedAt,
		ScanID:      response.ScanID,
		Target:      response.Target,
		SummaryText: fmt.Sprintf("Discovery completed with %d endpoint%s.", len(endpoints), pluralSuffix(len(endpoints))),
		Summary:     &summary,
		Delta:       &response.Delta,
		Items:       endpoints,
		Logs:        result.Logs,
		DurationMs:  response.DurationMs,
	})
}

func (s *apiServer) prepareDiscoveryCommand(ctx context.Context, request discoveryRunRequest) (*exec.Cmd, string, func(), error) {
	if !discoveryScriptAvailable(s.opts.DiscoveryScriptPath) {
		return nil, "", nil, fmt.Errorf("discovery script not found: %s", s.opts.DiscoveryScriptPath)
	}
	scriptPath, cleanupScript, err := materializeDiscoveryScript(s.opts.DiscoveryScriptPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("discovery script preparation failed: %w", err)
	}
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		cleanupScript()
		return nil, "", nil, fmt.Errorf("pwsh not available for discovery: %w", err)
	}
	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("pingmonitor_discovery_%d.csv", time.Now().UnixNano()))
	args := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-OutputPath", tempPath,
		"-SubnetMask", strconv.Itoa(request.SubnetMask),
		"-Timeout", strconv.Itoa(request.TimeoutMs),
		"-ThrottleLimit", strconv.Itoa(request.ThrottleLimit),
	}
	if request.TargetNetwork != "" {
		args = append(args, "-TargetNetwork", request.TargetNetwork)
	}
	cleanup := func() {
		_ = os.Remove(tempPath)
		cleanupScript()
	}
	return exec.CommandContext(ctx, pwsh, args...), tempPath, cleanup, nil
}

func discoveryScanID(endpoints []models.Endpoint) string {
	for _, endpoint := range endpoints {
		if value := strings.TrimSpace(endpoint.DiscoveryScanID); value != "" {
			return value
		}
	}
	return fmt.Sprintf("scan-%d", time.Now().UTC().UnixNano())
}

func discoveryTargetLabel(request discoveryRunRequest) string {
	if request.TargetNetwork == "" {
		return fmt.Sprintf("local/%d", request.SubnetMask)
	}
	return fmt.Sprintf("%s/%d", request.TargetNetwork, request.SubnetMask)
}

func discoveryHistoryPath(cfg config.Config, configPath string) string {
	historyPath := strings.TrimSpace(cfg.Discovery.HistoryPath)
	if historyPath == "" {
		historyPath = filepath.Join("data", "discovery")
	}
	if !filepath.IsAbs(historyPath) {
		historyPath = filepath.Join(filepath.Dir(configPath), historyPath)
	}
	return filepath.Clean(historyPath)
}

func (s *apiServer) persistDiscoverySnapshot(request discoveryRunRequest, response *discoveryResponse) error {
	cfg, ok := s.effectiveStatusConfig()
	if !ok {
		cfg = config.Defaults(filepath.Dir(s.opts.ConfigPath))
	}
	historyPath := discoveryHistoryPath(cfg, s.opts.ConfigPath)
	subnet, _ := discoverySubnetForTarget(cfg, response.Target)
	scansPath := filepath.Join(historyPath, "scans")
	if err := os.MkdirAll(scansPath, 0o755); err != nil {
		return err
	}

	targetHashBytes := sha256.Sum256([]byte(strings.ToLower(response.Target)))
	targetKey := hex.EncodeToString(targetHashBytes[:8])
	targetLatestPath := filepath.Join(historyPath, "latest_"+targetKey+".json")
	var previous discoverySnapshot
	if data, err := os.ReadFile(targetLatestPath); err == nil {
		_ = json.Unmarshal(data, &previous)
	}
	if subnet.AddressingMode == "dhcp" {
		for index := range previous.Items {
			previous.Items[index].DynamicAddress = true
		}
	}
	response.Delta = calculateDiscoveryDelta(previous, response.Items)
	response.changes.NewItems, response.changes.MissingItems, response.changes.UnresolvedDynamicItems = calculateDiscoveryItemChanges(previous.Items, response.Items)

	snapshot := discoverySnapshot{
		SchemaVersion:  1,
		ScanID:         response.ScanID,
		GeneratedAt:    response.GeneratedAt,
		Target:         response.Target,
		SubnetID:       subnet.ID,
		SubnetName:     subnet.Name,
		SubnetVLAN:     subnet.VLAN,
		SubnetLocation: subnet.Location,
		AddressingMode: subnet.AddressingMode,
		RoutingDomain:  subnet.RoutingDomain,
		Request:        request,
		Summary:        response.Summary,
		Delta:          response.Delta,
		Items:          response.Items,
		DurationMs:     response.DurationMs,
	}
	stamp := strings.NewReplacer("-", "", ":", "").Replace(response.GeneratedAt)
	stamp = strings.TrimSuffix(stamp, "Z")
	scanPath := filepath.Join(scansPath, fmt.Sprintf("%s_%s.json", stamp, sanitizeFileComponent(response.ScanID)))
	if err := writeJSONAtomic(scanPath, snapshot); err != nil {
		return err
	}
	if err := writeJSONAtomic(targetLatestPath, snapshot); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(historyPath, "latest.json"), snapshot); err != nil {
		return err
	}
	index, err := loadDiscoveryHistoryIndex(historyPath)
	if err != nil {
		return err
	}
	current := discoveryScanSummaryFromSnapshot(snapshot, filepath.Base(scanPath))
	replaced := false
	for position := range index.Scans {
		if index.Scans[position].ScanID == current.ScanID {
			index.Scans[position] = current
			replaced = true
			break
		}
	}
	if !replaced {
		index.Scans = append(index.Scans, current)
	}
	sortDiscoveryScanSummaries(index.Scans)
	if err := pruneDiscoveryHistory(historyPath, &index, cfg.Discovery.RetentionScans, cfg.Discovery.RetentionDays, time.Now()); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(historyPath, "history_index.json"), index)
}

func (s *apiServer) emitDiscoveryEvents(ctx context.Context, request discoveryRunRequest, response discoveryResponse) error {
	if s.opts.EmitDiscoveryEvents == nil {
		return nil
	}
	cycleID := "discovery-" + response.ScanID
	baselineAvailable := response.Delta.PreviousScanID != ""
	cfg, _ := s.effectiveStatusConfig()
	subnet, _ := discoverySubnetForTarget(cfg, response.Target)
	events := make([]json.RawMessage, 0, len(response.Items)+len(response.changes.MissingItems)+1)
	summary := models.DiscoveryScanEvent{
		SchemaVersion: models.SchemaVersion,
		EventID: stableDiscoveryEventID(
			s.opts.CollectorID, response.ScanID, response.Target, "scan_summary",
		),
		CollectorID:       s.opts.CollectorID,
		CollectorHost:     s.opts.CollectorHost,
		CycleID:           cycleID,
		Timestamp:         response.GeneratedAt,
		RecordType:        "discovery_scan_summary",
		EvidenceKind:      "icmp_subnet_discovery",
		ScanID:            response.ScanID,
		PreviousScanID:    response.Delta.PreviousScanID,
		ScheduleID:        request.ScheduleID,
		TargetNetwork:     response.Target,
		SubnetID:          subnet.ID,
		SubnetName:        subnet.Name,
		SubnetVLAN:        subnet.VLAN,
		SubnetLocation:    subnet.Location,
		AddressingMode:    subnet.AddressingMode,
		RoutingDomain:     subnet.RoutingDomain,
		BaselineAvailable: baselineAvailable,
		EndpointsObserved: len(response.Items),
		NewEndpoints:      response.Delta.New,
		MissingEndpoints:  response.Delta.Missing,
		Unchanged:         response.Delta.Unchanged,
		UnresolvedDynamic: response.Delta.UnresolvedDynamic,
		ScanDurationMs:    response.DurationMs,
		TimeoutMs:         request.TimeoutMs,
		ThrottleLimit:     request.ThrottleLimit,
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	events = append(events, encoded)

	newByIdentity := make(map[string]struct{}, len(response.changes.NewItems))
	for _, endpoint := range response.changes.NewItems {
		if key, ok := discoveryIdentityKey(endpoint); ok {
			newByIdentity[key] = struct{}{}
		}
	}
	unresolvedByIP := make(map[string]struct{}, len(response.changes.UnresolvedDynamicItems))
	for _, endpoint := range response.changes.UnresolvedDynamicItems {
		unresolvedByIP[normalizedDiscoveryIP(endpoint.IP)] = struct{}{}
	}
	for _, endpoint := range response.Items {
		deltaStatus := "unchanged"
		if _, unresolved := unresolvedByIP[normalizedDiscoveryIP(endpoint.IP)]; unresolved {
			deltaStatus = "unresolved_dynamic"
		} else if key, ok := discoveryIdentityKey(endpoint); ok {
			if _, isNew := newByIdentity[key]; isNew {
				deltaStatus = "new"
			}
		} else {
			deltaStatus = "unresolved_dynamic"
		}
		event, buildErr := s.discoveryEndpointEvent(request, response, endpoint, deltaStatus, true, baselineAvailable)
		if buildErr != nil {
			return buildErr
		}
		events = append(events, event)
	}
	for _, endpoint := range response.changes.MissingItems {
		event, buildErr := s.discoveryEndpointEvent(request, response, endpoint, "missing", false, baselineAvailable)
		if buildErr != nil {
			return buildErr
		}
		events = append(events, event)
	}
	return s.opts.EmitDiscoveryEvents(ctx, cycleID, events)
}

func (s *apiServer) discoveryEndpointEvent(
	request discoveryRunRequest,
	response discoveryResponse,
	endpoint models.Endpoint,
	deltaStatus string,
	observed bool,
	baselineAvailable bool,
) (json.RawMessage, error) {
	cfg, _ := s.effectiveStatusConfig()
	subnet, _ := discoverySubnetForTarget(cfg, response.Target)
	discoveryStatus := "observed"
	if !observed {
		discoveryStatus = "not_observed"
	}
	source := strings.TrimSpace(endpoint.DiscoverySource)
	if source == "" {
		source = "icmp_subnet_scan"
	}
	event := models.DiscoveryEvent{
		SchemaVersion:        models.SchemaVersion,
		EventID:              stableDiscoveryEventID(s.opts.CollectorID, response.ScanID, endpoint.IP, deltaStatus),
		CollectorID:          s.opts.CollectorID,
		CollectorHost:        s.opts.CollectorHost,
		CycleID:              "discovery-" + response.ScanID,
		Timestamp:            response.GeneratedAt,
		RecordType:           "discovery_observation",
		EvidenceKind:         "icmp_subnet_discovery",
		ScanID:               response.ScanID,
		PreviousScanID:       response.Delta.PreviousScanID,
		ScheduleID:           request.ScheduleID,
		TargetNetwork:        response.Target,
		SubnetID:             subnet.ID,
		SubnetName:           subnet.Name,
		SubnetVLAN:           subnet.VLAN,
		SubnetLocation:       subnet.Location,
		AddressingMode:       subnet.AddressingMode,
		RoutingDomain:        subnet.RoutingDomain,
		TargetIP:             endpoint.IP,
		EndpointID:           models.StableEndpointID(endpoint.EndpointID, endpoint.IP),
		Hostname:             endpoint.Hostname,
		FQDN:                 endpoint.FQDN,
		Dev:                  endpoint.Dev,
		DeviceMode:           endpoint.EffectiveDeviceMode(),
		MonitoringEnabled:    endpoint.IsMonitoringEnabled(),
		AlertingEnabled:      endpoint.IsAlertingEnabled(),
		AlertingReason:       endpoint.AlertingReason,
		MaintenanceUntil:     endpoint.MaintenanceUntil,
		MaintenanceReason:    endpoint.MaintenanceReason,
		AssetID:              endpoint.AssetID,
		DynamicAddress:       endpoint.DynamicAddress,
		ClassificationSource: endpoint.ClassificationSource,
		DiscoveryReviewState: endpoint.DiscoveryReviewState,
		DiscoveryReviewedAt:  endpoint.DiscoveryReviewedAt,
		DiscoveryReviewNote:  endpoint.DiscoveryReviewNote,
		Group:                endpoint.Group,
		Description:          endpoint.Description,
		EntityType:           endpoint.EntityType,
		Device:               endpoint.Device,
		Vendor:               endpoint.Vendor,
		Notes:                endpoint.AdditionalNotes,
		DNSStatus:            endpoint.DNSStatus,
		DNSForwardConfirmed:  endpoint.DNSForwardConfirmed,
		DiscoveredAt:         endpoint.DiscoveredAt,
		DiscoverySource:      source,
		DiscoveryLatencyMs:   endpoint.DiscoveryLatencyMs,
		DiscoveryStatus:      discoveryStatus,
		DiscoveryDelta:       deltaStatus,
		DiscoveryObserved:    observed,
		BaselineAvailable:    baselineAvailable,
		ScanDurationMs:       response.DurationMs,
	}
	return json.Marshal(event)
}

func stableDiscoveryEventID(parts ...string) string {
	normalized := make([]string, len(parts))
	for index, part := range parts {
		normalized[index] = strings.ToLower(strings.TrimSpace(part))
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "|")))
	return "discovery_" + hex.EncodeToString(sum[:16])
}

func normalizedDiscoveryIP(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func calculateDiscoveryDelta(previous discoverySnapshot, current []models.Endpoint) discoveryDelta {
	delta := discoveryDelta{PreviousScanID: previous.ScanID}
	statuses, matchedPrevious, identifiablePrevious := matchDiscoveryItems(previous.Items, current)
	for _, status := range statuses {
		switch status {
		case "unchanged":
			delta.Unchanged++
		case "new":
			delta.New++
		case "unresolved_dynamic":
			delta.UnresolvedDynamic++
		}
	}
	for index, identifiable := range identifiablePrevious {
		if identifiable && !matchedPrevious[index] {
			delta.Missing++
		}
	}
	return delta
}

func calculateDiscoveryItemChanges(previous []models.Endpoint, current []models.Endpoint) ([]models.Endpoint, []models.Endpoint, []models.Endpoint) {
	statuses, matchedPrevious, identifiablePrevious := matchDiscoveryItems(previous, current)
	newItems := make([]models.Endpoint, 0)
	missingItems := make([]models.Endpoint, 0)
	unresolvedItems := make([]models.Endpoint, 0)
	for index, endpoint := range current {
		switch statuses[index] {
		case "unresolved_dynamic":
			unresolvedItems = append(unresolvedItems, endpoint)
		case "new":
			newItems = append(newItems, endpoint)
		}
	}
	for index, endpoint := range previous {
		if identifiablePrevious[index] && !matchedPrevious[index] {
			missingItems = append(missingItems, endpoint)
		}
	}
	return newItems, missingItems, unresolvedItems
}

func matchDiscoveryItems(previous, current []models.Endpoint) ([]string, map[int]bool, []bool) {
	previousAliases, previousCounts := discoveryStableAliasSets(previous)
	currentAliases, currentCounts := discoveryStableAliasSets(current)
	previousByAlias := make(map[string]int)
	identifiablePrevious := make([]bool, len(previous))
	for index, aliases := range previousAliases {
		for _, alias := range aliases {
			if previousCounts[alias] == 1 {
				previousByAlias[alias] = index
				identifiablePrevious[index] = true
			}
		}
	}

	statuses := make([]string, len(current))
	matchedPrevious := make(map[int]bool)
	for index, endpoint := range current {
		matches := make(map[int]struct{})
		identifiable := false
		for _, alias := range currentAliases[index] {
			if currentCounts[alias] != 1 {
				continue
			}
			identifiable = true
			if previousIndex, exists := previousByAlias[alias]; exists {
				matches[previousIndex] = struct{}{}
			}
		}
		if !identifiable || len(matches) > 1 {
			if endpoint.DynamicAddress {
				statuses[index] = "unresolved_dynamic"
			} else {
				statuses[index] = "new"
			}
			continue
		}
		if len(matches) == 0 {
			statuses[index] = "new"
			continue
		}
		for previousIndex := range matches {
			statuses[index] = "unchanged"
			matchedPrevious[previousIndex] = true
		}
	}
	return statuses, matchedPrevious, identifiablePrevious
}

func discoveryStableAliasSets(items []models.Endpoint) ([][]string, map[string]int) {
	sets := make([][]string, len(items))
	counts := make(map[string]int)
	for index, endpoint := range items {
		sets[index] = stableDiscoveryAliases(endpoint)
		for _, alias := range sets[index] {
			counts[alias]++
		}
	}
	return sets, counts
}

func discoveryIdentityKey(endpoint models.Endpoint) (string, bool) {
	aliases := stableDiscoveryAliases(endpoint)
	if len(aliases) > 0 {
		return aliases[0], true
	}
	return "", false
}

func discoveryScanSummaryFromSnapshot(snapshot discoverySnapshot, fileName string) discoveryScanSummary {
	return discoveryScanSummary{
		ScanID: snapshot.ScanID, GeneratedAt: snapshot.GeneratedAt, Target: snapshot.Target,
		SubnetID: snapshot.SubnetID, SubnetName: snapshot.SubnetName, SubnetVLAN: snapshot.SubnetVLAN,
		SubnetLocation: snapshot.SubnetLocation, AddressingMode: snapshot.AddressingMode,
		RoutingDomain: snapshot.RoutingDomain,
		ScheduleID:    snapshot.Request.ScheduleID, Summary: snapshot.Summary, Delta: snapshot.Delta,
		DurationMs: snapshot.DurationMs, FileName: fileName,
	}
}

func (s *apiServer) enrichDiscoveryEndpoints(target string, endpoints []models.Endpoint) ([]models.Endpoint, error) {
	cfg, ok := s.effectiveStatusConfig()
	if !ok {
		cfg = config.Defaults(filepath.Dir(s.opts.ConfigPath))
	}
	classifier, err := classification.Compile(cfg.Classification.Rules)
	if err != nil {
		return nil, fmt.Errorf("classification rules: %w", err)
	}
	subnet, hasSubnet := discoverySubnetForTarget(cfg, target)
	enriched := make([]models.Endpoint, len(endpoints))
	for index, endpoint := range endpoints {
		if hasSubnet {
			endpoint.SubnetID = subnet.ID
			endpoint.SubnetName = subnet.Name
			endpoint.SubnetVLAN = subnet.VLAN
			endpoint.SubnetLocation = subnet.Location
			endpoint.AddressingMode = subnet.AddressingMode
			endpoint.RoutingDomain = subnet.RoutingDomain
			if subnet.AddressingMode == "dhcp" {
				endpoint.DynamicAddress = true
			}
		}
		enriched[index] = classifier.Apply(endpoint).Endpoint
	}
	return s.reconcileDiscoveryEndpoints(enriched)
}

func discoverySubnetForTarget(cfg config.Config, target string) (config.DiscoverySubnet, bool) {
	_, targetNetwork, err := net.ParseCIDR(strings.TrimSpace(target))
	if err != nil {
		return config.DiscoverySubnet{}, false
	}
	targetBits, targetSize := targetNetwork.Mask.Size()
	bestBits := -1
	var best config.DiscoverySubnet
	for _, subnet := range cfg.Discovery.Subnets {
		_, network, parseErr := net.ParseCIDR(strings.TrimSpace(subnet.CIDR))
		if parseErr != nil {
			continue
		}
		bits, size := network.Mask.Size()
		if size != targetSize || bits > targetBits || !network.Contains(targetNetwork.IP) {
			continue
		}
		if bits > bestBits {
			best = subnet
			bestBits = bits
		}
	}
	return best, bestBits >= 0
}

func normalizedCIDR(value string) string {
	trimmed := strings.TrimSpace(value)
	_, network, err := net.ParseCIDR(trimmed)
	if err != nil {
		return strings.ToLower(trimmed)
	}
	return strings.ToLower(network.String())
}

func sortDiscoveryScanSummaries(scans []discoveryScanSummary) {
	sort.SliceStable(scans, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, scans[i].GeneratedAt)
		right, rightErr := time.Parse(time.RFC3339, scans[j].GeneratedAt)
		if leftErr == nil && rightErr == nil {
			return left.After(right)
		}
		return scans[i].GeneratedAt > scans[j].GeneratedAt
	})
}

func loadDiscoveryHistoryIndex(historyPath string) (discoveryHistoryIndex, error) {
	index := discoveryHistoryIndex{SchemaVersion: 1, Scans: []discoveryScanSummary{}}
	indexPath := filepath.Join(historyPath, "history_index.json")
	if data, err := os.ReadFile(indexPath); err == nil {
		if json.Unmarshal(data, &index) == nil && index.SchemaVersion == 1 {
			if index.Scans == nil {
				index.Scans = []discoveryScanSummary{}
			}
			sortDiscoveryScanSummaries(index.Scans)
			return index, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return index, err
	}

	scansPath := filepath.Join(historyPath, "scans")
	entries, err := os.ReadDir(scansPath)
	if errors.Is(err, os.ErrNotExist) {
		return index, nil
	}
	if err != nil {
		return index, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(scansPath, entry.Name()))
		if readErr != nil {
			return index, readErr
		}
		var snapshot discoverySnapshot
		if unmarshalErr := json.Unmarshal(data, &snapshot); unmarshalErr != nil {
			return index, fmt.Errorf("read discovery history %s: %w", entry.Name(), unmarshalErr)
		}
		index.Scans = append(index.Scans, discoveryScanSummaryFromSnapshot(snapshot, entry.Name()))
	}
	sortDiscoveryScanSummaries(index.Scans)
	return index, nil
}

func loadDiscoverySnapshot(historyPath string, index discoveryHistoryIndex, scanID string) (discoverySnapshot, error) {
	for _, summary := range index.Scans {
		if summary.ScanID != scanID || summary.FileName == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(historyPath, "scans", filepath.Base(summary.FileName)))
		if err != nil {
			return discoverySnapshot{}, err
		}
		var snapshot discoverySnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return discoverySnapshot{}, err
		}
		if snapshot.ScanID == scanID {
			return snapshot, nil
		}
	}
	entries, err := os.ReadDir(filepath.Join(historyPath, "scans"))
	if err != nil {
		return discoverySnapshot{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(historyPath, "scans", entry.Name()))
		if readErr != nil {
			return discoverySnapshot{}, readErr
		}
		var snapshot discoverySnapshot
		if json.Unmarshal(data, &snapshot) == nil && snapshot.ScanID == scanID {
			return snapshot, nil
		}
	}
	return discoverySnapshot{}, os.ErrNotExist
}

func pruneDiscoveryHistory(historyPath string, index *discoveryHistoryIndex, retentionScans int, retentionDays int, now time.Time) error {
	if retentionScans <= 0 && retentionDays <= 0 {
		return nil
	}
	sortDiscoveryScanSummaries(index.Scans)
	cutoff := time.Time{}
	if retentionDays > 0 {
		cutoff = now.AddDate(0, 0, -retentionDays)
	}
	kept := make([]discoveryScanSummary, 0, len(index.Scans))
	for position, summary := range index.Scans {
		remove := retentionScans > 0 && position >= retentionScans
		if !remove && !cutoff.IsZero() {
			if generatedAt, err := time.Parse(time.RFC3339, summary.GeneratedAt); err == nil && generatedAt.Before(cutoff) {
				remove = true
			}
		}
		if !remove {
			kept = append(kept, summary)
			continue
		}
		if summary.FileName != "" {
			if err := os.Remove(filepath.Join(historyPath, "scans", filepath.Base(summary.FileName))); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	index.Scans = kept
	return nil
}

func sanitizeFileComponent(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "scan"
	}
	return builder.String()
}

func writeJSONAtomic(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return os.Rename(tempPath, path)
}

type discoveryScheduleRunState struct {
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

func (s *apiServer) runDiscoveryScheduler(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	states := make(map[string]discoveryScheduleRunState)
	for {
		s.evaluateDiscoverySchedules(ctx, states)
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func (s *apiServer) evaluateDiscoverySchedules(ctx context.Context, states map[string]discoveryScheduleRunState) {
	cfg, ok := s.effectiveStatusConfig()
	if !ok || len(cfg.Discovery.Schedules) == 0 || !discoveryScriptAvailable(s.opts.DiscoveryScriptPath) {
		return
	}
	historyPath := discoveryHistoryPath(cfg, s.opts.ConfigPath)
	statePath := filepath.Join(historyPath, "schedule_state.json")
	if len(states) == 0 {
		if data, err := os.ReadFile(statePath); err == nil {
			loaded := make(map[string]discoveryScheduleRunState)
			if json.Unmarshal(data, &loaded) == nil {
				for key, state := range loaded {
					states[key] = state
				}
			}
		}
	}
	now := time.Now()
	for _, schedule := range cfg.Discovery.Schedules {
		if !schedule.Enabled || len(schedule.Targets) == 0 {
			continue
		}
		state := states[schedule.ID]
		occurrence, err := previousScheduleOccurrence(schedule, now)
		if err != nil {
			if state.LastError != err.Error() {
				diagnostics.LogWarn("discovery schedule invalid", map[string]interface{}{"schedule_id": schedule.ID, "error": err.Error()})
			}
			state.LastError = err.Error()
			states[schedule.ID] = state
			continue
		}
		if !state.LastRunAt.IsZero() && !state.LastRunAt.Before(occurrence) {
			continue
		}
		if !state.LastAttemptAt.IsZero() && now.Sub(state.LastAttemptAt) < 15*time.Minute {
			continue
		}
		state.LastAttemptAt = now.UTC()
		state.LastError = ""
		states[schedule.ID] = state
		_ = writeJSONAtomic(statePath, states)

		runErr := s.runScheduledDiscovery(ctx, schedule)
		state = states[schedule.ID]
		if runErr != nil {
			state.LastError = runErr.Error()
			diagnostics.LogWarn("scheduled discovery failed", map[string]interface{}{"schedule_id": schedule.ID, "error": runErr.Error()})
		} else {
			state.LastRunAt = time.Now().UTC()
			state.LastError = ""
			diagnostics.LogInfo("scheduled discovery completed", map[string]interface{}{"schedule_id": schedule.ID, "targets": len(schedule.Targets)})
		}
		states[schedule.ID] = state
		_ = writeJSONAtomic(statePath, states)
	}
}

func (s *apiServer) runScheduledDiscovery(ctx context.Context, schedule config.DiscoverySchedule) error {
	for _, target := range schedule.Targets {
		request := discoveryRunRequest{
			TargetNetwork: target,
			TimeoutMs:     schedule.TimeoutMs,
			ThrottleLimit: schedule.Concurrency,
			ScheduleID:    schedule.ID,
		}
		if err := normalizeDiscoveryRunRequest(&request); err != nil {
			return err
		}
		if _, err := s.runDiscovery(ctx, request); err != nil {
			return err
		}
	}
	return nil
}

func discoveryScheduleStatuses(cfg config.Config, historyPath string, now time.Time) []discoveryScheduleStatus {
	states := make(map[string]discoveryScheduleRunState)
	if data, err := os.ReadFile(filepath.Join(historyPath, "schedule_state.json")); err == nil {
		_ = json.Unmarshal(data, &states)
	}
	statuses := make([]discoveryScheduleStatus, 0, len(cfg.Discovery.Schedules))
	for _, schedule := range cfg.Discovery.Schedules {
		state := states[schedule.ID]
		status := discoveryScheduleStatus{
			ID: schedule.ID, Enabled: schedule.Enabled, Targets: append([]string(nil), schedule.Targets...),
			Frequency: schedule.Frequency, Day: schedule.Day, Time: schedule.Time, Timezone: schedule.Timezone,
			LastError: state.LastError,
		}
		if !state.LastRunAt.IsZero() {
			status.LastRunAt = state.LastRunAt.UTC().Format(time.RFC3339)
		}
		if !state.LastAttemptAt.IsZero() {
			status.LastAttemptAt = state.LastAttemptAt.UTC().Format(time.RFC3339)
		}
		if schedule.Enabled {
			if occurrence, err := previousScheduleOccurrence(schedule, now); err == nil {
				status.Due = state.LastRunAt.IsZero() || state.LastRunAt.Before(occurrence)
				next := occurrence.AddDate(0, 0, 7)
				if status.Due {
					next = occurrence
				}
				status.NextRunAt = next.UTC().Format(time.RFC3339)
			} else if status.LastError == "" {
				status.LastError = err.Error()
			}
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func previousScheduleOccurrence(schedule config.DiscoverySchedule, now time.Time) (time.Time, error) {
	if schedule.Frequency != "weekly" {
		return time.Time{}, fmt.Errorf("schedule %q frequency must be weekly", schedule.ID)
	}
	location := time.Local
	if !strings.EqualFold(schedule.Timezone, "local") {
		loaded, err := time.LoadLocation(schedule.Timezone)
		if err != nil {
			return time.Time{}, fmt.Errorf("schedule %q timezone %q is invalid", schedule.ID, schedule.Timezone)
		}
		location = loaded
	}
	weekday, ok := parseWeekday(schedule.Day)
	if !ok {
		return time.Time{}, fmt.Errorf("schedule %q day %q is invalid", schedule.ID, schedule.Day)
	}
	var hour, minute int
	if _, err := fmt.Sscanf(schedule.Time, "%d:%d", &hour, &minute); err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("schedule %q time %q must use 24-hour HH:MM", schedule.ID, schedule.Time)
	}
	localNow := now.In(location)
	daysBack := (int(localNow.Weekday()) - int(weekday) + 7) % 7
	date := localNow.AddDate(0, 0, -daysBack)
	occurrence := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
	if occurrence.After(localNow) {
		occurrence = occurrence.AddDate(0, 0, -7)
	}
	return occurrence, nil
}

func parseWeekday(value string) (time.Weekday, bool) {
	for day := time.Sunday; day <= time.Saturday; day++ {
		if strings.EqualFold(value, day.String()) {
			return day, true
		}
	}
	return time.Sunday, false
}

func normalizeDiscoveryRunRequest(request *discoveryRunRequest) error {
	if request.SubnetMask == 0 {
		request.SubnetMask = 24
	}
	if request.TimeoutMs == 0 {
		request.TimeoutMs = 500
	}
	if request.ThrottleLimit == 0 {
		request.ThrottleLimit = 50
	}
	request.TargetNetwork = strings.TrimSpace(request.TargetNetwork)
	normalizedTarget, normalizedMask, err := normalizeDiscoveryTarget(request.TargetNetwork, request.SubnetMask)
	if err != nil {
		return err
	}
	request.TargetNetwork = normalizedTarget
	request.SubnetMask = normalizedMask
	if request.SubnetMask < 16 || request.SubnetMask > 30 {
		return errors.New("subnet_mask must be between 16 and 30")
	}
	return nil
}

func newDiscoveryProgressState(request discoveryRunRequest) discoveryProgressState {
	target := "local subnet"
	if request.TargetNetwork != "" {
		target = fmt.Sprintf("%s/%d", request.TargetNetwork, request.SubnetMask)
	}
	return discoveryProgressState{
		Target:      target,
		Stage:       "Preparing discovery",
		SummaryText: fmt.Sprintf("Preparing discovery for %s.", target),
	}
}

func updateDiscoveryProgressState(state *discoveryProgressState, line string) {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "Using explicit discovery target:"):
		state.Target = strings.TrimSpace(strings.TrimPrefix(trimmed, "Using explicit discovery target:"))
		state.Stage = "Preparing discovery"
	case strings.HasPrefix(trimmed, "Calculating subnet range"):
		state.Stage = "Calculating subnet range"
	case strings.HasPrefix(trimmed, "Hosts to scan:"):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "Hosts to scan:"))); err == nil {
			state.HostsToScan = value
		}
		state.Stage = "Preparing scan"
	case strings.Contains(trimmed, "Scanning network"):
		state.Stage = "Scanning network"
	case strings.HasPrefix(trimmed, "Found "):
		var activeHosts, scannedHosts int
		if _, err := fmt.Sscanf(trimmed, "Found %d active hosts out of %d scanned", &activeHosts, &scannedHosts); err == nil {
			state.ActiveHosts = activeHosts
			state.HostsToScan = scannedHosts
		}
		state.Stage = "Resolving hostnames"
	case strings.Contains(trimmed, "Resolving hostnames and classifying devices"):
		state.Stage = "Resolving hostnames"
	case strings.Contains(trimmed, "Discovery Summary"):
		state.Stage = "Summarizing results"
	case strings.HasPrefix(trimmed, "Exporting to "):
		state.Stage = "Writing results"
	case strings.Contains(trimmed, "Discovery Complete"):
		state.Stage = "Discovery complete"
	}
	state.SummaryText = formatDiscoveryProgressSummary(*state)
}

func formatDiscoveryProgressSummary(state discoveryProgressState) string {
	target := state.Target
	if strings.TrimSpace(target) == "" {
		target = "local subnet"
	}
	switch state.Stage {
	case "Scanning network":
		if state.HostsToScan > 0 {
			return fmt.Sprintf("Scanning %d host%s in %s.", state.HostsToScan, pluralSuffix(state.HostsToScan), target)
		}
		return fmt.Sprintf("Scanning %s.", target)
	case "Resolving hostnames":
		if state.ActiveHosts > 0 && state.HostsToScan > 0 {
			return fmt.Sprintf("Found %d active host%s out of %d scanned. Resolving details.", state.ActiveHosts, pluralSuffix(state.ActiveHosts), state.HostsToScan)
		}
		return "Resolving discovered host details."
	case "Writing results":
		if state.ActiveHosts > 0 {
			return fmt.Sprintf("Writing %d discovered endpoint%s to the staged result set.", state.ActiveHosts, pluralSuffix(state.ActiveHosts))
		}
		return "Writing discovery results."
	case "Discovery complete":
		if state.ActiveHosts > 0 {
			return fmt.Sprintf("Discovery finished with %d active endpoint%s.", state.ActiveHosts, pluralSuffix(state.ActiveHosts))
		}
		return "Discovery finished."
	case "Preparing scan", "Calculating subnet range", "Preparing discovery":
		if state.HostsToScan > 0 {
			return fmt.Sprintf("Preparing discovery for %s across %d host%s.", target, state.HostsToScan, pluralSuffix(state.HostsToScan))
		}
		return fmt.Sprintf("Preparing discovery for %s.", target)
	case "Summarizing results":
		if state.ActiveHosts > 0 {
			return fmt.Sprintf("Summarizing %d discovered endpoint%s.", state.ActiveHosts, pluralSuffix(state.ActiveHosts))
		}
		return "Summarizing discovery results."
	default:
		return fmt.Sprintf("Preparing discovery for %s.", target)
	}
}

func sanitizeDiscoveryLog(raw string) string {
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := sanitizeDiscoveryLine(line)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func sanitizeDiscoveryLine(line string) string {
	cleaned := ansiEscapePattern.ReplaceAllString(line, "")
	return strings.TrimSpace(strings.TrimSuffix(cleaned, "\r"))
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func normalizeDiscoveryTarget(target string, subnetMask int) (string, int, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", subnetMask, nil
	}
	if strings.Contains(trimmed, "/") {
		ip, ipNet, err := net.ParseCIDR(trimmed)
		if err != nil || ip == nil || ip.To4() == nil {
			return "", subnetMask, errors.New("target_network must be a valid IPv4 address or CIDR range")
		}
		ones, _ := ipNet.Mask.Size()
		if ones < 16 || ones > 30 {
			return "", subnetMask, errors.New("target_network CIDR mask must be between /16 and /30")
		}
		return ip.String(), ones, nil
	}
	ip := net.ParseIP(trimmed)
	if ip == nil || ip.To4() == nil {
		return "", subnetMask, errors.New("target_network must be a valid IPv4 address or CIDR range")
	}
	return ip.String(), subnetMask, nil
}

func probeHECOutput(ctx context.Context, cfg config.Config) (outputTestResponse, error) {
	url := strings.TrimSpace(cfg.HEC.URL)
	token := strings.TrimSpace(cfg.HEC.Token)
	if url == "" || token == "" {
		return outputTestResponse{}, errors.New("hec url and token are required to test event delivery")
	}
	warnings := make([]string, 0, 2)
	if !cfg.HEC.Enabled {
		warnings = append(warnings, "HEC is currently disabled in the config; this probe tests connectivity only.")
	}
	if cfg.OutputMode != "hec" && cfg.OutputMode != "both" {
		warnings = append(warnings, "Output mode is not currently set to hec or both.")
	}
	hostname := probeHostname()
	payload := models.HECEvent{
		Time:       float64(time.Now().UTC().UnixNano()) / float64(time.Second),
		Host:       hostname,
		Source:     "ping_monitor_ui",
		SourceType: firstNonEmptyString(cfg.HEC.SourceType, "ping_monitor"),
		Index:      cfg.HEC.Index,
		Event: map[string]interface{}{
			"record_type": "ui_connectivity_probe",
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
			"message":     "Ping Monitor admin UI event HEC connectivity probe",
		},
	}
	return executeOutputProbe(ctx, outputTestResponse{Target: "hec", URL: url, Warnings: warnings}, cfg.HEC.VerifySSL, cfg.HEC.SSLProtocol, url, token, payload)
}

func probeMetricsOutput(ctx context.Context, cfg config.Config) (outputTestResponse, error) {
	url := strings.TrimSpace(cfg.Metrics.HECURL)
	token := strings.TrimSpace(cfg.Metrics.Token)
	if url == "" || token == "" {
		return outputTestResponse{}, errors.New("metrics hec_url and token are required to test metrics delivery")
	}
	warnings := make([]string, 0, 2)
	if !cfg.Metrics.Enabled {
		warnings = append(warnings, "Metrics are currently disabled in the config; this probe tests connectivity only.")
	}
	hostname := probeHostname()
	payload := models.MetricsEvent{
		Time:       float64(time.Now().UTC().UnixNano()) / float64(time.Second),
		Host:       hostname,
		Source:     "ping_monitor_ui",
		SourceType: firstNonEmptyString(cfg.Metrics.SourceType, "ping_monitor:metrics"),
		Index:      cfg.Metrics.Index,
		Event:      "metric",
		Fields: map[string]interface{}{
			"metric_name:ping.ui_probe": float64(1),
			"probe":                     "ping_monitor_ui",
			"target":                    "metrics",
		},
	}
	return executeOutputProbe(ctx, outputTestResponse{Target: "metrics", URL: url, Warnings: warnings}, cfg.Metrics.VerifySSL, cfg.Metrics.SSLProtocol, url, token, payload)
}

func executeOutputProbe(ctx context.Context, base outputTestResponse, verifySSL bool, sslProtocol string, url string, token string, payload interface{}) (outputTestResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return outputTestResponse{}, err
	}
	client := httpcfg.NewClient(verifySSL, sslProtocol, 10*time.Second)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return outputTestResponse{}, err
	}
	req.Header.Set("Authorization", "Splunk "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		base.Success = false
		base.DurationMs = time.Since(start).Milliseconds()
		base.Message = err.Error()
		return base, nil
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	trimmedBody := strings.TrimSpace(string(responseBody))
	base.StatusCode = resp.StatusCode
	base.DurationMs = time.Since(start).Milliseconds()
	base.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	base.ResponseBody = trimmedBody
	if base.Success {
		base.Message = fmt.Sprintf("%s probe succeeded with HTTP %d", strings.ToUpper(base.Target), resp.StatusCode)
	} else {
		base.Message = fmt.Sprintf("%s probe failed with HTTP %d", strings.ToUpper(base.Target), resp.StatusCode)
	}
	if trimmedBody == "" {
		base.ResponseBody = http.StatusText(resp.StatusCode)
	}
	return base, nil
}

func probeHostname() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "ping-monitor-ui"
	}
	return hostname
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func decodeJSONBody(r *http.Request, target interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func summarizeEndpoints(endpoints []models.Endpoint) endpointSummary {
	groups := make(map[string]struct{})
	summary := endpointSummary{Total: len(endpoints)}
	for _, endpoint := range endpoints {
		group := strings.TrimSpace(endpoint.Group)
		if group == "" {
			group = "default"
		}
		groups[group] = struct{}{}
		if endpoint.Dev {
			summary.Dev++
			continue
		}
		summary.Production++
	}
	summary.Groups = len(groups)
	return summary
}

func staticHandler(root fs.FS, version string) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			serveIndex(w, r, version)
			return
		}
		if _, err := fs.Stat(root, cleanPath); err == nil {
			setNoStoreHeaders(w)
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, version)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, version string) {
	b, err := fs.ReadFile(staticFiles, "static/index.html")
	if err != nil {
		http.Error(w, "ui shell unavailable", http.StatusInternalServerError)
		return
	}
	assetVersion := url.QueryEscape(version)
	if assetVersion == "" {
		assetVersion = "development"
	}
	b = bytes.ReplaceAll(b, []byte("{{ASSET_VERSION}}"), []byte(assetVersion))
	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(b))
}

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "json encode failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func sortedGroups(endpoints []models.Endpoint) []string {
	seen := make(map[string]struct{})
	for _, endpoint := range endpoints {
		group := strings.TrimSpace(endpoint.Group)
		if group == "" {
			group = "default"
		}
		seen[group] = struct{}{}
	}
	groups := make([]string, 0, len(seen))
	for group := range seen {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups
}
