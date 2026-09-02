package hec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/httpcfg"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
	"github.com/google/uuid"
)

type Writer struct {
	cfg      config.HEC
	hostname string
	client   *http.Client
	channel  string
	ackURL   string
}

// DeliveryResult describes what Splunk confirmed for a completed HEC request.
// A successful HTTP response is only an acceptance signal unless indexer
// acknowledgment was requested and subsequently confirmed.
type DeliveryResult struct {
	StatusCode   int
	ResponseBody string
	ACKRequested bool
	ACKID        string
	ACKnowledged bool
}

func New(cfg config.HEC, hostname string, collectorID string) (*Writer, error) {
	if strings.TrimSpace(cfg.URL) == "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("hec enabled but url/token not configured")
	}
	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(cfg.URL))
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("HEC URL must be an absolute http(s) URL: %q", cfg.URL)
	}
	channel := strings.TrimSpace(cfg.Channel)
	if channel == "" {
		channel = uuid.NewSHA1(uuid.NameSpaceURL, []byte("ping-monitor:"+collectorID)).String()
	}
	if _, err := uuid.Parse(channel); err != nil {
		return nil, fmt.Errorf("HEC channel must be a GUID: %w", err)
	}
	ackURL := ""
	if cfg.UseACK {
		var err error
		ackURL, err = deriveACKURL(cfg.URL)
		if err != nil {
			return nil, err
		}
	}
	return &Writer{
		cfg: cfg, hostname: hostname,
		client:  httpcfg.NewClient(cfg.VerifySSL, cfg.SSLProtocol, 10*time.Second),
		channel: channel, ackURL: ackURL,
	}, nil
}

func (w *Writer) BatchSize() int {
	if w.cfg.BatchSize < 1 {
		return 100
	}
	return w.cfg.BatchSize
}

func (w *Writer) SendEvents(ctx context.Context, events []json.RawMessage) error {
	if len(events) == 0 {
		return nil
	}
	lines := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		var timestamp struct {
			Timestamp string `json:"timestamp"`
		}
		_ = json.Unmarshal(event, &timestamp)
		unix := util.UnixTimeFromISO(timestamp.Timestamp)
		if unix <= 0 {
			unix = float64(time.Now().UTC().UnixNano()) / float64(time.Second)
		}
		envelope := models.HECEvent{
			Time: unix, Host: w.hostname, Source: "ping_monitor",
			SourceType: w.cfg.SourceType, Index: w.cfg.Index, Event: event,
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		lines = append(lines, encoded)
	}
	return w.SendPayloads(ctx, lines)
}

// SendPayloads sends already-formed HEC JSON envelopes as one request.
func (w *Writer) SendPayloads(ctx context.Context, payloads []json.RawMessage) error {
	_, err := w.SendPayloadsWithResult(ctx, payloads)
	return err
}

// SendPayloadsWithResult sends already-formed HEC JSON envelopes and returns
// the exact confirmation level reached by Splunk.
func (w *Writer) SendPayloadsWithResult(ctx context.Context, payloads []json.RawMessage) (DeliveryResult, error) {
	if len(payloads) == 0 {
		return DeliveryResult{ACKRequested: w.cfg.UseACK}, nil
	}
	lines := make([][]byte, len(payloads))
	for i := range payloads {
		lines[i] = payloads[i]
	}
	body := bytes.Join(lines, []byte("\n"))
	return w.postWithRetry(ctx, body)
}

