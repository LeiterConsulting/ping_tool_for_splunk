package advisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/schedule"
)

func AnalyzeDeployment(ctx context.Context, opts AnalyzeOptions) Report {
	profileID := opts.Profile
	if profileID == "" {
		profileID = "standard"
	}
	report := Report{SchemaVersion: SchemaVersion, ProductVersion: opts.ProductVersion, GeneratedAt: time.Now().UTC(), ConfigPath: opts.ConfigPath, EndpointsPath: opts.EndpointsPath, Profile: profileID}
	if info, resolveErr := config.ResolveConfigSource(opts.ConfigPath); resolveErr == nil {
		report.ConfigRevision, _ = revision.File(info.Path)
	}
	report.EndpointRevision, _ = revision.File(opts.EndpointsPath)
	report.Inventory, report.Findings = inspectInventory(opts.EndpointsPath)

	cfg, source, err := config.Load(ctx, opts.ConfigPath, opts.RootDir)
	if err != nil {
		report.Findings = append(report.Findings, blocker("CFG_LOAD", "Configuration cannot be loaded", err.Error(), "Repair the configuration syntax before applying recommendations."))
	} else {
		report.ConfigSource = source
		report.LoadedConfig = &cfg
		analyzeConfig(&report, cfg)
		if report.Inventory.SchedulableEndpoints > 0 {
			plan := schedule.Analyze(cfg, report.Inventory.SchedulableEndpoints)
			report.Schedule = &plan
			if !plan.Fits {
				report.Findings = append(report.Findings, Finding{Code: "SCHED_CAPACITY", Severity: SeverityBlocker, Category: "schedule", Title: "Monitoring schedule cannot preserve its configured cadence", Message: plan.Error().Error(), Evidence: []string{
					fmt.Sprintf("%d unique endpoints", plan.EndpointCount), fmt.Sprintf("%.2fs worst-case probe budget per endpoint", float64(plan.ProbeBudgetMs)/1000), fmt.Sprintf("%.2fs modeled worst-case cycle", float64(plan.WorstCaseCycleMs)/1000),
				}, Recommendation: fmt.Sprintf("Use at least %d endpoint workers (%d recommended), increase the interval to at least %ds, or reduce timeout/ping count.", plan.RequiredWorkers, plan.RecommendedWorkers, plan.RequiredIntervalSeconds), FixID: "apply_profile"})
			} else if plan.CapacityHeadroomPct < 10 {
				report.Findings = append(report.Findings, Finding{Code: "SCHED_HEADROOM", Severity: SeverityWarning, Category: "schedule", Title: "Schedule has little outage headroom", Message: fmt.Sprintf("The current worker count is only %.1f%% above the queue-free minimum.", plan.CapacityHeadroomPct), Recommendation: fmt.Sprintf("Use %d endpoint workers for operational headroom.", plan.RecommendedWorkers), FixID: "apply_profile"})
			} else {
				report.Findings = append(report.Findings, Finding{Code: "SCHED_READY", Severity: SeverityInfo, Category: "schedule", Title: "Worst-case schedule fits", Message: fmt.Sprintf("The modeled cycle is %.2fs within a %ds interval with %.1f%% worker headroom.", float64(plan.WorstCaseCycleMs)/1000, plan.IntervalSeconds, plan.CapacityHeadroomPct)})
			}
			if profile, profileErr := GetProfile(profileID); profileErr != nil {
				report.Findings = append(report.Findings, blocker("PROFILE_UNKNOWN", "Optimization profile is not recognized", profileErr.Error(), "Select one of the published advisor profiles."))
			} else {
				proposal := BuildProposal(cfg, report.Inventory.SchedulableEndpoints, profile)
				report.Proposal = &proposal
				if len(proposal.Changes) > 0 {
					report.Fixes = append(report.Fixes, Fix{ID: "apply_profile", Safety: "confirmation_required", Title: "Apply " + profile.Name, Description: profile.Description, Target: "config", Changes: proposal.Changes, RequiresRestart: true})
				}
			}
		}
	}

	if len(report.Inventory.SafeDuplicateRows) > 0 && noUnsafeInventoryBlockers(report.Findings) {
		report.Fixes = append(report.Fixes, Fix{ID: "remove_identical_duplicates", Safety: "safe", Title: "Remove identical duplicate endpoints", Description: "Remove only later rows whose normalized metadata is identical to the first target row.", Target: "endpoints", Rows: report.Inventory.SafeDuplicateRows})
	}
	if hasFinding(report.Findings, "INV_WHITESPACE") && noUnsafeInventoryBlockers(report.Findings) {
		report.Fixes = append(report.Fixes, Fix{ID: "normalize_inventory", Safety: "safe", Title: "Normalize endpoint inventory", Description: "Trim surrounding whitespace, canonicalize IPs, apply default groups, and persist stable endpoint IDs.", Target: "endpoints"})
	}

	sortFindings(report.Findings)
	report.Summary = summarize(report.Findings, report.Fixes)
	return report
}

