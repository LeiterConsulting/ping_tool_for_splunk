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
	"strconv"
	"strings"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"gopkg.in/yaml.v3"
)

const (
	LegacySchemaVersion  = 1
	CurrentSchemaVersion = 2
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

type Health struct {
	DownAfterFailures      int `json:"down_after_failures" yaml:"down_after_failures"`
	RecoveryAfterSuccesses int `json:"recovery_after_successes" yaml:"recovery_after_successes"`
	StaleAfterIntervals    int `json:"stale_after_intervals" yaml:"stale_after_intervals"`
}

type Retry struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	MaxAttempts int    `json:"max_attempts" yaml:"max_attempts"`
	BaseDelayMs int    `json:"base_delay_ms" yaml:"base_delay_ms"`
	JitterPct   int    `json:"jitter_pct" yaml:"jitter_pct"`
	Backoff     string `json:"backoff" yaml:"backoff"`
}

type HEC struct {
	Enabled                  bool   `json:"enabled" yaml:"enabled"`
	URL                      string `json:"url" yaml:"url"`
	Token                    string `json:"token" yaml:"token"`
	Index                    string `json:"index" yaml:"index"`
	SourceType               string `json:"sourcetype" yaml:"sourcetype"`
	VerifySSL                bool   `json:"verify_ssl" yaml:"verify_ssl"`
	SSLProtocol              string `json:"ssl_protocol" yaml:"ssl_protocol"`
	BatchSize                int    `json:"batch_size" yaml:"batch_size"`
	DropOnFailure            bool   `json:"drop_on_failure" yaml:"drop_on_failure"`
	MaxBufferEvents          int    `json:"max_buffer_events" yaml:"max_buffer_events"`
	MaxBufferBytes           string `json:"max_buffer_bytes" yaml:"max_buffer_bytes"`
	Retry                    Retry  `json:"retry" yaml:"retry"`
	RetryCount               int    `json:"retry_count" yaml:"retry_count"`
	RetryDelayMs             int    `json:"retry_delay_ms" yaml:"retry_delay_ms"`
	DeadLetterPath           string `json:"dead_letter_path" yaml:"dead_letter_path"`
	DeadLetterRotationSizeMB int    `json:"dead_letter_rotation_size_mb" yaml:"dead_letter_rotation_size_mb"`
	UseACK                   bool   `json:"use_ack" yaml:"use_ack"`
	ACKTimeoutSeconds        int    `json:"ack_timeout_seconds" yaml:"ack_timeout_seconds"`
	ACKPollIntervalMs        int    `json:"ack_poll_interval_ms" yaml:"ack_poll_interval_ms"`
	Channel                  string `json:"channel" yaml:"channel"`
}

type Metrics struct {
	Enabled           bool   `json:"enabled" yaml:"enabled"`
	Mode              string `json:"mode" yaml:"mode"`
	Index             string `json:"index" yaml:"index"`
	HECURL            string `json:"hec_url" yaml:"hec_url"`
	Token             string `json:"token" yaml:"token"`
	VerifySSL         bool   `json:"verify_ssl" yaml:"verify_ssl"`
	SSLProtocol       string `json:"ssl_protocol" yaml:"ssl_protocol"`
	CompatMode        bool   `json:"compat_mode" yaml:"compat_mode"`
	SourceType        string `json:"sourcetype" yaml:"sourcetype"`
	EventName         string `json:"event_name" yaml:"event_name"`
	UseMetricsIndex   bool   `json:"use_metrics_index" yaml:"use_metrics_index"`
	BatchSize         int    `json:"batch_size" yaml:"batch_size"`
	MaxBufferEvents   int    `json:"max_buffer_events" yaml:"max_buffer_events"`
	MaxBufferBytes    string `json:"max_buffer_bytes" yaml:"max_buffer_bytes"`
	UseACK            bool   `json:"use_ack" yaml:"use_ack"`
	ACKTimeoutSeconds int    `json:"ack_timeout_seconds" yaml:"ack_timeout_seconds"`
	ACKPollIntervalMs int    `json:"ack_poll_interval_ms" yaml:"ack_poll_interval_ms"`
	Channel           string `json:"channel" yaml:"channel"`
}

