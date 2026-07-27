package models

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
)

const SchemaVersion = 4

// Endpoint matches endpoints.csv schema.
// Note: Field names match v4 payload keys (lowercase + underscores) where applicable.
type Endpoint struct {
	EndpointID          string   `json:"endpoint_id,omitempty"`
	IP                  string   `json:"ip"`
	Hostname            string   `json:"hostname"`
	FQDN                string   `json:"fqdn,omitempty"`
	Dev                 bool     `json:"dev"`
	MonitoringEnabled   *bool    `json:"monitoring_enabled,omitempty"`
	MaintenanceUntil    string   `json:"maintenance_until,omitempty"`
	MaintenanceReason   string   `json:"maintenance_reason,omitempty"`
	DNSStatus           string   `json:"dns_status,omitempty"`
	DNSForwardConfirmed bool     `json:"dns_forward_confirmed,omitempty"`
	DiscoveredAt        string   `json:"discovered_at,omitempty"`
	DiscoveryScanID     string   `json:"discovery_scan_id,omitempty"`
	DiscoverySource     string   `json:"discovery_source,omitempty"`
	DiscoveryLatencyMs  *float64 `json:"discovery_latency_ms,omitempty"`
	Group               string   `json:"group"`
	Description         string   `json:"description"`
	EntityType          string   `json:"entitytype"`
	Device              string   `json:"device"`
	Vendor              string   `json:"vendor"`
	AdditionalNotes     string   `json:"additional_notes"`
}

func (e Endpoint) IsMonitoringEnabled() bool {
	return e.MonitoringEnabled == nil || *e.MonitoringEnabled
}

func Bool(value bool) *bool {
	return &value
}

type PingEvent struct {
	SchemaVersion       int      `json:"schema_version"`
	EventID             string   `json:"event_id"`
	CollectorID         string   `json:"collector_id"`
	EndpointID          string   `json:"endpoint_id"`
	CycleID             string   `json:"cycle_id"`
	Timestamp           string   `json:"timestamp"`
	SentAt              string   `json:"sent_at"`
	ReceivedAt          string   `json:"received_at,omitempty"`
	TargetIP            string   `json:"target_ip"`
	Hostname            string   `json:"hostname"`
	FQDN                string   `json:"fqdn,omitempty"`
	Dev                 bool     `json:"dev"`
	MonitoringEnabled   bool     `json:"monitoring_enabled"`
	Group               string   `json:"group"`
	Description         string   `json:"description"`
	EntityType          string   `json:"entitytype"`
	Device              string   `json:"device"`
	Vendor              string   `json:"vendor"`
	Notes               string   `json:"additional_notes"`
	Status              string   `json:"status"`
	ObservationStatus   string   `json:"observation_status"`
	MeasurementValid    bool     `json:"measurement_valid"`
	ProbeBackend        string   `json:"probe_backend"`
	LatencyMs           *float64 `json:"latency_ms,omitempty"`
	LatencySource       string   `json:"latency_source,omitempty"`
	LatencyResolutionMs *float64 `json:"latency_resolution_ms,omitempty"`
	LatencyCensored     bool     `json:"latency_censored"`
	LatencyUpperBoundMs *float64 `json:"latency_upper_bound_ms,omitempty"`
	ProbeElapsedMs      *float64 `json:"probe_elapsed_ms,omitempty"`
	ICMPStatusCode      *uint32  `json:"icmp_status_code,omitempty"`
	TTL                 *int     `json:"ttl,omitempty"`
	PingNumber          int      `json:"ping_number"`
	PingsInCycle        int      `json:"pings_in_cycle"`
	ErrorMessage        *string  `json:"error_message,omitempty"`
	RecordType          string   `json:"record_type"`
}

type SummaryEvent struct {
	SchemaVersion          int      `json:"schema_version"`
	EventID                string   `json:"event_id"`
	CollectorID            string   `json:"collector_id"`
	EndpointID             string   `json:"endpoint_id"`
	CycleID                string   `json:"cycle_id"`
	Timestamp              string   `json:"timestamp"`
	TargetIP               string   `json:"target_ip"`
	Hostname               string   `json:"hostname"`
	FQDN                   string   `json:"fqdn,omitempty"`
	Dev                    bool     `json:"dev"`
	MonitoringEnabled      bool     `json:"monitoring_enabled"`
	MaintenanceUntil       string   `json:"maintenance_until,omitempty"`
	MaintenanceReason      string   `json:"maintenance_reason,omitempty"`
	Group                  string   `json:"group"`
	Description            string   `json:"description"`
	EntityType             string   `json:"entitytype"`
	Device                 string   `json:"device"`
	Vendor                 string   `json:"vendor"`
	Notes                  string   `json:"additional_notes"`
	RecordType             string   `json:"record_type"`
	ProbeBackend           string   `json:"probe_backend"`
	MeasurementValid       bool     `json:"measurement_valid"`
	ObservationStatus      string   `json:"observation_status"`
	ObservationError       string   `json:"observation_error,omitempty"`
	State                  string   `json:"state"`
	StateReason            string   `json:"state_reason"`
	StateConfidence        string   `json:"state_confidence"`
	DownAfterFailures      int      `json:"down_after_failures"`
	RecoveryAfterSuccesses int      `json:"recovery_after_successes"`
	CycleIntervalSeconds   int      `json:"cycle_interval_seconds"`
	StaleAfterIntervals    int      `json:"stale_after_intervals"`
	StaleAfterSeconds      int      `json:"stale_after_seconds"`
	PreviousState          string   `json:"previous_state"`
	StateChangedAt         string   `json:"state_changed_at"`
	ConsecutiveSuccesses   int      `json:"consecutive_successes"`
	ConsecutiveFailures    int      `json:"consecutive_failures"`
	PingsSent              int      `json:"pings_sent"`
	PingsSuccessful        int      `json:"pings_successful"`
	PingsFailed            int      `json:"pings_failed"`
	PacketLossPct          *float64 `json:"packet_loss_pct,omitempty"`
	AvgLatencyMs           *float64 `json:"avg_latency_ms,omitempty"`
	MinLatencyMs           *float64 `json:"min_latency_ms,omitempty"`
	MaxLatencyMs           *float64 `json:"max_latency_ms,omitempty"`
	LatencySampleCount     int      `json:"latency_sample_count"`
	LatencyCensoredCount   int      `json:"latency_censored_count"`
	LatencySource          string   `json:"latency_source,omitempty"`
	LatencyResolutionMs    *float64 `json:"latency_resolution_ms,omitempty"`
	LatencyUpperBoundMs    *float64 `json:"latency_upper_bound_ms,omitempty"`
}

