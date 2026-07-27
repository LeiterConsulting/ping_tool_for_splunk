package output

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/fileout"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/hec"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/metrics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/outbox"
)

type Manager struct {
	cfg            config.Config
	collectorHost  string
	collectorID    string
	fileWriter     *fileout.Writer
	hecWriter      *hec.Writer
	metricsSender  *metrics.Sender
	outbox         *outbox.Store
	cycleEvents    []json.RawMessage
	cycleSummaries []models.SummaryEvent
	queue          chan outputRequest
	done           chan struct{}
	deliveryWake   chan struct{}
	deliveryIdle   chan struct{}
	deliveryDone   chan struct{}
	deliveryCancel context.CancelFunc
	closeOnce      sync.Once
	statusMu       sync.RWMutex
	status         outbox.Status
}

type outputRequest struct {
	individual []models.PingEvent
	summary    models.SummaryEvent
	events     []json.RawMessage
	batchID    string
	external   bool
	flush      bool
	ctx        context.Context
	result     chan error
}

func NewManager(cfg config.Config, collectorHost string, collectorID string) (*Manager, error) {
	m := &Manager{cfg: cfg, collectorHost: collectorHost, collectorID: collectorID}
	useFile := cfg.OutputMode == "file" || cfg.OutputMode == "both"
	useHEC := (cfg.OutputMode == "hec" || cfg.OutputMode == "both") && cfg.HEC.Enabled && cfg.Metrics.Mode != "metrics_only"

	if useFile {
		writer, err := fileout.NewWithOptions(fileout.Options{
			Path: cfg.LogPath, MaxSizeMB: cfg.LogRotationSizeMB,
			RetentionFiles: cfg.LogRetentionFiles, RetentionDays: cfg.LogRetentionDays,
			CompressRotated: cfg.LogCompressRotated,
		})
		if err != nil {
			return nil, err
		}
		m.fileWriter = writer
	}
	if useHEC {
		writer, err := hec.New(cfg.HEC, collectorHost, collectorID)
		if err != nil {
			return nil, err
		}
		m.hecWriter = writer
	}
	if cfg.Metrics.Enabled {
		sender, err := metrics.New(cfg.Metrics, collectorHost, collectorID)
		if err != nil {
			return nil, err
		}
		m.metricsSender = sender
	}
	if m.hecWriter != nil || m.metricsSender != nil {
		store, err := outbox.Open(cfg.Delivery.SpoolPath, cfg.Delivery.MaxSpoolBytes, cfg.Delivery.MaxEnvelopes)
		if err != nil {
			return nil, err
		}
		m.outbox = store
		if existing, err := outbox.ReadStatus(store.Directory()); err == nil {
			m.status = existing
		} else {
			m.status = outbox.Status{State: "starting", UpdatedAt: time.Now().UTC()}
		}
		m.status.ConfirmationMode = deliveryConfirmationMode(cfg, m.hecWriter != nil, m.metricsSender != nil)
	}

	queueSize := cfg.ParallelThreads * 4
	if queueSize < 256 {
		queueSize = 256
	}
	m.queue = make(chan outputRequest, queueSize)
	m.done = make(chan struct{})
	m.deliveryWake = make(chan struct{}, 1)
	m.deliveryIdle = make(chan struct{}, 1)
	m.deliveryDone = make(chan struct{})
	go m.run()
	deliveryCtx, cancel := context.WithCancel(context.Background())
	m.deliveryCancel = cancel
	go m.runDelivery(deliveryCtx)
	if m.outbox != nil {
		if stats, statsErr := m.outbox.Stats(); statsErr != nil {
			m.Close()
			return nil, statsErr
		} else if stats.PendingEnvelopes > 0 {
			m.signalDelivery()
		} else if statusErr := m.setDeliveryHealthy(); statusErr != nil {
			m.Close()
			return nil, statusErr
		}
	}
	return m, nil
}

func deliveryConfirmationMode(cfg config.Config, eventsEnabled bool, metricsEnabled bool) string {
	confirmed := 0
	total := 0
	if eventsEnabled {
		total++
		if cfg.HEC.UseACK {
			confirmed++
		}
	}
	if metricsEnabled {
		total++
		if cfg.Metrics.UseACK {
			confirmed++
		}
	}
	switch {
	case total == 0:
		return "not_applicable"
	case confirmed == total:
		return "indexed_acknowledged"
	case confirmed == 0:
		return "hec_accepted_only"
	default:
		return "mixed"
	}
}

func (m *Manager) HandleResult(ctx context.Context, individual []models.PingEvent, summary models.SummaryEvent) error {
	request := outputRequest{individual: append([]models.PingEvent(nil), individual...), summary: summary}
	select {
	case m.queue <- request:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return errors.New("output manager is closed")
	}
}