func analyzeConfig(report *Report, cfg config.Config) {
	if cfg.TimeoutMs > 2000 {
		report.Findings = append(report.Findings, Finding{Code: "CFG_TIMEOUT_HIGH", Severity: SeverityWarning, Category: "signal", Title: "No-reply timeout is high", Message: fmt.Sprintf("Each missing reply may consume %d ms. This does not improve latency precision; it only delays no-reply classification.", cfg.TimeoutMs), Recommendation: "Use 1000 ms for ordinary LAN/WAN monitoring or 1500 ms for known high-latency paths."})
	}
	if cfg.PingsPerCycle < 3 {
		report.Findings = append(report.Findings, Finding{Code: "CFG_SAMPLE_SMALL", Severity: SeverityWarning, Category: "signal", Title: "Packet-loss sample is small", Message: fmt.Sprintf("Only %d attempt(s) contribute to each observation.", cfg.PingsPerCycle), Recommendation: "Use at least four attempts for standard monitoring unless resource constraints are explicit."})
	}
	if cfg.PingsPerCycle > 10 {
		report.Findings = append(report.Findings, Finding{Code: "CFG_SAMPLE_LARGE", Severity: SeverityOpportunity, Category: "performance", Title: "Ping sample may be larger than necessary", Message: fmt.Sprintf("%d attempts are clustered within each observation and increase worst-case probe time.", cfg.PingsPerCycle), Recommendation: "Prefer a shorter observation interval when temporal coverage matters more than within-observation sample size."})
	}
	downSeconds := cfg.CycleIntervalSeconds * cfg.Health.DownAfterFailures
	report.Findings = append(report.Findings, Finding{Code: "SLA_DOWN_TIME", Severity: SeverityInfo, Category: "sla", Title: "Modeled down-state confirmation", Message: fmt.Sprintf("A continuously unreachable endpoint is confirmed down after approximately %d seconds (%d cycles).", downSeconds, cfg.Health.DownAfterFailures), Recommendation: "Confirm this matches the contractor SLA; current no-reply observations remain visible before state confirmation."})
	if (cfg.OutputMode == "hec" || cfg.OutputMode == "both") && (!cfg.HEC.Enabled || strings.TrimSpace(cfg.HEC.URL) == "" || strings.TrimSpace(cfg.HEC.Token) == "") {
		report.Findings = append(report.Findings, blocker("HEC_INCOMPLETE", "HEC output is incomplete", "The selected output mode requires HEC, but enabled/url/token settings are incomplete.", "Complete HEC settings or select file-only output."))
	}
	if cfg.Metrics.Enabled && (strings.TrimSpace(cfg.Metrics.HECURL) == "" || strings.TrimSpace(cfg.Metrics.Token) == "" || strings.TrimSpace(cfg.Metrics.Index) == "") {
		report.Findings = append(report.Findings, blocker("METRICS_INCOMPLETE", "Metrics output is incomplete", "Metrics are enabled but URL, token, or index is missing.", "Complete the metrics destination before service startup."))
	}
	for code, label := range map[string]string{"PATH_LOG": "Log directory", "PATH_OUTBOX": "Durable outbox directory"} {
		target := ""
		if code == "PATH_LOG" {
			target = cfg.LogPath
		} else {
			target = cfg.Delivery.SpoolPath
		}
		if target == "" {
			continue
		}
		directory := target
		if filepath.Ext(target) != "" {
			directory = filepath.Dir(target)
		}
		if info, err := os.Stat(directory); err == nil && !info.IsDir() {
			report.Findings = append(report.Findings, blocker(code, label+" is not a directory", directory, "Correct the path before startup."))
		} else if err != nil && !os.IsNotExist(err) {
			report.Findings = append(report.Findings, Finding{Code: code, Severity: SeverityWarning, Category: "filesystem", Title: label + " cannot be inspected", Message: err.Error(), Recommendation: "Verify service-account permissions."})
		}
	}
}

func blocker(code string, title string, message string, recommendation string) Finding {
	return Finding{Code: code, Severity: SeverityBlocker, Category: categoryForCode(code), Title: title, Message: message, Recommendation: recommendation}
}

func categoryForCode(code string) string {
	if strings.HasPrefix(code, "INV_") {
		return "inventory"
	}
	if strings.HasPrefix(code, "SCHED_") {
		return "schedule"
	}
	if strings.HasPrefix(code, "HEC_") || strings.HasPrefix(code, "METRICS_") {
		return "delivery"
	}
	if strings.HasPrefix(code, "PATH_") {
		return "filesystem"
	}
	return "configuration"
}

func summarize(findings []Finding, fixes []Fix) Summary {
	result := Summary{}
	for _, finding := range findings {
		switch finding.Severity {
		case SeverityBlocker:
			result.Blockers++
		case SeverityWarning:
			result.Warnings++
		case SeverityOpportunity:
			result.Opportunities++
		case SeverityInfo:
			result.Info++
		}
	}
	for _, fix := range fixes {
		if fix.Safety == "safe" {
			result.SafeFixes++
		}
	}
	result.ReadyToRun = result.Blockers == 0
	return result
}

func sortFindings(findings []Finding) {
	order := map[Severity]int{SeverityBlocker: 0, SeverityWarning: 1, SeverityOpportunity: 2, SeverityInfo: 3}
	sort.SliceStable(findings, func(i, j int) bool {
		if order[findings[i].Severity] != order[findings[j].Severity] {
			return order[findings[i].Severity] < order[findings[j].Severity]
		}
		return findings[i].Code < findings[j].Code
	})
}

func countSeverity(findings []Finding, severity Severity) int {
	count := 0
	for _, finding := range findings {
		if finding.Severity == severity {
			count++
		}
	}
	return count
}
func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
func noUnsafeInventoryBlockers(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Category == "inventory" && finding.Severity == SeverityBlocker {
			return false
		}
	}
	return true
}
