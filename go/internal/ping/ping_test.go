package ping

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestProbeIntervalIsBounded(t *testing.T) {
	if got := probeInterval(100 * time.Millisecond); got != 50*time.Millisecond {
		t.Fatalf("probeInterval(100ms) = %v, want 50ms", got)
	}
	if got := probeInterval(time.Second); got != 250*time.Millisecond {
		t.Fatalf("probeInterval(1s) = %v, want 250ms", got)
	}
}

func TestParsePingOutputPreservesAvailablePrecision(t *testing.T) {
	var output string
	if runtime.GOOS == "windows" {
		output = "Reply from 127.0.0.1: bytes=32 time<1ms TTL=128"
	} else {
		output = "64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.041 ms"
	}
	measurement, _, ok := parsePingOutput(output)
	if !ok {
		t.Fatal("parsePingOutput() did not recognize a reply")
	}
	if runtime.GOOS == "windows" {
		if measurement.Measured || !measurement.Censored || measurement.UpperBoundMs != 1 || measurement.ResolutionMs != 1 {
			t.Fatalf("sub-millisecond Windows result was not preserved as a censored bound: %#v", measurement)
		}
	} else if !measurement.Measured || measurement.ValueMs != 0.041 || measurement.ResolutionMs != 0.001 {
		t.Fatalf("Unix latency precision was not preserved: %#v", measurement)
	}
}

func TestSummarizePingErrorClassifiesMissingExecutable(t *testing.T) {
	err := &exec.Error{Name: "ping", Err: errors.New("not found")}
	if got := summarizePingError(err, ""); got != "backend_unavailable" {
		t.Fatalf("summarizePingError() = %q, want backend_unavailable", got)
	}
}