// HandleEvents durably accepts an event-only batch, such as discovery evidence,
// into the same serialized file and HEC pipeline used by monitoring cycles. It
// returns after local file flush/outbox persistence, not after network delivery.
func (m *Manager) HandleEvents(ctx context.Context, batchID string, events []json.RawMessage) error {
	if strings.TrimSpace(batchID) == "" {
		return errors.New("output event batch_id is required")
	}
	if len(events) == 0 {
		return nil
	}
	copied := make([]json.RawMessage, len(events))
	for index := range events {
		if !json.Valid(events[index]) {
			return fmt.Errorf("output event %d is not valid JSON", index)
		}
		copied[index] = append(json.RawMessage(nil), events[index]...)
	}
	result := make(chan error, 1)
	request := outputRequest{
		events: copied, batchID: strings.TrimSpace(batchID), external: true,
		ctx: ctx, result: result,
	}
	select {
	case m.queue <- request:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return errors.New("output manager is closed")
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return errors.New("output manager closed before event batch was persisted")
	}
}

func (m *Manager) handleResult(individual []models.PingEvent, summary models.SummaryEvent) error {
	emitEvents := !(m.cfg.Metrics.Enabled && m.cfg.Metrics.Mode == "metrics_only")
	if emitEvents && m.fileWriter != nil {
		if len(individual) > 0 {
			if err := m.fileWriter.WritePingEvents(individual); err != nil {
				return err
			}
		}
		if err := m.fileWriter.WriteOne(summary); err != nil {
			return err
		}
	}
	if m.hecWriter != nil {
		for _, event := range individual {
			encoded, err := json.Marshal(event)
			if err != nil {
				return err
			}
			m.cycleEvents = append(m.cycleEvents, encoded)
		}
		encoded, err := json.Marshal(summary)
		if err != nil {
			return err
		}
		m.cycleEvents = append(m.cycleEvents, encoded)
	}
	if m.metricsSender != nil {
		m.cycleSummaries = append(m.cycleSummaries, summary)
	}
	return nil
}

func (m *Manager) handleEvents(batchID string, events []json.RawMessage) error {
	emitEvents := !(m.cfg.Metrics.Enabled && m.cfg.Metrics.Mode == "metrics_only")
	if !emitEvents {
		return nil
	}
	if m.fileWriter != nil {
		for _, event := range events {
			if err := m.fileWriter.WriteOne(event); err != nil {
				return err
			}
		}
		if err := m.fileWriter.Flush(); err != nil {
			return err
		}
	}
	if m.hecWriter == nil {
		return nil
	}
	if m.outbox == nil {
		return errors.New("HEC event output is enabled without a durable outbox")
	}
	_, err := m.outbox.Enqueue(outbox.Envelope{
		ID:              m.collectorID + "-" + batchID,
		CycleID:         batchID,
		CollectorID:     m.collectorID,
		CreatedAt:       time.Now().UTC(),
		Events:          append([]json.RawMessage(nil), events...),
		EventsRequired:  true,
		MetricsRequired: false,
	})
	if err != nil {
		return err
	}
	if err := m.setDeliveryQueued(); err != nil {
		return err
	}
	m.signalDelivery()
	return nil
}

func (m *Manager) FlushCycle(ctx context.Context) error {
	result := make(chan error, 1)
	request := outputRequest{flush: true, ctx: ctx, result: result}
	select {
	case m.queue <- request:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return errors.New("output manager is closed")
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return errors.New("output manager closed before flush completed")
	}
}

func (m *Manager) persistCycle() error {
	if m.outbox == nil || (len(m.cycleEvents) == 0 && len(m.cycleSummaries) == 0) {
		return nil
	}
	cycleID := ""
	if len(m.cycleSummaries) > 0 {
		cycleID = m.cycleSummaries[0].CycleID
	}
	if cycleID == "" && len(m.cycleEvents) > 0 {
		var identity struct {
			CycleID string `json:"cycle_id"`
		}
		_ = json.Unmarshal(m.cycleEvents[0], &identity)
		cycleID = identity.CycleID
	}
	if cycleID == "" {
		return errors.New("cannot persist output cycle without cycle_id")
	}
	_, err := m.outbox.Enqueue(outbox.Envelope{
		ID: m.collectorID + "-" + cycleID, CycleID: cycleID, CollectorID: m.collectorID,
		CreatedAt: time.Now().UTC(), Events: append([]json.RawMessage(nil), m.cycleEvents...),
		Summaries:      append([]models.SummaryEvent(nil), m.cycleSummaries...),
		EventsRequired: m.hecWriter != nil, MetricsRequired: m.metricsSender != nil,
	})
	if err != nil {
		return err
	}
	m.cycleEvents = m.cycleEvents[:0]
	m.cycleSummaries = m.cycleSummaries[:0]
	return nil
}

