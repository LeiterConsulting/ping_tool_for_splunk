package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntimePath_DefaultPrefersExecutableRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	rootConfig := filepath.Join(root, "config.psd1")
	if err := os.WriteFile(rootConfig, []byte("@{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer os.Chdir(original)
	if err := os.Chdir(other); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	resolved := resolveRuntimePath("config.psd1", root, "config.psd1")
	if resolved != rootConfig {
		t.Fatalf("resolveRuntimePath() = %q, want %q", resolved, rootConfig)
	}
}

func TestResolveRuntimePath_CustomRelativePrefersCurrentWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	cwdConfig := filepath.Join(other, "custom.psd1")
	if err := os.WriteFile(cwdConfig, []byte("@{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer os.Chdir(original)
	if err := os.Chdir(other); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	resolved := resolveRuntimePath("custom.psd1", root, "config.psd1")
	if resolved != "custom.psd1" {
		t.Fatalf("resolveRuntimePath() = %q, want %q", resolved, "custom.psd1")
	}
}

func TestResolveRuntimePath_DefaultFallsBackToExecutableRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer os.Chdir(original)
	if err := os.Chdir(other); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	resolved := resolveRuntimePath("endpoints.csv", root, "endpoints.csv")
	want := filepath.Join(root, "endpoints.csv")
	if resolved != want {
		t.Fatalf("resolveRuntimePath() = %q, want %q", resolved, want)
	}
}
