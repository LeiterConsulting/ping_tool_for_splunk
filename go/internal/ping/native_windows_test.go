//go:build windows

package ping

import (
	"context"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsPingerLoopback(t *testing.T) {
	pinger := &WindowsPinger{}
	results, err := pinger.Ping(context.Background(), "127.0.0.1", 2, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("Ping(loopback) error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for index, result := range results {
		if !result.Success || result.Sequence != index+1 || (!result.LatencyMeasured && !result.LatencyCensored) {
			t.Fatalf("result[%d] = %#v", index, result)
		}
	}
}

func TestClassifyWindowsCallError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{syscall.Errno(ipRequestTimedOut), "timeout"},
		{syscall.Errno(ipDestinationHost), "unreachable"},
		{windows.ERROR_INVALID_PARAMETER, "backend_error"},
		{syscall.Errno(12345), "backend_error"},
	}
	for _, test := range tests {
		if got := classifyWindowsCallError(test.err); got != test.want {
			t.Errorf("classifyWindowsCallError(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}
