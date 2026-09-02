//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/rotatelog"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	defaultServiceName        = "SplunkPingMonitor"
	defaultServiceDisplayName = "Splunk Ping Monitor (Go v6)"
	serviceStartupTimeout     = 30 * time.Second
	serviceLogMaxBytes        = 10 * 1024 * 1024
	serviceLogRetention       = 5
)

type serviceDeploymentOptions struct {
	Name          string
	DisplayName   string
	ConfigPath    string
	EndpointsPath string
	UIListen      string
	DisableUI     bool
	LogDir        string
	Startup       string
	NoStart       bool
	Force         bool
	Wait          time.Duration
}

type serviceStatusResponse struct {
	Installed        bool   `json:"installed"`
	Name             string `json:"name"`
	DisplayName      string `json:"display_name,omitempty"`
	State            string `json:"state,omitempty"`
	ProcessID        uint32 `json:"process_id,omitempty"`
	Startup          string `json:"startup,omitempty"`
	DelayedAutoStart bool   `json:"delayed_auto_start,omitempty"`
	BinaryPath       string `json:"binary_path,omitempty"`
	HostModel        string `json:"host_model,omitempty"`
	NativeV6         bool   `json:"native_v6"`
}

func runServiceCommand(args []string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	if strings.EqualFold(args[0], "service-run") {
		return true, runWindowsService(args[1:])
	}
	if !strings.EqualFold(args[0], "service") {
		return false, 0
	}
	if len(args) < 2 {
		printServiceUsage()
		return true, 2
	}
	action := strings.ToLower(strings.TrimSpace(args[1]))
	switch action {
	case "install", "validate":
		options, code := parseServiceDeploymentFlags(action, args[2:])
		if code < 0 {
			return true, 0
		}
		if code != 0 {
			return true, code
		}
		if err := validateServiceDeployment(options); err != nil {
			fmt.Fprintln(os.Stderr, "service validation failed:", err)
			return true, 2
		}
		collectorArgs := serviceCollectorArgs(options)
		if code := runCollector(append(collectorArgs, "-validate"), context.Background(), false, nil); code != 0 {
			return true, code
		}
		if action == "validate" {
			fmt.Println("native Windows service validation successful")
			return true, 0
		}
		if err := installNativeService(options, collectorArgs); err != nil {
			fmt.Fprintln(os.Stderr, "service install failed:", err)
			return true, 1
		}
		status, statusErr := queryNativeService(options.Name)
		if statusErr != nil {
			fmt.Fprintln(os.Stderr, "service installed but status verification failed:", statusErr)
			return true, 1
		}
		printServiceStatus(status, false)
		return true, 0
	case "status":
		name, jsonOutput, wait, code := parseServiceControlFlags(action, args[2:])
		if code < 0 {
			return true, 0
		}
		_ = wait
		if code != 0 {
			return true, code
		}
		status, err := queryNativeService(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "service status failed:", err)
			return true, 1
		}
		printServiceStatus(status, jsonOutput)
		return true, 0
	case "start", "stop", "restart", "uninstall":
		name, _, wait, code := parseServiceControlFlags(action, args[2:])
		if code < 0 {
			return true, 0
		}
		if code != 0 {
			return true, code
		}
		if err := controlNativeService(action, name, wait); err != nil {
			fmt.Fprintf(os.Stderr, "service %s failed: %v\n", action, err)
			return true, 1
		}
		if action == "uninstall" {
			fmt.Printf("service %s uninstalled\n", name)
			return true, 0
		}
		status, statusErr := queryNativeService(name)
		if statusErr != nil {
			fmt.Fprintln(os.Stderr, "service action completed but status verification failed:", statusErr)
			return true, 1
		}
		printServiceStatus(status, false)
		return true, 0
	default:
		fmt.Fprintf(os.Stderr, "unknown service action %q\n", action)
		printServiceUsage()
		return true, 2
	}
}

func printServiceUsage() {
	fmt.Fprintln(os.Stderr, "usage: pingmonitor service <validate|install|status|start|stop|restart|uninstall> [options]")
}