type Delivery struct {
	SpoolPath         string `json:"spool_path" yaml:"spool_path"`
	MaxSpoolBytes     string `json:"max_spool_bytes" yaml:"max_spool_bytes"`
	MaxEnvelopes      int    `json:"max_envelopes" yaml:"max_envelopes"`
	DrainMaxEnvelopes int    `json:"drain_max_envelopes" yaml:"drain_max_envelopes"`
}

type DiscoverySchedule struct {
	ID           string   `json:"id" yaml:"id"`
	Enabled      bool     `json:"enabled" yaml:"enabled"`
	Targets      []string `json:"targets" yaml:"targets"`
	Frequency    string   `json:"frequency" yaml:"frequency"`
	Day          string   `json:"day" yaml:"day"`
	Time         string   `json:"time" yaml:"time"`
	Timezone     string   `json:"timezone" yaml:"timezone"`
	TimeoutMs    int      `json:"timeout_ms" yaml:"timeout_ms"`
	Concurrency  int      `json:"concurrency" yaml:"concurrency"`
	ImportPolicy string   `json:"import_policy" yaml:"import_policy"`
}

type Discovery struct {
	HistoryPath string              `json:"history_path" yaml:"history_path"`
	Schedules   []DiscoverySchedule `json:"schedules" yaml:"schedules"`
}

type Config struct {
	ConfigSchemaVersion  int         `json:"config_schema_version,omitempty" yaml:"config_schema_version,omitempty"`
	PingsPerCycle        int         `json:"pings_per_cycle" yaml:"pings_per_cycle"`
	CycleIntervalSeconds int         `json:"cycle_interval_seconds" yaml:"cycle_interval_seconds"`
	TimeoutMs            int         `json:"timeout_ms" yaml:"timeout_ms"`
	ParallelThreads      int         `json:"parallel_threads" yaml:"parallel_threads"`
	OutputMode           string      `json:"output_mode" yaml:"output_mode"`
	LogPath              string      `json:"log_path" yaml:"log_path"`
	LogRotationSizeMB    int         `json:"log_rotation_size_mb" yaml:"log_rotation_size_mb"`
	LogRetentionFiles    int         `json:"log_retention_files" yaml:"log_retention_files"`
	LogRetentionDays     int         `json:"log_retention_days" yaml:"log_retention_days"`
	LogCompressRotated   bool        `json:"log_compress_rotated" yaml:"log_compress_rotated"`
	EmitIndividualPings  bool        `json:"emit_individual_pings" yaml:"emit_individual_pings"`
	Ping                 Ping        `json:"ping" yaml:"ping"`
	Health               Health      `json:"health" yaml:"health"`
	Diagnostics          Diagnostics `json:"diagnostics" yaml:"diagnostics"`
	Debug                Debug       `json:"debug" yaml:"debug"`
	HEC                  HEC         `json:"hec" yaml:"hec"`
	Metrics              Metrics     `json:"metrics" yaml:"metrics"`
	Delivery             Delivery    `json:"delivery" yaml:"delivery"`
	Discovery            Discovery   `json:"discovery" yaml:"discovery"`
}