// DiscoveryEvent is point-in-time subnet discovery evidence. It intentionally
// does not use monitoring state or packet-loss fields: absence from one scan is
// "not_observed", not proof that an asset is down or decommissioned.
type DiscoveryEvent struct {
	SchemaVersion       int      `json:"schema_version"`
	EventID             string   `json:"event_id"`
	CollectorID         string   `json:"collector_id"`
	CollectorHost       string   `json:"collector_host"`
	CycleID             string   `json:"cycle_id"`
	Timestamp           string   `json:"timestamp"`
	RecordType          string   `json:"record_type"`
	EvidenceKind        string   `json:"evidence_kind"`
	ScanID              string   `json:"scan_id"`
	PreviousScanID      string   `json:"previous_scan_id,omitempty"`
	ScheduleID          string   `json:"schedule_id,omitempty"`
	TargetNetwork       string   `json:"target_network"`
	TargetIP            string   `json:"target_ip"`
	EndpointID          string   `json:"endpoint_id"`
	Hostname            string   `json:"hostname,omitempty"`
	FQDN                string   `json:"fqdn,omitempty"`
	Dev                 bool     `json:"dev"`
	MonitoringEnabled   bool     `json:"monitoring_enabled"`
	MaintenanceUntil    string   `json:"maintenance_until,omitempty"`
	MaintenanceReason   string   `json:"maintenance_reason,omitempty"`
	Group               string   `json:"group,omitempty"`
	Description         string   `json:"description,omitempty"`
	EntityType          string   `json:"entitytype,omitempty"`
	Device              string   `json:"device,omitempty"`
	Vendor              string   `json:"vendor,omitempty"`
	Notes               string   `json:"additional_notes,omitempty"`
	DNSStatus           string   `json:"dns_status,omitempty"`
	DNSForwardConfirmed bool     `json:"dns_forward_confirmed"`
	DiscoveredAt        string   `json:"discovered_at,omitempty"`
	DiscoverySource     string   `json:"discovery_source,omitempty"`
	DiscoveryLatencyMs  *float64 `json:"discovery_latency_ms,omitempty"`
	DiscoveryStatus     string   `json:"discovery_status"`
	DiscoveryDelta      string   `json:"discovery_delta_status"`
	DiscoveryObserved   bool     `json:"discovery_observed"`
	BaselineAvailable   bool     `json:"baseline_available"`
	ScanDurationMs      int64    `json:"scan_duration_ms,omitempty"`
}

// DiscoveryScanEvent records that a scan completed even when it observed no
// endpoints. It is the audit/control-plane companion to DiscoveryEvent.
type DiscoveryScanEvent struct {
	SchemaVersion     int    `json:"schema_version"`
	EventID           string `json:"event_id"`
	CollectorID       string `json:"collector_id"`
	CollectorHost     string `json:"collector_host"`
	CycleID           string `json:"cycle_id"`
	Timestamp         string `json:"timestamp"`
	RecordType        string `json:"record_type"`
	EvidenceKind      string `json:"evidence_kind"`
	ScanID            string `json:"scan_id"`
	PreviousScanID    string `json:"previous_scan_id,omitempty"`
	ScheduleID        string `json:"schedule_id,omitempty"`
	TargetNetwork     string `json:"target_network"`
	BaselineAvailable bool   `json:"baseline_available"`
	EndpointsObserved int    `json:"endpoints_observed"`
	NewEndpoints      int    `json:"new_endpoints"`
	MissingEndpoints  int    `json:"missing_endpoints"`
	Unchanged         int    `json:"unchanged_endpoints"`
	ScanDurationMs    int64  `json:"scan_duration_ms,omitempty"`
	TimeoutMs         int    `json:"timeout_ms"`
	ThrottleLimit     int    `json:"throttle_limit"`
}

type MetricsEvent struct {
	Time       float64                `json:"time"`
	Host       string                 `json:"host"`
	Source     string                 `json:"source"`
	SourceType string                 `json:"sourcetype"`
	Index      string                 `json:"index"`
	Event      string                 `json:"event"`
	Fields     map[string]interface{} `json:"fields"`
}

type HECEvent struct {
	Time       float64 `json:"time"`
	Host       string  `json:"host"`
	Source     string  `json:"source"`
	SourceType string  `json:"sourcetype"`
	Index      string  `json:"index"`
	// event_id is inside Event payload (search-time dedupe)
	Event interface{} `json:"event"`
}

func StableEndpointID(explicitID string, target string) string {
	if id := strings.TrimSpace(explicitID); id != "" {
		return id
	}
	normalizedTarget := strings.ToLower(strings.TrimSpace(target))
	if ip := net.ParseIP(normalizedTarget); ip != nil {
		normalizedTarget = ip.String()
	}
	sum := sha256.Sum256([]byte(normalizedTarget))
	return "ep_" + hex.EncodeToString(sum[:12])
}
