package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

func TestConfigUpgradeCheckDoesNotWriteAndApplyCreatesSchemaV2(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "config.psd1")
	target := filepath.Join(root, "config.json")
	cfg := config.Defaults(root)
	if _, err := config.SaveConfig(t.Context(), source, root, cfg); err != nil {
		t.Fatal(err)
	}
	handled, exitCode := runConfigCommand([]string{"config", "upgrade", "--config", source, "--to", target, "--check"})
	if !handled || exitCode != 0 {
		t.Fatalf("check handled=%t exit=%d", handled, exitCode)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("check wrote target: %v", err)
	}
	handled, exitCode = runConfigCommand([]string{"config", "upgrade", "--config", source, "--to", target, "--apply"})
	if !handled || exitCode != 0 {
		t.Fatalf("apply handled=%t exit=%d", handled, exitCode)
	}
	upgraded, _, err := config.LoadEditable(t.Context(), target, root)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.ConfigSchemaVersion != config.CurrentSchemaVersion {
		t.Fatalf("schema = %d", upgraded.ConfigSchemaVersion)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was removed: %v", err)
	}
}
