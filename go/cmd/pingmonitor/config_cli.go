package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
)

type configUpgradeReport struct {
	Action        string `json:"action"`
	SourcePath    string `json:"source_path"`
	SourceFormat  string `json:"source_format"`
	SourceSchema  int    `json:"source_schema"`
	TargetPath    string `json:"target_path"`
	TargetFormat  string `json:"target_format"`
	TargetSchema  int    `json:"target_schema"`
	Applied       bool   `json:"applied"`
	BackupPolicy  string `json:"backup_policy"`
	ServiceAction string `json:"service_action"`
}

func runConfigCommand(args []string) (bool, int) {
	if len(args) == 0 || args[0] != "config" {
		return false, 0
	}
	if len(args) < 2 || args[1] != "upgrade" {
		fmt.Fprintln(os.Stderr, "usage: pingmonitor config upgrade --config <path> [--to <config.json>] [--check|--apply] [--format json]")
		return true, 2
	}

	flags := flag.NewFlagSet("config upgrade", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	sourceArg := flags.String("config", "config.psd1", "Source PSD1, YAML, or JSON configuration")
	targetArg := flags.String("to", "", "Target schema-v2 JSON path")
	check := flags.Bool("check", false, "Analyze and validate the upgrade without writing")
	apply := flags.Bool("apply", false, "Write and verify the upgraded JSON configuration")
	format := flags.String("format", "text", "Output format: text|json")
	if err := flags.Parse(args[2:]); err != nil {
		return true, 2
	}
	if *check && *apply {
		fmt.Fprintln(os.Stderr, "select either --check or --apply, not both")
		return true, 2
	}
	if !*check && !*apply {
		*check = true
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be text or json")
		return true, 2
	}

	sourcePath, err := filepath.Abs(strings.TrimSpace(*sourceArg))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return true, 2
	}
	targetPath := strings.TrimSpace(*targetArg)
	if targetPath == "" {
		targetPath = filepath.Join(filepath.Dir(sourcePath), "config.json")
	}
	targetPath, err = filepath.Abs(targetPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return true, 2
	}
	if !strings.EqualFold(filepath.Ext(targetPath), ".json") {
		fmt.Fprintln(os.Stderr, "schema-v2 upgrade target must use a .json extension")
		return true, 2
	}
	if strings.EqualFold(sourcePath, targetPath) {
		fmt.Fprintln(os.Stderr, "source and target must be different paths so the legacy configuration remains available for rollback")
		return true, 2
	}

	root := filepath.Dir(sourcePath)
	cfg, source, err := config.LoadEditable(context.Background(), sourcePath, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "upgrade analysis failed: %v\n", err)
		return true, 2
	}
	sourceSchema := cfg.ConfigSchemaVersion
	cfg.ConfigSchemaVersion = config.CurrentSchemaVersion
	if _, err := config.MarshalCurrentJSON(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "upgrade render failed: %v\n", err)
		return true, 2
	}

	report := configUpgradeReport{
		Action:        "check",
		SourcePath:    source.Path,
		SourceFormat:  source.Format,
		SourceSchema:  sourceSchema,
		TargetPath:    targetPath,
		TargetFormat:  "json",
		TargetSchema:  config.CurrentSchemaVersion,
		Applied:       false,
		BackupPolicy:  "the source remains untouched; an existing target receives a timestamped backup",
		ServiceAction: fmt.Sprintf("after review, point the service --config argument at %s", targetPath),
	}

	if *apply {
		if _, err := config.SaveConfig(context.Background(), targetPath, root, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "upgrade write failed: %v\n", err)
			return true, 2
		}
		reloaded, _, err := config.LoadEditable(context.Background(), targetPath, root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upgrade verification failed: %v\n", err)
			return true, 2
		}
		expected := cfg
		expected.ConfigSchemaVersion = 0
		reloaded.ConfigSchemaVersion = 0
		if !reflect.DeepEqual(expected, reloaded) {
			fmt.Fprintln(os.Stderr, "upgrade verification failed: effective configuration changed during round trip")
			return true, 2
		}
		report.Action = "apply"
		report.Applied = true
	}

	if *format == "json" {
		encoded, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(encoded))
		return true, 0
	}
	fmt.Printf("Configuration upgrade %s successful\n", report.Action)
	fmt.Printf("  source: %s (%s schema %d)\n", report.SourcePath, report.SourceFormat, report.SourceSchema)
	fmt.Printf("  target: %s (json schema %d)\n", report.TargetPath, report.TargetSchema)
	fmt.Printf("  applied: %t\n", report.Applied)
	fmt.Printf("  rollback: the source file was not modified\n")
	fmt.Printf("  service: %s\n", report.ServiceAction)
	return true, 0
}
