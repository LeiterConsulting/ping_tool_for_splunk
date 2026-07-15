package outbox

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

func TestStorePersistsProgressAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, "1MB", 10)
	if err != nil {
		t.Fatal(err)
	}
	event, _ := json.Marshal(models.SummaryEvent{EventID: "event-1", CycleID: "cycle-1"})
	item, err := store.Enqueue(Envelope{
		ID: "cycle-1", CycleID: "cycle-1", CollectorID: "collector", CreatedAt: time.Now().UTC(),
		Events: []json.RawMessage{event}, Summaries: []models.SummaryEvent{{EventID: "event-1"}},
		EventsRequired: true, MetricsRequired: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	item.Envelope.EventOffset = 1
	if _, err := store.Update(item); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir, "1MB", 10)
	if err != nil {
		t.Fatal(err)
	}
	items, err := reopened.Pending(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Envelope.EventOffset != 1 || items[0].Envelope.MetricOffset != 0 {
		t.Fatalf("unexpected recovered envelope: %#v", items)
	}
	items[0].Envelope.MetricOffset = 1
	updated, err := reopened.Update(items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Envelope.Complete() {
		t.Fatal("envelope should be complete")
	}
	if err := reopened.Delete(updated); err != nil {
		t.Fatal(err)
	}
	remaining, _ := reopened.Pending(10)
	if len(remaining) != 0 {
		t.Fatalf("pending envelopes = %d, want 0", len(remaining))
	}
}

func TestStoreFailsClosedAtCapacity(t *testing.T) {
	store, err := Open(t.TempDir(), "1KB", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Enqueue(Envelope{ID: "first", EventsRequired: true, Events: []json.RawMessage{json.RawMessage(`{"event":"one"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Enqueue(Envelope{ID: "second", EventsRequired: true, Events: []json.RawMessage{json.RawMessage(`{"event":"two"}`)}})
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("error = %v, want ErrCapacity", err)
	}
	items, listErr := store.Pending(10)
	if listErr != nil || len(items) != 1 || items[0].Envelope.ID != "first" {
		t.Fatalf("existing envelope was not preserved: items=%#v err=%v", items, listErr)
	}
}

func TestStatusIsDurable(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir, "1MB", 10)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.WriteStatus(Status{State: "impaired", LastFailureAt: &now, LastError: "test failure"}); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "impaired" || status.LastError != "test failure" || status.UpdatedAt.IsZero() {
		t.Fatalf("unexpected status: %#v", status)
	}
	if info, err := os.Stat(dir + string(os.PathSeparator) + "status.json"); err != nil || info.Size() == 0 {
		t.Fatalf("status file missing or empty: info=%v err=%v", info, err)
	}
}
