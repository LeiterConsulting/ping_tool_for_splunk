package advisor

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func WriteText(writer io.Writer, report Report) {
	status := "READY"
	if !report.Summary.ReadyToRun {
		status = "BLOCKED"
	}
	fmt.Fprintf(writer, "Ping Monitor Configuration Advisor: %s\n", status)
	fmt.Fprintf(writer, "Profile: %s | Endpoints: %d rows / %d unique schedulable\n", report.Profile, report.Inventory.Rows, report.Inventory.SchedulableEndpoints)
	fmt.Fprintf(writer, "Findings: %d blockers, %d warnings, %d opportunities, %d informational\n", report.Summary.Blockers, report.Summary.Warnings, report.Summary.Opportunities, report.Summary.Info)
	if report.Schedule != nil {
		plan := report.Schedule
		fmt.Fprintf(writer, "Schedule: %.2fs probe budget, %.2fs modeled worst case, %d current / %d required / %d recommended workers\n",
			float64(plan.ProbeBudgetMs)/1000, float64(plan.WorstCaseCycleMs)/1000, plan.Workers, plan.RequiredWorkers, plan.RecommendedWorkers)
	}
	for _, finding := range report.Findings {
		fmt.Fprintf(writer, "\n[%s] %s (%s)\n", strings.ToUpper(string(finding.Severity)), finding.Title, finding.Code)
		fmt.Fprintln(writer, finding.Message)
		for _, evidence := range finding.Evidence {
			fmt.Fprintf(writer, "  - %s\n", evidence)
		}
		if finding.Recommendation != "" {
			fmt.Fprintf(writer, "  Recommendation: %s\n", finding.Recommendation)
		}
	}
	if report.Proposal != nil && len(report.Proposal.Changes) > 0 {
		fmt.Fprintf(writer, "\nProposed %s changes:\n", report.Proposal.Profile.Name)
		for _, change := range report.Proposal.Changes {
			fmt.Fprintf(writer, "  %s: %v -> %v (%s)\n", change.Path, change.Before, change.After, change.Reason)
		}
	}
	if report.Summary.SafeFixes > 0 {
		fmt.Fprintf(writer, "\n%d safe fix set(s) are available. Preview with 'pingmonitor optimize' before applying.\n", report.Summary.SafeFixes)
	}
}
