package config

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyJSONRemainsValidAndReceivesCompatibilityDefaults(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "legacy.json")
	content := `{
  "pings_per_cycle": 7,
  "cycle_interval_seconds": 90,
  "timeout_ms": 1200,
  "parallel_threads": 30,
  "output_mode": "file",
  "log_path": "./logs/results.log",
  "log_rotation_size_mb": 50,
  "emit_individual_pings": false
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadEditable(context.Background(), path, root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigSchemaVersion != LegacySchemaVersion {
		t.Fatalf("schema = %d, want %d", cfg.ConfigSchemaVersion, LegacySchemaVersion)
	}
	if cfg.PingsPerCycle != 7 || cfg.LogRetentionFiles != 0 || cfg.LogCompressRotated {
		t.Fatalf("legacy behavior changed: %+v", cfg)
	}
}

func TestCurrentJSONRoundTripPreservesEffectiveConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	cfg := CurrentDefaults(root)
	cfg.PingsPerCycle = 8
	cfg.LogPath = "./logs/current.log"
	cfg.Discovery.Schedules = []DiscoverySchedule{{
		ID: "weekly-core", Enabled: true, Targets: []string{"10.10.0.0/16"},
		Frequency: "weekly", Day: "sunday", Time: "02:00", Timezone: "UTC",
		TimeoutMs: 750, Concurrency: 20, ImportPolicy: "review",
	}}
	if _, err := SaveConfig(context.Background(), path, root, cfg); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"config_schema_version": 2`, `"monitoring"`, `"logging"`, `"outputs"`, `"discovery"`} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("current JSON missing %s:\n%s", expected, content)
		}
	}
	loaded, _, err := LoadEditable(context.Background(), path, root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("round trip mismatch\nwant: %#v\n got: %#v", cfg, loaded)
	}
}

func TestPSD1RoundTripPreservesDiscoverySchedulesAndRotationPolicy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.psd1")
	cfg := Defaults(root)
	cfg.LogRetentionFiles = 5
	cfg.LogRetentionDays = 30
	cfg.LogCompressRotated = true
	cfg.Discovery.Schedules = []DiscoverySchedule{{
		ID: "weekly-lab", Enabled: true, Targets: []string{"10.0.0.0/24", "10.0.1.0/24"},
		Frequency: "weekly", Day: "monday", Time: "03:15", Timezone: "UTC",
		TimeoutMs: 500, Concurrency: 15, ImportPolicy: "review",
	}}
	if _, err := SaveConfig(context.Background(), path, root, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadEditable(context.Background(), path, root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("PSD1 round trip mismatch\nwant: %#v\n got: %#v", cfg, loaded)
	}
}

func TestLoadHonorsExplicitJSONWhenLegacyPSD1RemainsBesideIt(t *testing.T) {
	root := t.TempDir()
	psd1Path := filepath.Join(root, "config.psd1")
	jsonPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(psd1Path, []byte("@{ pings_per_cycle = 3 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := CurrentDefaults(root)
	cfg.PingsPerCycle = 9
	if _, err := SaveConfig(context.Background(), jsonPath, root, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, source, err := Load(context.Background(), jsonPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PingsPerCycle != 9 || source != "config.json" {
		t.Fatalf("explicit JSON was shadowed: pings=%d source=%q", loaded.PingsPerCycle, source)
	}
}

func TestPSD1RejectsUnsupportedFutureSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.psd1")
	if err := os.WriteFile(path, []byte("@{ config_schema_version = 99 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(context.Background(), path, root); err == nil || !strings.Contains(err.Error(), "unsupported config_schema_version 99") {
		t.Fatalf("future PSD1 schema error = %v", err)
	}
}
