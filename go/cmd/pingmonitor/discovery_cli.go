package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/discovery"
)

type discoveryCLIResult struct {
	GeneratedAt string             `json:"generated_at"`
	ScanID      string             `json:"scan_id"`
	Target      string             `json:"target"`
	DurationMs  int64              `json:"duration_ms"`
	Evidence    discovery.Evidence `json:"evidence"`
	Items       interface{}        `json:"items"`
	Logs        []string           `json:"logs,omitempty"`
}

func runDiscoveryCommand(args []string) (bool, int) {
	if len(args) == 0 || !strings.EqualFold(args[0], "discovery") {
		return false, 0
	}
	flags := flag.NewFlagSet("pingmonitor discovery", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	target := flags.String("target", "", "IPv4 address or CIDR to scan; empty selects the local network")
	subnetMask := flags.Int("subnet-mask", 24, "CIDR prefix used when --target is an IPv4 address (16-30)")
	timeoutMs := flags.Int("timeout-ms", 500, "Per-address ICMP timeout in milliseconds")
	concurrency := flags.Int("concurrency", 50, "Requested ICMP concurrency (safely capped by the native engine)")
	pingMode := flags.String("ping-mode", "auto", "Ping mode: auto, raw, or exec")
	format := flags.String("format", "", "Output format: csv or json (inferred from --output when omitted)")
	outputPath := flags.String("output", "discovery_results.csv", "Output path, or - for standard output")
	force := flags.Bool("force", false, "Replace an existing output file after staging the complete export")
	quiet := flags.Bool("quiet", false, "Suppress progress messages on standard error")
	if err := flags.Parse(args[1:]); err != nil {
		return true, 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected discovery arguments: %s\n", strings.Join(flags.Args(), " "))
		return true, 2
	}
	if *timeoutMs < 1 {
		fmt.Fprintln(os.Stderr, "discovery timeout-ms must be at least 1")
		return true, 2
	}
	if *concurrency < 1 {
		fmt.Fprintln(os.Stderr, "discovery concurrency must be at least 1")
		return true, 2
	}
	mode := strings.ToLower(strings.TrimSpace(*pingMode))
	if mode != "auto" && mode != "raw" && mode != "exec" {
		fmt.Fprintln(os.Stderr, "discovery ping-mode must be auto, raw, or exec")
		return true, 2
	}
	outputFormat := strings.ToLower(strings.TrimSpace(*format))
	if outputFormat == "" {
		if strings.EqualFold(filepath.Ext(*outputPath), ".json") {
			outputFormat = "json"
		} else {
			outputFormat = "csv"
		}
	}
	if outputFormat != "csv" && outputFormat != "json" {
		fmt.Fprintln(os.Stderr, "discovery format must be csv or json")
		return true, 2
	}

	progress := func(update discovery.Progress) error {
		if !*quiet && strings.TrimSpace(update.Message) != "" {
			fmt.Fprintln(os.Stderr, update.Message)
		}
		return nil
	}
	result, err := discovery.Run(context.Background(), discovery.Options{
		TargetNetwork: *target,
		SubnetMask:    *subnetMask,
		Timeout:       time.Duration(*timeoutMs) * time.Millisecond,
		Concurrency:   *concurrency,
		PingMode:      mode,
		Progress:      progress,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "discovery failed:", err)
		return true, 1
	}

	var encoded []byte
	if outputFormat == "csv" {
		encoded, err = config.EncodeEndpointsCSV(result.Items)
	} else {
		encoded, err = json.MarshalIndent(discoveryCLIResult{
			GeneratedAt: result.GeneratedAt.Format(time.RFC3339Nano),
			ScanID:      result.ScanID,
			Target:      result.Target,
			DurationMs:  result.Duration.Milliseconds(),
			Evidence:    result.Evidence,
			Items:       result.Items,
			Logs:        result.Logs,
		}, "", "  ")
		encoded = append(encoded, '\n')
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode discovery results:", err)
		return true, 1
	}
	if err := writeDiscoveryCLIOutput(*outputPath, encoded, *force); err != nil {
		fmt.Fprintln(os.Stderr, "write discovery results:", err)
		return true, 1
	}
	if !*quiet && *outputPath != "-" {
		fmt.Fprintf(os.Stderr, "Discovery complete: %d observed endpoint(s) written to %s\n", len(result.Items), filepath.Clean(*outputPath))
	}
	return true, 0
}

func writeDiscoveryCLIOutput(path string, data []byte, force bool) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("output path must not be empty")
	}
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return err
	}
	if !force {
		file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("%s already exists; choose another path or use --force", absolute)
			}
			return err
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	}
	temporary, err := os.CreateTemp(filepath.Dir(absolute), ".pingmonitor-discovery-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, absolute); err != nil {
		return fmt.Errorf("staged replacement failed: %w", err)
	}
	return nil
}
