package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/buildinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/engine"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/identity"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/singleinstance"
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
		validateOnly  = flag.Bool("validate", false, "Validate config, endpoints, and scheduler capacity, then exit without probing")
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

	resolvedConfigPath := resolveRuntimePath(*configPath, root, "config.psd1")
	resolvedEndpointsPath := resolveRuntimePath(*endpointsPath, root, "endpoints.csv")

	var deploymentLock *singleinstance.Lock
	if !*uiOnly && !*validateOnly {
		lockPath := resolvedConfigPath + ".lock"
		var lockErr error
		deploymentLock, lockErr = singleinstance.Acquire(lockPath)
		if lockErr != nil {
			fmt.Fprintf(os.Stderr, "startup blocked: %v (lock: %s)\n", lockErr, lockPath)
			os.Exit(2)
		}
		defer deploymentLock.Close()
	}
	collectorID := ""
	if !*validateOnly {
		var err error
		collectorID, err = identity.LoadOrCreateCollectorID(resolvedConfigPath + ".collector_id")
		if err != nil {
			fmt.Fprintf(os.Stderr, "collector identity initialization failed: %v\n", err)
			os.Exit(2)
		}
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	cfg, cfgSource, err := config.Load(ctx, resolvedConfigPath, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(2)
	}
	if *pingMode != "" {
		if *pingMode != "auto" && *pingMode != "raw" && *pingMode != "exec" {
			fmt.Fprintf(os.Stderr, "invalid ping-mode %q; expected auto, raw, or exec\n", *pingMode)
			os.Exit(2)
		}
		cfg.Ping.Mode = *pingMode
	}

	if *uiOnly {
		warnIfRemoteUI(*uiListen)
		if err := webui.Start(ctx, webui.Options{
			ListenAddr: *uiListen, ConfigPath: resolvedConfigPath, EndpointsPath: resolvedEndpointsPath,
			RootDir: root, Version: buildinfo.Version, CollectorID: collectorID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "web ui start failed: %v\n", err)
			os.Exit(2)
		}
		diagnostics.LogInfo("web ui only mode enabled", map[string]interface{}{"listen_addr": *uiListen, "ui_only": true})
		<-ctx.Done()
		return
	}

	endpointReloader, endpoints, err := config.NewEndpointReloader(resolvedEndpointsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoints load failed: %v\n", err)
		os.Exit(2)
	}
	if *validateOnly {
		if err := engine.ValidateSchedule(cfg, len(endpoints)); err != nil {
			fmt.Fprintf(os.Stderr, "scheduler validation failed: %v\n", err)
			os.Exit(2)
		}
		fmt.Printf("validation successful: config=%s endpoints=%d\n", cfgSource, len(endpoints))
		return
	}

	diagnostics.LogStartup(cfgSource, cfg, len(endpoints))
	if cfg.Diagnostics.Enabled || cfg.Debug.EmitMemoryStats {
		diagnostics.LogRuntimeSnapshot("startup", runtime.NumGoroutine())
	}

	if *uiListen != "" {
		warnIfRemoteUI(*uiListen)
		err := webui.Start(ctx, webui.Options{
			ListenAddr:    *uiListen,
			ConfigPath:    resolvedConfigPath,
			EndpointsPath: resolvedEndpointsPath,
			RootDir:       root,
			Version:       buildinfo.Version,
			CollectorID:   collectorID,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "web ui start failed: %v\n", err)
			os.Exit(2)
		}
	}

	opts := engine.Options{
		RunOnce:         *runOnce,
		MaxCycles:       *maxCycles,
		EndpointsPath:   resolvedEndpointsPath,
		CollectorID:     collectorID,
		StatePath:       resolvedConfigPath + ".state.json",
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

func warnIfRemoteUI(address string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return
	}
	diagnostics.LogWarn("admin UI is listening beyond loopback without built-in authentication or TLS", map[string]interface{}{
		"listen_addr": address,
	})
}

func resolveRuntimePath(path string, root string, defaultName string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return filepath.Join(root, defaultName)
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	cwdCandidate := filepath.Clean(trimmed)
	rootCandidate := filepath.Join(root, trimmed)
	if trimmed == defaultName {
		if _, err := os.Stat(rootCandidate); err == nil {
			return rootCandidate
		}
		if _, err := os.Stat(cwdCandidate); err == nil {
			return cwdCandidate
		}
		return rootCandidate
	}
	if _, err := os.Stat(cwdCandidate); err == nil {
		return cwdCandidate
	}
	if _, err := os.Stat(rootCandidate); err == nil {
		return rootCandidate
	}
	return rootCandidate
}
