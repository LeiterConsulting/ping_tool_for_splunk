package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePSD1_BasicNested(t *testing.T) {
	input := `# comment
@{
  pings_per_cycle = 4
  output_mode = file
  emit_individual_pings = $true
  ping = @{ mode = auto }
  hec = @{ enabled = $false; url = 'https://example'; dead_letter_rotation_size_mb = 10 }
  metrics = @{ enabled = $true; mode = 'dual' }
}`

	m, err := parsePSD1(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parsePSD1 error: %v", err)
	}
	if got, ok := m["pings_per_cycle"].(int); !ok || got != 4 {
		t.Fatalf("pings_per_cycle: got %#v", m["pings_per_cycle"])
	}
	if got, ok := m["emit_individual_pings"].(bool); !ok || got != true {
		t.Fatalf("emit_individual_pings: got %#v", m["emit_individual_pings"])
	}
	pm, ok := m["ping"].(map[string]interface{})
	if !ok {
		t.Fatalf("ping map: got %#v", m["ping"])
	}
	if pm["mode"] != "auto" {
		t.Fatalf("ping.mode: got %#v", pm["mode"])
	}
	hm, ok := m["hec"].(map[string]interface{})
	if !ok {
		t.Fatalf("hec map: got %#v", m["hec"])
	}
	if hm["enabled"].(bool) != false {
		t.Fatalf("hec.enabled: got %#v", hm["enabled"])
	}
	if hm["url"].(string) != "https://example" {
		t.Fatalf("hec.url: got %#v", hm["url"])
	}
}

func TestParsePSD1SupportsPowerShellDataFileCommentsAndEscapes(t *testing.T) {
	input := "<# deployment comment\ncontinues #>\n@{ path = \"line`nnext``value`\"\"; quote = 'it''s safe' }"
	values, err := parsePSD1(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if values["path"] != "line\nnext`value\"" || values["quote"] != "it's safe" {
		t.Fatalf("escaped values = %#v", values)
	}
}

func TestParsePSD1RejectsDuplicateKeysCaseInsensitively(t *testing.T) {
	_, err := parsePSD1(strings.NewReader("@{ timeout_ms = 1000\nTIMEOUT_MS = 2000 }"))
	if err == nil || !strings.Contains(err.Error(), "duplicate key") || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestParsePSD1ReportsUnterminatedStringLocation(t *testing.T) {
	_, err := parsePSD1(strings.NewReader("@{\n  token = 'unfinished\n}"))
	if err == nil || !strings.Contains(err.Error(), "psd1:2:11") || !strings.Contains(err.Error(), "unterminated string") {
		t.Fatalf("unterminated string error = %v", err)
	}
}

func TestParsePSD1RejectsIntegerOverflow(t *testing.T) {
	_, err := parsePSD1(strings.NewReader("@{ timeout_ms = 999999999999999999999999999999 }"))
	if err == nil || !strings.Contains(err.Error(), "invalid integer") {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestParsePSD1LoadsShippedConfiguration(t *testing.T) {
	path := filepath.Join("..", "..", "..", "config.psd1")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("shipped config unavailable: %v", err)
	}
	values, err := parsePSD1File(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["pings_per_cycle"] != 4 || values["output_mode"] != "file" {
		t.Fatalf("shipped configuration values = %#v", values)
	}
}
