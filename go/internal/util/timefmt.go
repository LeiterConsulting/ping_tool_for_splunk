package util

import (
	"time"
)

// FormatDotNetO formats time like .NET DateTime "o" (round-trip):
// yyyy-MM-ddTHH:mm:ss.fffffffK (we emit UTC with Z).
func FormatDotNetO(t time.Time) string {
	t = t.UTC()
	// 7 fractional digits = 100ns ticks. Go has ns; truncate to 100ns.
	frac := t.Nanosecond() / 100 // 0..9,999,999
	base := t.Format("2006-01-02T15:04:05")
	return base + "." + pad7(frac) + "Z"
}

func pad7(v int) string {
	b := [7]byte{'0', '0', '0', '0', '0', '0', '0'}
	for i := 6; i >= 0; i-- {
		b[i] = byte('0' + (v % 10))
		v /= 10
	}
	return string(b[:])
}

// UnixSecondsFromISO tries to parse .NET "o" timestamps, falling back safely.
func UnixSecondsFromISO(ts string) int64 {
	return int64(UnixTimeFromISO(ts))
}

// UnixTimeFromISO preserves subsecond precision for Splunk HEC event time.
func UnixTimeFromISO(ts string) float64 {
	if ts == "" {
		return float64(time.Now().UTC().UnixNano()) / float64(time.Second)
	}
	// Accept RFC3339Nano as well; time.Parse handles offsets.
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return float64(t.UnixNano()) / float64(time.Second)
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return float64(t.UnixNano()) / float64(time.Second)
	}
	return float64(time.Now().UTC().UnixNano()) / float64(time.Second)
}
