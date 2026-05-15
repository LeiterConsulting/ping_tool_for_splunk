package config

import (
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
