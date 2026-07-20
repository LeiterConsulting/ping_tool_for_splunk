package advisor

import (
	"context"
	"fmt"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

type ApplyOptions struct {
	AnalyzeOptions
	ApplySafe    bool
	ApplyProfile bool
}

func Apply(ctx context.Context, opts ApplyOptions) (ApplyResult, error) {
	if opts.ApplySafe && opts.ApplyProfile {
		return ApplyResult{}, fmt.Errorf("apply safe inventory fixes and a profile in separate transactions so each file change can be verified independently")
	}
	report := AnalyzeDeployment(ctx, opts.AnalyzeOptions)
	result := ApplyResult{Report: report}
	if report.LoadedConfig == nil {
		return result, fmt.Errorf("configuration is not loadable; no changes were applied")
	}

	if opts.ApplySafe {
		if !noUnsafeInventoryBlockers(report.Findings) && (len(report.Inventory.SafeDuplicateRows) > 0 || hasFinding(report.Findings, "INV_WHITESPACE")) {
			return result, fmt.Errorf("inventory has ambiguous blockers; safe normalization was not applied")
		}
		if len(report.Inventory.SafeDuplicateRows) > 0 || hasFinding(report.Findings, "INV_WHITESPACE") {
			rows := filterSafeDuplicateRows(report.Inventory.NormalizedRows, report.Inventory.SafeDuplicateRows)
			if err := config.SaveEndpoints(opts.EndpointsPath, rows); err != nil {
				return result, fmt.Errorf("apply safe endpoint fixes: %w", err)
			}
			result.EndpointsChanged = true
			if len(report.Inventory.SafeDuplicateRows) > 0 {
				result.AppliedFixes = append(result.AppliedFixes, "remove_identical_duplicates")
			}
			if hasFinding(report.Findings, "INV_WHITESPACE") {
				result.AppliedFixes = append(result.AppliedFixes, "normalize_inventory")
			}
		}
	}

	if opts.ApplyProfile {
		if !noUnsafeInventoryBlockers(report.Findings) {
			return result, fmt.Errorf("profile cannot be sized while the inventory has ambiguous or invalid rows; repair the inventory and analyze again")
		}
		if report.Proposal == nil {
			return result, fmt.Errorf("profile proposal is unavailable; no configuration changes were applied")
		}
		if len(report.Proposal.Changes) > 0 {
			if _, err := config.SaveConfig(ctx, opts.ConfigPath, opts.RootDir, report.Proposal.Config); err != nil {
				return result, fmt.Errorf("apply profile: %w", err)
			}
			result.ConfigChanged = true
			result.RestartRequired = true
			result.AppliedFixes = append(result.AppliedFixes, "apply_profile")
		}
	}

	result.Report = AnalyzeDeployment(ctx, opts.AnalyzeOptions)
	return result, nil
}

func filterSafeDuplicateRows(rows []models.Endpoint, csvRows []int) []models.Endpoint {
	remove := make(map[int]struct{}, len(csvRows))
	for _, csvRow := range csvRows {
		remove[csvRow] = struct{}{}
	}
	result := make([]models.Endpoint, 0, len(rows))
	for index, row := range rows {
		csvRow := index + 2
		if _, shouldRemove := remove[csvRow]; shouldRemove {
			continue
		}
		result = append(result, row)
	}
	return result
}
