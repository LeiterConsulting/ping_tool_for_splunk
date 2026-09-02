package metrics

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output/hec"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/util"
)

type Sender struct {
	cfg       config.Metrics
	hostname  string
	transport *hec.Writer
}

func New(cfg config.Metrics, hostname string, collectorID string) (*Sender, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.HECURL == "" || cfg.Token == "" {
		return nil, errors.New("metrics enabled but hec_url/token not configured")
	}
	transport, err := hec.New(config.HEC{
		Enabled: true, URL: cfg.HECURL, Token: cfg.Token, VerifySSL: cfg.VerifySSL,
		SSLProtocol: cfg.SSLProtocol, BatchSize: cfg.BatchSize,
		Retry:  config.Retry{Enabled: true, MaxAttempts: 3, BaseDelayMs: 250, JitterPct: 20, Backoff: "exponential"},
		UseACK: cfg.UseACK, ACKTimeoutSeconds: cfg.ACKTimeoutSeconds,
		ACKPollIntervalMs: cfg.ACKPollIntervalMs, Channel: cfg.Channel,
	}, hostname, collectorID+":metrics")
	if err != nil {
		return nil, err
	}
	return &Sender{cfg: cfg, hostname: hostname, transport: transport}, nil
}

func (s *Sender) BatchSize() int {
	if s.cfg.BatchSize < 1 {
		return 100
	}
	return s.cfg.BatchSize
}

func (s *Sender) SendSummaries(ctx context.Context, summaries []models.SummaryEvent) error {
	payloads := make([]json.RawMessage, 0, len(summaries))
	for _, summary := range summaries {
		encoded, err := json.Marshal(buildPayload(summary, s.cfg, s.hostname))
		if err != nil {
			return err
		}
		payloads = append(payloads, encoded)
	}
	return s.transport.SendPayloads(ctx, payloads)
}

func (s *Sender) Close() error { return s.transport.Close() }

func buildPayload(sum models.SummaryEvent, cfg config.Metrics, hostname string) models.MetricsEvent {
	unix := util.UnixTimeFromISO(sum.Timestamp)
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
		eventName = "metric"
	}

	fields := map[string]interface{}{
		// Metric aggregation timestamps such as _time are bucket boundaries, not
		// necessarily the time of the observation selected by latest(). Emit the
		// collector timestamp as a metric value so Splunk can make exact freshness
		// decisions without mistaking a live endpoint for a stale one.
		"metric_name:ping.observed_at_epoch":   unix,
		"metric_name:ping.pings_sent":          sum.PingsSent,
		"metric_name:ping.pings_successful":    sum.PingsSuccessful,
		"metric_name:ping.measurement_valid":   boolMetric(sum.MeasurementValid),
		"metric_name:ping.alerting_enabled":    boolMetric(sum.AlertingEnabled),
		"metric_name:ping.schema_version":      sum.SchemaVersion,
		"metric_name:ping.state_code":          stateMetric(sum.State),
		"metric_name:ping.observation_code":    observationMetric(sum.ObservationStatus),
		"metric_name:ping.state_confidence":    confidenceMetric(sum.StateConfidence),
		"metric_name:ping.stale_after_seconds": sum.StaleAfterSeconds,
		"hostname":                             sum.Hostname, "target_ip": sum.TargetIP,
		"collector_id": sum.CollectorID, "endpoint_id": sum.EndpointID,
		"state": sum.State, "observation_status": sum.ObservationStatus,
		"state_reason": sum.StateReason, "probe_backend": sum.ProbeBackend, "record_type": sum.RecordType,
		"dev": sum.Dev, "device_mode": sum.DeviceMode, "group": sum.Group, "description": sum.Description,
		"entitytype": sum.EntityType, "device": sum.Device, "vendor": sum.Vendor,
		"additional_notes": sum.Notes, "fqdn": sum.FQDN,
		"monitoring_enabled": sum.MonitoringEnabled, "alerting_enabled": sum.AlertingEnabled,
		"alerting_reason": sum.AlertingReason, "asset_id": sum.AssetID, "dynamic_address": sum.DynamicAddress,
		"subnet_id": sum.SubnetID, "subnet_name": sum.SubnetName, "subnet_vlan": sum.SubnetVLAN,
		"subnet_location": sum.SubnetLocation, "addressing_mode": sum.AddressingMode, "routing_domain": sum.RoutingDomain,
		"classification_source": sum.ClassificationSource,
		"maintenance_until":     sum.MaintenanceUntil, "maintenance_reason": sum.MaintenanceReason,
	}
	if sum.PacketLossPct != nil {
		fields["metric_name:ping.packet_loss_pct"] = *sum.PacketLossPct
	}
	if sum.AvgLatencyMs != nil {
		fields["metric_name:ping.avg_latency_ms"] = *sum.AvgLatencyMs
	}
	if sum.MinLatencyMs != nil {
		fields["metric_name:ping.min_latency_ms"] = *sum.MinLatencyMs
	}
	if sum.MaxLatencyMs != nil {
		fields["metric_name:ping.max_latency_ms"] = *sum.MaxLatencyMs
	}

	return models.MetricsEvent{Time: unix, Host: hostname, Source: "ping_monitor", SourceType: sourcetype, Index: cfg.Index, Event: eventName, Fields: fields}
}

func boolMetric(value bool) int {
	if value {
		return 1
	}
	return 0
}

func stateMetric(value string) float64 {
	switch value {
	case "up":
		return 1
	case "degraded":
		return 0.5
	case "down":
		return 0
	default:
		return -1
	}
}

func observationMetric(value string) float64 {
	switch value {
	case "reply":
		return 1
	case "partial_reply":
		return 0.5
	case "no_reply":
		return 0
	default:
		return -1
	}
}

func confidenceMetric(value string) float64 {
	switch value {
	case "confirmed":
		return 1
	case "pending":
		return 0.5
	default:
		return 0
	}
}
