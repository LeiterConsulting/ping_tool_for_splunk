//go:build windows

package ping

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ipSuccess              = 0
	ipDestinationNet       = 11002
	ipDestinationHost      = 11003
	ipDestinationProto     = 11004
	ipDestinationPort      = 11005
	ipRequestTimedOut      = 11010
	ipPacketTooBig         = 11009
	ipBadRoute             = 11012
	ipTTLExpiredTransit    = 11013
	ipTTLExpiredReassembly = 11014
	ipParamProblem         = 11015
	ipSourceQuench         = 11016
	ipBadDestination       = 11018
)

var (
	iphlpapi         = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreate   = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpClose    = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho = iphlpapi.NewProc("IcmpSendEcho")
)

type ipOptionInformation struct {
	TTL         byte
	TOS         byte
	Flags       byte
	OptionsSize byte
	OptionsData *byte
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

type WindowsPinger struct {
	fallback          Pinger
	onFallback        func(ip string, from string, to string, reason string)
	nativeDisabled    atomic.Bool
	fallbackAnnounced atomic.Bool
}

func (p *WindowsPinger) Backend() string {
	if p.nativeDisabled.Load() && p.fallback != nil {
		return p.fallback.Backend()
	}
	return "windows_icmp"
}

func (p *WindowsPinger) Ping(ctx context.Context, target string, count int, perPingTimeout time.Duration) ([]PingResult, error) {
	if p.nativeDisabled.Load() && p.fallback != nil {
		return p.fallback.Ping(ctx, target, count, perPingTimeout)
	}
	ip := net.ParseIP(target)
	if ip == nil || ip.To4() == nil {
		if p.fallback != nil {
			return p.fallback.Ping(ctx, target, count, perPingTimeout)
		}
		return nil, fmt.Errorf("windows native ICMP currently requires an IPv4 target: %s", target)
	}

	handle, _, createErr := procIcmpCreate.Call()
	if handle == uintptr(windows.InvalidHandle) {
		return p.fallbackOrError(ctx, target, count, perPingTimeout, createErr)
	}
	defer procIcmpClose.Call(handle)

	if count < 1 {
		count = 1
	}
	interval := probeInterval(perPingTimeout)
	results := make([]PingResult, 0, count)
	lastStarted := time.Time{}
	for sequence := 1; sequence <= count; sequence++ {
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
						select {
						case <-timer.C:
						default:
						}
					}
					return nil, ctx.Err()
				}
			}
		}
		lastStarted = time.Now()
		results = append(results, sendWindowsEcho(handle, ip.To4(), sequence, perPingTimeout))
	}
	return results, nil
}

func (p *WindowsPinger) fallbackOrError(ctx context.Context, target string, count int, timeout time.Duration, cause error) ([]PingResult, error) {
	if p.fallback == nil {
		return nil, fmt.Errorf("windows ICMP API unavailable: %w", cause)
	}
	p.nativeDisabled.Store(true)
	if p.fallbackAnnounced.CompareAndSwap(false, true) && p.onFallback != nil {
		p.onFallback(target, "windows_icmp", p.fallback.Backend(), cause.Error())
	}
	return p.fallback.Ping(ctx, target, count, timeout)
}

func sendWindowsEcho(handle uintptr, ip net.IP, sequence int, timeout time.Duration) PingResult {
	payload := []byte("pingmonitor-v2")
	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + len(payload) + 8
	replyBuffer := make([]byte, replySize)
	destination := binary.LittleEndian.Uint32(ip)
	timeoutMs := timeout.Milliseconds()
	if timeoutMs < 1 {
		timeoutMs = 1
	}
	if timeoutMs > int64(^uint32(0)) {
		timeoutMs = int64(^uint32(0))
	}

	sentAt := time.Now()
	replies, _, callErr := procIcmpSendEcho.Call(
		handle,
		uintptr(destination),
		uintptr(unsafe.Pointer(&payload[0])),
		uintptr(uint16(len(payload))),
		0,
		uintptr(unsafe.Pointer(&replyBuffer[0])),
		uintptr(len(replyBuffer)),
		uintptr(uint32(timeoutMs)),
	)
	completedAt := time.Now()
	base := PingResult{
		Sequence: sequence, SentAt: sentAt, Timestamp: completedAt, TTL: -1, Backend: "windows_icmp",
		ProbeElapsedMs: float64(completedAt.Sub(sentAt)) / float64(time.Millisecond),
	}
	if replies == 0 {
		base.Error = classifyWindowsCallError(callErr)
		if code, ok := windowsErrorCode(callErr); ok {
			base.ICMPStatusCode = &code
		}
		return base
	}

	reply := (*icmpEchoReply)(unsafe.Pointer(&replyBuffer[0]))
	statusCode := reply.Status
	base.ICMPStatusCode = &statusCode
	if reply.Status != ipSuccess {
		switch reply.Status {
		case ipRequestTimedOut:
			base.Error = "timeout"
		case ipDestinationNet, ipDestinationHost, ipDestinationProto, ipDestinationPort, ipBadRoute, ipParamProblem, ipSourceQuench, ipBadDestination:
			base.Error = "unreachable"
		case ipPacketTooBig:
			base.Error = "packet_too_big"
		case ipTTLExpiredTransit, ipTTLExpiredReassembly:
			base.Error = "ttl_expired"
		default:
			base.Error = fmt.Sprintf("icmp_status_%d", reply.Status)
		}
		return base
	}

	base.LatencySource = "windows_icmp_rtt"
	base.LatencyResolutionMs = 1
	if reply.RoundTripTime == 0 {
		base.LatencyCensored = true
		base.LatencyUpperBoundMs = 1
	} else {
		base.LatencyMs = float64(reply.RoundTripTime)
		base.LatencyMeasured = true
	}
	base.Success = true
	base.ReceivedAt = completedAt
	base.TTL = int(reply.Options.TTL)
	return base
}

func classifyWindowsCallError(callErr error) string {
	var errno syscall.Errno
	if !errors.As(callErr, &errno) {
		return "backend_error"
	}
	code := uint32(errno)
	switch code {
	case 0, ipRequestTimedOut, uint32(windows.ERROR_TIMEOUT):
		return "timeout"
	case ipDestinationNet, ipDestinationHost, ipDestinationProto, ipDestinationPort, ipBadRoute, ipParamProblem, ipSourceQuench, ipBadDestination:
		return "unreachable"
	case ipPacketTooBig:
		return "packet_too_big"
	case ipTTLExpiredTransit, ipTTLExpiredReassembly:
		return "ttl_expired"
	case uint32(windows.ERROR_ACCESS_DENIED), uint32(windows.ERROR_INVALID_HANDLE),
		uint32(windows.ERROR_INVALID_PARAMETER), uint32(windows.ERROR_INSUFFICIENT_BUFFER),
		uint32(windows.ERROR_NOT_ENOUGH_MEMORY):
		return "backend_error"
	default:
		return "backend_error"
	}
}

func windowsErrorCode(callErr error) (uint32, bool) {
	var errno syscall.Errno
	if !errors.As(callErr, &errno) {
		return 0, false
	}
	return uint32(errno), true
}