func parseServiceDeploymentFlags(action string, args []string) (serviceDeploymentOptions, int) {
	flags := flag.NewFlagSet("pingmonitor service "+action, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := serviceDeploymentOptions{}
	flags.StringVar(&options.Name, "name", defaultServiceName, "Windows service name")
	flags.StringVar(&options.DisplayName, "display-name", defaultServiceDisplayName, "Windows service display name")
	flags.StringVar(&options.ConfigPath, "config", "config.psd1", "Configuration path")
	flags.StringVar(&options.EndpointsPath, "endpoints", "endpoints.csv", "Endpoint inventory path")
	flags.StringVar(&options.UIListen, "ui-listen", "0.0.0.0:8080", "Admin UI listen address")
	flags.BoolVar(&options.DisableUI, "disable-ui", false, "Run without the admin UI")
	flags.StringVar(&options.LogDir, "log-dir", "", "Native service log directory (default: logs beside the config)")
	flags.StringVar(&options.Startup, "startup", "delayed-auto", "Service startup: delayed-auto, auto, or manual")
	flags.DurationVar(&options.Wait, "wait", 45*time.Second, "Maximum time to wait for service state changes")
	if action == "install" {
		flags.BoolVar(&options.NoStart, "no-start", false, "Install without starting the service")
		flags.BoolVar(&options.Force, "force", false, "Replace an existing service definition after validation")
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return options, -1
		}
		return options, 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected service arguments: %s\n", strings.Join(flags.Args(), " "))
		return options, 2
	}
	return options, 0
}

func parseServiceControlFlags(action string, args []string) (string, bool, time.Duration, int) {
	flags := flag.NewFlagSet("pingmonitor service "+action, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", defaultServiceName, "Windows service name")
	jsonOutput := flags.Bool("json", false, "Emit machine-readable JSON (status only)")
	wait := flags.Duration("wait", 45*time.Second, "Maximum time to wait for the requested state")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return *name, *jsonOutput, *wait, -1
		}
		return "", false, 0, 2
	}
	if flags.NArg() != 0 || (*jsonOutput && action != "status") {
		fmt.Fprintln(os.Stderr, "invalid service control arguments")
		return "", false, 0, 2
	}
	return *name, *jsonOutput, *wait, 0
}

func validateServiceDeployment(options serviceDeploymentOptions) error {
	if strings.TrimSpace(options.Name) == "" {
		return errors.New("service name must not be empty")
	}
	if options.Wait < time.Second {
		return errors.New("service wait must be at least one second")
	}
	startup := strings.ToLower(strings.TrimSpace(options.Startup))
	if startup != "delayed-auto" && startup != "auto" && startup != "manual" {
		return errors.New("service startup must be delayed-auto, auto, or manual")
	}
	configPath, err := filepath.Abs(options.ConfigPath)
	if err != nil {
		return err
	}
	if info, err := os.Stat(configPath); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("path is a directory")
		}
		return fmt.Errorf("config path %s: %w", configPath, err)
	}
	endpointsPath, err := filepath.Abs(options.EndpointsPath)
	if err != nil {
		return err
	}
	if info, err := os.Stat(endpointsPath); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("path is a directory")
		}
		return fmt.Errorf("endpoints path %s: %w", endpointsPath, err)
	}
	if !options.DisableUI {
		_, port, err := net.SplitHostPort(options.UIListen)
		if err != nil {
			return fmt.Errorf("ui-listen %q: %w", options.UIListen, err)
		}
		if port == "" {
			return errors.New("ui-listen must include a port")
		}
	}
	return nil
}

func serviceCollectorArgs(options serviceDeploymentOptions) []string {
	configPath, _ := filepath.Abs(options.ConfigPath)
	endpointsPath, _ := filepath.Abs(options.EndpointsPath)
	args := []string{"-config", configPath, "-endpoints", endpointsPath}
	if !options.DisableUI {
		args = append(args, "-ui-listen", options.UIListen)
	}
	return args
}

func installNativeService(options serviceDeploymentOptions, collectorArgs []string) error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager (run elevated): %w", err)
	}
	defer manager.Disconnect()
	if existing, err := manager.OpenService(options.Name); err == nil {
		existing.Close()
		if !options.Force {
			status, _ := queryNativeService(options.Name)
			return fmt.Errorf("service already exists using %s hosting at %q; inspect with 'service status --json' or rerun with --force to replace the definition", status.HostModel, status.BinaryPath)
		}
		if err := removeService(manager, options.Name, options.Wait); err != nil {
			return fmt.Errorf("replace existing service: %w", err)
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}

	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	logDir := options.LogDir
	if strings.TrimSpace(logDir) == "" {
		configPath, _ := filepath.Abs(options.ConfigPath)
		logDir = filepath.Join(filepath.Dir(configPath), "logs")
	} else {
		logDir, err = filepath.Abs(logDir)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create service log directory: %w", err)
	}
	startType := uint32(mgr.StartAutomatic)
	delayed := true
	switch strings.ToLower(options.Startup) {
	case "auto":
		delayed = false
	case "manual":
		startType = mgr.StartManual
		delayed = false
	}
	serviceArgs := []string{"service-run", "--service-name", options.Name, "--service-log-dir", logDir, "--"}
	serviceArgs = append(serviceArgs, collectorArgs...)
	service, err := manager.CreateService(options.Name, executable, mgr.Config{
		DisplayName: options.DisplayName,
		Description: "Collects truthful ICMP reachability and latency evidence for Splunk and provides the Ping Monitor administration interface.",
		StartType:   startType, ErrorControl: mgr.ErrorNormal, DelayedAutoStart: delayed,
	}, serviceArgs...)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = stopService(service, options.Wait)
			_ = service.Delete()
		}
		service.Close()
	}()
	recovery := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	if err := service.SetRecoveryActions(recovery, uint32((24*time.Hour)/time.Second)); err != nil {
		return fmt.Errorf("set service recovery actions: %w", err)
	}
	if err := service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("enable recovery for non-crash failures: %w", err)
	}
	if !options.NoStart {
		if err := service.Start(); err != nil {
			return fmt.Errorf("start newly installed service: %w", err)
		}
		if _, err := waitForServiceState(service, svc.Running, options.Wait); err != nil {
			return fmt.Errorf("new service did not stabilize: %w; inspect %s", err, filepath.Join(logDir, "service.log"))
		}
	}
	rollback = false
	return nil
}

