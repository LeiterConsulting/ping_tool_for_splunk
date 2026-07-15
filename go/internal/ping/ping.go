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
	Sequence            int
	SentAt              time.Time
	ReceivedAt          time.Time
	Timestamp           time.Time
	Success             bool
	LatencyMs           float64
	LatencyMeasured     bool
	LatencySource       string
	LatencyResolutionMs float64
	LatencyCensored     bool
	LatencyUpperBoundMs float64
	ProbeElapsedMs      float64
	ICMPStatusCode      *uint32
	TTL                 int
	Error               string
	Backend             string
}

type Pinger interface {
	Ping(ctx context.Context, ip string, count int, perPingTimeout time.Duration) ([]PingResult, error)
	Backend() string
}

type GoPinger struct {
	fallback    Pinger
	onFallback  func(ip string, from string, to string, reason string)
	rawDisabled atomic.Bool
}

func NewPinger(mode string, onFallback func(ip string, from string, to string, reason string)) Pinger {
	return newPinger(mode, onFallback)
}

func (p *GoPinger) Backend() string {
	if p.fallback != nil && p.rawDisabled.Load() {
		return p.fallback.Backend()
	}
	return "udp_icmp"
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
	pg.Interval = probeInterval(perPingTimeout)
	pg.Timeout = time.Duration(count-1)*pg.Interval + perPingTimeout + 50*time.Millisecond

	sequenceOrder := make([]int, 0, count)
	sentAt := make(map[int]time.Time, count)
	received := make(map[int]PingResult, count)
	pg.OnSend = func(pkt *ping.Packet) {
		sequenceOrder = append(sequenceOrder, pkt.Seq)
		sentAt[pkt.Seq] = time.Now()
	}

	pg.OnRecv = func(pkt *ping.Packet) {
		ttl := -1
		// Packet.Ttl is set on some platforms.
		if pkt.Ttl > 0 {
			ttl = pkt.Ttl
		}
		receivedAt := time.Now()
		received[pkt.Seq] = PingResult{
			Sequence:            pkt.Seq,
			SentAt:              sentAt[pkt.Seq],
			ReceivedAt:          receivedAt,
			Timestamp:           receivedAt,
			Success:             true,
			LatencyMs:           float64(pkt.Rtt) / float64(time.Millisecond),
			LatencyMeasured:     true,
			LatencySource:       "go_ping_rtt",
			LatencyResolutionMs: 0.001,
			TTL:                 ttl,
			Backend:             "udp_icmp",
		}
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

	completedAt := time.Now()
	for len(sequenceOrder) < count {
		sequenceOrder = append(sequenceOrder, -1-len(sequenceOrder))
	}
	results := make([]PingResult, 0, count)
	for index, sequence := range sequenceOrder[:count] {
		if result, ok := received[sequence]; ok {
			result.Sequence = index + 1
			results = append(results, result)
			continue
		}
		sent := sentAt[sequence]
		if sent.IsZero() {
			sent = time.Now()
		}
		results = append(results, PingResult{
			Sequence:  index + 1,
			SentAt:    sent,
			Timestamp: completedAt,
			Success:   false,
			TTL:       -1,
			Error:     "timeout",
			Backend:   "udp_icmp",
		})
	}
	return results, nil
}

func probeInterval(perPingTimeout time.Duration) time.Duration {
	interval := perPingTimeout / 2
	if interval < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	if interval > 250*time.Millisecond {
		return 250 * time.Millisecond
	}
	return interval
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

func (p *ExecPinger) Backend() string { return "exec" }

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
	interval := probeInterval(perPingTimeout)
	lastStarted := time.Time{}
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !lastStarted.IsZero() {
			remaining := interval - time.Since(lastStarted)
			if remaining > 0 {
				timer := time.NewTimer(remaining)
				select {
				case <-timer.C:
				case <-ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					return nil, ctx.Err()
				}
			}
		}
		lastStarted = time.Now()
		pr := p.pingOnce(ctx, ip, i+1, perPingTimeout)
		results = append(results, pr)
	}
	return results, nil
}

