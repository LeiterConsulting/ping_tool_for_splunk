package config

import "testing"

func TestValidateStructuredEnrichmentAcceptsSubnetAndRegexPairs(t *testing.T) {
	cfg := Config{
		Discovery: Discovery{Subnets: []DiscoverySubnet{{
			ID: "users", CIDR: "10.20.30.0/24", AddressingMode: "dhcp",
		}}},
		Classification: Classification{Rules: []ClassificationRule{{
			ID: "site", Enabled: true, Source: "hostname", Pattern: `^(?P<site>[a-z]{3})-`,
			Assignments: map[string]string{"group": "${site}"},
		}}},
	}
	if err := ValidateStructuredEnrichment(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateStructuredEnrichmentRejectsUnsafePairs(t *testing.T) {
	tests := []Config{
		{Discovery: Discovery{Subnets: []DiscoverySubnet{{ID: "bad", CIDR: "10.0.0.0/8", AddressingMode: "dhcp"}}}},
		{Discovery: Discovery{Subnets: []DiscoverySubnet{{ID: "bad", CIDR: "10.0.0.0/24", AddressingMode: "unknown"}}}},
		{Classification: Classification{Rules: []ClassificationRule{{ID: "bad", Enabled: true, Source: "hostname", Pattern: "[", Assignments: map[string]string{"group": "x"}}}}},
		{Classification: Classification{Rules: []ClassificationRule{{ID: "bad", Enabled: true, Source: "hostname", Pattern: "x"}}}},
	}
	for index, cfg := range tests {
		if err := ValidateStructuredEnrichment(cfg); err == nil {
			t.Fatalf("case %d unexpectedly succeeded", index)
		}
	}
}
