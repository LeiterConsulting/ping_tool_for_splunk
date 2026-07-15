package singleinstance

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireRejectsSecondOwnerAndAllowsReacquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire(first) error = %v", err)
	}
	defer first.Close()

	second, err := Acquire(path)
	if second != nil {
		_ = second.Close()
		t.Fatal("Acquire(second) unexpectedly returned a lock")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Acquire(second) error = %v, want ErrAlreadyRunning", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}
	reacquired, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire(after close) error = %v", err)
	}
	if err := reacquired.Close(); err != nil {
		t.Fatalf("Close(reacquired) error = %v", err)
	}
}
