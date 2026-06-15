package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/buildinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/engine"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/webui"
)

func main() {
	var (
		configPath    = flag.String("config", "config.psd1", "Path to config.psd1 (preferred) or config.yaml/config.json")
		endpointsPath = flag.String("endpoints", "endpoints.csv", "Path to endpoints.csv")
		runOnce       = flag.Bool("run-once", false, "Run a single cycle and exit")
		maxCycles     = flag.Int("max-cycles", 0, "Maximum cycles to run (0 = unlimited)")
		pingMode      = flag.String("ping-mode", "", "Ping mode: auto|raw|exec (empty = use config)")
		uiListen      = flag.String("ui-listen", "", "Listen address for optional local web UI (for example 127.0.0.1:8080)")
		uiOnly        = flag.Bool("ui-only", false, "Serve the web UI without starting the monitoring engine (requires -ui-listen)")
		version       = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Println("Ping Monitor v5 (Go) - " + buildinfo.Version)
		return
	}
	if *uiOnly && *uiListen == "" {
		fmt.Fprintln(os.Stderr, "ui-only requires -ui-listen")
		os.Exit(2)
	}

	exe, _ := os.Executable()
	root := filepath.Dir(exe)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	cfg, cfgSource, err := config.Load(ctx, *configPath, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(2)
	}
	if *pingMode != "" {
		cfg.Ping.Mode = *pingMode
	}

	endpointReloader, endpoints, err := config.NewEndpointReloader(*endpointsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoints load failed: %v\n", err)
		os.Exit(2)
	}

	diagnostics.LogStartup(cfgSource, cfg, len(endpoints))
	if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
		diagnostics.LogRuntimeSnapshot("startup", runtime.NumGoroutine())
	}

	if *uiListen != "" {
		err := webui.Start(ctx, webui.Options{
			ListenAddr:    *uiListen,
			EndpointsPath: *endpointsPath,
			Version:       buildinfo.Version,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "web ui start failed: %v\n", err)
			os.Exit(2)
		}
	}

	if *uiOnly {
		diagnostics.LogInfo("web ui only mode enabled", map[string]interface{}{
			"listen_addr": *uiListen,
			"ui_only":     true,
		})
		<-ctx.Done()
		return
	}

	opts := engine.Options{
		RunOnce:         *runOnce,
		MaxCycles:       *maxCycles,
		EndpointsPath:   *endpointsPath,
		ReloadEndpoints: endpointReloader.ReloadIfChanged,
	}

	if err := engine.Run(ctx, cfg, endpoints, opts); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "shutdown requested")
			return
		}
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}

	if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
		diagnostics.LogRuntimeSnapshot("exit", runtime.NumGoroutine())
	}

	time.Sleep(25 * time.Millisecond) // allow log flush in some environments
}
