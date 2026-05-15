package ping

import (
	"context"
	"errors"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-ping/ping"
)

type PingResult struct {
	Timestamp time.Time
	Success   bool
	LatencyMs int
	TTL       int
	Error     string
}

type Pinger interface {
	Ping(ctx context.Context, ip string, count int, perPingTimeout time.Duration) ([]PingResult, error)
}

type GoPinger struct {
	fallback    Pinger
	onFallback  func(ip string, from string, to string, reason string)
	rawDisabled atomic.Bool
}

func NewPinger(mode string, onFallback func(ip string, from string, to string, reason string)) Pinger {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "exec":
		return &ExecPinger{}
	case "raw":
		return &GoPinger{fallback: nil, onFallback: onFallback}
	case "", "auto":
		return &GoPinger{fallback: &ExecPinger{}, onFallback: onFallback}
	default:
		// Unknown value: fail-safe to auto.
		return &GoPinger{fallback: &ExecPinger{}, onFallback: onFallback}
	}
}

func (p *GoPinger) Ping(ctx context.Context, ip string, count int, perPingTimeout time.Duration) ([]PingResult, error) {
	// If raw ICMP is known to be unavailable in this runtime environment,
	// short-circuit directly to exec mode (when configured).
	if p.fallback != nil && p.rawDisabled.Load() {
		return p.fallback.Ping(ctx, ip, count, perPingTimeout)
	}

	pg, err := ping.NewPinger(ip)
	if err != nil {
		return nil, err
	}
	// Use unprivileged mode when possible; go-ping will fall back as needed.
	pg.SetPrivileged(false)
	pg.Count = count
	pg.Interval = 10 * time.Millisecond
	pg.Timeout = time.Duration(count)*perPingTimeout + 250*time.Millisecond

	results := make([]PingResult, 0, count)
	seen := 0

	pg.OnRecv = func(pkt *ping.Packet) {
		seen++
		ttl := -1
		// Packet.Ttl is set on some platforms.
		if pkt.Ttl > 0 {
			ttl = pkt.Ttl
		}
		results = append(results, PingResult{
			Timestamp: time.Now(),
			Success:   true,
			LatencyMs: int(pkt.Rtt.Milliseconds()),
			TTL:       ttl,
			Error:     "",
		})
	}

	// If we don't receive, we'll fill failures after Run.
	errCh := make(chan error, 1)
	go func() {
		errCh <- pg.Run()
	}()

	select {
	case <-ctx.Done():
		pg.Stop()
		return nil, ctx.Err()
	case err := <-errCh:
		if err != nil {
			// Common on Windows or locked-down environments: no ICMP socket support.
			if p.fallback != nil && looksLikeICMPUnavailable(err) {
				// Only announce the fallback once; after that we run purely in exec mode.
				if p.rawDisabled.CompareAndSwap(false, true) {
					if p.onFallback != nil {
						p.onFallback(ip, "raw", "exec", err.Error())
					}
				}
				return p.fallback.Ping(ctx, ip, count, perPingTimeout)
			}
			return nil, err
		}
	}

	// Fill missing results as failures.
	for i := seen; i < count; i++ {
		results = append(results, PingResult{
			Timestamp: time.Now(),
			Success:   false,
			LatencyMs: -1,
			TTL:       -1,
			Error:     "timeout",
		})
	}
	return results, nil
}

func looksLikeICMPUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "requested protocol") {
		return true
	}
	if strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission") {
		return true
	}
	if strings.Contains(msg, "protocol not supported") {
		return true
	}
	return false
}

// ExecPinger uses the OS `ping` binary as a compatibility fallback.
// It is slower than raw ICMP, but works in many restricted environments.
type ExecPinger struct{}

var (
	winTimeRe  = regexp.MustCompile(`(?i)time[=<]\s*(\d+)\s*ms`)
	winTTLRe   = regexp.MustCompile(`(?i)ttl=(\d+)`)
	unixTimeRe = regexp.MustCompile(`(?i)time=\s*([0-9.]+)\s*ms`)
	unixTTLRe  = regexp.MustCompile(`(?i)ttl=\s*(\d+)`)
)

func (p *ExecPinger) Ping(ctx context.Context, ip string, count int, perPingTimeout time.Duration) ([]PingResult, error) {
	if count < 1 {
		count = 1
	}
	results := make([]PingResult, 0, count)
	for i := 0; i < count; i++ {
		pr := p.pingOnce(ctx, ip, perPingTimeout)
		results = append(results, pr)
	}
	return results, nil
}

func (p *ExecPinger) pingOnce(ctx context.Context, ip string, perPingTimeout time.Duration) PingResult {
	t := time.Now()
	cmd, args := buildPingCommand(ip, perPingTimeout)
	out, err := exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	text := string(out)
	if err != nil {
		// Non-zero exit often means timeout/unreachable.
		return PingResult{Timestamp: t, Success: false, LatencyMs: -1, TTL: -1, Error: summarizePingError(err, text)}
	}

	lat, ttl, ok := parsePingOutput(text)
	if !ok {
		// Some ping variants still return 0 even if all failed.
		if strings.Contains(strings.ToLower(text), "timed out") || strings.Contains(strings.ToLower(text), "unreachable") {
			return PingResult{Timestamp: t, Success: false, LatencyMs: -1, TTL: -1, Error: "timeout"}
		}
		return PingResult{Timestamp: t, Success: false, LatencyMs: -1, TTL: -1, Error: "no_reply"}
	}
	return PingResult{Timestamp: t, Success: true, LatencyMs: lat, TTL: ttl, Error: ""}
}

func buildPingCommand(ip string, perPingTimeout time.Duration) (string, []string) {
	ms := int(perPingTimeout.Milliseconds())
	if ms < 1 {
		ms = 1000
	}
	switch runtime.GOOS {
	case "windows":
		// -n 1 = one echo request; -w timeout in ms
		return "ping", []string{"-n", "1", "-w", strconv.Itoa(ms), ip}
	case "darwin":
		// macOS: -c 1 one packet; -W timeout in ms (supported on modern macOS)
		return "ping", []string{"-c", "1", "-W", strconv.Itoa(ms), ip}
	default:
		// Linux: -c 1 one packet; -W timeout in seconds (integer)
		sec := int(math.Ceil(float64(ms) / 1000.0))
		if sec < 1 {
			sec = 1
		}
		return "ping", []string{"-c", "1", "-W", strconv.Itoa(sec), ip}
	}
}

func parsePingOutput(text string) (latencyMs int, ttl int, ok bool) {
	ttl = -1
	latencyMs = -1

	if runtime.GOOS == "windows" {
		if m := winTimeRe.FindStringSubmatch(text); len(m) == 2 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				latencyMs = v
			}
		}
		if m := winTTLRe.FindStringSubmatch(text); len(m) == 2 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				ttl = v
			}
		}
		return latencyMs, ttl, latencyMs >= 0
	}

	if m := unixTimeRe.FindStringSubmatch(text); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			latencyMs = int(math.Round(v))
		}
	}
	if m := unixTTLRe.FindStringSubmatch(text); len(m) == 2 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			ttl = v
		}
	}
	return latencyMs, ttl, latencyMs >= 0
}

func summarizePingError(err error, output string) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	low := strings.ToLower(output)
	if strings.Contains(low, "timed out") {
		return "timeout"
	}
	if strings.Contains(low, "unreachable") {
		return "unreachable"
	}
	if strings.Contains(low, "could not find host") || strings.Contains(low, "unknown host") {
		return "dns"
	}
	return "ping_failed"
}
