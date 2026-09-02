package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/advisor"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/buildinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/identity"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/output"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/singleinstance"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/webui"
)

func main() {
	if handled, exitCode := runConfigCommand(os.Args[1:]); handled {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}
	if handled, exitCode := runAdvisorCommand(os.Args[1:]); handled {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}
	var (
		configPath    = flag.String("config", "config.psd1", "Path to an existing config.psd1/config.yaml/config.json; new deployments use config.json")
		endpointsPath = flag.String("endpoints", "endpoints.csv", "Path to endpoints.csv")
		runOnce       = flag.Bool("run-once", false, "Run a single cycle and exit")
		maxCycles     = flag.Int("max-cycles", 0, "Maximum cycles to run (0 = unlimited)")
		pingMode      = flag.String("ping-mode", "", "Ping mode: auto|raw|exec (empty = use config)")
		discoveryPath = flag.String("discovery-script", "", "Optional explicit path to a custom DiscoverEndpoints.ps1; the version-matched embedded script is used by default")
		uiListen      = flag.String("ui-listen", "", "Listen address for optional web UI (for example 0.0.0.0:8080)")
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

	resolvedConfigPath := resolveConfigRuntimePath(*configPath, root)
	resolvedEndpointsPath := resolveRuntimePath(*endpointsPath, root, "endpoints.csv")
	if *validateOnly {
		report := advisor.AnalyzeDeployment(context.Background(), advisor.AnalyzeOptions{
			ConfigPath: resolvedConfigPath, EndpointsPath: resolvedEndpointsPath,
			RootDir: root, Profile: "current", ProductVersion: buildinfo.Version,
		})
		advisor.WriteText(os.Stdout, report)
		if !report.Summary.ReadyToRun {
			os.Exit(2)
		}
		fmt.Printf("\nvalidation successful: config=%s endpoints=%d\n", report.ConfigSource, report.Inventory.SchedulableEndpoints)
		return
	}

	var deploymentLock *singleinstance.Lock
	if !*uiOnly {
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
	var err error
	collectorID, err = identity.LoadOrCreateCollectorID(resolvedConfigPath + ".collector_id")
	if err != nil {
		fmt.Fprintf(os.Stderr, "collector identity initialization failed: %v\n", err)
		os.Exit(2)
	}
	collectorHost, _ := os.Hostname()
	if strings.TrimSpace(collectorHost) == "" {
		collectorHost = "unknown"
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if *uiOnly {
		cfg, _, loadErr := config.Load(ctx, resolvedConfigPath, root)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "config load failed: %v\n", loadErr)
			os.Exit(2)
		}
		if overrideErr := applyPingModeOverride(&cfg, *pingMode); overrideErr != nil {
			fmt.Fprintln(os.Stderr, overrideErr)
			os.Exit(2)
		}
		configRevision, _ := revision.File(resolvedConfigPath)
		endpointsRevision, _ := revision.File(resolvedEndpointsPath)
		editableEndpoints, _ := config.LoadEditableEndpoints(resolvedEndpointsPath)
		runtimeTracker := runtimeinfo.New("ui_only", configRevision, endpointsRevision, len(editableEndpoints))
		manager, managerErr := output.NewManager(cfg, collectorHost, collectorID)
		if managerErr != nil {
			fmt.Fprintf(os.Stderr, "output pipeline initialization failed: %v\n", managerErr)
			os.Exit(2)
		}
		outputs := newOutputManagerStore(manager)
		defer outputs.ClearAndClose()
		warnIfRemoteUI(*uiListen)
		if err := webui.Start(ctx, webui.Options{
			ListenAddr: *uiListen, ConfigPath: resolvedConfigPath, EndpointsPath: resolvedEndpointsPath,
			RootDir: root, Version: buildinfo.Version, CollectorID: collectorID, CollectorHost: collectorHost,
			DiscoveryScriptPath: *discoveryPath,
			Runtime:             runtimeTracker, EffectiveConfig: &cfg, EmitDiscoveryEvents: outputs.Emit,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "web ui start failed: %v\n", err)
			os.Exit(2)
		}
		diagnostics.LogInfo("web ui only mode enabled", map[string]interface{}{"listen_addr": *uiListen, "ui_only": true})
		<-ctx.Done()
		return
	}

	deployment, err := loadRuntimeDeployment(ctx, resolvedConfigPath, resolvedEndpointsPath, root, *pingMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	runtimeTracker := runtimeinfo.New("monitor", deployment.ConfigRevision, deployment.EndpointsRevision, len(deployment.Endpoints))
	effectiveConfig := newEffectiveConfigStore(deployment.Config)
	manager, err := output.NewManager(deployment.Config, collectorHost, collectorID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "output pipeline initialization failed: %v\n", err)
		os.Exit(2)
	}
	outputs := newOutputManagerStore(manager)
	defer outputs.ClearAndClose()
	restartRequests := make(chan webui.RestartRequest, 1)
	requestRestart := func(request webui.RestartRequest) error {
		select {
		case restartRequests <- request:
			return nil
		default:
			return fmt.Errorf("collector restart is already queued")
		}
	}

	if *uiListen != "" {
		warnIfRemoteUI(*uiListen)
		err := webui.Start(ctx, webui.Options{
			ListenAddr:              *uiListen,
			ConfigPath:              resolvedConfigPath,
			EndpointsPath:           resolvedEndpointsPath,
			RootDir:                 root,
			DiscoveryScriptPath:     *discoveryPath,
			Version:                 buildinfo.Version,
			CollectorID:             collectorID,
			CollectorHost:           collectorHost,
			Runtime:                 runtimeTracker,
			EffectiveConfig:         &deployment.Config,
			EffectiveConfigProvider: effectiveConfig.Get,
			RequestRestart:          requestRestart,
			EmitDiscoveryEvents:     outputs.Emit,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "web ui start failed: %v\n", err)
			os.Exit(2)
		}
	} else {
		webui.StartDiscoveryScheduler(ctx, webui.Options{
			ConfigPath:              resolvedConfigPath,
			EndpointsPath:           resolvedEndpointsPath,
			RootDir:                 root,
			DiscoveryScriptPath:     *discoveryPath,
			Version:                 buildinfo.Version,
			CollectorID:             collectorID,
			CollectorHost:           collectorHost,
			Runtime:                 runtimeTracker,
			EffectiveConfig:         &deployment.Config,
			EffectiveConfigProvider: effectiveConfig.Get,
			EmitDiscoveryEvents:     outputs.Emit,
		})
	}

	err = runMonitorLoop(ctx, deployment, resolvedConfigPath, resolvedEndpointsPath, root, *pingMode, collectorHost, collectorID, *runOnce, *maxCycles, runtimeTracker, effectiveConfig, outputs, restartRequests)
	if err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "shutdown requested")
			return
		}
		runtimeTracker.Failed(err)
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
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

func resolveConfigRuntimePath(value string, root string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" && trimmed != "config.psd1" {
		return resolveRuntimePath(trimmed, root, "config.psd1")
	}
	for _, directory := range []string{root, "."} {
		for _, name := range []string{"config.psd1", "config.json", "config.yaml", "config.yml"} {
			candidate := filepath.Join(directory, name)
			if _, err := os.Stat(candidate); err == nil {
				return filepath.Clean(candidate)
			}
		}
	}
	return filepath.Join(root, "config.json")
}
