//go:build windows

package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"golang.org/x/sys/windows/svc"
)

func TestServiceHostModelIdentifiesNativeAndCompatibilityDefinitions(t *testing.T) {
	cases := map[string]string{
		`"C:\tools\pingmonitor.exe" service-run --service-name SplunkPingMonitor`: "native_go_v6",
		`C:\tools\nssm.exe`:        "nssm",
		`C:\tools\pingmonitor.exe`: "direct_legacy_or_unknown",
		`C:\vendor\wrapper.exe`:    "other_wrapper",
	}
	for input, want := range cases {
		if got := serviceHostModel(input); got != want {
			t.Errorf("serviceHostModel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNativeServiceHandlerStartsServesAndStopsWithoutPause(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	cfg := config.CurrentDefaults(root)
	cfg.PingsPerCycle = 1
	cfg.CycleIntervalSeconds = 2
	cfg.TimeoutMs = 100
	cfg.ParallelThreads = 1
	cfg.OutputMode = "file"
	cfg.LogPath = filepath.Join(root, "logs", "ping_results.log")
	cfg.EmitIndividualPings = false
	cfg.HEC.Enabled = false
	cfg.Metrics.Enabled = false
	if _, err := config.SaveConfig(context.Background(), configPath, root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname,endpoint_id,dev\r\n127.0.0.1,loopback,service-test,false\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()

	handler := &nativeServiceHandler{
		collectorArgs: []string{"-config", configPath, "-endpoints", endpointsPath, "-ui-listen", address},
		logDir:        filepath.Join(root, "service-logs"),
	}
	requests := make(chan svc.ChangeRequest)
	statuses := make(chan svc.Status)
	type handlerResult struct {
		specific bool
		code     uint32
	}
	result := make(chan handlerResult, 1)
	go func() {
		specific, code := handler.Execute(nil, requests, statuses)
		result <- handlerResult{specific: specific, code: code}
	}()
	if status := receiveServiceTestStatus(t, statuses); status.State != svc.StartPending {
		t.Fatalf("first state = %s", serviceStateName(status.State))
	}
	if status := receiveServiceTestStatus(t, statuses); status.State != svc.Running || status.Accepts&svc.AcceptPauseAndContinue != 0 {
		t.Fatalf("running status = %#v", status)
	}

	healthURL := "http://" + address + "/healthz"
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, requestErr := http.Get(healthURL)
		if requestErr == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("service health did not become ready: %v", requestErr)
		}
		time.Sleep(50 * time.Millisecond)
	}

	requests <- svc.ChangeRequest{Cmd: svc.Stop, CurrentStatus: svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}}
	if status := receiveServiceTestStatus(t, statuses); status.State != svc.StopPending {
		t.Fatalf("stop state = %s", serviceStateName(status.State))
	}
	select {
	case completed := <-result:
		if completed.specific || completed.code != 0 {
			t.Fatalf("handler result = %#v", completed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("native service handler did not stop")
	}
	serviceLog, err := os.ReadFile(filepath.Join(root, "service-logs", "service.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serviceLog), "native service host starting") || !strings.Contains(string(serviceLog), "native service host stopped") {
		t.Fatalf("service log = %s", serviceLog)
	}
}

func receiveServiceTestStatus(t *testing.T, statuses <-chan svc.Status) svc.Status {
	t.Helper()
	select {
	case status := <-statuses:
		return status
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for service status")
		return svc.Status{}
	}
}

func TestValidateServiceDeploymentPreservesExistingConfigInputs(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.psd1")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	if err := os.WriteFile(configPath, []byte("@{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endpointsPath, []byte("ip,hostname\n127.0.0.1,loopback\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := serviceDeploymentOptions{
		Name: defaultServiceName, ConfigPath: configPath, EndpointsPath: endpointsPath,
		UIListen: "0.0.0.0:8080", Startup: "delayed-auto", Wait: 30 * time.Second,
	}
	if err := validateServiceDeployment(options); err != nil {
		t.Fatal(err)
	}
	args := serviceCollectorArgs(options)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, configPath) || !strings.Contains(joined, endpointsPath) || !strings.Contains(joined, "0.0.0.0:8080") {
		t.Fatalf("collector args = %#v", args)
	}
}

func TestValidateServiceDeploymentRejectsInvalidListenerAndStartup(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.psd1")
	endpointsPath := filepath.Join(root, "endpoints.csv")
	_ = os.WriteFile(configPath, []byte("@{}"), 0o644)
	_ = os.WriteFile(endpointsPath, []byte("ip,hostname\n127.0.0.1,loopback\n"), 0o644)
	base := serviceDeploymentOptions{
		Name: defaultServiceName, ConfigPath: configPath, EndpointsPath: endpointsPath,
		UIListen: "missing-port", Startup: "delayed-auto", Wait: 30 * time.Second,
	}
	if err := validateServiceDeployment(base); err == nil {
		t.Fatal("invalid listener was accepted")
	}
	base.UIListen = "127.0.0.1:8080"
	base.Startup = "sometimes"
	if err := validateServiceDeployment(base); err == nil {
		t.Fatal("invalid startup type was accepted")
	}
}
