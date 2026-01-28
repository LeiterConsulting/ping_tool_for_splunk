package config

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"gopkg.in/yaml.v3"
)

type Diagnostics struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"`
	HandleProbeMode string `json:"handle_probe_mode" yaml:"handle_probe_mode"`
}

type Debug struct {
	EmitMemoryStats bool `json:"emit_memory_stats" yaml:"emit_memory_stats"`
}

type Ping struct {
	Mode string `json:"mode" yaml:"mode"`
}

type Retry struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	MaxAttempts int    `json:"max_attempts" yaml:"max_attempts"`
	BaseDelayMs int    `json:"base_delay_ms" yaml:"base_delay_ms"`
	JitterPct   int    `json:"jitter_pct" yaml:"jitter_pct"`
	Backoff     string `json:"backoff" yaml:"backoff"`
}

type HEC struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"`
	URL             string `json:"url" yaml:"url"`
	Token           string `json:"token" yaml:"token"`
	Index           string `json:"index" yaml:"index"`
	SourceType      string `json:"sourcetype" yaml:"sourcetype"`
	VerifySSL       bool   `json:"verify_ssl" yaml:"verify_ssl"`
	SSLProtocol     string `json:"ssl_protocol" yaml:"ssl_protocol"`
	BatchSize       int    `json:"batch_size" yaml:"batch_size"`
	DropOnFailure   bool   `json:"drop_on_failure" yaml:"drop_on_failure"`
	MaxBufferEvents int    `json:"max_buffer_events" yaml:"max_buffer_events"`
	MaxBufferBytes  string `json:"max_buffer_bytes" yaml:"max_buffer_bytes"`
	Retry           Retry  `json:"retry" yaml:"retry"`
	RetryCount      int    `json:"retry_count" yaml:"retry_count"`
	RetryDelayMs    int    `json:"retry_delay_ms" yaml:"retry_delay_ms"`
	DeadLetterPath  string `json:"dead_letter_path" yaml:"dead_letter_path"`
	DeadLetterRotationSizeMB int `json:"dead_letter_rotation_size_mb" yaml:"dead_letter_rotation_size_mb"`
}

type Metrics struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"`
	Mode            string `json:"mode" yaml:"mode"`
	Index           string `json:"index" yaml:"index"`
	HECURL          string `json:"hec_url" yaml:"hec_url"`
	Token           string `json:"token" yaml:"token"`
	VerifySSL       bool   `json:"verify_ssl" yaml:"verify_ssl"`
	SSLProtocol     string `json:"ssl_protocol" yaml:"ssl_protocol"`
	CompatMode      bool   `json:"compat_mode" yaml:"compat_mode"`
	SourceType      string `json:"sourcetype" yaml:"sourcetype"`
	EventName       string `json:"event_name" yaml:"event_name"`
	UseMetricsIndex bool   `json:"use_metrics_index" yaml:"use_metrics_index"`
	BatchSize       int    `json:"batch_size" yaml:"batch_size"`
	MaxBufferEvents int    `json:"max_buffer_events" yaml:"max_buffer_events"`
	MaxBufferBytes  string `json:"max_buffer_bytes" yaml:"max_buffer_bytes"`
}

type Config struct {
	PingsPerCycle        int         `json:"pings_per_cycle" yaml:"pings_per_cycle"`
	CycleIntervalSeconds int         `json:"cycle_interval_seconds" yaml:"cycle_interval_seconds"`
	TimeoutMs            int         `json:"timeout_ms" yaml:"timeout_ms"`
	ParallelThreads      int         `json:"parallel_threads" yaml:"parallel_threads"`
	OutputMode           string      `json:"output_mode" yaml:"output_mode"`
	LogPath              string      `json:"log_path" yaml:"log_path"`
	LogRotationSizeMB    int         `json:"log_rotation_size_mb" yaml:"log_rotation_size_mb"`
	EmitIndividualPings  bool        `json:"emit_individual_pings" yaml:"emit_individual_pings"`
	Ping                 Ping        `json:"ping" yaml:"ping"`
	Diagnostics          Diagnostics `json:"diagnostics" yaml:"diagnostics"`
	Debug                Debug       `json:"debug" yaml:"debug"`
	HEC                  HEC         `json:"hec" yaml:"hec"`
	Metrics              Metrics     `json:"metrics" yaml:"metrics"`
}

