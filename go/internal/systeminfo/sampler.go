package systeminfo

import (
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const defaultCacheDuration = 2 * time.Second

type Snapshot struct {
	ObservedAt        string          `json:"observed_at"`
	SampleAgeMs       int64           `json:"sample_age_ms"`
	Platform          string          `json:"platform"`
	Architecture      string          `json:"architecture"`
	LogicalCPUs       int             `json:"logical_cpus"`
	Process           ProcessSnapshot `json:"process"`
	Host              HostSnapshot    `json:"host"`
	Go                GoSnapshot      `json:"go_runtime"`
	UnavailableFields []string        `json:"unavailable_fields,omitempty"`
}

type ProcessSnapshot struct {
	PID                      int      `json:"pid"`
	CPUHostCapacityPercent   *float64 `json:"cpu_host_capacity_percent"`
	CPUCoreEquivalentPercent *float64 `json:"cpu_core_equivalent_percent"`
	WorkingSetBytes          *uint64  `json:"working_set_bytes"`
	PeakWorkingSetBytes      *uint64  `json:"peak_working_set_bytes"`
	PrivateBytes             *uint64  `json:"private_bytes"`
	CommittedBytes           *uint64  `json:"committed_bytes"`
	HandleCount              *uint64  `json:"handle_count"`
	ThreadCount              *uint64  `json:"thread_count"`
}

type HostSnapshot struct {
	CPUPercent           *float64 `json:"cpu_percent"`
	TotalMemoryBytes     *uint64  `json:"total_memory_bytes"`
	AvailableMemoryBytes *uint64  `json:"available_memory_bytes"`
	MemoryUsedPercent    *float64 `json:"memory_used_percent"`
}

type GoSnapshot struct {
	HeapAllocBytes    uint64   `json:"heap_alloc_bytes"`
	HeapInUseBytes    uint64   `json:"heap_in_use_bytes"`
	HeapSysBytes      uint64   `json:"heap_sys_bytes"`
	StackInUseBytes   uint64   `json:"stack_in_use_bytes"`
	TotalSysBytes     uint64   `json:"total_sys_bytes"`
	NextGCBytes       uint64   `json:"next_gc_bytes"`
	MemoryLimitBytes  *int64   `json:"memory_limit_bytes"`
	MemoryLimitSource string   `json:"memory_limit_source"`
	Goroutines        int      `json:"goroutines"`
	GOMAXPROCS        int      `json:"gomaxprocs"`
	GCCycles          uint32   `json:"gc_cycles"`
	GCPauseTotalMs    float64  `json:"gc_pause_total_ms"`
	GCLastPauseMs     *float64 `json:"gc_last_pause_ms"`
	GCCPUFraction     float64  `json:"gc_cpu_fraction"`
}

// Sampler keeps OS queries behind a short cache. Multiple open dashboard tabs
// therefore observe the same sample instead of multiplying process-wide calls.
type Sampler struct {
	mu            sync.Mutex
	cacheDuration time.Duration
	observedAt    time.Time
	current       Snapshot
	previous      platformCounters
}

func NewSampler() *Sampler {
	return NewSamplerWithCache(defaultCacheDuration)
}

func NewSamplerWithCache(cacheDuration time.Duration) *Sampler {
	if cacheDuration < 0 {
		cacheDuration = 0
	}
	s := &Sampler{cacheDuration: cacheDuration}
	s.collectLocked(time.Now())
	return s
}

func (s *Sampler) Snapshot() Snapshot {
	if s == nil {
		s = NewSampler()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.observedAt.IsZero() || now.Sub(s.observedAt) >= s.cacheDuration {
		s.collectLocked(now)
	}
	result := s.current
	result.SampleAgeMs = max(0, time.Since(s.observedAt).Milliseconds())
	result.UnavailableFields = append([]string(nil), s.current.UnavailableFields...)
	return result
}

func (s *Sampler) collectLocked(now time.Time) {
	elapsed := time.Duration(0)
	if !s.observedAt.IsZero() {
		elapsed = now.Sub(s.observedAt)
	}
	process, host, counters, unavailable := collectPlatform(s.previous, elapsed)

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	limit := debug.SetMemoryLimit(-1)
	var memoryLimit *int64
	limitSource := "runtime_default"
	if limit < math.MaxInt64 {
		value := limit
		memoryLimit = &value
		if os.Getenv("GOMEMLIMIT") != "" {
			limitSource = "GOMEMLIMIT"
		} else {
			limitSource = "runtime_configured"
		}
	}
	var lastPause *float64
	if stats.NumGC > 0 {
		value := float64(stats.PauseNs[(stats.NumGC+255)%256]) / float64(time.Millisecond)
		lastPause = &value
	}

	s.observedAt = now
	s.previous = counters
	s.current = Snapshot{
		ObservedAt:        now.UTC().Format(time.RFC3339Nano),
		Platform:          runtime.GOOS,
		Architecture:      runtime.GOARCH,
		LogicalCPUs:       runtime.NumCPU(),
		Process:           process,
		Host:              host,
		UnavailableFields: unavailable,
		Go: GoSnapshot{
			HeapAllocBytes:    stats.HeapAlloc,
			HeapInUseBytes:    stats.HeapInuse,
			HeapSysBytes:      stats.HeapSys,
			StackInUseBytes:   stats.StackInuse,
			TotalSysBytes:     stats.Sys,
			NextGCBytes:       stats.NextGC,
			MemoryLimitBytes:  memoryLimit,
			MemoryLimitSource: limitSource,
			Goroutines:        runtime.NumGoroutine(),
			GOMAXPROCS:        runtime.GOMAXPROCS(0),
			GCCycles:          stats.NumGC,
			GCPauseTotalMs:    float64(stats.PauseTotalNs) / float64(time.Millisecond),
			GCLastPauseMs:     lastPause,
			GCCPUFraction:     stats.GCCPUFraction,
		},
	}
}
