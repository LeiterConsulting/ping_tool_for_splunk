package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIPreferencesDefaultSaveAndRevisionConflict(t *testing.T) {
	root := t.TempDir()
	handler, err := newHandler(Options{RootDir: root, ConfigPath: filepath.Join(root, "config.psd1"), EndpointsPath: filepath.Join(root, "endpoints.csv")})
	if err != nil {
		t.Fatal(err)
	}

	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/ui-preferences", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("default GET status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var defaults uiPreferencesResponse
	if err := json.Unmarshal(getResponse.Body.Bytes(), &defaults); err != nil {
		t.Fatal(err)
	}
	if defaults.Source != "built-in defaults" || defaults.Revision != uiPreferencesDefaultRevision {
		t.Fatalf("default response = %#v", defaults)
	}
	if defaults.Preferences.ColorScheme != "signal-blue" || defaults.Preferences.Density != "comfortable" || defaults.Preferences.Motion != "system" {
		t.Fatalf("default preferences = %#v", defaults.Preferences)
	}

	request := uiPreferencesWriteRequest{
		Revision: defaults.Revision,
		Preferences: uiPreferences{
			SchemaVersion: 1,
			ColorScheme:   "orbit-violet",
			Density:       "compact",
			Motion:        "reduced",
		},
	}
	body, _ := json.Marshal(request)
	putResponse := httptest.NewRecorder()
	handler.ServeHTTP(putResponse, httptest.NewRequest(http.MethodPut, "/api/ui-preferences", bytes.NewReader(body)))
	if putResponse.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putResponse.Code, putResponse.Body.String())
	}
	var saved uiPreferencesResponse
	if err := json.Unmarshal(putResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Source != "deployment file" || saved.Revision == "" || saved.Revision == defaults.Revision {
		t.Fatalf("saved response = %#v", saved)
	}
	if saved.Preferences.UpdatedAt == "" {
		t.Fatal("saved preferences do not include updated_at")
	}
	content, err := os.ReadFile(filepath.Join(root, uiPreferencesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"color_scheme": "orbit-violet"`) || !strings.Contains(string(content), `"density": "compact"`) {
		t.Fatalf("saved preference file = %s", content)
	}

	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, httptest.NewRequest(http.MethodPut, "/api/ui-preferences", bytes.NewReader(body)))
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale PUT status = %d, body = %s", staleResponse.Code, staleResponse.Body.String())
	}
}

func TestUIPreferencesRejectUnsupportedValues(t *testing.T) {
	root := t.TempDir()
	handler, err := newHandler(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"revision":"built-in-v1","preferences":{"schema_version":1,"color_scheme":"snmp-green","density":"comfortable","motion":"system"}}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/ui-preferences", strings.NewReader(body)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unsupported color_scheme") {
		t.Fatalf("unsupported theme response = %d %s", response.Code, response.Body.String())
	}
}

func TestUIPreferencesCorruptFileFallsBackWithoutBreakingUI(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, uiPreferencesFileName)
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/ui-preferences", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("corrupt GET status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload uiPreferencesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Preferences.ColorScheme != "signal-blue" || payload.Warning == "" || payload.Revision == uiPreferencesDefaultRevision {
		t.Fatalf("corrupt fallback response = %#v", payload)
	}
}

func TestUIPreferencesMethodIsBounded(t *testing.T) {
	handler, err := newHandler(Options{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/ui-preferences", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", response.Code)
	}
}