func Defaults(root string) Config {
	return Config{
		PingsPerCycle:        4,
		CycleIntervalSeconds: 60,
		TimeoutMs:            1000,
		ParallelThreads:      10,
		OutputMode:           "file",
		LogPath:              filepath.Join(root, "logs", "ping_results.log"),
		LogRotationSizeMB:    50,
		EmitIndividualPings:  true,
		Ping:                 Ping{Mode: "auto"},
		Diagnostics:          Diagnostics{Enabled: false, HandleProbeMode: "none"},
		Debug:                Debug{EmitMemoryStats: false},
		HEC: HEC{
			Enabled:         false,
			URL:             "",
			Token:           "",
			Index:           "main",
			SourceType:      "ping_monitor",
			VerifySSL:       true,
			SSLProtocol:     "Default",
			BatchSize:       100,
			DropOnFailure:   true,
			MaxBufferEvents: 5000,
			MaxBufferBytes:  "5MB",
			Retry:           Retry{Enabled: false, MaxAttempts: 3, BaseDelayMs: 250, JitterPct: 20, Backoff: "exponential"},
			RetryCount:      0,
			RetryDelayMs:    250,
			DeadLetterPath:  "",
			DeadLetterRotationSizeMB: 0,
		},
		Metrics: Metrics{
			Enabled:         false,
			Mode:            "dual",
			Index:           "",
			HECURL:          "",
			Token:           "",
			VerifySSL:       true,
			SSLProtocol:     "Default",
			CompatMode:      true,
			SourceType:      "ping_monitor:metrics",
			EventName:       "metric",
			UseMetricsIndex: false,
			BatchSize:       100,
			MaxBufferEvents: 5000,
			MaxBufferBytes:  "5MB",
		},
	}
}

func Load(ctx context.Context, path string, root string) (Config, string, error) {
	// If caller passed config.psd1, try that first.
	if strings.HasSuffix(strings.ToLower(path), ".psd1") {
		if _, err := os.Stat(path); err == nil {
			cfg, err := loadFromPSD1(ctx, path, root)
			if err == nil {
				cfg = resolvePaths(cfg, filepath.Dir(path), root)
				return cfg, "config.psd1", nil
			}
			// fall through to other sources
		}
	}

	// Search fallbacks in same directory as provided path.
	baseDir := filepath.Dir(path)
	candidates := []string{
		filepath.Join(baseDir, "config.psd1"),
		filepath.Join(baseDir, "config.yaml"),
		filepath.Join(baseDir, "config.yml"),
		filepath.Join(baseDir, "config.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		switch strings.ToLower(filepath.Ext(c)) {
		case ".psd1":
			cfg, err := loadFromPSD1(ctx, c, root)
			if err == nil {
				cfg = resolvePaths(cfg, filepath.Dir(c), root)
				return cfg, "config.psd1", nil
			}
		case ".yaml", ".yml":
			cfg, err := loadFromYAML(c, root)
			if err == nil {
				cfg = resolvePaths(cfg, filepath.Dir(c), root)
				return cfg, "config.yaml", nil
			}
		case ".json":
			cfg, err := loadFromJSON(c, root)
			if err == nil {
				cfg = resolvePaths(cfg, filepath.Dir(c), root)
				return cfg, "config.json", nil
			}
		}
	}

	// Nothing usable found: generate a Go-native fallback config.
	cfg := Defaults(root)
	outPath := filepath.Join(baseDir, "config.yaml")
	if err := writeYAML(outPath, cfg); err != nil {
		return Config{}, "", fmt.Errorf("no config found and failed to write fallback config.yaml: %w", err)
	}
	cfg = resolvePaths(cfg, filepath.Dir(outPath), root)
	return cfg, "generated config.yaml", nil
}

func loadFromYAML(path string, root string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Defaults(root)
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return normalize(cfg), nil
}

func loadFromJSON(path string, root string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Defaults(root)
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return normalize(cfg), nil
}

func writeYAML(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func loadFromPSD1(ctx context.Context, path string, root string) (Config, error) {
	// Use pwsh Import-PowerShellDataFile to avoid implementing a full psd1 parser.
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		return Config{}, errors.New("pwsh not found; cannot parse config.psd1")
	}

	// Import-PowerShellDataFile reads data safely (no arbitrary code execution).
	cmd := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf("Import-PowerShellDataFile -Path '%s' | ConvertTo-Json -Depth 50 -Compress", strings.ReplaceAll(path, "'", "''")),
	)
	out, err := cmd.Output()
	if err != nil {
		return Config{}, fmt.Errorf("psd1 parse failed: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Config{}, fmt.Errorf("psd1 json decode failed: %w", err)
	}

	cfg := Defaults(root)
	applyPSD1Map(&cfg, raw)
	return normalize(cfg), nil
}

