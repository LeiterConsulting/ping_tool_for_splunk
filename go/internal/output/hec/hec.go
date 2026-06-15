package hec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/httpcfg"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
)

type Writer struct {
	cfg           config.HEC
	hostname      string
	client        *http.Client
	buf           bytes.Buffer
	eventCount    int
	bufferBytes   int
	droppedEvents int
	capThreshold  int // bytes
	lastWarn      time.Time
	nextAttempt   time.Time
	failures      int
}

func New(cfg config.HEC, hostname string) (*Writer, error) {
	if cfg.URL == "" || cfg.Token == "" {
		return nil, errors.New("hec enabled but url/token not configured")
	}

	client := httpcfg.NewClient(cfg.VerifySSL, cfg.SSLProtocol, 10*time.Second)

	w := &Writer{cfg: cfg, hostname: hostname, client: client, capThreshold: 2 * 1024 * 1024}
	return w, nil
}

func (w *Writer) AddOne(ev interface{}) error {
	return w.add(ev)
}

func (w *Writer) AddMany(list interface{}) error {
	// Manager calls AddMany for []models.PingEvent.
	switch vv := list.(type) {
	case []models.PingEvent:
		for i := range vv {
			if err := w.add(vv[i]); err != nil {
				return err
			}
		}
	default:
		// If unknown slice type, let json marshal it as array (still acceptable for buffering caps).
		return w.add(vv)
	}
	return nil
}

func (w *Writer) add(event interface{}) error {
	// Expect event has a timestamp field formatted as ISO 8601.
	unix := int64(0)
	switch e := event.(type) {
	case models.PingEvent:
		unix = util.UnixSecondsFromISO(e.Timestamp)
	case models.SummaryEvent:
		unix = util.UnixSecondsFromISO(e.Timestamp)
	default:
		// best-effort: no timestamp
		unix = time.Now().UTC().Unix()
	}

	he := models.HECEvent{
		Time:       unix,
		Host:       w.hostname,
		Source:     "ping_monitor",
		SourceType: w.cfg.SourceType,
		Index:      w.cfg.Index,
		Event:      event,
	}

	b, err := json.Marshal(he)
	if err != nil {
		return err
	}
	bytesToAdd := len(b) + 1

	if (w.eventCount+1) > w.cfg.MaxBufferEvents || (w.bufferBytes+bytesToAdd) > parseSizeBytes(w.cfg.MaxBufferBytes, 5*1024*1024) {
		w.droppedEvents++
		return nil
	}

	if w.eventCount > 0 {
		w.buf.WriteByte('\n')
	}
	w.buf.Write(b)
	w.eventCount++
	w.bufferBytes += bytesToAdd

	if w.eventCount >= w.cfg.BatchSize {
		flushCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return w.Flush(flushCtx)
	}
	return nil
}

func (w *Writer) Flush(ctx context.Context) error {
	if w.eventCount == 0 {
		return nil
	}
	// Avoid tight retry loops when Add triggers Flush on every event after BatchSize.
	if !w.nextAttempt.IsZero() && time.Now().Before(w.nextAttempt) {
		return nil
	}
	body := w.buf.Bytes()
	ok := w.postWithRetry(ctx, body)
	if ok {
		w.failures = 0
		w.nextAttempt = time.Time{}
		w.resetBuffer()
		return nil
	}

	w.failures++
	w.nextAttempt = time.Now().Add(w.nextAttemptDelay())
	w.warnRateLimited("hec delivery failed; will retry", map[string]interface{}{
		"hec_url":          w.cfg.URL,
		"buffer_events":    w.eventCount,
		"buffer_bytes":     w.bufferBytes,
		"drop_on_failure":  w.cfg.DropOnFailure,
		"consec_failures":  w.failures,
		"next_attempt_sec": int(w.nextAttempt.Sub(time.Now()).Seconds()),
	})

	if w.cfg.DropOnFailure {
		w.resetBuffer()
		return nil
	}
	// Keep buffer for a later retry, but do not fail the whole process.
	return nil
}

