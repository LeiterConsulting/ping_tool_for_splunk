package models

// Endpoint matches endpoints.csv schema.
// Note: Field names match v4 payload keys (lowercase + underscores) where applicable.
type Endpoint struct {
	IP              string `json:"ip"`
	Hostname        string `json:"hostname"`
	Group           string `json:"group"`
	Description     string `json:"description"`
	EntityType      string `json:"entitytype"`
	Device          string `json:"device"`
	Vendor          string `json:"vendor"`
	AdditionalNotes string `json:"additional_notes"`
}

type PingEvent struct {
	EventID      string  `json:"event_id"`
	Timestamp    string  `json:"timestamp"`
	TargetIP     string  `json:"target_ip"`
	Hostname     string  `json:"hostname"`
	Group        string  `json:"group"`
	Description  string  `json:"description"`
	EntityType   string  `json:"entitytype"`
	Device       string  `json:"device"`
	Vendor       string  `json:"vendor"`
	Notes        string  `json:"additional_notes"`
	Status       string  `json:"status"`
	LatencyMs    int     `json:"latency_ms"`
	TTL          int     `json:"ttl"`
	PingNumber   int     `json:"ping_number"`
	PingsInCycle int     `json:"pings_in_cycle"`
	ErrorMessage *string `json:"error_message,omitempty"`
	RecordType   string  `json:"record_type"`
}

type SummaryEvent struct {
	EventID         string  `json:"event_id"`
	Timestamp       string  `json:"timestamp"`
	TargetIP        string  `json:"target_ip"`
	Hostname        string  `json:"hostname"`
	Group           string  `json:"group"`
	Description     string  `json:"description"`
	EntityType      string  `json:"entitytype"`
	Device          string  `json:"device"`
	Vendor          string  `json:"vendor"`
	Notes           string  `json:"additional_notes"`
	RecordType      string  `json:"record_type"`
	PingsSent       int     `json:"pings_sent"`
	PingsSuccessful int     `json:"pings_successful"`
	PingsFailed     int     `json:"pings_failed"`
	PacketLossPct   float64 `json:"packet_loss_pct"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	MinLatencyMs    int     `json:"min_latency_ms"`
	MaxLatencyMs    int     `json:"max_latency_ms"`
}

type MetricsEvent struct {
	Time       int64                  `json:"time"`
	Host       string                 `json:"host"`
	Source     string                 `json:"source"`
	SourceType string                 `json:"sourcetype"`
	Index      string                 `json:"index"`
	Event      string                 `json:"event"`
	Fields     map[string]interface{} `json:"fields"`
}

type HECEvent struct {
	Time       int64  `json:"time"`
	Host       string `json:"host"`
	Source     string `json:"source"`
	SourceType string `json:"sourcetype"`
	Index      string `json:"index"`
	// event_id is inside Event payload (search-time dedupe)
	Event interface{} `json:"event"`
}