func resolvePaths(cfg Config, configDir string, root string) Config {
	// v4 resolves relative paths relative to the script directory.
	// For v5, resolve relative paths relative to the config file directory first,
	// then fall back to the provided root.
	if cfg.LogPath != "" && !filepath.IsAbs(cfg.LogPath) {
		cfg.LogPath = filepath.Clean(filepath.Join(configDir, cfg.LogPath))
	}
	if cfg.HEC.DeadLetterPath != "" && !filepath.IsAbs(cfg.HEC.DeadLetterPath) {
		cfg.HEC.DeadLetterPath = filepath.Clean(filepath.Join(configDir, cfg.HEC.DeadLetterPath))
	}
	// If configDir is empty for some reason, ensure defaults still resolve under root.
	if cfg.LogPath == "" {
		cfg.LogPath = filepath.Join(root, "logs", "ping_results.log")
	}
	return cfg
}

func applyPSD1Map(cfg *Config, raw map[string]interface{}) {
	cfg.PingsPerCycle = getInt(raw, "pings_per_cycle", cfg.PingsPerCycle)
	cfg.CycleIntervalSeconds = getInt(raw, "cycle_interval_seconds", cfg.CycleIntervalSeconds)
	cfg.TimeoutMs = getInt(raw, "timeout_ms", cfg.TimeoutMs)
	cfg.ParallelThreads = getInt(raw, "parallel_threads", cfg.ParallelThreads)
	cfg.OutputMode = getString(raw, "output_mode", cfg.OutputMode)
	cfg.LogPath = getString(raw, "log_path", cfg.LogPath)
	cfg.LogRotationSizeMB = getInt(raw, "log_rotation_size_mb", cfg.LogRotationSizeMB)
	cfg.EmitIndividualPings = getBool(raw, "emit_individual_pings", cfg.EmitIndividualPings)

	if m, ok := getMap(raw, "ping"); ok {
		cfg.Ping.Mode = getString(m, "mode", cfg.Ping.Mode)
	}

	if m, ok := getMap(raw, "diagnostics"); ok {
		cfg.Diagnostics.Enabled = getBool(m, "enabled", cfg.Diagnostics.Enabled)
		cfg.Diagnostics.HandleProbeMode = getString(m, "handle_probe_mode", cfg.Diagnostics.HandleProbeMode)
	}
	if m, ok := getMap(raw, "debug"); ok {
		cfg.Debug.EmitMemoryStats = getBool(m, "emit_memory_stats", cfg.Debug.EmitMemoryStats)
	}
	if m, ok := getMap(raw, "hec"); ok {
		cfg.HEC.Enabled = getBool(m, "enabled", cfg.HEC.Enabled)
		cfg.HEC.URL = getString(m, "url", cfg.HEC.URL)
		cfg.HEC.Token = getString(m, "token", cfg.HEC.Token)
		cfg.HEC.Index = getString(m, "index", cfg.HEC.Index)
		cfg.HEC.SourceType = getString(m, "sourcetype", cfg.HEC.SourceType)
		cfg.HEC.VerifySSL = getBool(m, "verify_ssl", cfg.HEC.VerifySSL)
		cfg.HEC.SSLProtocol = getString(m, "ssl_protocol", cfg.HEC.SSLProtocol)
		cfg.HEC.BatchSize = getInt(m, "batch_size", cfg.HEC.BatchSize)
		cfg.HEC.DropOnFailure = getBool(m, "drop_on_failure", cfg.HEC.DropOnFailure)
		cfg.HEC.MaxBufferEvents = getInt(m, "max_buffer_events", cfg.HEC.MaxBufferEvents)
		cfg.HEC.MaxBufferBytes = getString(m, "max_buffer_bytes", cfg.HEC.MaxBufferBytes)
		cfg.HEC.RetryCount = getInt(m, "retry_count", cfg.HEC.RetryCount)
		cfg.HEC.RetryDelayMs = getInt(m, "retry_delay_ms", cfg.HEC.RetryDelayMs)
		cfg.HEC.DeadLetterPath = getString(m, "dead_letter_path", cfg.HEC.DeadLetterPath)
		cfg.HEC.DeadLetterRotationSizeMB = getInt(m, "dead_letter_rotation_size_mb", cfg.HEC.DeadLetterRotationSizeMB)
		if r, ok := getMap(m, "retry"); ok {
			cfg.HEC.Retry.Enabled = getBool(r, "enabled", cfg.HEC.Retry.Enabled)
			cfg.HEC.Retry.MaxAttempts = getInt(r, "max_attempts", cfg.HEC.Retry.MaxAttempts)
			cfg.HEC.Retry.BaseDelayMs = getInt(r, "base_delay_ms", cfg.HEC.Retry.BaseDelayMs)
			cfg.HEC.Retry.JitterPct = getInt(r, "jitter_pct", cfg.HEC.Retry.JitterPct)
			cfg.HEC.Retry.Backoff = getString(r, "backoff", cfg.HEC.Retry.Backoff)
		}
	}
	if m, ok := getMap(raw, "metrics"); ok {
		cfg.Metrics.Enabled = getBool(m, "enabled", cfg.Metrics.Enabled)
		cfg.Metrics.Mode = getString(m, "mode", cfg.Metrics.Mode)
		cfg.Metrics.Index = getString(m, "index", cfg.Metrics.Index)
		cfg.Metrics.HECURL = getString(m, "hec_url", cfg.Metrics.HECURL)
		cfg.Metrics.Token = getString(m, "token", cfg.Metrics.Token)
		cfg.Metrics.VerifySSL = getBool(m, "verify_ssl", cfg.Metrics.VerifySSL)
		cfg.Metrics.SSLProtocol = getString(m, "ssl_protocol", cfg.Metrics.SSLProtocol)
		cfg.Metrics.CompatMode = getBool(m, "compat_mode", cfg.Metrics.CompatMode)
		cfg.Metrics.SourceType = getString(m, "sourcetype", cfg.Metrics.SourceType)
		cfg.Metrics.EventName = getString(m, "event_name", cfg.Metrics.EventName)
		cfg.Metrics.UseMetricsIndex = getBool(m, "use_metrics_index", cfg.Metrics.UseMetricsIndex)
		cfg.Metrics.BatchSize = getInt(m, "batch_size", cfg.Metrics.BatchSize)
		cfg.Metrics.MaxBufferEvents = getInt(m, "max_buffer_events", cfg.Metrics.MaxBufferEvents)
		cfg.Metrics.MaxBufferBytes = getString(m, "max_buffer_bytes", cfg.Metrics.MaxBufferBytes)
	}
}

