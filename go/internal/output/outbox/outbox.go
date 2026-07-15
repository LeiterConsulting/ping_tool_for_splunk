package outbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

const envelopeVersion = 1

var ErrCapacity = errors.New("durable outbox capacity exceeded")

type Envelope struct {
	Version           int                   `json:"version"`
	ID                string                `json:"id"`
	CycleID           string                `json:"cycle_id"`
	CollectorID       string                `json:"collector_id"`
	CreatedAt         time.Time             `json:"created_at"`
	Events            []json.RawMessage     `json:"events,omitempty"`
	Summaries         []models.SummaryEvent `json:"summaries,omitempty"`
	EventsRequired    bool                  `json:"events_required"`
	MetricsRequired   bool                  `json:"metrics_required"`
	EventOffset       int                   `json:"event_offset"`
	MetricOffset      int                   `json:"metric_offset"`
	DeliveryAttempts  int                   `json:"delivery_attempts"`
	LastAttemptAt     *time.Time            `json:"last_attempt_at,omitempty"`
	LastDeliveryError string                `json:"last_delivery_error,omitempty"`
}

func (e *Envelope) Normalize() {
	if e.Version == 0 {
		e.Version = envelopeVersion
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	if e.EventOffset < 0 {
		e.EventOffset = 0
	}
	if e.MetricOffset < 0 {
		e.MetricOffset = 0
	}
	if e.EventOffset > len(e.Events) {
		e.EventOffset = len(e.Events)
	}
	if e.MetricOffset > len(e.Summaries) {
		e.MetricOffset = len(e.Summaries)
	}
}

func (e Envelope) Complete() bool {
	eventsDone := !e.EventsRequired || e.EventOffset >= len(e.Events)
	metricsDone := !e.MetricsRequired || e.MetricOffset >= len(e.Summaries)
	return eventsDone && metricsDone
}

type Item struct {
	Path     string
	Envelope Envelope
	Size     int64
}

type Stats struct {
	PendingEnvelopes int       `json:"pending_envelopes"`
	PendingBytes     int64     `json:"pending_bytes"`
	OldestCreatedAt  time.Time `json:"oldest_created_at,omitempty"`
}

type Status struct {
	State              string     `json:"state"`
	ConfirmationMode   string     `json:"confirmation_mode"`
	UpdatedAt          time.Time  `json:"updated_at"`
	PendingEnvelopes   int        `json:"pending_envelopes"`
	PendingBytes       int64      `json:"pending_bytes"`
	OldestCreatedAt    *time.Time `json:"oldest_created_at,omitempty"`
	OldestAgeSeconds   int64      `json:"oldest_age_seconds"`
	LastSuccessAt      *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt      *time.Time `json:"last_failure_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	DeliveredEnvelopes uint64     `json:"delivered_envelopes"`
	DeliveredEvents    uint64     `json:"delivered_events"`
	DeliveredMetrics   uint64     `json:"delivered_metrics"`
}

type Store struct {
	dir          string
	maxBytes     int64
	maxEnvelopes int
	mu           sync.Mutex
}

func Open(dir string, maxBytesText string, maxEnvelopes int) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("durable outbox path is required")
	}
	if maxEnvelopes < 1 {
		return nil, errors.New("durable outbox max_envelopes must be positive")
	}
	maxBytes, err := parseBytes(maxBytesText)
	if err != nil {
		return nil, fmt.Errorf("parse durable outbox max_spool_bytes: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create durable outbox: %w", err)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmp") {
			_ = os.Remove(filepath.Join(abs, entry.Name()))
		}
	}
	return &Store{dir: abs, maxBytes: maxBytes, maxEnvelopes: maxEnvelopes}, nil
}

func (s *Store) Directory() string { return s.dir }

func (s *Store) Enqueue(envelope Envelope) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	envelope.Normalize()
	if strings.TrimSpace(envelope.ID) == "" {
		return Item{}, errors.New("durable outbox envelope id is required")
	}
	if envelope.Complete() {
		return Item{}, errors.New("refusing to enqueue an already-complete envelope")
	}
	body, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return Item{}, err
	}
	body = append(body, '\n')
	stats, err := s.statsLocked()
	if err != nil {
		return Item{}, err
	}
	if stats.PendingEnvelopes+1 > s.maxEnvelopes || stats.PendingBytes+int64(len(body)) > s.maxBytes {
		return Item{}, fmt.Errorf("%w: pending_envelopes=%d pending_bytes=%d new_bytes=%d limits=%d/%d",
			ErrCapacity, stats.PendingEnvelopes, stats.PendingBytes, len(body), s.maxEnvelopes, s.maxBytes)
	}
	name := fmt.Sprintf("%020d_%s.json", envelope.CreatedAt.UnixNano(), safeName(envelope.ID))
	path := filepath.Join(s.dir, name)
	if _, err := os.Stat(path); err == nil {
		return Item{}, fmt.Errorf("durable outbox envelope already exists: %s", envelope.ID)
	}
	if err := writeAtomic(path, body, 0o600); err != nil {
		return Item{}, fmt.Errorf("persist durable outbox envelope: %w", err)
	}
	return Item{Path: path, Envelope: envelope, Size: int64(len(body))}, nil
}

func (s *Store) Pending(limit int) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.pendingLocked()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *Store) Update(item Item) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item.Envelope.Normalize()
	body, err := json.MarshalIndent(item.Envelope, "", "  ")
	if err != nil {
		return Item{}, err
	}
	body = append(body, '\n')
	if err := writeAtomic(item.Path, body, 0o600); err != nil {
		return Item{}, fmt.Errorf("update durable outbox envelope: %w", err)
	}
	item.Size = int64(len(body))
	return item, nil
}

func (s *Store) Delete(item Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clean := filepath.Clean(item.Path)
	if filepath.Dir(clean) != s.dir {
		return fmt.Errorf("refusing to delete outbox item outside store: %s", clean)
	}
	if err := os.Remove(clean); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete durable outbox envelope: %w", err)
	}
	return nil
}

func (s *Store) Stats() (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statsLocked()
}

func (s *Store) WriteStatus(status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status.UpdatedAt = time.Now().UTC()
	body, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeAtomic(filepath.Join(s.dir, "status.json"), body, 0o600)
}

func ReadStatus(dir string) (Status, error) {
	body, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal(body, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (s *Store) pendingLocked() ([]Item, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "status.json" {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read durable outbox envelope %s: %w", path, err)
		}
		var envelope Envelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("decode durable outbox envelope %s: %w", path, err)
		}
		if envelope.Version != envelopeVersion {
			return nil, fmt.Errorf("unsupported durable outbox envelope version %d in %s", envelope.Version, path)
		}
		envelope.Normalize()
		items = append(items, Item{Path: path, Envelope: envelope, Size: int64(len(body))})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Envelope.CreatedAt.Equal(items[j].Envelope.CreatedAt) {
			return items[i].Path < items[j].Path
		}
		return items[i].Envelope.CreatedAt.Before(items[j].Envelope.CreatedAt)
	})
	return items, nil
}

func (s *Store) statsLocked() (Stats, error) {
	items, err := s.pendingLocked()
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{PendingEnvelopes: len(items)}
	for _, item := range items {
		stats.PendingBytes += item.Size
		if stats.OldestCreatedAt.IsZero() || item.Envelope.CreatedAt.Before(stats.OldestCreatedAt) {
			stats.OldestCreatedAt = item.Envelope.CreatedAt
		}
	}
	return stats, nil
}

func writeAtomic(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp) }
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		cleanup()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		cleanup()
		return err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func safeName(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "envelope"
	}
	return builder.String()
}

func parseBytes(value string) (int64, error) {
	s := strings.TrimSpace(strings.ToUpper(value))
	if s == "" {
		return 0, errors.New("size is empty")
	}
	multiplier := int64(1)
	for _, unit := range []struct {
		suffix     string
		multiplier int64
	}{
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	} {
		if strings.HasSuffix(s, unit.suffix) {
			multiplier = unit.multiplier
			s = strings.TrimSpace(strings.TrimSuffix(s, unit.suffix))
			break
		}
	}
	number, err := strconv.ParseFloat(s, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	return int64(number * float64(multiplier)), nil
}
