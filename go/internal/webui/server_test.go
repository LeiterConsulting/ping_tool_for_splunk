package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointsAPI(t *testing.T) {
	tempDir := t.TempDir()
	endpointsPath := filepath.Join(tempDir, "endpoints.csv")
	content := "ip,hostname,group,description,entitytype,device,vendor,additional_notes,dev\n" +
		"10.0.0.1,core-router,network,Core Router,network,router,Cisco,,false\n" +
		"10.0.0.25,lab-api,development,Lab API,service,vm,VMware,,true\n"
	if err := os.WriteFile(endpointsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	handler, err := newHandler(Options{EndpointsPath: endpointsPath, Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/endpoints", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var payload endpointsResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.Summary.Total != 2 {
		t.Fatalf("summary.total = %d, want 2", payload.Summary.Total)
	}
	if payload.Summary.Production != 1 {
		t.Fatalf("summary.production = %d, want 1", payload.Summary.Production)
	}
	if payload.Summary.Dev != 1 {
		t.Fatalf("summary.dev = %d, want 1", payload.Summary.Dev)
	}
	if payload.Summary.Groups != 2 {
		t.Fatalf("summary.groups = %d, want 2", payload.Summary.Groups)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(payload.Items))
	}
	if !payload.Items[1].Dev {
		t.Fatal("items[1].dev = false, want true")
	}
}

func TestStaticShellServesIndex(t *testing.T) {
	handler, err := newHandler(Options{EndpointsPath: "endpoints.csv", Version: "test"})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if body := resp.Body.String(); !containsAll(body, "Ping Monitor", "Endpoint Inventory", "Dev Devices") {
		t.Fatalf("body missing expected shell markers: %q", body)
	}
}

func containsAll(body string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			return false
		}
	}
	return true
}