func (m *Manager) flush(ctx context.Context, persistCurrent bool) error {
	if m.fileWriter != nil {
		if err := m.fileWriter.Flush(); err != nil {
			return err
		}
	}
	if m.outbox == nil {
		return nil
	}
	if persistCurrent {
		if err := m.persistCycle(); err != nil {
			_ = m.setDeliveryFailure(err)
			return err
		}
	}
	if err := m.setDeliveryQueued(); err != nil {
		return err
	}
	m.signalDelivery()
	return nil
}

func (m *Manager) setDeliveryQueued() error {
	previous := m.DeliveryStatus()
	status, err := m.statusWithStats()
	if err != nil {
		return err
	}
	if status.PendingEnvelopes > 0 {
		if previous.State == "impaired" {
			status.State = "impaired"
			status.LastError = previous.LastError
		} else {
			status.State = "backlogged"
		}
	} else {
		status.State = "healthy"
		status.LastError = ""
	}
	m.storeStatus(status)
	return m.outbox.WriteStatus(status)
}

func (m *Manager) drain(ctx context.Context) error {
	limit := m.cfg.Delivery.DrainMaxEnvelopes
	items, err := m.outbox.Pending(limit)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return m.setDeliveryHealthy()
	}

	for _, original := range items {
		item := original
		now := time.Now().UTC()
		item.Envelope.DeliveryAttempts++
		item.Envelope.LastAttemptAt = &now

		if item.Envelope.EventsRequired && item.Envelope.EventOffset < len(item.Envelope.Events) {
			if m.hecWriter == nil {
				return m.setDeliveryFailure(fmt.Errorf("outbox envelope %s requires event delivery but the HEC event sink is disabled", item.Envelope.ID))
			}
			batchSize := m.hecWriter.BatchSize()
			for item.Envelope.EventOffset < len(item.Envelope.Events) {
				end := min(item.Envelope.EventOffset+batchSize, len(item.Envelope.Events))
				if err := m.hecWriter.SendEvents(ctx, item.Envelope.Events[item.Envelope.EventOffset:end]); err != nil {
					item.Envelope.LastDeliveryError = err.Error()
					if _, updateErr := m.outbox.Update(item); updateErr != nil {
						return errors.Join(err, updateErr)
					}
					return m.setDeliveryFailure(err)
				}
				m.statusMu.Lock()
				m.status.DeliveredEvents += uint64(end - item.Envelope.EventOffset)
				m.statusMu.Unlock()
				item.Envelope.EventOffset = end
				item.Envelope.LastDeliveryError = ""
				var updateErr error
				item, updateErr = m.outbox.Update(item)
				if updateErr != nil {
					return updateErr
				}
			}
		}

		if item.Envelope.MetricsRequired && item.Envelope.MetricOffset < len(item.Envelope.Summaries) {
			if m.metricsSender == nil {
				return m.setDeliveryFailure(fmt.Errorf("outbox envelope %s requires metrics delivery but the metrics sink is disabled", item.Envelope.ID))
			}
			batchSize := m.metricsSender.BatchSize()
			for item.Envelope.MetricOffset < len(item.Envelope.Summaries) {
				end := min(item.Envelope.MetricOffset+batchSize, len(item.Envelope.Summaries))
				if err := m.metricsSender.SendSummaries(ctx, item.Envelope.Summaries[item.Envelope.MetricOffset:end]); err != nil {
					item.Envelope.LastDeliveryError = err.Error()
					if _, updateErr := m.outbox.Update(item); updateErr != nil {
						return errors.Join(err, updateErr)
					}
					return m.setDeliveryFailure(err)
				}
				m.statusMu.Lock()
				m.status.DeliveredMetrics += uint64(end - item.Envelope.MetricOffset)
				m.statusMu.Unlock()
				item.Envelope.MetricOffset = end
				item.Envelope.LastDeliveryError = ""
				var updateErr error
				item, updateErr = m.outbox.Update(item)
				if updateErr != nil {
					return updateErr
				}
			}
		}

		if !item.Envelope.Complete() {
			return fmt.Errorf("durable outbox envelope %s remained incomplete without a delivery error", item.Envelope.ID)
		}
		if err := m.outbox.Delete(item); err != nil {
			return err
		}
		m.statusMu.Lock()
		m.status.DeliveredEnvelopes++
		m.statusMu.Unlock()
	}
	return m.setDeliveryHealthy()
}

