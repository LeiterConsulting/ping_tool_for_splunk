//go:build !windows

package systeminfo

import (
	"os"
	"time"
)

func collectPlatform(_ platformCounters, _ time.Duration) (ProcessSnapshot, HostSnapshot, platformCounters, []string) {
	return ProcessSnapshot{PID: os.Getpid()}, HostSnapshot{}, platformCounters{}, []string{
		"process_cpu: unavailable on this platform",
		"process_memory: unavailable on this platform",
		"process_handles: unavailable on this platform",
		"process_threads: unavailable on this platform",
		"host_cpu: unavailable on this platform",
		"host_memory: unavailable on this platform",
	}
}
