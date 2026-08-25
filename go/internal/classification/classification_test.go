package classification

import (
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

func TestRegexCaptureAssignmentsFillBlankFields(t *testing.T) {
	engine, err := Compile([]config.ClassificationRule{{
		ID: "server-name", Enabled: true, Source: "hostname",
		Pattern: `^(?P<site>[a-z]{3})-(?P<class>srv)-`,
		Assignments: map[string]string{
			"group": "${site} servers", "entitytype": "server", "device": "${class}", "vendor": "Acme",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Apply(models.Endpoint{Hostname: "nyc-srv-001"})
	if result.Endpoint.Group != "nyc servers" || result.Endpoint.Device != "srv" || result.Endpoint.Vendor != "Acme" {
		t.Fatalf("unexpected classification: %#v", result)
	}
	if result.Endpoint.ClassificationSource != "rule:server-name" || len(result.MatchedRules) != 1 {
		t.Fatalf("missing rule provenance: %#v", result)
	}
}

func TestRuleDoesNotOverwriteHumanValueByDefault(t *testing.T) {
	engine, err := Compile([]config.ClassificationRule{{
		ID: "network", Enabled: true, Source: "either", Pattern: `(?i)switch`,
		Assignments: map[string]string{"vendor": "Automatic", "device": "switch"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Apply(models.Endpoint{Hostname: "core-switch-1", Vendor: "Human Value"})
	if result.Endpoint.Vendor != "Human Value" || result.Endpoint.Device != "switch" {
		t.Fatalf("fill-blank behavior failed: %#v", result.Endpoint)
	}
}

func TestCompileRejectsInvalidSourcePatternAndAssignment(t *testing.T) {
	tests := []config.ClassificationRule{
		{ID: "source", Enabled: true, Source: "ip", Pattern: `x`, Assignments: map[string]string{"group": "x"}},
		{ID: "pattern", Enabled: true, Source: "hostname", Pattern: `[`, Assignments: map[string]string{"group": "x"}},
		{ID: "field", Enabled: true, Source: "hostname", Pattern: `x`, Assignments: map[string]string{"hostname": "x"}},
	}
	for _, rule := range tests {
		if _, err := Compile([]config.ClassificationRule{rule}); err == nil {
			t.Fatalf("Compile(%s) unexpectedly succeeded", rule.ID)
		}
	}
}
