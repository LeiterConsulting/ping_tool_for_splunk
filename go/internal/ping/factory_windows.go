//go:build windows

package ping

import "strings"

func newPinger(mode string, onFallback func(ip string, from string, to string, reason string)) Pinger {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "exec":
		return &ExecPinger{}
	case "raw", "native":
		return &WindowsPinger{onFallback: onFallback}
	case "", "auto":
		return &WindowsPinger{fallback: &ExecPinger{}, onFallback: onFallback}
	default:
		return &WindowsPinger{fallback: &ExecPinger{}, onFallback: onFallback}
	}
}
