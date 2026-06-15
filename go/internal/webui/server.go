package webui

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/httpcfg"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed assets/DiscoverEndpoints.ps1
var embeddedDiscoveryScript []byte

const embeddedDiscoveryScriptPath = "embedded:DiscoverEndpoints.ps1"

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

type Options struct {
	ListenAddr          string
	ConfigPath          string
	EndpointsPath       string
	RootDir             string
	DiscoveryScriptPath string
	Version             string
}

type statusResponse struct {
	Product             string `json:"product"`
	Version             string `json:"version"`
	ConfigPath          string `json:"config_path"`
	ConfigFormat        string `json:"config_format"`
	EndpointsPath       string `json:"endpoints_path"`
	DiscoveryAvailable  bool   `json:"discovery_available"`
	DiscoveryScriptPath string `json:"discovery_script_path,omitempty"`
	Mode                string `json:"mode"`
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
}

type endpointsWriteRequest struct {
	Items []models.Endpoint `json:"items"`
}

type configResponse struct {
	GeneratedAt  string        `json:"generated_at"`
	ConfigPath   string        `json:"config_path"`
	ConfigFormat string        `json:"config_format"`
	Config       config.Config `json:"config"`
}

type configWriteRequest struct {
	Config config.Config `json:"config"`
}

type discoveryRunRequest struct {
	TargetNetwork string `json:"target_network"`
	SubnetMask    int    `json:"subnet_mask"`
	TimeoutMs     int    `json:"timeout_ms"`
	ThrottleLimit int    `json:"throttle_limit"`
}

type discoveryResponse struct {
	GeneratedAt string            `json:"generated_at"`
	Summary     endpointSummary   `json:"summary"`
	Items       []models.Endpoint `json:"items"`
	Logs        string            `json:"logs,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
}

type discoveryStreamEvent struct {
	Type        string            `json:"type"`
	SummaryText string            `json:"summary_text,omitempty"`
	LogLine     string            `json:"log_line,omitempty"`
	GeneratedAt string            `json:"generated_at,omitempty"`
	Summary     *endpointSummary  `json:"summary,omitempty"`
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

type apiServer struct {
	opts Options
}

func Start(ctx context.Context, opts Options) error {
	handler, err := newHandler(opts)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return err
	}

	server := &http.Server{
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
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.LogError("web ui shutdown failed", err, nil)
		}
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.LogError("web ui serve failed", err, map[string]interface{}{
				"listen_addr": listener.Addr().String(),
			})
		}
	}()

	return nil
}

func newHandler(opts Options) (http.Handler, error) {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
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
	mux.HandleFunc("/api/endpoints", server.handleEndpoints)
	mux.HandleFunc("/api/config", server.handleConfig)
	mux.HandleFunc("/api/discovery/run", server.handleDiscoveryRun)
	mux.HandleFunc("/api/discovery/stream", server.handleDiscoveryStream)
	mux.HandleFunc("/api/output/test", server.handleOutputTest)
	mux.Handle("/", staticHandler(staticRoot))

	return mux, nil
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
	opts.DiscoveryScriptPath = resolveDiscoveryScriptPath(opts)
	return &apiServer{opts: opts}
}

func resolveDiscoveryScriptPath(opts Options) string {
	if opts.DiscoveryScriptPath != "" {
		candidate := cleanPathForStatus(opts.DiscoveryScriptPath)
		if discoveryScriptAvailable(candidate) {
			return candidate
		}
		if hasEmbeddedDiscoveryScript() {
			return embeddedDiscoveryScriptPath
		}
		return candidate
	}

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

	if hasEmbeddedDiscoveryScript() {
		return embeddedDiscoveryScriptPath
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
	writeJSON(w, http.StatusOK, statusResponse{
		Product:             "Ping Monitor",
		Version:             s.opts.Version,
		ConfigPath:          info.Path,
		ConfigFormat:        info.Format,
		EndpointsPath:       filepath.Clean(s.opts.EndpointsPath),
		DiscoveryAvailable:  discoveryScriptAvailable(s.opts.DiscoveryScriptPath),
		DiscoveryScriptPath: s.opts.DiscoveryScriptPath,
		Mode:                "editable",
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
		writeJSON(w, http.StatusOK, endpointsResponse{
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			EndpointsPath: filepath.Clean(s.opts.EndpointsPath),
			Summary:       summarizeEndpoints(endpoints),
			Items:         endpoints,
		})
	case http.MethodPut:
		var request endpointsWriteRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := config.SaveEndpoints(s.opts.EndpointsPath, request.Items); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
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
		writeJSON(w, http.StatusOK, configResponse{
			GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
			ConfigPath:   info.Path,
			ConfigFormat: info.Format,
			Config:       cfg,
		})
	case http.MethodPut:
		var request configWriteRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		info, err := config.SaveConfig(r.Context(), s.opts.ConfigPath, s.opts.RootDir, request.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		cfg, _, err := config.LoadEditable(r.Context(), s.opts.ConfigPath, s.opts.RootDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		diagnostics.LogInfo("web ui saved config", map[string]interface{}{
			"config_path": info.Path,
			"format":      info.Format,
		})
		writeJSON(w, http.StatusOK, configResponse{
			GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
			ConfigPath:   info.Path,
			ConfigFormat: info.Format,
			Config:       cfg,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

	var (
		result outputTestResponse
		err    error
	)
	switch strings.ToLower(strings.TrimSpace(request.Target)) {
	case "hec":
		result, err = probeHECOutput(r.Context(), request.Config)
	case "metrics":
		result, err = probeMetricsOutput(r.Context(), request.Config)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target must be hec or metrics"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) runDiscovery(ctx context.Context, request discoveryRunRequest) (discoveryResponse, error) {
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
	return discoveryResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     summarizeEndpoints(endpoints),
		Items:       endpoints,
		Logs:        sanitizeDiscoveryLog(string(out)),
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

func (s *apiServer) streamDiscoveryRun(ctx context.Context, request discoveryRunRequest, emit func(discoveryStreamEvent) error) error {
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
	summary := summarizeEndpoints(endpoints)
	return emit(discoveryStreamEvent{
		Type:        "complete",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		SummaryText: fmt.Sprintf("Discovery completed with %d endpoint%s.", len(endpoints), pluralSuffix(len(endpoints))),
		Summary:     &summary,
		Items:       endpoints,
		Logs:        result.Logs,
		DurationMs:  time.Since(start).Milliseconds(),
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
		Time:       time.Now().UTC().Unix(),
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
		Time:       time.Now().UTC().Unix(),
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

func staticHandler(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." || cleanPath == "" {
			serveIndex(w, r, fileServer)
			return
		}
		if _, err := fs.Stat(root, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, fileServer)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, fileServer http.Handler) {
	b, err := fs.ReadFile(staticFiles, "static/index.html")
	if err != nil {
		http.Error(w, "ui shell unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(b))
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
