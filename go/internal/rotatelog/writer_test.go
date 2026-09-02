package rotatelog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriterRotatesAndBoundsArchives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	w, err := New(path, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first\n", "second\n", "third\n", "fourth\n"} {
		if _, err := w.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("expected %s: %v", expected, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected archive beyond retention: %v", err)
	}
}

func TestWriterRotatesOversizedExistingFileAtOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	if err := os.WriteFile(path, []byte("already too large"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := New(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("new active log = %#v, %v", info, err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("startup archive missing: %v", err)
	}
}
