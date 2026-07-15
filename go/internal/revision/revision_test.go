package revision

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileTracksContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := File(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := File(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("revision did not change: %q", first)
	}
}
