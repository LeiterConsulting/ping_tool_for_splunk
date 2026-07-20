package schedule

import (
	"strings"
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

func TestAnalyzeCustomerWorkloadRequiresQueueFreeWorkers(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 4
	cfg.TimeoutMs = 3000
	cfg.CycleIntervalSeconds = 60
	cfg.ParallelThreads = 30
	plan := Analyze(cfg, 166)
	if plan.Fits {
		t.Fatal("30 workers unexpectedly fit")
	}
	if plan.ProbeBudgetMs != 12500 {
		t.Fatalf("ProbeBudgetMs = %d, want 12500", plan.ProbeBudgetMs)
	}
	if plan.RequiredWorkers != 44 || plan.RecommendedWorkers != 50 {
		t.Fatalf("workers = required %d recommended %d, want 44/50", plan.RequiredWorkers, plan.RecommendedWorkers)
	}
	if plan.WorstCaseCycleMs <= 60000 {
		t.Fatalf("WorstCaseCycleMs = %d, want overrun", plan.WorstCaseCycleMs)
	}
	if err := plan.Error(); err == nil || !strings.Contains(err.Error(), "at least 44 workers") {
		t.Fatalf("Error() = %v", err)
	}
}

func TestAnalyzeStandardProfileHasHeadroom(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.ParallelThreads = 20
	plan := Analyze(cfg, 166)
	if !plan.Fits {
		t.Fatalf("standard plan did not fit: %#v", plan)
	}
	if plan.RequiredWorkers != 14 || plan.RecommendedWorkers != 20 {
		t.Fatalf("workers = required %d recommended %d, want 14/20", plan.RequiredWorkers, plan.RecommendedWorkers)
	}
}

func TestAnalyzeRejectsProbeBudgetAtInterval(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.PingsPerCycle = 2
	cfg.TimeoutMs = 30000
	plan := Analyze(cfg, 2)
	if plan.Fits || plan.Reason == "" {
		t.Fatalf("expected non-fitting plan with reason: %#v", plan)
	}
}
