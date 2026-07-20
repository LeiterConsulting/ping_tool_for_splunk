package advisor

import (
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/schedule"
)

const SchemaVersion = 1

type Severity string

const (
	SeverityBlocker     Severity = "blocker"
	SeverityWarning     Severity = "warning"
	SeverityOpportunity Severity = "opportunity"
	SeverityInfo        Severity = "info"
)

type Finding struct {
	Code           string         `json:"code"`
	Severity       Severity       `json:"severity"`
	Category       string         `json:"category"`
	Title          string         `json:"title"`
	Message        string         `json:"message"`
	Evidence       []string       `json:"evidence,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
	FixID          string         `json:"fix_id,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

type Change struct {
	Path            string `json:"path"`
	Before          any    `json:"before"`
	After           any    `json:"after"`
	Reason          string `json:"reason"`
	RequiresRestart bool   `json:"requires_restart"`
}

type Fix struct {
	ID              string   `json:"id"`
	Safety          string   `json:"safety"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Target          string   `json:"target"`
	Changes         []Change `json:"changes,omitempty"`
	Rows            []int    `json:"rows,omitempty"`
	RequiresRestart bool     `json:"requires_restart"`
}

type Summary struct {
	Blockers      int  `json:"blockers"`
	Warnings      int  `json:"warnings"`
	Opportunities int  `json:"opportunities"`
	Info          int  `json:"info"`
	ReadyToRun    bool `json:"ready_to_run"`
	SafeFixes     int  `json:"safe_fixes"`
}

type Inventory struct {
	Path                 string            `json:"path"`
	Rows                 int               `json:"rows"`
	SchedulableEndpoints int               `json:"schedulable_endpoints"`
	DuplicateTargets     int               `json:"duplicate_targets"`
	DuplicateIDs         int               `json:"duplicate_ids"`
	InvalidAddresses     int               `json:"invalid_addresses"`
	MissingHostnames     int               `json:"missing_hostnames"`
	SafeDuplicateRows    []int             `json:"safe_duplicate_rows,omitempty"`
	NormalizedRows       []models.Endpoint `json:"-"`
}

type Proposal struct {
	Profile       Profile       `json:"profile"`
	Config        config.Config `json:"-"`
	Schedule      schedule.Plan `json:"schedule"`
	Changes       []Change      `json:"changes"`
	RestartNeeded bool          `json:"restart_needed"`
}

type Report struct {
	SchemaVersion    int            `json:"schema_version"`
	ProductVersion   string         `json:"product_version"`
	GeneratedAt      time.Time      `json:"generated_at"`
	ConfigPath       string         `json:"config_path"`
	EndpointsPath    string         `json:"endpoints_path"`
	ConfigSource     string         `json:"config_source,omitempty"`
	ConfigRevision   string         `json:"config_revision,omitempty"`
	EndpointRevision string         `json:"endpoints_revision,omitempty"`
	Profile          string         `json:"profile"`
	Summary          Summary        `json:"summary"`
	Inventory        Inventory      `json:"inventory"`
	Schedule         *schedule.Plan `json:"schedule,omitempty"`
	Findings         []Finding      `json:"findings"`
	Fixes            []Fix          `json:"fixes,omitempty"`
	Proposal         *Proposal      `json:"proposal,omitempty"`
	LoadedConfig     *config.Config `json:"-"`
}

type AnalyzeOptions struct {
	ConfigPath     string
	EndpointsPath  string
	RootDir        string
	Profile        string
	ProductVersion string
}

type ApplyResult struct {
	AppliedFixes     []string `json:"applied_fixes"`
	ConfigChanged    bool     `json:"config_changed"`
	EndpointsChanged bool     `json:"endpoints_changed"`
	RestartRequired  bool     `json:"restart_required"`
	Report           Report   `json:"report"`
}
