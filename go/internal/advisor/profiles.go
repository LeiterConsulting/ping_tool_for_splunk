package advisor

import (
	"fmt"
	"sort"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/schedule"
)

type Profile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PingsPerCycle   int    `json:"pings_per_cycle"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutMs       int    `json:"timeout_ms"`
}

var profiles = map[string]Profile{
	"standard":        {ID: "standard", Name: "Standard Monitoring", Description: "One-minute observations with four attempts and a one-second no-reply threshold.", PingsPerCycle: 4, IntervalSeconds: 60, TimeoutMs: 1000},
	"sla":             {ID: "sla", Name: "SLA / High Confidence", Description: "Six attempts per one-minute observation for a larger packet-loss sample.", PingsPerCycle: 6, IntervalSeconds: 60, TimeoutMs: 1000},
	"high-latency":    {ID: "high-latency", Name: "High-Latency WAN", Description: "One-minute observations with a 1.5-second no-reply threshold.", PingsPerCycle: 4, IntervalSeconds: 60, TimeoutMs: 1500},
	"large-inventory": {ID: "large-inventory", Name: "Large Inventory", Description: "One-minute observations tuned for broad inventories and bounded probe time.", PingsPerCycle: 4, IntervalSeconds: 60, TimeoutMs: 1000},
	"low-resource":    {ID: "low-resource", Name: "Low Resource", Description: "Two-minute observations that reduce concurrency and output pressure.", PingsPerCycle: 4, IntervalSeconds: 120, TimeoutMs: 1000},
	"current":         {ID: "current", Name: "Current Signal Semantics", Description: "Preserve ping, timeout, and cadence settings; right-size endpoint workers only."},
}

func Profiles() []Profile {
	result := make([]Profile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, profile)
	}
	sort.Slice(result, func(i int, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func GetProfile(id string) (Profile, error) {
	if id == "" {
		id = "standard"
	}
	profile, ok := profiles[id]
	if !ok {
		return Profile{}, fmt.Errorf("unknown advisor profile %q", id)
	}
	return profile, nil
}

func BuildProposal(current config.Config, endpointCount int, profile Profile) Proposal {
	proposed := current
	changes := make([]Change, 0, 4)
	if profile.ID != "current" {
		changes = appendIntChange(changes, "pings_per_cycle", current.PingsPerCycle, profile.PingsPerCycle, "profile observation sample size")
		changes = appendIntChange(changes, "cycle_interval_seconds", current.CycleIntervalSeconds, profile.IntervalSeconds, "profile observation cadence")
		changes = appendIntChange(changes, "timeout_ms", current.TimeoutMs, profile.TimeoutMs, "profile no-reply threshold")
		proposed.PingsPerCycle = profile.PingsPerCycle
		proposed.CycleIntervalSeconds = profile.IntervalSeconds
		proposed.TimeoutMs = profile.TimeoutMs
	}
	initialPlan := schedule.Analyze(proposed, endpointCount)
	recommendedWorkers := initialPlan.RecommendedWorkers
	if recommendedWorkers < 1 {
		recommendedWorkers = max(1, proposed.ParallelThreads)
	}
	changes = appendIntChange(changes, "parallel_threads", current.ParallelThreads, recommendedWorkers, "queue-free worst-case capacity with operational headroom")
	proposed.ParallelThreads = recommendedWorkers
	plan := schedule.Analyze(proposed, endpointCount)
	return Proposal{Profile: profile, Config: proposed, Schedule: plan, Changes: changes, RestartNeeded: len(changes) > 0}
}

func appendIntChange(changes []Change, path string, before int, after int, reason string) []Change {
	if before == after {
		return changes
	}
	return append(changes, Change{Path: path, Before: before, After: after, Reason: reason, RequiresRestart: true})
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
