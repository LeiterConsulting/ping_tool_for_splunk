package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

func TestSaveEndpoints_RoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	input := []models.Endpoint{
		{IP: "10.0.0.1", Hostname: "edge-router", Group: "network", Description: "Edge Router", EntityType: "network", Device: "router", Vendor: "Cisco", AdditionalNotes: "Primary", Dev: false},
		{IP: "10.0.0.25", Hostname: "qa-api", Group: "development", Description: "QA API", EntityType: "service", Device: "vm", Vendor: "VMware", AdditionalNotes: "Excluded", Dev: true},
	}

	if err := SaveEndpoints(path, input); err != nil {
		t.Fatalf("SaveEndpoints() error = %v", err)
	}

	loaded, err := LoadEndpoints(path)
	if err != nil {
		t.Fatalf("LoadEndpoints() error = %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len(loaded) = %d, want 2", len(loaded))
	}
	if loaded[1].Hostname != "qa-api" || !loaded[1].Dev {
		t.Fatalf("unexpected second endpoint: %#v", loaded[1])
	}
	backups, err := filepath.Glob(path + ".*.bak")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("unexpected backup files for first write: %#v", backups)
	}

	input[0].Description = "Updated Edge Router"
	if err := SaveEndpoints(path, input); err != nil {
		t.Fatalf("SaveEndpoints() second write error = %v", err)
	}
	backups, err = filepath.Glob(path + ".*.bak")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("expected backup file to be created on second write")
	}
}

func TestSaveEndpoints_EmptyRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	if err := SaveEndpoints(path, nil); err != nil {
		t.Fatalf("SaveEndpoints() error = %v", err)
	}

	loaded, err := LoadEditableEndpoints(path)
	if err != nil {
		t.Fatalf("LoadEditableEndpoints() error = %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("len(loaded) = %d, want 0", len(loaded))
	}
}

func TestSaveConfigPSD1_RoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "config.psd1")
	input := Defaults(root)
	input.PingsPerCycle = 7
	input.OutputMode = "both"
	input.LogPath = "./logs/custom.log"
	input.Diagnostics.Enabled = true
	input.HEC.Enabled = true
	input.HEC.URL = "https://splunk.example.com:8088"
	input.HEC.Token = "abc123"
	input.Metrics.Enabled = true
	input.Metrics.Index = "metrics"

	info, err := SaveConfig(context.Background(), path, root, input)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if info.Format != "psd1" {
		t.Fatalf("info.Format = %q, want psd1", info.Format)
	}

	loaded, source, err := LoadEditable(context.Background(), path, root)
	if err != nil {
		t.Fatalf("LoadEditable() error = %v", err)
	}
	if source.Format != "psd1" {
		t.Fatalf("source.Format = %q, want psd1", source.Format)
	}
	if loaded.PingsPerCycle != 7 || loaded.OutputMode != "both" || loaded.LogPath != "./logs/custom.log" {
		t.Fatalf("unexpected config round-trip: %#v", loaded)
	}
	if !loaded.Diagnostics.Enabled || !loaded.HEC.Enabled || loaded.HEC.URL != input.HEC.URL || !loaded.Metrics.Enabled {
		t.Fatalf("unexpected nested config round-trip: %#v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
}