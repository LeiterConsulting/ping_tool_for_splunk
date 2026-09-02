package hec

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

func TestWriterWaitsForIndexerACK(t *testing.T) {
	var ackPolls atomic.Int32
	var channel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestChannel := r.Header.Get("X-Splunk-Request-Channel")
		if requestChannel == "" {
			t.Error("missing X-Splunk-Request-Channel")
		}
		if channel == "" {
			channel = requestChannel
		}
		if requestChannel != channel {
			t.Errorf("channel changed: %q != %q", requestChannel, channel)
		}
		switch r.URL.Path {
		case "/services/collector/event":
			_, _ = w.Write([]byte(`{"ackId":7}`))
		case "/services/collector/ack":
			poll := ackPolls.Add(1)
			_, _ = w.Write([]byte(`{"acks":{"7":` + map[bool]string{true: "true", false: "false"}[poll >= 2] + `}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writer, err := New(config.HEC{
		URL: server.URL + "/services/collector/event", Token: "token", SourceType: "ping_monitor", Index: "ping",
		VerifySSL: true, BatchSize: 100, UseACK: true, ACKTimeoutSeconds: 2, ACKPollIntervalMs: 50,
	}, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]interface{}{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "event_id": "one"})
	result, err := writer.SendPayloadsWithResult(context.Background(), []json.RawMessage{payload})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || !result.ACKRequested || !result.ACKnowledged || result.ACKID != "7" {
		t.Fatalf("delivery result = %#v", result)
	}
	if ackPolls.Load() < 2 {
		t.Fatalf("ack polls = %d, want at least 2", ackPolls.Load())
	}
}

func TestWriterReportsAcceptedOnlyWithoutIndexerACK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if channel := r.Header.Get("X-Splunk-Request-Channel"); channel != "" {
			t.Fatalf("unexpected request channel %q", channel)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	defer server.Close()

	writer, err := New(config.HEC{
		URL: server.URL, Token: "token", VerifySSL: true, BatchSize: 100,
	}, "collector", "collector-id")
	if err != nil {
		t.Fatal(err)
	}
	result, err := writer.SendPayloadsWithResult(context.Background(), []json.RawMessage{json.RawMessage(`{"event":"probe"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusCreated || result.ACKRequested || result.ACKnowledged {
		t.Fatalf("delivery result = %#v", result)
	}
}

func TestParseACKIDAcceptsStringAndNumber(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{`{"ackId":12}`, "12"}, {`{"ackID":"13"}`, "13"},
	} {
		got, err := parseACKID([]byte(test.body))
		if err != nil || got != test.want {
			t.Fatalf("parseACKID(%s) = %q, %v; want %q", test.body, got, err, test.want)
		}
	}
}

func TestDeriveACKURL(t *testing.T) {
	got, err := deriveACKURL("https://splunk.example:8088/services/collector/event?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://splunk.example:8088/services/collector/ack" {
		t.Fatalf("ack URL = %q", got)
	}
}

func TestNewRejectsInvalidURLBeforeDelivery(t *testing.T) {
	cfg := config.Defaults(t.TempDir()).HEC
	cfg.Enabled = true
	cfg.URL = "splunk:8088/services/collector/event"
	cfg.Token = "token"
	if _, err := New(cfg, "collector", "collector-id"); err == nil {
		t.Fatal("New() accepted a non-absolute HEC URL")
	}
}