func Defaults(root string) Config {
	return Config{
		ConfigSchemaVersion:  LegacySchemaVersion,
		PingsPerCycle:        4,
		CycleIntervalSeconds: 60,
		TimeoutMs:            1000,
		ParallelThreads:      10,
		OutputMode:           "file",
		LogPath:              filepath.Join(root, "logs", "ping_results.log"),
		LogRotationSizeMB:    50,
		LogRetentionFiles:    0,
		LogRetentionDays:     0,
		LogCompressRotated:   false,
		EmitIndividualPings:  true,
		Ping:                 Ping{Mode: "auto"},
		Health:               Health{DownAfterFailures: 3, RecoveryAfterSuccesses: 2, StaleAfterIntervals: 2},
		Diagnostics:          Diagnostics{Enabled: false, HandleProbeMode: "none"},
		Debug:                Debug{EmitMemoryStats: false},
		HEC: HEC{
			Enabled:                  false,
			URL:                      "",
			Token:                    "",
			Index:                    "main",
			SourceType:               "ping_monitor",
			VerifySSL:                true,
			SSLProtocol:              "Default",
			BatchSize:                100,
			DropOnFailure:            false,
			MaxBufferEvents:          5000,
			MaxBufferBytes:           "5MB",
			Retry:                    Retry{Enabled: false, MaxAttempts: 3, BaseDelayMs: 250, JitterPct: 20, Backoff: "exponential"},
			RetryCount:               0,
			RetryDelayMs:             250,
			DeadLetterPath:           "",
			DeadLetterRotationSizeMB: 0,
			UseACK:                   false,
			ACKTimeoutSeconds:        60,
			ACKPollIntervalMs:        1000,
		},
		Metrics: Metrics{
			Enabled:           false,
			Mode:              "dual",
			Index:             "",
			HECURL:            "",
			Token:             "",
			VerifySSL:         true,
			SSLProtocol:       "Default",
			CompatMode:        true,
			SourceType:        "ping_monitor:metrics",
			EventName:         "metric",
			UseMetricsIndex:   false,
			BatchSize:         100,
			MaxBufferEvents:   5000,
			MaxBufferBytes:    "5MB",
			UseACK:            false,
			ACKTimeoutSeconds: 60,
			ACKPollIntervalMs: 1000,
		},
		Delivery: Delivery{
			SpoolPath:         filepath.Join(root, "data", "outbox"),
			MaxSpoolBytes:     "512MB",
			MaxEnvelopes:      10000,
			DrainMaxEnvelopes: 100,
		},
		Discovery: Discovery{
			HistoryPath: filepath.Join(root, "data", "discovery"),
		},
	}
}

func CurrentDefaults(root string) Config {
	cfg := Defaults(root)
	cfg.ConfigSchemaVersion = CurrentSchemaVersion
	cfg.LogRetentionFiles = 10
	cfg.LogRetentionDays = 14
	cfg.LogCompressRotated = true
	return cfg
}

func Load(ctx context.Context, path string, root string) (Config, string, error) {
	// An existing explicitly selected supported file is authoritative. This is
	// essential when an operator keeps config.psd1 as a rollback source while
	// activating an upgraded config.json with --config.
	if isSupportedConfigExt(path) {
		if _, err := os.Stat(path); err == nil {
			return loadConfigPath(ctx, path, root)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, "", err
		} else if !strings.EqualFold(filepath.Base(path), "config.psd1") {
			cfg := Defaults(root)
			if strings.EqualFold(filepath.Ext(path), ".json") {
				cfg = CurrentDefaults(root)
			}
			if _, saveErr := SaveConfig(ctx, path, root, cfg); saveErr != nil {
				return Config{}, "", fmt.Errorf("config %s does not exist and could not be initialized: %w", path, saveErr)
			}
			loaded, source, loadErr := loadConfigPath(ctx, path, root)
			if loadErr != nil {
				return Config{}, "", loadErr
			}
			return loaded, "generated " + source, nil
		}
	}

	// Search fallbacks in same directory as provided path.
	baseDir := filepath.Dir(path)
	candidates := []string{
		filepath.Join(baseDir, "config.psd1"),
		filepath.Join(baseDir, "config.json"),
		filepath.Join(baseDir, "config.yaml"),
		filepath.Join(baseDir, "config.yml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		return loadConfigPath(ctx, c, root)
	}

	// Nothing usable found: generate the current, versioned JSON configuration.
	cfg := CurrentDefaults(root)
	outPath := filepath.Join(baseDir, "config.json")
	if err := writeJSONFile(outPath, cfg); err != nil {
		return Config{}, "", fmt.Errorf("no config found and failed to write fallback config.json: %w", err)
	}
	cfg = resolvePaths(cfg, filepath.Dir(outPath), root)
	return cfg, "generated config.json", nil
}

func loadConfigPath(ctx context.Context, path string, root string) (Config, string, error) {
	var (
		cfg    Config
		source string
		err    error
	)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".psd1":
		source = "config.psd1"
		cfg, err = loadFromPSD1(ctx, path, root)
	case ".yaml", ".yml":
		source = "config.yaml"
		cfg, err = loadFromYAML(path, root)
	case ".json":
		source = "config.json"
		cfg, err = loadFromJSON(path, root)
	default:
		return Config{}, "", fmt.Errorf("unsupported config format: %s", filepath.Ext(path))
	}
	if err != nil {
		return Config{}, "", fmt.Errorf("load %s: %w", path, err)
	}
	return resolvePaths(cfg, filepath.Dir(path), root), source, nil
}

