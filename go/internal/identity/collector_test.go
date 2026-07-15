package identity

import (
	"path/filepath"
	"testing"
)

func TestLoadOrCreateCollectorIDIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector_id")
	first, err := LoadOrCreateCollectorID(path)
	if err != nil {
		t.Fatalf("LoadOrCreateCollectorID(first) error = %v", err)
	}
	second, err := LoadOrCreateCollectorID(path)
	if err != nil {
		t.Fatalf("LoadOrCreateCollectorID(second) error = %v", err)
	}
	if first == "" || first != second {
		t.Fatalf("collector IDs = %q and %q, want one stable non-empty value", first, second)
	}
}
