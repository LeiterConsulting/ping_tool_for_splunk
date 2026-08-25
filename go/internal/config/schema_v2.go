package config

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

type monitoringDocumentV2 struct {
	PingsPerCycle        int  `json:"pings_per_cycle" yaml:"pings_per_cycle"`
	CycleIntervalSeconds int  `json:"cycle_interval_seconds" yaml:"cycle_interval_seconds"`
	TimeoutMs            int  `json:"timeout_ms" yaml:"timeout_ms"`
	ParallelThreads      int  `json:"parallel_threads" yaml:"parallel_threads"`
	EmitIndividualPings  bool `json:"emit_individual_pings" yaml:"emit_individual_pings"`
	Ping                 Ping `json:"ping" yaml:"ping"`
}

type resultLogDocumentV2 struct {
	Path            string `json:"path" yaml:"path"`
	MaxSizeMB       int    `json:"max_size_mb" yaml:"max_size_mb"`
	RetentionFiles  int    `json:"retention_files" yaml:"retention_files"`
	RetentionDays   int    `json:"retention_days" yaml:"retention_days"`
	CompressRotated bool   `json:"compress_rotated" yaml:"compress_rotated"`
}

type loggingDocumentV2 struct {
	Results resultLogDocumentV2 `json:"results" yaml:"results"`
}

type outputsDocumentV2 struct {
	Mode     string   `json:"mode" yaml:"mode"`
	HEC      HEC      `json:"hec" yaml:"hec"`
	Metrics  Metrics  `json:"metrics" yaml:"metrics"`
	Delivery Delivery `json:"delivery" yaml:"delivery"`
}

type configDocumentV2 struct {
	ConfigSchemaVersion int                  `json:"config_schema_version" yaml:"config_schema_version"`
	Monitoring          monitoringDocumentV2 `json:"monitoring" yaml:"monitoring"`
	Health              Health               `json:"health" yaml:"health"`
	Logging             loggingDocumentV2    `json:"logging" yaml:"logging"`
	Outputs             outputsDocumentV2    `json:"outputs" yaml:"outputs"`
	Discovery           Discovery            `json:"discovery" yaml:"discovery"`
	Classification      Classification       `json:"classification,omitempty" yaml:"classification,omitempty"`
	Diagnostics         Diagnostics          `json:"diagnostics" yaml:"diagnostics"`
	Debug               Debug                `json:"debug" yaml:"debug"`
}

func configToDocumentV2(cfg Config) configDocumentV2 {
	return configDocumentV2{
		ConfigSchemaVersion: CurrentSchemaVersion,
		Monitoring: monitoringDocumentV2{
			PingsPerCycle:        cfg.PingsPerCycle,
			CycleIntervalSeconds: cfg.CycleIntervalSeconds,
			TimeoutMs:            cfg.TimeoutMs,
			ParallelThreads:      cfg.ParallelThreads,
			EmitIndividualPings:  cfg.EmitIndividualPings,
			Ping:                 cfg.Ping,
		},
		Health: cfg.Health,
		Logging: loggingDocumentV2{Results: resultLogDocumentV2{
			Path:            cfg.LogPath,
			MaxSizeMB:       cfg.LogRotationSizeMB,
			RetentionFiles:  cfg.LogRetentionFiles,
			RetentionDays:   cfg.LogRetentionDays,
			CompressRotated: cfg.LogCompressRotated,
		}},
		Outputs: outputsDocumentV2{
			Mode: cfg.OutputMode, HEC: cfg.HEC, Metrics: cfg.Metrics, Delivery: cfg.Delivery,
		},
		Discovery:      cfg.Discovery,
		Classification: cfg.Classification,
		Diagnostics:    cfg.Diagnostics,
		Debug:          cfg.Debug,
	}
}

func configFromDocumentV2(doc configDocumentV2, root string) (Config, error) {
	if doc.ConfigSchemaVersion != CurrentSchemaVersion {
		return Config{}, fmt.Errorf("unsupported config_schema_version %d; this runtime supports versions 1 and %d", doc.ConfigSchemaVersion, CurrentSchemaVersion)
	}
	cfg := CurrentDefaults(root)
	cfg.PingsPerCycle = doc.Monitoring.PingsPerCycle
	cfg.CycleIntervalSeconds = doc.Monitoring.CycleIntervalSeconds
	cfg.TimeoutMs = doc.Monitoring.TimeoutMs
	cfg.ParallelThreads = doc.Monitoring.ParallelThreads
	cfg.EmitIndividualPings = doc.Monitoring.EmitIndividualPings
	cfg.Ping = doc.Monitoring.Ping
	cfg.Health = doc.Health
	cfg.LogPath = doc.Logging.Results.Path
	cfg.LogRotationSizeMB = doc.Logging.Results.MaxSizeMB
	cfg.LogRetentionFiles = doc.Logging.Results.RetentionFiles
	cfg.LogRetentionDays = doc.Logging.Results.RetentionDays
	cfg.LogCompressRotated = doc.Logging.Results.CompressRotated
	cfg.OutputMode = doc.Outputs.Mode
	cfg.HEC = doc.Outputs.HEC
	cfg.Metrics = doc.Outputs.Metrics
	cfg.Delivery = doc.Outputs.Delivery
	cfg.Discovery = doc.Discovery
	cfg.Classification = doc.Classification
	cfg.Diagnostics = doc.Diagnostics
	cfg.Debug = doc.Debug
	return normalize(cfg), nil
}

func configSchemaVersionJSON(data []byte) (int, error) {
	var probe struct {
		ConfigSchemaVersion int `json:"config_schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, err
	}
	if probe.ConfigSchemaVersion == 0 {
		return LegacySchemaVersion, nil
	}
	return probe.ConfigSchemaVersion, nil
}

func configSchemaVersionYAML(data []byte) (int, error) {
	var probe struct {
		ConfigSchemaVersion int `yaml:"config_schema_version"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return 0, err
	}
	if probe.ConfigSchemaVersion == 0 {
		return LegacySchemaVersion, nil
	}
	return probe.ConfigSchemaVersion, nil
}

func marshalConfigJSON(cfg Config) ([]byte, error) {
	if cfg.ConfigSchemaVersion >= CurrentSchemaVersion {
		return json.MarshalIndent(configToDocumentV2(cfg), "", "  ")
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func marshalConfigYAML(cfg Config) ([]byte, error) {
	if cfg.ConfigSchemaVersion >= CurrentSchemaVersion {
		return yaml.Marshal(configToDocumentV2(cfg))
	}
	return yaml.Marshal(cfg)
}

func MarshalCurrentJSON(cfg Config) ([]byte, error) {
	cfg.ConfigSchemaVersion = CurrentSchemaVersion
	return marshalConfigJSON(cfg)
}