func (w *Writer) postWithRetry(ctx context.Context, body []byte) (DeliveryResult, error) {
	maxAttempts := 1
	baseDelay := 0
	jitterPct := 0
	backoff := "fixed"
	if w.cfg.Retry.Enabled {
		maxAttempts = max(1, w.cfg.Retry.MaxAttempts)
		baseDelay = max(0, w.cfg.Retry.BaseDelayMs)
		jitterPct = max(0, w.cfg.Retry.JitterPct)
		backoff = strings.ToLower(w.cfg.Retry.Backoff)
	} else if w.cfg.RetryCount > 0 {
		maxAttempts = max(1, w.cfg.RetryCount+1)
		baseDelay = max(0, w.cfg.RetryDelayMs)
	}

	var lastErr error
	var lastResult DeliveryResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if result, err := w.postOnce(ctx, body); err == nil {
			return result, nil
		} else {
			lastResult = result
			lastErr = err
		}
		if attempt == maxAttempts {
			break
		}
		delay := time.Duration(baseDelay) * time.Millisecond
		if backoff == "exponential" {
			delay *= time.Duration(1 << min(attempt-1, 6))
		}
		if jitterPct > 0 && delay > 0 {
			span := int64(delay) * int64(jitterPct) / 100
			if span > 0 {
				delay += time.Duration((time.Now().UnixNano() % (span*2 + 1)) - span)
			}
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return lastResult, errors.Join(lastErr, ctx.Err())
		}
	}
	return lastResult, fmt.Errorf("HEC delivery failed after %d attempt(s): %w", maxAttempts, lastErr)
}

func (w *Writer) postOnce(ctx context.Context, body []byte) (DeliveryResult, error) {
	result := DeliveryResult{ACKRequested: w.cfg.UseACK}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	w.setHeaders(req)
	resp, err := w.client.Do(req)
	if err != nil {
		return result, err
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	closeErr := resp.Body.Close()
	result.StatusCode = resp.StatusCode
	result.ResponseBody = strings.TrimSpace(string(responseBody))
	if readErr != nil {
		return result, readErr
	}
	if closeErr != nil {
		return result, closeErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("HEC returned HTTP %d: %s", resp.StatusCode, result.ResponseBody)
	}
	if !w.cfg.UseACK {
		return result, nil
	}
	ackID, err := parseACKID(responseBody)
	if err != nil {
		return result, fmt.Errorf("HEC indexer acknowledgment response invalid: %w", err)
	}
	result.ACKID = ackID
	if err := w.waitForACK(ctx, ackID); err != nil {
		return result, err
	}
	result.ACKnowledged = true
	return result, nil
}

func (w *Writer) waitForACK(parent context.Context, ackID string) error {
	timeout := time.Duration(max(1, w.cfg.ACKTimeoutSeconds)) * time.Second
	poll := time.Duration(max(50, w.cfg.ACKPollIntervalMs)) * time.Millisecond
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	requestBody, _ := json.Marshal(map[string][]json.RawMessage{
		"acks": {json.RawMessage(ackID)},
	})
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.ackURL, bytes.NewReader(requestBody))
		if err != nil {
			return err
		}
		w.setHeaders(req)
		resp, err := w.client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()
			if readErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				var result struct {
					ACKs map[string]bool `json:"acks"`
				}
				if json.Unmarshal(body, &result) == nil && result.ACKs[ackID] {
					return nil
				}
			}
		}
		select {
		case <-time.After(poll):
		case <-ctx.Done():
			return fmt.Errorf("HEC indexer acknowledgment %s was not confirmed within %s: %w", ackID, timeout, ctx.Err())
		}
	}
}

func (w *Writer) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Splunk "+w.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	if w.cfg.UseACK {
		req.Header.Set("X-Splunk-Request-Channel", w.channel)
	}
}

func deriveACKURL(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse HEC URL: %w", err)
	}
	marker := "/services/collector"
	index := strings.Index(parsed.Path, marker)
	if index < 0 {
		return "", fmt.Errorf("HEC URL must contain %s", marker)
	}
	parsed.Path = parsed.Path[:index] + marker + "/ack"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func parseACKID(body []byte) (string, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	var raw json.RawMessage
	for key, value := range response {
		if strings.EqualFold(key, "ackId") || strings.EqualFold(key, "ackID") {
			raw = value
			break
		}
	}
	if len(raw) == 0 {
		return "", errors.New("ackId is missing")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if _, err := strconv.ParseInt(text, 10, 64); err != nil {
			return "", fmt.Errorf("ackId %q is not numeric", text)
		}
		return text, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", fmt.Errorf("ackId has unsupported type: %s", raw)
	}
	if _, err := number.Int64(); err != nil {
		return "", fmt.Errorf("ackId %q is not an integer", number.String())
	}
	return number.String(), nil
}

func (w *Writer) Close() error { return nil }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
