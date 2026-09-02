package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoveryCommandRejectsInvalidModeBeforeNetworkUse(t *testing.T) {
	handled, code := runDiscoveryCommand([]string{"discovery", "--ping-mode", "invented"})
	if !handled || code != 2 {
		t.Fatalf("handled/code = %t/%d, want true/2", handled, code)
	}
}

func TestWriteDiscoveryCLIOutputProtectsExistingExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery.csv")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeDiscoveryCLIOutput(path, []byte("replacement"), false); err == nil {
		t.Fatal("existing export was overwritten without --force")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "original" {
		t.Fatalf("protected export = %q", contents)
	}
	if err := writeDiscoveryCLIOutput(path, []byte("replacement"), true); err != nil {
		t.Fatal(err)
	}
	contents, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "replacement" {
		t.Fatalf("replaced export = %q", contents)
	}
}

func TestWriteDiscoveryCLIOutputCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "discovery.json")
	if err := writeDiscoveryCLIOutput(path, []byte("{}\n"), false); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "{}\n" {
		t.Fatalf("created export = %q, %v", contents, err)
	}
}
