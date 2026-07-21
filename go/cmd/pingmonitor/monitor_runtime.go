package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/engine"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/runtimeinfo"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/webui"
)

type runtimeDeployment struct {
	Config            config.Config
	ConfigSource      string
	ConfigRevision    string
	EndpointsRevision string
	Endpoints         []models.Endpoint
	EndpointReloader  *config.EndpointReloader
}

type effectiveConfigStore struct {
	mu     sync.RWMutex
	config config.Config
	set    bool
}

func newEffectiveConfigStore(cfg config.Config) *effectiveConfigStore {
	return &effectiveConfigStore{config: cfg, set: true}
}

func (s *effectiveConfigStore) Get() (config.Config, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config, s.set
}

func (s *effectiveConfigStore) Set(cfg config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
	s.set = true
}

func loadRuntimeDeployment(ctx context.Context, configPath string, endpointsPath string, root string, pingMode string) (runtimeDeployment, error) {
	cfg, source, err := config.Load(ctx, configPath, root)
	if err != nil {
		return runtimeDeployment{}, fmt.Errorf("config load failed: %w", err)
	}
	if err := applyPingModeOverride(&cfg, pingMode); err != nil {
		return runtimeDeployment{}, err
	}
	reloader, endpoints, err := config.NewEndpointReloader(endpointsPath)
	if err != nil {
		return runtimeDeployment{}, fmt.Errorf("endpoints load failed: %w", err)
	}
	if err := engine.ValidateSchedule(cfg, len(endpoints)); err != nil {
		return runtimeDeployment{}, fmt.Errorf("scheduler validation failed: %w", err)
	}
	configInfo, err := config.ResolveConfigSource(configPath)
	if err != nil {
		return runtimeDeployment{}, fmt.Errorf("resolve config source: %w", err)
	}
	configRevision, err := revision.File(configInfo.Path)
	if err != nil {
		return runtimeDeployment{}, fmt.Errorf("read config revision: %w", err)
	}
	endpointsRevision, err := revision.File(endpointsPath)
	if err != nil {
		return runtimeDeployment{}, fmt.Errorf("read endpoints revision: %w", err)
	}
	return runtimeDeployment{
		Config: cfg, ConfigSource: source, ConfigRevision: configRevision,
		EndpointsRevision: endpointsRevision, Endpoints: endpoints, EndpointReloader: reloader,
	}, nil
}

func applyPingModeOverride(cfg *config.Config, pingMode string) error {
	if pingMode == "" {
		return nil
	}
	if pingMode != "auto" && pingMode != "raw" && pingMode != "exec" {
		return fmt.Errorf("invalid ping-mode %q; expected auto, raw, or exec", pingMode)
	}
	cfg.Ping.Mode = pingMode
	return nil
}

func runMonitorLoop(
	ctx context.Context,
	initial runtimeDeployment,
	configPath string,
	endpointsPath string,
	root string,
	pingMode string,
	collectorID string,
	runOnce bool,
	maxCycles int,
	tracker *runtimeinfo.Tracker,
	effectiveConfig *effectiveConfigStore,
	restartRequests <-chan webui.RestartRequest,
) error {
	current := initial
	for {
		diagnostics.LogStartup(current.ConfigSource, current.Config, len(current.Endpoints))
		if current.Config.Diagnostics.Enabled || current.Config.Debug.EmitMemoryStats {
			diagnostics.LogRuntimeSnapshot("startup", runtime.NumGoroutine())
		}

		engineCtx, cancelEngine := context.WithCancel(ctx)
		resultCh := make(chan error, 1)
		opts := engine.Options{
			RunOnce: runOnce, MaxCycles: maxCycles, EndpointsPath: endpointsPath,
			CollectorID: collectorID, StatePath: configPath + ".state.json",
			ReloadEndpoints: current.EndpointReloader.ReloadIfChanged, Runtime: tracker,
		}
		go func(deployment runtimeDeployment) {
			resultCh <- engine.Run(engineCtx, deployment.Config, deployment.Endpoints, opts)
		}(current)

		select {
		case <-ctx.Done():
			cancelEngine()
			<-resultCh
			return nil
		case request := <-restartRequests:
			cancelEngine()
			engineErr := <-resultCh
			if engineErr != nil && engineCtx.Err() == nil {
				tracker.RestartFailed(engineErr)
				return engineErr
			}
			next, err := loadRuntimeDeployment(ctx, configPath, endpointsPath, root, pingMode)
			if err == nil && (next.ConfigRevision != request.ConfigRevision || next.EndpointsRevision != request.EndpointsRevision) {
				err = fmt.Errorf("deployment files changed while the controlled restart was beginning; no new configuration was activated")
			}
			if err != nil {
				tracker.RestartFailed(err)
				diagnostics.LogError("controlled collector restart failed; resuming last known good configuration", err, nil)
				continue
			}
			current = next
			effectiveConfig.Set(current.Config)
			tracker.Restarted(current.ConfigRevision, current.EndpointsRevision, len(current.Endpoints), time.Now())
			diagnostics.LogInfo("controlled collector restart activated saved configuration", map[string]interface{}{
				"config_revision": current.ConfigRevision, "endpoints_revision": current.EndpointsRevision,
				"endpoints": len(current.Endpoints),
			})
			continue
		case err := <-resultCh:
			cancelEngine()
			if err != nil {
				tracker.Failed(err)
				return err
			}
			if current.Config.Diagnostics.Enabled || current.Config.Debug.EmitMemoryStats {
				diagnostics.LogRuntimeSnapshot("exit", runtime.NumGoroutine())
			}
			return nil
		}
	}
}
