package config

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

var allowedClassificationAssignments = map[string]struct{}{
	"group": {}, "entitytype": {}, "device": {}, "vendor": {},
}

// ValidateStructuredEnrichment rejects configuration that could otherwise be
// saved successfully but fail later when discovery attempts to use it.
func ValidateStructuredEnrichment(cfg Config) error {
	seenSubnetIDs := make(map[string]int, len(cfg.Discovery.Subnets))
	seenCIDRs := make(map[string]int, len(cfg.Discovery.Subnets))
	for index, subnet := range cfg.Discovery.Subnets {
		id := strings.TrimSpace(subnet.ID)
		if id == "" {
			return fmt.Errorf("discovery subnet %d is missing id", index+1)
		}
		idKey := strings.ToLower(id)
		if first, exists := seenSubnetIDs[idKey]; exists {
			return fmt.Errorf("discovery subnet %d duplicates id %q from subnet %d", index+1, id, first)
		}
		seenSubnetIDs[idKey] = index + 1

		prefix, err := netip.ParsePrefix(strings.TrimSpace(subnet.CIDR))
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 16 || prefix.Bits() > 30 {
			return fmt.Errorf("discovery subnet %q has invalid CIDR %q; use an IPv4 /16 through /30", id, subnet.CIDR)
		}
		cidrKey := prefix.Masked().String()
		if first, exists := seenCIDRs[cidrKey]; exists {
			return fmt.Errorf("discovery subnet %d duplicates CIDR %q from subnet %d", index+1, cidrKey, first)
		}
		seenCIDRs[cidrKey] = index + 1

		switch strings.ToLower(strings.TrimSpace(subnet.AddressingMode)) {
		case "", "static", "dhcp":
		default:
			return fmt.Errorf("discovery subnet %q has invalid addressing_mode %q; use static or dhcp", id, subnet.AddressingMode)
		}
	}

	seenRuleIDs := make(map[string]int, len(cfg.Classification.Rules))
	for index, rule := range cfg.Classification.Rules {
		if !rule.Enabled {
			continue
		}
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			return fmt.Errorf("classification rule %d is missing id", index+1)
		}
		idKey := strings.ToLower(id)
		if first, exists := seenRuleIDs[idKey]; exists {
			return fmt.Errorf("classification rule %d duplicates id %q from rule %d", index+1, id, first)
		}
		seenRuleIDs[idKey] = index + 1
		switch strings.ToLower(strings.TrimSpace(rule.Source)) {
		case "hostname", "fqdn", "either":
		default:
			return fmt.Errorf("classification rule %q has invalid source %q", id, rule.Source)
		}
		if strings.TrimSpace(rule.Pattern) == "" {
			return fmt.Errorf("classification rule %q is missing pattern", id)
		}
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("classification rule %q pattern: %w", id, err)
		}
		if len(rule.Assignments) == 0 {
			return fmt.Errorf("classification rule %q needs at least one field assignment", id)
		}
		for field := range rule.Assignments {
			field = strings.ToLower(strings.TrimSpace(field))
			if _, allowed := allowedClassificationAssignments[field]; !allowed {
				return fmt.Errorf("classification rule %q cannot assign field %q", id, field)
			}
		}
	}
	return nil
}
