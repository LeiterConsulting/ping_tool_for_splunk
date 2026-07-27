package advisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

func TestAnalyzeDeploymentReportsDuplicateMissingOctetAndCapacity(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	cfg := config.Defaults(root)
	cfg.TimeoutMs = 3000
	cfg.CycleIntervalSeconds = 10
	cfg.ParallelThreads = 1
	if _, err := config.SaveConfig(context.Background(), configPath, root, cfg); err != nil {
		t.Fatal(err)
	}
	content := "ip,hostname,endpoint_id,dev\n192.168.1.67,router-a,,false\n192.168.1.67,router-b,,false\n192.168.1,broken,,false\n"
	if err := os.WriteFile(endpointsPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	report := AnalyzeDeployment(context.Background(), AnalyzeOptions{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: root, Profile: "standard", ProductVersion: "test"})
	if !hasFinding(report.Findings, "INV_DUPLICATE_IP") || !hasFinding(report.Findings, "INV_INVALID_IP") || !hasFinding(report.Findings, "SCHED_CAPACITY") {
		t.Fatalf("missing expected findings: %#v", report.Findings)
	}
	if report.Summary.Blockers < 3 || report.Summary.ReadyToRun {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
}

func TestApplySafeRemovesOnlyIdenticalDuplicate(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	cfg := config.Defaults(root)
	if _, err := config.SaveConfig(context.Background(), configPath, root, cfg); err != nil {
		t.Fatal(err)
	}
	content := "ip,hostname,group,endpoint_id,dev\n 192.168.1.67 ,router-a,network,,false\n192.168.1.67,router-a,network,,false\n"
	if err := os.WriteFile(endpointsPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	report := AnalyzeDeployment(context.Background(), AnalyzeOptions{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: root, Profile: "current"})
	if report.Summary.ReadyToRun || report.Summary.SafeFixes == 0 {
		t.Fatalf("identical duplicate should block startup while offering a safe fix: %#v", report.Summary)
	}
	result, err := Apply(context.Background(), ApplyOptions{AnalyzeOptions: AnalyzeOptions{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: root, Profile: "current"}, ApplySafe: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.EndpointsChanged {
		t.Fatal("endpoints were not changed")
	}
	loaded, err := config.LoadEndpoints(endpointsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].IP != "192.168.1.67" {
		t.Fatalf("unexpected endpoints: %#v", loaded)
	}
}

func TestProfileProposalPreservesNonScheduleSettings(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.HEC.Index = "customer-index"
	profile, _ := GetProfile("sla")
	proposal := BuildProposal(cfg, 166, profile)
	if proposal.Config.HEC.Index != "customer-index" {
		t.Fatal("profile changed unrelated HEC config")
	}
	if proposal.Config.PingsPerCycle != 6 || proposal.Config.ParallelThreads < proposal.Schedule.RequiredWorkers || !proposal.Schedule.Fits {
		t.Fatalf("invalid proposal: %#v", proposal)
	}
}

func TestAnalyzeConfigWarnsWhenMetricsOnlySuppressesDiscoveryEvents(t *testing.T) {
	cfg := config.Defaults(t.TempDir())
	cfg.Metrics.Enabled = true
	cfg.Metrics.Mode = "metrics_only"
	cfg.Metrics.HECURL = "https://splunk.example.test:8088/services/collector"
	cfg.Metrics.Token = "token"
	cfg.Metrics.Index = "ping_metrics"
	report := Report{}
	analyzeConfig(&report, cfg)
	if !hasFinding(report.Findings, "DISCOVERY_EVENTS_SUPPRESSED") {
		t.Fatalf("metrics-only config did not report discovery event suppression: %#v", report.Findings)
	}
}

func TestBenchmarkIsBoundedAndNonSLA(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	cfg := config.Defaults(root)
	cfg.OutputMode = "file"
	cfg.Metrics.Enabled = false
	if _, err := config.SaveConfig(context.Background(), configPath, root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n127.0.0.1,loopback\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := BenchmarkDeployment(context.Background(), AnalyzeOptions{ConfigPath: configPath, EndpointsPath: endpointsPath, RootDir: root, Profile: "current"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.NonSLA || result.LoopbackTarget != "127.0.0.1" || result.PingBackend == "" {
		t.Fatalf("unexpected benchmark identity: %#v", result)
	}
	if result.PlannerIterations != 50000 || result.FilesystemWrites > 64 || result.LoopbackAttempts > 3 {
		t.Fatalf("benchmark exceeded its bounds: %#v", result)
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".pingmonitor-benchmark-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("benchmark temp files were not cleaned up: %v, %v", leftovers, err)
	}
}
