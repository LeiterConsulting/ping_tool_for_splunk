package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

func TestSaveEndpoints_RoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "endpoints.csv")
	discoveryLatency := 12.75
	input := []models.Endpoint{
		{
			IP: "10.0.0.1", Hostname: "edge-router", FQDN: "edge-router.example.com",
			Group: "network", Description: "Edge Router", EntityType: "network", Device: "router",
			Vendor: "Cisco", AdditionalNotes: "Primary", Dev: false,
			DNSStatus: "forward_confirmed", DNSForwardConfirmed: true,
			DiscoveredAt: "2026-07-27T12:00:00Z", DiscoveryScanID: "scan-123",
			DiscoverySource: "icmp_subnet_scan", DiscoveryLatencyMs: &discoveryLatency,
			DeviceMode: models.DeviceModeProduction, AlertingEnabled: models.Bool(false), AlertingReason: "planned silence",
			AssetID: "asset-100", DynamicAddress: true, ClassificationSource: "rule:network",
			DiscoveryReviewState: models.DiscoveryReviewApproved, DiscoveryReviewedAt: "2026-08-25T12:00:00Z", DiscoveryReviewNote: "Added to inventory",
			SubnetID: "users", SubnetName: "User LAN", SubnetVLAN: "230", SubnetLocation: "NYC",
			AddressingMode: "dhcp", RoutingDomain: "corp",
		},
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
	if loaded[0].EndpointID == "" || loaded[1].EndpointID == "" || loaded[0].EndpointID == loaded[1].EndpointID {
		t.Fatalf("stable endpoint IDs were not generated: %#v", loaded)
	}
	if loaded[0].FQDN != "edge-router.example.com" || loaded[0].DNSStatus != "forward_confirmed" ||
		!loaded[0].DNSForwardConfirmed || loaded[0].DiscoveredAt != "2026-07-27T12:00:00Z" ||
		loaded[0].DiscoveryScanID != "scan-123" || loaded[0].DiscoverySource != "icmp_subnet_scan" ||
		loaded[0].DiscoveryLatencyMs == nil || *loaded[0].DiscoveryLatencyMs != discoveryLatency {
		t.Fatalf("discovery evidence did not survive endpoint round trip: %#v", loaded[0])
	}
	if loaded[0].DeviceMode != models.DeviceModeProduction || loaded[0].IsAlertingEnabled() ||
		loaded[0].AlertingReason != "planned silence" || loaded[0].AssetID != "asset-100" ||
		!loaded[0].DynamicAddress || loaded[0].ClassificationSource != "rule:network" ||
		loaded[0].DiscoveryReviewState != models.DiscoveryReviewApproved || loaded[0].DiscoveryReviewedAt != "2026-08-25T12:00:00Z" ||
		loaded[0].DiscoveryReviewNote != "Added to inventory" ||
		loaded[0].SubnetID != "users" || loaded[0].SubnetName != "User LAN" || loaded[0].SubnetVLAN != "230" ||
		loaded[0].SubnetLocation != "NYC" || loaded[0].AddressingMode != "dhcp" || loaded[0].RoutingDomain != "corp" {
		t.Fatalf("v5.11 policy fields did not survive endpoint round trip: %#v", loaded[0])
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

func TestValidateEndpointsRejectsDuplicateStableAssetID(t *testing.T) {
	err := ValidateEndpoints([]models.Endpoint{
		{IP: "10.0.0.1", Hostname: "one", AssetID: "Asset-42"},
		{IP: "10.0.0.2", Hostname: "two", AssetID: "asset-42"},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicates asset_id") {
		t.Fatalf("ValidateEndpoints() error = %v, want duplicate asset identity", err)
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
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("config permissions = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestLoadDoesNotHideInvalidPreferredConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "config.psd1")
	if err := os.WriteFile(path, []byte("@{ broken = @{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(context.Background(), path, root); err == nil {
		t.Fatal("Load() silently replaced an invalid preferred config")
	}
	if _, err := os.Stat(filepath.Join(root, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("Load() created a fallback config after parse failure: %v", err)
	}
}
