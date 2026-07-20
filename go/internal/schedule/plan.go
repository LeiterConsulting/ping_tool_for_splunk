package schedule

import (
	"fmt"
	"math"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

const recommendationHeadroom = 0.10

// Plan is the deterministic worst-case schedule model used by runtime
// admission, the configuration advisor, and service preflight.
type Plan struct {
	EndpointCount           int           `json:"endpoint_count"`
	PingsPerCycle           int           `json:"pings_per_cycle"`
	TimeoutMs               int           `json:"timeout_ms"`
	Workers                 int           `json:"workers"`
	IntervalSeconds         int           `json:"interval_seconds"`
	ProbeBudgetMs           int64         `json:"probe_budget_ms"`
	DispatchDelayMs         int64         `json:"dispatch_delay_ms"`
	WorstCaseCycleMs        int64         `json:"worst_case_cycle_ms"`
	AverageWorkerLoadPct    float64       `json:"average_worker_load_pct"`
	RequiredWorkers         int           `json:"required_workers"`
	RecommendedWorkers      int           `json:"recommended_workers"`
	RequiredIntervalSeconds int           `json:"required_interval_seconds"`
	CapacityHeadroomPct     float64       `json:"capacity_headroom_pct"`
	Fits                    bool          `json:"fits"`
	Reason                  string        `json:"reason,omitempty"`
	ProbeBudgetDuration     time.Duration `json:"-"`
	DispatchDelayDuration   time.Duration `json:"-"`
	IntervalDuration        time.Duration `json:"-"`
}

func Analyze(cfg config.Config, endpointCount int) Plan {
	count := max(1, cfg.PingsPerCycle)
	workers := max(1, cfg.ParallelThreads)
	timeout := time.Duration(max(1, cfg.TimeoutMs)) * time.Millisecond
	interval := time.Duration(max(0, cfg.CycleIntervalSeconds)) * time.Second

	// The slowest supported backend performs attempts sequentially. When an
	// attempt consumes its full timeout, the normal inter-probe spacing has
	// already elapsed and must not be added again. The guard covers dispatch,
	// result handling, and platform/process overhead.
	probeBudget := time.Duration(count)*timeout + 500*time.Millisecond
	plan := Plan{
		EndpointCount: endpointCount, PingsPerCycle: count, TimeoutMs: cfg.TimeoutMs,
		Workers: workers, IntervalSeconds: cfg.CycleIntervalSeconds, ProbeBudgetMs: probeBudget.Milliseconds(),
		ProbeBudgetDuration: probeBudget, IntervalDuration: interval,
	}

	if endpointCount <= 0 {
		plan.Fits = false
		plan.Reason = "no endpoints are available to schedule"
		return plan
	}
	if interval <= 0 {
		plan.RequiredWorkers = endpointCount
		plan.RecommendedWorkers = endpointCount
		plan.Fits = false
		plan.Reason = "cycle interval must be greater than zero"
		return plan
	}

	plan.AverageWorkerLoadPct = round1(float64(endpointCount) * float64(probeBudget) / (float64(workers) * float64(interval)) * 100)
	if endpointCount == 1 {
		plan.RequiredWorkers = 1
		plan.RecommendedWorkers = 1
		plan.WorstCaseCycleMs = probeBudget.Milliseconds()
		plan.RequiredIntervalSeconds = int(math.Ceil(probeBudget.Seconds()))
		plan.CapacityHeadroomPct = round1(float64(workers-1) * 100)
		plan.Fits = probeBudget <= interval
		if !plan.Fits {
			plan.Reason = "one endpoint's worst-case probe budget exceeds the cycle interval"
		}
		return plan
	}
	if probeBudget >= interval {
		plan.RequiredWorkers = endpointCount
		plan.RecommendedWorkers = endpointCount
		plan.WorstCaseCycleMs = probeBudget.Milliseconds()
		plan.RequiredIntervalSeconds = int(math.Floor(probeBudget.Seconds())) + 1
		plan.Fits = false
		plan.Reason = "the per-endpoint worst-case probe budget leaves no dispatch window"
		return plan
	}

	dispatchDelay := (interval - probeBudget) / time.Duration(endpointCount-1)
	plan.DispatchDelayMs = dispatchDelay.Milliseconds()
	plan.DispatchDelayDuration = dispatchDelay
	plan.RequiredWorkers = int(math.Ceil(float64(probeBudget) / float64(dispatchDelay)))
	plan.RequiredWorkers = min(endpointCount, max(1, plan.RequiredWorkers))
	plan.RecommendedWorkers = roundWorkers(int(math.Ceil(float64(plan.RequiredWorkers) / (1 - recommendationHeadroom))))
	plan.RecommendedWorkers = min(endpointCount, max(plan.RequiredWorkers, plan.RecommendedWorkers))
	plan.RequiredIntervalSeconds = int(math.Ceil((probeBudget + time.Duration(endpointCount-1)*probeBudget/time.Duration(workers)).Seconds()))
	worstCase := simulateWorstCase(endpointCount, workers, probeBudget, dispatchDelay)
	plan.WorstCaseCycleMs = worstCase.Milliseconds()
	plan.Fits = workers >= plan.RequiredWorkers && worstCase <= interval
	plan.CapacityHeadroomPct = round1((float64(workers-plan.RequiredWorkers) / float64(plan.RequiredWorkers)) * 100)
	if !plan.Fits {
		plan.Reason = fmt.Sprintf("%d workers cannot keep the staggered dispatcher queue-free; at least %d are required", workers, plan.RequiredWorkers)
	}
	return plan
}

func (p Plan) Error() error {
	if p.Fits {
		return nil
	}
	return fmt.Errorf("configured schedule cannot preserve a %ds observation interval in the worst case: %d endpoints, %d workers, %.2fs per-endpoint probe budget, estimated %.2fs worst-case cycle; use at least %d workers (%d recommended for headroom), increase cycle_interval_seconds to at least %d, or reduce timeout_ms/pings_per_cycle",
		p.IntervalSeconds, p.EndpointCount, p.Workers, float64(p.ProbeBudgetMs)/1000,
		float64(p.WorstCaseCycleMs)/1000, p.RequiredWorkers, p.RecommendedWorkers, p.RequiredIntervalSeconds)
}

func simulateWorstCase(endpointCount int, workers int, budget time.Duration, dispatchDelay time.Duration) time.Duration {
	available := make([]time.Duration, max(1, workers))
	producerReady := time.Duration(0)
	lastFinish := time.Duration(0)
	for job := 0; job < endpointCount; job++ {
		workerIndex := 0
		for index := 1; index < len(available); index++ {
			if available[index] < available[workerIndex] {
				workerIndex = index
			}
		}
		start := producerReady
		if available[workerIndex] > start {
			start = available[workerIndex]
		}
		finish := start + budget
		available[workerIndex] = finish
		if finish > lastFinish {
			lastFinish = finish
		}
		if job < endpointCount-1 {
			producerReady = start + dispatchDelay
		}
	}
	return lastFinish
}

func roundWorkers(workers int) int {
	if workers <= 10 {
		return workers
	}
	return int(math.Ceil(float64(workers)/5) * 5)
}

func round1(value float64) float64 { return math.Round(value*10) / 10 }
func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
