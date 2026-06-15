package metrics

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
)

type Buffer struct {
	cfg          config.Metrics
	hostname     string
	client       *http.Client
	buf          bytes.Buffer
	count        int
	bytes        int
	capThreshold int
	lastWarn     time.Time
	nextAttempt  time.Time
	failures     int
}

func New(cfg config.Metrics, hostname string) *Buffer {
	tr := &http.Transport{}
	if !cfg.VerifySSL {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	return &Buffer{cfg: cfg, hostname: hostname, client: client, capThreshold: 2 * 1024 * 1024}
}

func (b *Buffer) AddSummary(sum models.SummaryEvent) error {
	if !b.cfg.Enabled {
		return nil
	}
	me := buildPayload(sum, b.cfg, b.hostname)
	js, err := json.Marshal(me)
	if err != nil {
		return err
	}
	bytesToAdd := len(js) + 1
	maxEvents := b.cfg.MaxBufferEvents
	if maxEvents < 1 {
		maxEvents = 5000
	}
	maxBytes := parseSizeBytes(b.cfg.MaxBufferBytes, 5*1024*1024)
	if (b.count+1) > maxEvents || (b.bytes+bytesToAdd) > maxBytes {
		return nil // drop-newest
	}
	if b.count > 0 {
		b.buf.WriteByte('\n')
	}
	b.buf.Write(js)
	b.count++
	b.bytes += bytesToAdd
	return nil
}

func (b *Buffer) Flush(ctx context.Context) error {
	if !b.cfg.Enabled {
		return nil
	}
	if b.count == 0 {
		return nil
	}
	if b.cfg.HECURL == "" || b.cfg.Token == "" {
		b.reset()
		return errors.New("metrics enabled but hec_url/token not configured")
	}
	if !b.nextAttempt.IsZero() && time.Now().Before(b.nextAttempt) {
		return nil
	}

	body := b.buf.Bytes()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.cfg.HECURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Splunk "+b.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		b.failures++
		b.nextAttempt = time.Now().Add(b.nextAttemptDelay())
		b.warnRateLimited("metrics delivery failed; will retry", map[string]interface{}{
			"hec_url":          b.cfg.HECURL,
			"buffer_events":    b.count,
			"buffer_bytes":     b.bytes,
			"consec_failures":  b.failures,
			"next_attempt_sec": int(b.nextAttempt.Sub(time.Now()).Seconds()),
		})
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b.failures++
		b.nextAttempt = time.Now().Add(b.nextAttemptDelay())
		b.warnRateLimited("metrics delivery failed (non-2xx); will retry", map[string]interface{}{
			"hec_url":          b.cfg.HECURL,
			"status":           resp.StatusCode,
			"buffer_events":    b.count,
			"buffer_bytes":     b.bytes,
			"consec_failures":  b.failures,
			"next_attempt_sec": int(b.nextAttempt.Sub(time.Now()).Seconds()),
		})
		return nil
	}
	b.failures = 0
	b.nextAttempt = time.Time{}
	b.reset()
	return nil
}

func (b *Buffer) nextAttemptDelay() time.Duration {
	baseDelayMs := 1000
	d := time.Duration(baseDelayMs) * time.Millisecond
	shift := b.failures - 1
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

func (b *Buffer) warnRateLimited(msg string, fields map[string]interface{}) {
	if time.Since(b.lastWarn) < 30*time.Second {
		return
	}
	b.lastWarn = time.Now()
	diagnostics.LogWarn(msg, fields)
}

func (b *Buffer) Close() error {
	b.reset()
	return nil
}

func (b *Buffer) reset() {
	if cap(b.buf.Bytes()) > b.capThreshold {
		b.buf = bytes.Buffer{}
	} else {
		b.buf.Reset()
	}
	b.count = 0
	b.bytes = 0
}

func buildPayload(sum models.SummaryEvent, cfg config.Metrics, hostname string) models.MetricsEvent {
	unix := util.UnixSecondsFromISO(sum.Timestamp)
	eventName := cfg.EventName
	if eventName == "" {
		eventName = "metric"
	}
	sourcetype := cfg.SourceType
	if sourcetype == "" {
		sourcetype = "ping_monitor:metrics"
	}
	useCompat := cfg.CompatMode && !cfg.UseMetricsIndex
	if !useCompat {
		eventName = "metric" // required by metrics index
	}

	fields := map[string]interface{}{
		"metric_name:ping.avg_latency_ms":   sum.AvgLatencyMs,
		"metric_name:ping.min_latency_ms":   float64(sum.MinLatencyMs),
		"metric_name:ping.max_latency_ms":   float64(sum.MaxLatencyMs),
		"metric_name:ping.packet_loss_pct":  sum.PacketLossPct,
		"metric_name:ping.pings_sent":       sum.PingsSent,
		"metric_name:ping.pings_successful": sum.PingsSuccessful,
		"hostname":                          sum.Hostname,
		"target_ip":                         sum.TargetIP,
		"dev":                               sum.Dev,
		"group":                             sum.Group,
		"description":                       sum.Description,
		"entitytype":                        sum.EntityType,
		"device":                            sum.Device,
		"vendor":                            sum.Vendor,
		"additional_notes":                  sum.Notes,
	}

	return models.MetricsEvent{
		Time:       unix,
		Host:       hostname,
		Source:     "ping_monitor",
		SourceType: sourcetype,
		Index:      cfg.Index,
		Event:      eventName,
		Fields:     fields,
	}
}

func parseSizeBytes(s string, def int) int {
	// shared with hec; keep minimal duplication
	if s == "" {
		return def
	}
	// crude parse: digits + suffix
	// Accept same set as v4: KB/MB/GB/TB.
	s = strings.TrimSpace(strings.ToUpper(s))
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