func (p *ExecPinger) pingOnce(ctx context.Context, ip string, sequence int, perPingTimeout time.Duration) PingResult {
	sentAt := time.Now()
	cmd, args := buildPingCommand(ip, perPingTimeout)
	attemptCtx, cancel := context.WithTimeout(ctx, perPingTimeout+500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(attemptCtx, cmd, args...).CombinedOutput()
	completedAt := time.Now()
	text := string(out)
	if err != nil {
		// Non-zero exit often means timeout/unreachable.
		return PingResult{Sequence: sequence, SentAt: sentAt, Timestamp: completedAt, Success: false, TTL: -1, Error: summarizePingError(err, text), Backend: "exec", ProbeElapsedMs: float64(completedAt.Sub(sentAt)) / float64(time.Millisecond)}
	}

	measurement, ttl, ok := parsePingOutput(text)
	if !ok {
		// Some ping variants still return 0 even if all failed.
		if strings.Contains(strings.ToLower(text), "timed out") || strings.Contains(strings.ToLower(text), "unreachable") {
			return PingResult{Sequence: sequence, SentAt: sentAt, Timestamp: completedAt, Success: false, TTL: -1, Error: "timeout", Backend: "exec", ProbeElapsedMs: float64(completedAt.Sub(sentAt)) / float64(time.Millisecond)}
		}
		return PingResult{Sequence: sequence, SentAt: sentAt, Timestamp: completedAt, Success: false, TTL: -1, Error: "backend_parse_error", Backend: "exec", ProbeElapsedMs: float64(completedAt.Sub(sentAt)) / float64(time.Millisecond)}
	}
	return PingResult{
		Sequence: sequence, SentAt: sentAt, ReceivedAt: completedAt, Timestamp: completedAt, Success: true,
		LatencyMs: measurement.ValueMs, LatencyMeasured: measurement.Measured,
		LatencySource: measurement.Source, LatencyResolutionMs: measurement.ResolutionMs,
		LatencyCensored: measurement.Censored, LatencyUpperBoundMs: measurement.UpperBoundMs,
		ProbeElapsedMs: float64(completedAt.Sub(sentAt)) / float64(time.Millisecond), TTL: ttl, Backend: "exec",
	}
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

type latencyMeasurement struct {
	ValueMs      float64
	Measured     bool
	Source       string
	ResolutionMs float64
	Censored     bool
	UpperBoundMs float64
}

func parsePingOutput(text string) (measurement latencyMeasurement, ttl int, ok bool) {
	ttl = -1

	if runtime.GOOS == "windows" {
		if m := winTimeRe.FindStringSubmatch(text); len(m) == 2 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				if strings.Contains(strings.ToLower(m[0]), "time<") {
					measurement = latencyMeasurement{Source: "os_ping_reported", ResolutionMs: 1, Censored: true, UpperBoundMs: v}
				} else {
					measurement = latencyMeasurement{ValueMs: v, Measured: true, Source: "os_ping_reported", ResolutionMs: 1}
				}
			}
		}
		if m := winTTLRe.FindStringSubmatch(text); len(m) == 2 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				ttl = v
			}
		}
		return measurement, ttl, measurement.Measured || measurement.Censored
	}

	if m := unixTimeRe.FindStringSubmatch(text); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			resolution := decimalResolution(m[1])
			measurement = latencyMeasurement{ValueMs: v, Measured: true, Source: "os_ping_reported", ResolutionMs: resolution}
		}
	}
	if m := unixTTLRe.FindStringSubmatch(text); len(m) == 2 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			ttl = v
		}
	}
	return measurement, ttl, measurement.Measured
}

func decimalResolution(value string) float64 {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return 1
	}
	resolution := 1.0
	for range parts[1] {
		resolution /= 10
	}
	return resolution
}

func summarizePingError(err error, output string) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var executableError *exec.Error
	if errors.As(err, &executableError) {
		return "backend_unavailable"
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
