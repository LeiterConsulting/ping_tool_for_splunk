package systeminfo

import (
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestSamplerReportsGoRuntimeAndUsesCache(t *testing.T) {
	sampler := NewSamplerWithCache(time.Hour)
	first := sampler.Snapshot()
	second := sampler.Snapshot()
	if first.ObservedAt == "" || first.Platform == "" || first.Architecture == "" {
		t.Fatalf("missing identity fields: %#v", first)
	}
	if first.Go.HeapAllocBytes == 0 || first.Go.TotalSysBytes == 0 || first.Go.Goroutines < 1 {
		t.Fatalf("missing Go runtime metrics: %#v", first.Go)
	}
	if first.Process.PID <= 0 || first.LogicalCPUs < 1 {
		t.Fatalf("missing process fields: %#v", first)
	}
	if second.ObservedAt != first.ObservedAt {
		t.Fatalf("cached sample changed: first=%q second=%q", first.ObservedAt, second.ObservedAt)
	}
	if first.Platform == "windows" && !containsUnavailable(first.UnavailableFields, "awaiting a second timed sample") {
		t.Fatalf("first Windows CPU sample must disclose warm-up state: %#v", first.UnavailableFields)
	}
}

func containsUnavailable(fields []string, fragment string) bool {
	for _, field := range fields {
		if strings.Contains(field, fragment) {
			return true
		}
	}
	return false
}

func TestSamplerRefreshesExpiredSample(t *testing.T) {
	sampler := NewSamplerWithCache(0)
	first := sampler.Snapshot()
	time.Sleep(time.Millisecond)
	second := sampler.Snapshot()
	if second.ObservedAt == first.ObservedAt {
		t.Fatalf("expired sample did not refresh: %q", first.ObservedAt)
	}
}

func TestSamplerReportsConfiguredGoMemoryLimit(t *testing.T) {
	previous := debug.SetMemoryLimit(64 * 1024 * 1024)
	defer debug.SetMemoryLimit(previous)
	t.Setenv("GOMEMLIMIT", "64MiB")

	snapshot := NewSamplerWithCache(time.Hour).Snapshot()
	if snapshot.Go.MemoryLimitBytes == nil || *snapshot.Go.MemoryLimitBytes != 64*1024*1024 {
		t.Fatalf("memory limit = %v", snapshot.Go.MemoryLimitBytes)
	}
	if snapshot.Go.MemoryLimitSource != "GOMEMLIMIT" {
		t.Fatalf("memory limit source = %q", snapshot.Go.MemoryLimitSource)
	}
}