func (m *Manager) setDeliveryFailure(deliveryErr error) error {
	now := time.Now().UTC()
	previous := m.DeliveryStatus()
	status, err := m.statusWithStats()
	if err != nil {
		return errors.Join(deliveryErr, err)
	}
	status.State = "impaired"
	status.LastFailureAt = &now
	status.LastError = deliveryErr.Error()
	m.storeStatus(status)
	if err := m.outbox.WriteStatus(status); err != nil {
		return errors.Join(deliveryErr, err)
	}
	if previous.State != "impaired" || previous.LastError != status.LastError || previous.LastFailureAt == nil || now.Sub(*previous.LastFailureAt) >= 30*time.Second {
		diagnostics.LogWarn("durable delivery impaired; data retained in outbox", map[string]interface{}{
			"pending_envelopes": status.PendingEnvelopes, "pending_bytes": status.PendingBytes,
			"oldest_age_seconds": status.OldestAgeSeconds, "error": status.LastError,
		})
	}
	return nil
}

func (m *Manager) setDeliveryHealthy() error {
	now := time.Now().UTC()
	status, err := m.statusWithStats()
	if err != nil {
		return err
	}
	if status.PendingEnvelopes > 0 {
		status.State = "backlogged"
	} else {
		status.State = "healthy"
	}
	status.LastSuccessAt = &now
	status.LastError = ""
	m.storeStatus(status)
	return m.outbox.WriteStatus(status)
}

func (m *Manager) statusWithStats() (outbox.Status, error) {
	stats, err := m.outbox.Stats()
	if err != nil {
		return outbox.Status{}, err
	}
	m.statusMu.RLock()
	status := m.status
	m.statusMu.RUnlock()
	status.PendingEnvelopes = stats.PendingEnvelopes
	status.PendingBytes = stats.PendingBytes
	status.OldestCreatedAt = nil
	status.OldestAgeSeconds = 0
	if !stats.OldestCreatedAt.IsZero() {
		oldest := stats.OldestCreatedAt
		status.OldestCreatedAt = &oldest
		status.OldestAgeSeconds = max64(0, int64(time.Since(oldest).Seconds()))
	}
	return status, nil
}

func (m *Manager) storeStatus(status outbox.Status) {
	m.statusMu.Lock()
	m.status = status
	m.statusMu.Unlock()
}

func (m *Manager) DeliveryStatus() outbox.Status {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return m.status
}

func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.queue)
		<-m.done
		if m.outbox != nil && m.DeliveryStatus().State != "impaired" {
			select {
			case <-m.deliveryIdle:
			default:
			}
			m.signalDelivery()
			select {
			case <-m.deliveryIdle:
			case <-time.After(5 * time.Second):
			}
		}
		if m.deliveryCancel != nil {
			m.deliveryCancel()
		}
		if m.deliveryDone != nil {
			<-m.deliveryDone
		}
	})
}

func (m *Manager) signalDelivery() {
	if m.outbox == nil {
		return
	}
	select {
	case m.deliveryWake <- struct{}{}:
	default:
	}
}

func (m *Manager) runDelivery(ctx context.Context) {
	defer close(m.deliveryDone)
	if m.outbox == nil {
		return
	}
	for {
		select {
		case <-m.deliveryWake:
			if err := m.drain(ctx); err != nil && ctx.Err() == nil {
				diagnostics.LogError("durable delivery worker failed", err, nil)
			}
			status := m.DeliveryStatus()
			if status.State == "backlogged" {
				m.signalDelivery()
			}
			select {
			case m.deliveryIdle <- struct{}{}:
			default:
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) run() {
	defer close(m.done)
	var pendingErr error
	for request := range m.queue {
		if request.external {
			request.result <- m.handleEvents(request.batchID, request.events)
			continue
		}
		if request.flush {
			flushCtx := request.ctx
			if flushCtx == nil {
				flushCtx = context.Background()
			}
			err := m.flush(flushCtx, true)
			request.result <- errors.Join(pendingErr, err)
			pendingErr = nil
			continue
		}
		if err := m.handleResult(request.individual, request.summary); err != nil {
			pendingErr = errors.Join(pendingErr, err)
		}
	}
	if m.fileWriter != nil {
		_ = m.fileWriter.Flush()
		_ = m.fileWriter.Close()
	}
	if m.hecWriter != nil {
		_ = m.hecWriter.Close()
	}
	if m.metricsSender != nil {
		_ = m.metricsSender.Close()
	}
}

func min(a, b int) int {
	if a < b {
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