func loadFromYAML(path string, root string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	schemaVersion, err := configSchemaVersionYAML(b)
	if err != nil {
		return Config{}, err
	}
	if schemaVersion == CurrentSchemaVersion {
		var doc configDocumentV2
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return Config{}, err
		}
		return configFromDocumentV2(doc, root)
	}
	if schemaVersion != LegacySchemaVersion {
		return Config{}, fmt.Errorf("unsupported config_schema_version %d; this runtime supports versions 1 and %d", schemaVersion, CurrentSchemaVersion)
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
	schemaVersion, err := configSchemaVersionJSON(b)
	if err != nil {
		return Config{}, err
	}
	if schemaVersion == CurrentSchemaVersion {
		var doc configDocumentV2
		if err := json.Unmarshal(b, &doc); err != nil {
			return Config{}, err
		}
		return configFromDocumentV2(doc, root)
	}
	if schemaVersion != LegacySchemaVersion {
		return Config{}, fmt.Errorf("unsupported config_schema_version %d; this runtime supports versions 1 and %d", schemaVersion, CurrentSchemaVersion)
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
	// Prefer pwsh Import-PowerShellDataFile when available; it is strict and battle-tested.
	// On macOS/Linux, pwsh might not be installed; fall back to our native, non-executing parser.
	var raw map[string]interface{}
	if pwsh, err := exec.LookPath("pwsh"); err == nil {
		cmd := exec.CommandContext(ctx, pwsh, "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("Import-PowerShellDataFile -Path '%s' | ConvertTo-Json -Depth 50 -Compress", strings.ReplaceAll(path, "'", "''")),
		)
		out, err := cmd.Output()
		if err != nil {
			return Config{}, fmt.Errorf("psd1 parse failed: %w", err)
		}
		if err := json.Unmarshal(out, &raw); err != nil {
			return Config{}, fmt.Errorf("psd1 json decode failed: %w", err)
		}
	} else {
		m, err := parsePSD1File(path)
		if err != nil {
			return Config{}, fmt.Errorf("psd1 parse failed (native): %w", err)
		}
		raw = m
	}

	cfg := Defaults(root)
	applyPSD1Map(&cfg, raw)
	if cfg.ConfigSchemaVersion != LegacySchemaVersion && cfg.ConfigSchemaVersion != CurrentSchemaVersion {
		return Config{}, fmt.Errorf("unsupported config_schema_version %d; this runtime supports versions 1 and %d", cfg.ConfigSchemaVersion, CurrentSchemaVersion)
	}
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
	if cfg.Delivery.SpoolPath != "" && !filepath.IsAbs(cfg.Delivery.SpoolPath) {
		cfg.Delivery.SpoolPath = filepath.Clean(filepath.Join(configDir, cfg.Delivery.SpoolPath))
	}
	if cfg.Discovery.HistoryPath != "" && !filepath.IsAbs(cfg.Discovery.HistoryPath) {
		cfg.Discovery.HistoryPath = filepath.Clean(filepath.Join(configDir, cfg.Discovery.HistoryPath))
	}
	// If configDir is empty for some reason, ensure defaults still resolve under root.
	if cfg.LogPath == "" {
		cfg.LogPath = filepath.Join(root, "logs", "ping_results.log")
	}
	return cfg
}

func applyPSD1Map(cfg *Config, raw map[string]interface{}) {
	cfg.ConfigSchemaVersion = getInt(raw, "config_schema_version", cfg.ConfigSchemaVersion)
	cfg.PingsPerCycle = getInt(raw, "pings_per_cycle", cfg.PingsPerCycle)
	cfg.CycleIntervalSeconds = getInt(raw, "cycle_interval_seconds", cfg.CycleIntervalSeconds)
	cfg.TimeoutMs = getInt(raw, "timeout_ms", cfg.TimeoutMs)
	cfg.ParallelThreads = getInt(raw, "parallel_threads", cfg.ParallelThreads)
	cfg.OutputMode = getString(raw, "output_mode", cfg.OutputMode)
	cfg.LogPath = getString(raw, "log_path", cfg.LogPath)
	cfg.LogRotationSizeMB = getInt(raw, "log_rotation_size_mb", cfg.LogRotationSizeMB)
	cfg.LogRetentionFiles = getInt(raw, "log_retention_files", cfg.LogRetentionFiles)
	cfg.LogRetentionDays = getInt(raw, "log_retention_days", cfg.LogRetentionDays)
	cfg.LogCompressRotated = getBool(raw, "log_compress_rotated", cfg.LogCompressRotated)
	cfg.EmitIndividualPings = getBool(raw, "emit_individual_pings", cfg.EmitIndividualPings)

	if m, ok := getMap(raw, "ping"); ok {
		cfg.Ping.Mode = getString(m, "mode", cfg.Ping.Mode)
	}
	if m, ok := getMap(raw, "health"); ok {
		cfg.Health.DownAfterFailures = getInt(m, "down_after_failures", cfg.Health.DownAfterFailures)
		cfg.Health.RecoveryAfterSuccesses = getInt(m, "recovery_after_successes", cfg.Health.RecoveryAfterSuccesses)
		cfg.Health.StaleAfterIntervals = getInt(m, "stale_after_intervals", cfg.Health.StaleAfterIntervals)
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
		cfg.HEC.UseACK = getBool(m, "use_ack", cfg.HEC.UseACK)
		cfg.HEC.ACKTimeoutSeconds = getInt(m, "ack_timeout_seconds", cfg.HEC.ACKTimeoutSeconds)
		cfg.HEC.ACKPollIntervalMs = getInt(m, "ack_poll_interval_ms", cfg.HEC.ACKPollIntervalMs)
		cfg.HEC.Channel = getString(m, "channel", cfg.HEC.Channel)
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
		cfg.Metrics.UseACK = getBool(m, "use_ack", cfg.Metrics.UseACK)
		cfg.Metrics.ACKTimeoutSeconds = getInt(m, "ack_timeout_seconds", cfg.Metrics.ACKTimeoutSeconds)
		cfg.Metrics.ACKPollIntervalMs = getInt(m, "ack_poll_interval_ms", cfg.Metrics.ACKPollIntervalMs)
		cfg.Metrics.Channel = getString(m, "channel", cfg.Metrics.Channel)
	}
	if m, ok := getMap(raw, "delivery"); ok {
		cfg.Delivery.SpoolPath = getString(m, "spool_path", cfg.Delivery.SpoolPath)
		cfg.Delivery.MaxSpoolBytes = getString(m, "max_spool_bytes", cfg.Delivery.MaxSpoolBytes)
		cfg.Delivery.MaxEnvelopes = getInt(m, "max_envelopes", cfg.Delivery.MaxEnvelopes)
		cfg.Delivery.DrainMaxEnvelopes = getInt(m, "drain_max_envelopes", cfg.Delivery.DrainMaxEnvelopes)
	}
	if m, ok := getMap(raw, "discovery"); ok {
		cfg.Discovery.HistoryPath = getString(m, "history_path", cfg.Discovery.HistoryPath)
		cfg.Discovery.Schedules = getDiscoverySchedules(m, "schedules")
	}
}

func normalize(cfg Config) Config {
	if cfg.ConfigSchemaVersion < LegacySchemaVersion {
		cfg.ConfigSchemaVersion = LegacySchemaVersion
	}
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
	if cfg.LogRetentionFiles < 0 {
		cfg.LogRetentionFiles = 0
	}
	if cfg.LogRetentionDays < 0 {
		cfg.LogRetentionDays = 0
	}
	if cfg.Ping.Mode == "" {
		cfg.Ping.Mode = "auto"
	}
	if cfg.Health.DownAfterFailures < 1 {
		cfg.Health.DownAfterFailures = 3
	}
	if cfg.Health.RecoveryAfterSuccesses < 1 {
		cfg.Health.RecoveryAfterSuccesses = 2
	}
	if cfg.Health.StaleAfterIntervals < 1 {
		cfg.Health.StaleAfterIntervals = 2
	}
	if cfg.OutputMode != "file" && cfg.OutputMode != "hec" && cfg.OutputMode != "both" {
		cfg.OutputMode = "file"
	}
	if cfg.Metrics.Mode == "" {
		cfg.Metrics.Mode = "dual"
	}
	if cfg.HEC.ACKTimeoutSeconds < 1 {
		cfg.HEC.ACKTimeoutSeconds = 60
	}
	if cfg.HEC.ACKPollIntervalMs < 50 {
		cfg.HEC.ACKPollIntervalMs = 1000
	}
	if cfg.Metrics.ACKTimeoutSeconds < 1 {
		cfg.Metrics.ACKTimeoutSeconds = 60
	}
	if cfg.Metrics.ACKPollIntervalMs < 50 {
		cfg.Metrics.ACKPollIntervalMs = 1000
	}
	if cfg.Delivery.SpoolPath == "" {
		cfg.Delivery.SpoolPath = filepath.Join("data", "outbox")
	}
	if cfg.Delivery.MaxSpoolBytes == "" {
		cfg.Delivery.MaxSpoolBytes = "512MB"
	}
	if cfg.Delivery.MaxEnvelopes < 1 {
		cfg.Delivery.MaxEnvelopes = 10000
	}
	if cfg.Delivery.DrainMaxEnvelopes < 1 {
		cfg.Delivery.DrainMaxEnvelopes = 100
	}
	if cfg.Discovery.HistoryPath == "" {
		cfg.Discovery.HistoryPath = filepath.Join("data", "discovery")
	}
	for i := range cfg.Discovery.Schedules {
		normalizeDiscoverySchedule(&cfg.Discovery.Schedules[i])
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

func getDiscoverySchedules(m map[string]interface{}, key string) []DiscoverySchedule {
	value, ok := m[key]
	if !ok || value == nil {
		return nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	schedules := make([]DiscoverySchedule, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		schedule := DiscoverySchedule{
			ID:           getString(raw, "id", ""),
			Enabled:      getBool(raw, "enabled", false),
			Targets:      getStringSlice(raw, "targets"),
			Frequency:    getString(raw, "frequency", ""),
			Day:          getString(raw, "day", ""),
			Time:         getString(raw, "time", ""),
			Timezone:     getString(raw, "timezone", ""),
			TimeoutMs:    getInt(raw, "timeout_ms", 0),
			Concurrency:  getInt(raw, "concurrency", 0),
			ImportPolicy: getString(raw, "import_policy", ""),
		}
		normalizeDiscoverySchedule(&schedule)
		schedules = append(schedules, schedule)
	}
	return schedules
}

func getStringSlice(m map[string]interface{}, key string) []string {
	value, ok := m[key]
	if !ok || value == nil {
		return nil
	}
	switch items := value.(type) {
	case []interface{}:
		values := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				values = append(values, text)
			}
		}
		return values
	case []string:
		return append([]string(nil), items...)
	default:
		if text := strings.TrimSpace(fmt.Sprint(items)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func normalizeDiscoverySchedule(schedule *DiscoverySchedule) {
	schedule.ID = strings.TrimSpace(schedule.ID)
	schedule.Frequency = strings.ToLower(strings.TrimSpace(schedule.Frequency))
	if schedule.Frequency == "" {
		schedule.Frequency = "weekly"
	}
	schedule.Day = strings.ToLower(strings.TrimSpace(schedule.Day))
	if schedule.Day == "" {
		schedule.Day = "sunday"
	}
	schedule.Time = strings.TrimSpace(schedule.Time)
	if schedule.Time == "" {
		schedule.Time = "02:00"
	}
	schedule.Timezone = strings.TrimSpace(schedule.Timezone)
	if schedule.Timezone == "" {
		schedule.Timezone = "Local"
	}
	if schedule.TimeoutMs < 100 {
		schedule.TimeoutMs = 500
	}
	if schedule.Concurrency < 1 {
		schedule.Concurrency = 25
	}
	schedule.ImportPolicy = strings.ToLower(strings.TrimSpace(schedule.ImportPolicy))
	if schedule.ImportPolicy == "" {
		schedule.ImportPolicy = "review"
	}
	for i := range schedule.Targets {
		schedule.Targets[i] = strings.TrimSpace(schedule.Targets[i])
	}
}

func LoadEndpoints(path string) ([]models.Endpoint, error) {
	return loadEndpoints(path, false)
}

func LoadEditableEndpoints(path string) ([]models.Endpoint, error) {
	return loadEndpoints(path, true)
}

func loadEndpoints(path string, allowEmpty bool) ([]models.Endpoint, error) {
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
		name := strings.ToLower(strings.TrimSpace(h))
		if name == "" {
			continue
		}
		if _, exists := idx[name]; exists {
			return nil, fmt.Errorf("endpoints CSV has duplicate %q header", name)
		}
		idx[name] = i
	}
	for _, required := range []string{"ip", "hostname"} {
		if _, ok := idx[required]; !ok {
			return nil, fmt.Errorf("endpoints CSV is missing required %q header", required)
		}
	}
	get := func(row []string, name string) string {
		p, ok := idx[name]
		if !ok || p < 0 || p >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[p])
	}

	var eps []models.Endpoint
	record := 1
	for {
		row, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		record++
		ip := get(row, "ip")
		hn := get(row, "hostname")
		if ip == "" || hn == "" {
			return nil, fmt.Errorf("endpoints CSV record %d must contain both ip and hostname", record)
		}
		dev, err := parseCSVBoolStrict(get(row, "dev"))
		if err != nil {
			return nil, fmt.Errorf("endpoints CSV record %d: %w", record, err)
		}
		monitoringEnabled, err := parseCSVBoolDefault(get(row, "monitoring_enabled"), true)
		if err != nil {
			return nil, fmt.Errorf("endpoints CSV record %d: invalid monitoring_enabled value: %w", record, err)
		}
		dnsForwardConfirmed, err := parseCSVBoolDefault(get(row, "dns_forward_confirmed"), false)
		if err != nil {
			return nil, fmt.Errorf("endpoints CSV record %d: invalid dns_forward_confirmed value: %w", record, err)
		}
		var discoveryLatency *float64
		if value := get(row, "discovery_latency_ms"); value != "" {
			parsed, parseErr := strconv.ParseFloat(value, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("endpoints CSV record %d: invalid discovery_latency_ms value %q", record, value)
			}
			discoveryLatency = &parsed
		}
		ep := models.Endpoint{
			EndpointID:          models.StableEndpointID(get(row, "endpoint_id"), ip),
			IP:                  ip,
			Hostname:            hn,
			FQDN:                get(row, "fqdn"),
			Dev:                 dev,
			MonitoringEnabled:   models.Bool(monitoringEnabled),
			MaintenanceUntil:    get(row, "maintenance_until"),
			MaintenanceReason:   get(row, "maintenance_reason"),
			DNSStatus:           get(row, "dns_status"),
			DNSForwardConfirmed: dnsForwardConfirmed,
			DiscoveredAt:        get(row, "discovered_at"),
			DiscoveryScanID:     get(row, "discovery_scan_id"),
			DiscoverySource:     get(row, "discovery_source"),
			DiscoveryLatencyMs:  discoveryLatency,
			Group:               firstNonEmpty(get(row, "group"), "default"),
			Description:         get(row, "description"),
			EntityType:          get(row, "entitytype"),
			Device:              get(row, "device"),
			Vendor:              get(row, "vendor"),
			AdditionalNotes:     get(row, "additional_notes"),
		}
		eps = append(eps, ep)
	}
	if len(eps) == 0 && !allowEmpty {
		return nil, errors.New("no valid endpoints found in CSV")
	}
	if err := ValidateEndpoints(eps); err != nil {
		return nil, fmt.Errorf("invalid endpoints CSV: %w", err)
	}
	return eps, nil
}

func writeEndpointsTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := "ip,hostname,fqdn,group,description,entitytype,device,vendor,additional_notes,endpoint_id,dev,monitoring_enabled,maintenance_until,maintenance_reason\n" +
		"127.0.0.1,localhost,,default,loopback,,,,,,false,true,,\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func firstNonEmpty(v string, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func parseCSVBool(v string) bool {
	parsed, _ := parseCSVBoolStrict(v)
	return parsed
}

func parseCSVBoolStrict(v string) (bool, error) {
	s := strings.TrimSpace(strings.ToLower(v))
	switch s {
	case "1", "true", "yes", "y", "on", "dev":
		return true, nil
	case "", "0", "false", "no", "n", "off", "prod":
		return false, nil
	default:
		return false, fmt.Errorf("invalid dev value %q", v)
	}
}

func parseCSVBoolDefault(value string, defaultValue bool) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}
	return parseCSVBoolStrict(value)
}