func (w *Writer) nextAttemptDelay() time.Duration {
	baseDelayMs := 1000
	if w.cfg.Retry.Enabled {
		baseDelayMs = max(100, w.cfg.Retry.BaseDelayMs)
	} else if w.cfg.RetryDelayMs > 0 {
		baseDelayMs = max(100, w.cfg.RetryDelayMs)
	}

	d := time.Duration(baseDelayMs) * time.Millisecond
	// Exponential backoff between flush calls, capped.
	shift := w.failures - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 6 {
		shift = 6
	}
	d = d * time.Duration(1<<shift)
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func (w *Writer) warnRateLimited(msg string, fields map[string]interface{}) {
	// Keep logs helpful during outages without spamming.
	if time.Since(w.lastWarn) < 30*time.Second {
		return
	}
	w.lastWarn = time.Now()
	diagnostics.LogWarn(msg, fields)
}

func (w *Writer) postWithRetry(ctx context.Context, body []byte) bool {
	attempts := 0
	maxAttempts := 1
	baseDelay := 0
	jitterPct := 0
	backoff := "fixed"

	if w.cfg.Retry.Enabled {
		if w.cfg.Retry.MaxAttempts > 0 {
			maxAttempts = w.cfg.Retry.MaxAttempts
		}
		baseDelay = w.cfg.Retry.BaseDelayMs
		jitterPct = w.cfg.Retry.JitterPct
		backoff = w.cfg.Retry.Backoff
	} else if w.cfg.RetryCount > 0 {
		maxAttempts = max(1, w.cfg.RetryCount+1)
		baseDelay = max(0, w.cfg.RetryDelayMs)
		jitterPct = 0
		backoff = "fixed"
	}

	for attempts < maxAttempts {
		attempts++
		if w.postOnce(ctx, body) {
			return true
		}
		if attempts >= maxAttempts {
			break
		}
		delay := time.Duration(baseDelay) * time.Millisecond
		if backoff == "exponential" {
			shift := attempts - 1
			if shift < 0 {
				shift = 0
			}
			delay = delay * time.Duration(1<<shift)
		}
		if jitterPct > 0 {
			// deterministic jitter is fine; avoid rand import here.
			adj := int64(delay) * int64(jitterPct) / 100
			delay = time.Duration(max64(0, int64(delay)-adj))
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return false
		}
	}
	return false
}

func (w *Writer) postOnce(ctx context.Context, body []byte) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Splunk "+w.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		w.deadLetter(body)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	w.deadLetter(body)
	return false
}

func (w *Writer) deadLetter(body []byte) {
	if !w.cfg.DropOnFailure {
		return
	}
	if strings.TrimSpace(w.cfg.DeadLetterPath) == "" {
		return
	}
	if w.cfg.DeadLetterRotationSizeMB > 0 {
		_ = rotateIfNeeded(w.cfg.DeadLetterPath, w.cfg.DeadLetterRotationSizeMB)
	}
	// Best-effort append; ignore errors.
	_ = appendFile(w.cfg.DeadLetterPath, body)
}

func (w *Writer) resetBuffer() {
	if cap(w.buf.Bytes()) > w.capThreshold {
		w.buf = bytes.Buffer{}
	} else {
		w.buf.Reset()
	}
	w.eventCount = 0
	w.bufferBytes = 0
}

func (w *Writer) Close() error {
	w.resetBuffer()
	return nil
}

func parseSizeBytes(s string, def int) int {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return def
	}
	var n float64
	var unit string
	_, _ = fmt.Sscanf(s, "%f%s", &n, &unit)
	mult := 1.0
	switch unit {
	case "KB":
		mult = 1024
	case "MB":
		mult = 1024 * 1024
	case "GB":
		mult = 1024 * 1024 * 1024
	case "TB":
		mult = 1024 * 1024 * 1024 * 1024
	case "":
		mult = 1
	default:
		mult = 1
	}
	if n <= 0 {
		return def
	}
	return int(n * mult)
}

func appendFile(path string, body []byte) error {
	// Ensure newline in file.
	b := body
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	f, err := openAppend(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func openAppend(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func rotateIfNeeded(path string, maxMB int) error {
	if maxMB <= 0 {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if st.Size() < int64(maxMB)*1024*1024 {
		return nil
	}
	stamp := time.Now().UTC().Format("20060102_150405")
	arch := path
	if filepath.Ext(arch) == ".log" {
		arch = arch[:len(arch)-4]
	}
	arch = fmt.Sprintf("%s_%s.log", arch, stamp)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.Rename(path, arch)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