func controlNativeService(action, name string, wait time.Duration) error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager (run elevated): %w", err)
	}
	defer manager.Disconnect()
	if action == "uninstall" {
		return removeService(manager, name, wait)
	}
	service, err := manager.OpenService(name)
	if err != nil {
		return err
	}
	defer service.Close()
	switch action {
	case "start":
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State != svc.Running {
			if err := service.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
				return err
			}
		}
		_, err = waitForServiceState(service, svc.Running, wait)
		return err
	case "stop":
		return stopService(service, wait)
	case "restart":
		if err := stopService(service, wait); err != nil {
			return err
		}
		if err := service.Start(); err != nil {
			return err
		}
		_, err := waitForServiceState(service, svc.Running, wait)
		return err
	default:
		return fmt.Errorf("unsupported service action %q", action)
	}
}

func removeService(manager *mgr.Mgr, name string, wait time.Duration) error {
	service, err := manager.OpenService(name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := stopService(service, wait); err != nil {
		service.Close()
		return err
	}
	if err := service.Delete(); err != nil {
		service.Close()
		return err
	}
	// The SCM retains a service marked for deletion until every open handle is
	// closed. Release our handle before polling or a successful uninstall can
	// appear to hang until the timeout expires.
	service.Close()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		probe, openErr := manager.OpenService(name)
		if errors.Is(openErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		if openErr == nil {
			probe.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for service deletion")
}

func stopService(service *mgr.Service, wait time.Duration) error {
	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State != svc.StopPending {
		if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return err
		}
	}
	_, err = waitForServiceState(service, svc.Stopped, wait)
	return err
}

func waitForServiceState(service *mgr.Service, target svc.State, timeout time.Duration) (svc.Status, error) {
	deadline := time.Now().Add(timeout)
	var last svc.Status
	for {
		status, err := service.Query()
		if err != nil {
			return status, err
		}
		last = status
		if status.State == target {
			return status, nil
		}
		if target == svc.Running && status.State == svc.Stopped {
			return status, fmt.Errorf("service stopped during startup (win32=%d service=%d)", status.Win32ExitCode, status.ServiceSpecificExitCode)
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("timed out after %s waiting for %s; current state is %s", timeout, serviceStateName(target), serviceStateName(last.State))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func queryNativeService(name string) (serviceStatusResponse, error) {
	status := serviceStatusResponse{Installed: false, Name: name}
	managerHandle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return status, err
	}
	defer windows.CloseServiceHandle(managerHandle)
	serviceName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return status, err
	}
	serviceHandle, err := windows.OpenService(managerHandle, serviceName, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	service := &mgr.Service{Name: name, Handle: serviceHandle}
	defer service.Close()
	query, err := service.Query()
	if err != nil {
		return status, err
	}
	configuration, err := service.Config()
	if err != nil {
		return status, err
	}
	status.Installed = true
	status.DisplayName = configuration.DisplayName
	status.State = serviceStateName(query.State)
	status.ProcessID = query.ProcessId
	status.Startup = serviceStartName(configuration.StartType)
	status.DelayedAutoStart = configuration.DelayedAutoStart
	status.BinaryPath = configuration.BinaryPathName
	status.HostModel = serviceHostModel(configuration.BinaryPathName)
	status.NativeV6 = status.HostModel == "native_go_v6"
	return status, nil
}

func printServiceStatus(status serviceStatusResponse, jsonOutput bool) {
	if jsonOutput {
		encoded, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	if !status.Installed {
		fmt.Printf("service %s is not installed\n", status.Name)
		return
	}
	fmt.Printf("service %s: %s (PID %d, %s, startup %s)\n", status.Name, status.State, status.ProcessID, status.HostModel, status.Startup)
}

func serviceHostModel(binaryPath string) string {
	lower := strings.ToLower(binaryPath)
	switch {
	case strings.Contains(lower, "nssm"):
		return "nssm"
	case strings.Contains(lower, "pingmonitor") && strings.Contains(lower, "service-run"):
		return "native_go_v6"
	case strings.Contains(lower, "pingmonitor"):
		return "direct_legacy_or_unknown"
	default:
		return "other_wrapper"
	}
}

func serviceStateName(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start_pending"
	case svc.StopPending:
		return "stop_pending"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "continue_pending"
	case svc.PausePending:
		return "pause_pending"
	case svc.Paused:
		return "paused"
	default:
		return fmt.Sprintf("unknown_%d", state)
	}
}

func serviceStartName(startType uint32) string {
	switch startType {
	case mgr.StartAutomatic:
		return "automatic"
	case mgr.StartManual:
		return "manual"
	case mgr.StartDisabled:
		return "disabled"
	default:
		return fmt.Sprintf("unknown_%d", startType)
	}
}

type nativeServiceHandler struct {
	collectorArgs []string
	logDir        string
}

func runWindowsService(args []string) int {
	delimiter := -1
	for index, value := range args {
		if value == "--" {
			delimiter = index
			break
		}
	}
	if delimiter < 0 {
		fmt.Fprintln(os.Stderr, "native service arguments are missing the collector delimiter")
		return 2
	}
	internal := flag.NewFlagSet("pingmonitor service-run", flag.ContinueOnError)
	internal.SetOutput(os.Stderr)
	name := internal.String("service-name", defaultServiceName, "internal service name")
	logDir := internal.String("service-log-dir", "", "internal service log directory")
	if err := internal.Parse(args[:delimiter]); err != nil {
		return 2
	}
	if *logDir == "" {
		fmt.Fprintln(os.Stderr, "native service log directory is missing")
		return 2
	}
	if err := svc.Run(*name, &nativeServiceHandler{collectorArgs: args[delimiter+1:], logDir: *logDir}); err != nil {
		fmt.Fprintln(os.Stderr, "Windows service dispatcher failed:", err)
		return 1
	}
	return 0
}

func (h *nativeServiceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	cleanup, err := captureServiceOutput(h.logDir)
	if err != nil {
		return true, 1
	}
	defer cleanup()
	fmt.Printf("native service host starting at %s\n", time.Now().UTC().Format(time.RFC3339Nano))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	result := make(chan int, 1)
	go func() {
		result <- runCollector(h.collectorArgs, ctx, false, func() { close(ready) })
	}()
	startupTimer := time.NewTimer(serviceStartupTimeout)
	defer startupTimer.Stop()
	select {
	case code := <-result:
		fmt.Printf("collector exited before reporting ready with code %d\n", code)
		if code == 0 {
			code = 1
		}
		return true, uint32(code)
	case <-startupTimer.C:
		cancel()
		fmt.Println("collector startup timed out before reporting ready")
		return true, 1
	case <-ready:
		statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	}

	for {
		select {
		case code := <-result:
			if code != 0 {
				fmt.Printf("collector exited unexpectedly with code %d\n", code)
				return true, uint32(code)
			}
			return false, 0
		case request, ok := <-requests:
			if !ok {
				cancel()
				return true, 1
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case code := <-result:
					if code != 0 {
						return true, uint32(code)
					}
					fmt.Printf("native service host stopped at %s\n", time.Now().UTC().Format(time.RFC3339Nano))
					return false, 0
				case <-time.After(30 * time.Second):
					fmt.Println("collector did not stop within 30 seconds")
					return true, 1
				}
			}
		}
	}
}

func captureServiceOutput(logDir string) (func(), error) {
	writer, err := rotatelog.New(filepath.Join(logDir, "service.log"), serviceLogMaxBytes, serviceLogRetention)
	if err != nil {
		return nil, err
	}
	reader, pipeWriter, err := os.Pipe()
	if err != nil {
		writer.Close()
		return nil, err
	}
	previousStdout, previousStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = pipeWriter, pipeWriter
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(writer, reader)
		close(done)
	}()
	return func() {
		os.Stdout, os.Stderr = previousStdout, previousStderr
		_ = pipeWriter.Close()
		<-done
		_ = reader.Close()
		_ = writer.Sync()
		_ = writer.Close()
	}, nil
}
