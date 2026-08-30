package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/diagnostics"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/revision"
)

const (
	uiPreferencesFileName        = "ui_preferences.json"
	uiPreferencesSchemaVersion   = 1
	uiPreferencesDefaultRevision = "built-in-v1"
)

var supportedUIColorSchemes = map[string]struct{}{
	"signal-blue":     {},
	"orbit-violet":    {},
	"radar-coral":     {},
	"daylight":        {},
	"high-visibility": {},
}

var supportedUIDensities = map[string]struct{}{
	"comfortable": {},
	"compact":     {},
}

var supportedUIMotionModes = map[string]struct{}{
	"system":  {},
	"reduced": {},
}

type uiPreferences struct {
	SchemaVersion int    `json:"schema_version"`
	ColorScheme   string `json:"color_scheme"`
	Density       string `json:"density"`
	Motion        string `json:"motion"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type uiPreferencesResponse struct {
	GeneratedAt string        `json:"generated_at"`
	Path        string        `json:"path"`
	Source      string        `json:"source"`
	Revision    string        `json:"revision"`
	Preferences uiPreferences `json:"preferences"`
	Warning     string        `json:"warning,omitempty"`
}

type uiPreferencesWriteRequest struct {
	Preferences uiPreferences `json:"preferences"`
	Revision    string        `json:"revision"`
}

func defaultUIPreferences() uiPreferences {
	return uiPreferences{
		SchemaVersion: uiPreferencesSchemaVersion,
		ColorScheme:   "signal-blue",
		Density:       "comfortable",
		Motion:        "system",
	}
}

func normalizeUIPreferences(candidate uiPreferences) (uiPreferences, error) {
	defaults := defaultUIPreferences()
	if candidate.SchemaVersion == 0 {
		candidate.SchemaVersion = defaults.SchemaVersion
	}
	if candidate.SchemaVersion != uiPreferencesSchemaVersion {
		return uiPreferences{}, fmt.Errorf("unsupported UI preferences schema_version %d", candidate.SchemaVersion)
	}

	candidate.ColorScheme = strings.TrimSpace(candidate.ColorScheme)
	if candidate.ColorScheme == "" {
		candidate.ColorScheme = defaults.ColorScheme
	}
	if _, ok := supportedUIColorSchemes[candidate.ColorScheme]; !ok {
		return uiPreferences{}, fmt.Errorf("unsupported color_scheme %q", candidate.ColorScheme)
	}

	candidate.Density = strings.TrimSpace(candidate.Density)
	if candidate.Density == "" {
		candidate.Density = defaults.Density
	}
	if _, ok := supportedUIDensities[candidate.Density]; !ok {
		return uiPreferences{}, fmt.Errorf("unsupported density %q", candidate.Density)
	}

	candidate.Motion = strings.TrimSpace(candidate.Motion)
	if candidate.Motion == "" {
		candidate.Motion = defaults.Motion
	}
	if _, ok := supportedUIMotionModes[candidate.Motion]; !ok {
		return uiPreferences{}, fmt.Errorf("unsupported motion %q", candidate.Motion)
	}
	return candidate, nil
}

func uiPreferencesPath(rootDir string) string {
	return filepath.Join(rootDir, uiPreferencesFileName)
}

func readUIPreferences(path string) (uiPreferences, string, string, string, error) {
	defaults := defaultUIPreferences()
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, "built-in defaults", uiPreferencesDefaultRevision, "", nil
	}
	if err != nil {
		return uiPreferences{}, "", "", "", err
	}
	currentRevision, revisionErr := revision.File(path)
	if revisionErr != nil {
		return uiPreferences{}, "", "", "", revisionErr
	}

	var stored uiPreferences
	if err := json.Unmarshal(content, &stored); err != nil {
		return defaults, "built-in defaults", currentRevision, "The saved appearance file is invalid; built-in defaults are active until appearance is saved again.", nil
	}
	normalized, err := normalizeUIPreferences(stored)
	if err != nil {
		return defaults, "built-in defaults", currentRevision, fmt.Sprintf("The saved appearance file is not supported (%v); built-in defaults are active until appearance is saved again.", err), nil
	}
	return normalized, "deployment file", currentRevision, "", nil
}

func (s *apiServer) handleUIPreferences(w http.ResponseWriter, r *http.Request) {
	preferencesPath := uiPreferencesPath(s.opts.RootDir)
	switch r.Method {
	case http.MethodGet:
		preferences, source, currentRevision, warning, err := readUIPreferences(preferencesPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, uiPreferencesResponse{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Path:        filepath.Clean(preferencesPath),
			Source:      source,
			Revision:    currentRevision,
			Preferences: preferences,
			Warning:     warning,
		})
	case http.MethodPut:
		var request uiPreferencesWriteRequest
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		if err := decodeJSONBody(r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		preferences, err := normalizeUIPreferences(request.Preferences)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		_, _, currentRevision, _, err := readUIPreferences(preferencesPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if request.Revision == "" || request.Revision != currentRevision {
			writeRevisionConflict(w, "appearance preferences changed after this page was loaded", currentRevision)
			return
		}

		preferences.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeJSONAtomic(preferencesPath, preferences); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		newRevision, err := revision.File(preferencesPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		diagnostics.LogInfo("web ui saved appearance preferences", map[string]interface{}{
			"preferences_path": preferencesPath,
			"color_scheme":     preferences.ColorScheme,
			"density":          preferences.Density,
			"motion":           preferences.Motion,
		})
		writeJSON(w, http.StatusOK, uiPreferencesResponse{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Path:        filepath.Clean(preferencesPath),
			Source:      "deployment file",
			Revision:    newRevision,
			Preferences: preferences,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
