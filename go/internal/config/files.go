package config

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
	"gopkg.in/yaml.v3"
)

type SourceInfo struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Label  string `json:"label"`
}

func ResolveConfigSource(preferredPath string) (SourceInfo, error) {
	if preferredPath == "" {
		preferredPath = "config.psd1"
	}
	if isSupportedConfigExt(preferredPath) {
		if _, err := os.Stat(preferredPath); err == nil {
			return newSourceInfo(preferredPath), nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return SourceInfo{}, err
		}
		return newSourceInfo(preferredPath), nil
	}

	baseDir := filepath.Dir(preferredPath)
	candidates := []string{
		filepath.Join(baseDir, "config.psd1"),
		filepath.Join(baseDir, "config.yaml"),
		filepath.Join(baseDir, "config.yml"),
		filepath.Join(baseDir, "config.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return newSourceInfo(candidate), nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return SourceInfo{}, err
		}
	}

	return newSourceInfo(filepath.Join(baseDir, "config.yaml")), nil
}

func LoadEditable(ctx context.Context, preferredPath string, root string) (Config, SourceInfo, error) {
	info, err := ResolveConfigSource(preferredPath)
	if err != nil {
		return Config{}, SourceInfo{}, err
	}

	if _, err := os.Stat(info.Path); errors.Is(err, os.ErrNotExist) {
		cfg := Defaults(root)
		if _, saveErr := SaveConfig(ctx, preferredPath, root, cfg); saveErr != nil {
			return Config{}, SourceInfo{}, saveErr
		}
	} else if err != nil {
		return Config{}, SourceInfo{}, err
	}

	switch info.Format {
	case "psd1":
		cfg, err := loadFromPSD1(ctx, info.Path, root)
		return cfg, info, err
	case "yaml", "yml":
		cfg, err := loadFromYAML(info.Path, root)
		return cfg, info, err
	case "json":
		cfg, err := loadFromJSON(info.Path, root)
		return cfg, info, err
	default:
		return Config{}, SourceInfo{}, fmt.Errorf("unsupported config format: %s", info.Format)
	}
}

func SaveConfig(ctx context.Context, preferredPath string, root string, cfg Config) (SourceInfo, error) {
	_ = ctx
	info, err := ResolveConfigSource(preferredPath)
	if err != nil {
		return SourceInfo{}, err
	}

	if _, err := os.Stat(info.Path); errors.Is(err, os.ErrNotExist) {
		if !isSupportedConfigExt(info.Path) {
			info = newSourceInfo(filepath.Join(filepath.Dir(preferredPath), "config.yaml"))
		}
	} else if err != nil {
		return SourceInfo{}, err
	}

	cfg = normalize(cfg)
	var writeErr error
	switch info.Format {
	case "psd1":
		writeErr = writePSD1File(info.Path, cfg)
	case "yaml", "yml":
		writeErr = writeYAMLFile(info.Path, cfg)
	case "json":
		writeErr = writeJSONFile(info.Path, cfg)
	default:
		writeErr = fmt.Errorf("unsupported config format: %s", info.Format)
	}
	if writeErr != nil {
		return SourceInfo{}, writeErr
	}
	return info, nil
}

func SaveEndpoints(path string, endpoints []models.Endpoint) error {
	if err := ValidateEndpoints(endpoints); err != nil {
		return err
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"ip", "hostname", "group", "description", "entitytype", "device", "vendor", "additional_notes", "dev"}); err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		group := strings.TrimSpace(endpoint.Group)
		if group == "" {
			group = "default"
		}
		record := []string{
			strings.TrimSpace(endpoint.IP),
			strings.TrimSpace(endpoint.Hostname),
			group,
			strings.TrimSpace(endpoint.Description),
			strings.TrimSpace(endpoint.EntityType),
			strings.TrimSpace(endpoint.Device),
			strings.TrimSpace(endpoint.Vendor),
			strings.TrimSpace(endpoint.AdditionalNotes),
			strconv.FormatBool(endpoint.Dev),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return writeWithBackup(path, buf.Bytes())
}

func ValidateEndpoints(endpoints []models.Endpoint) error {
	for index, endpoint := range endpoints {
		if strings.TrimSpace(endpoint.IP) == "" {
			return fmt.Errorf("endpoint %d is missing ip", index+1)
		}
		if strings.TrimSpace(endpoint.Hostname) == "" {
			return fmt.Errorf("endpoint %d is missing hostname", index+1)
		}
	}
	return nil
}

func writeJSONFile(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeWithBackup(path, b)
}

func writeYAMLFile(path string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeWithBackup(path, b)
}

func writeWithBackup(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		backupPath := fmt.Sprintf("%s.%s.bak", path, time.Now().UTC().Format("20060102T150405Z"))
		if copyErr := copyFile(path, backupPath); copyErr != nil {
			return copyErr
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tempPath := filepath.Join(filepath.Dir(path), fmt.Sprintf(".%s.%d.tmp", filepath.Base(path), time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, content, 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			_ = os.Remove(tempPath)
			return err
		}
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func copyFile(src string, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func isSupportedConfigExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".psd1", ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func newSourceInfo(path string) SourceInfo {
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if format == "" {
		format = "yaml"
	}
	label := filepath.Base(path)
	if label == "" {
		label = fmt.Sprintf("config.%s", format)
	}
	return SourceInfo{
		Path:   filepath.Clean(path),
		Format: format,
		Label:  label,
	}
}