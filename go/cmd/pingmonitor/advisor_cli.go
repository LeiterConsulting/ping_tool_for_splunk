package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/advisor"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/buildinfo"
)

func runAdvisorCommand(args []string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	command := strings.ToLower(args[0])
	if command != "analyze" && command != "optimize" && command != "benchmark" && command != "profiles" {
		return false, 0
	}
	if command == "profiles" {
		if err := advisor.WriteJSON(os.Stdout, advisor.Profiles()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return true, 2
		}
		return true, 0
	}

	exe, _ := os.Executable()
	root := filepath.Dir(exe)
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", "config.psd1", "Path to deployment configuration")
	endpointsPath := flags.String("endpoints", "endpoints.csv", "Path to endpoint inventory")
	profile := flags.String("profile", "standard", "Advisor profile: standard|sla|high-latency|large-inventory|low-resource|current")
	format := flags.String("format", "text", "Output format: text|json")
	applySafe := false
	applyProfile := false
	if command == "optimize" {
		flags.BoolVar(&applySafe, "apply-safe", false, "Apply only unambiguous inventory cleanup")
		flags.BoolVar(&applyProfile, "apply-profile", false, "Apply the selected profile after showing its proposal")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return true, 2
	}
	resolvedConfig := resolveRuntimePath(*configPath, root, "config.psd1")
	resolvedEndpoints := resolveRuntimePath(*endpointsPath, root, "endpoints.csv")
	opts := advisor.AnalyzeOptions{ConfigPath: resolvedConfig, EndpointsPath: resolvedEndpoints, RootDir: root, Profile: *profile, ProductVersion: buildinfo.Version}
	ctx := context.Background()

	switch command {
	case "analyze":
		report := advisor.AnalyzeDeployment(ctx, opts)
		if err := writeAdvisorOutput(*format, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return true, 2
		}
		if !report.Summary.ReadyToRun {
			return true, 1
		}
	case "optimize":
		if applySafe || applyProfile {
			result, err := advisor.Apply(ctx, advisor.ApplyOptions{AnalyzeOptions: opts, ApplySafe: applySafe, ApplyProfile: applyProfile})
			if err != nil {
				fmt.Fprintln(os.Stderr, "optimization failed:", err)
				return true, 1
			}
			if *format == "json" {
				if err := advisor.WriteJSON(os.Stdout, result); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return true, 2
				}
			} else {
				fmt.Printf("Applied fixes: %s\nConfig changed: %t | Endpoints changed: %t | Restart required: %t\n\n", strings.Join(result.AppliedFixes, ", "), result.ConfigChanged, result.EndpointsChanged, result.RestartRequired)
				advisor.WriteText(os.Stdout, result.Report)
			}
			if !result.Report.Summary.ReadyToRun {
				return true, 1
			}
		} else {
			report := advisor.AnalyzeDeployment(ctx, opts)
			if err := writeAdvisorOutput(*format, report); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return true, 2
			}
			if !report.Summary.ReadyToRun {
				return true, 1
			}
		}
	case "benchmark":
		result, err := advisor.BenchmarkDeployment(ctx, opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "benchmark failed:", err)
			return true, 1
		}
		if *format == "json" {
			if err := advisor.WriteJSON(os.Stdout, result); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return true, 2
			}
		} else {
			fmt.Printf("Non-SLA benchmark complete in %d ms\nPlanner: %.0f ops/s | Ping backend: %s (%d/%d loopback replies) | Deployment filesystem: %.1f MiB/s (%d writes)\n", result.DurationMs, result.PlannerOpsPerSecond, result.PingBackend, result.LoopbackSuccesses, result.LoopbackAttempts, result.FilesystemMBPerSecond, result.FilesystemWrites)
			if result.PingFallback != "" {
				fmt.Println("Ping backend fallback:", result.PingFallback)
			}
			for _, warning := range result.Warnings {
				fmt.Println("Warning:", warning)
			}
		}
	}
	return true, 0
}

func writeAdvisorOutput(format string, report advisor.Report) error {
	switch strings.ToLower(format) {
	case "text":
		advisor.WriteText(os.Stdout, report)
		return nil
	case "json":
		return advisor.WriteJSON(os.Stdout, report)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}