func normalize(cfg Config) Config {
	if cfg.PingsPerCycle < 1 {
		cfg.PingsPerCycle = 4
	}
	if cfg.CycleIntervalSeconds < 1 {
		cfg.CycleIntervalSeconds = 60
	}
	if cfg.TimeoutMs < 100 {
		cfg.TimeoutMs = 1000
	}
	if cfg.ParallelThreads < 1 {
		cfg.ParallelThreads = 10
	}
	if cfg.LogRotationSizeMB < 1 {
		cfg.LogRotationSizeMB = 50
	}
	if cfg.Ping.Mode == "" {
		cfg.Ping.Mode = "auto"
	}
	if cfg.OutputMode != "file" && cfg.OutputMode != "hec" && cfg.OutputMode != "both" {
		cfg.OutputMode = "file"
	}
	if cfg.Metrics.Mode == "" {
		cfg.Metrics.Mode = "dual"
	}
	return cfg
}

func getMap(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	mm, ok := v.(map[string]interface{})
	return mm, ok
}

func getString(m map[string]interface{}, key string, def string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprint(v)
}

func getBool(m map[string]interface{}, key string, def bool) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	b, ok := v.(bool)
	if ok {
		return b
	}
	// tolerate string values
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "true" || s == "1" || s == "yes" {
			return true
		}
		if s == "false" || s == "0" || s == "no" {
			return false
		}
	}
	return def
}

func getInt(m map[string]interface{}, key string, def int) int {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case float32:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		var i int
		_, _ = fmt.Sscanf(t, "%d", &i)
		if i != 0 {
			return i
		}
	}
	return def
}

func LoadEndpoints(path string) ([]models.Endpoint, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = writeEndpointsTemplate(path)
			return nil, fmt.Errorf("endpoints file not found: %s (template created)", path)
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	headers, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range headers {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, name string) string {
		p, ok := idx[name]
		if !ok || p < 0 || p >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[p])
	}

	var eps []models.Endpoint
	for {
		row, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		ip := get(row, "ip")
		hn := get(row, "hostname")
		if ip == "" || hn == "" {
			continue
		}
		ep := models.Endpoint{
			IP:              ip,
			Hostname:        hn,
			Group:           firstNonEmpty(get(row, "group"), "default"),
			Description:     get(row, "description"),
			EntityType:      get(row, "entitytype"),
			Device:          get(row, "device"),
			Vendor:          get(row, "vendor"),
			AdditionalNotes: get(row, "additional_notes"),
		}
		eps = append(eps, ep)
	}
	if len(eps) == 0 {
		return nil, errors.New("no valid endpoints found in CSV")
	}
	return eps, nil
}

func writeEndpointsTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := "ip,hostname,group,description,entitytype,device,vendor,additional_notes\n" +
		"127.0.0.1,localhost,default,loopback,,, ,\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func firstNonEmpty(v string, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
