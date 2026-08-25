package classification

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

var allowedAssignmentFields = map[string]struct{}{
	"group": {}, "entitytype": {}, "device": {}, "vendor": {},
}

type compiledRule struct {
	config config.ClassificationRule
	expr   *regexp.Regexp
}

type Engine struct {
	rules []compiledRule
}

type Result struct {
	Endpoint     models.Endpoint   `json:"endpoint"`
	MatchedRules []string          `json:"matched_rules"`
	Changes      map[string]string `json:"changes"`
}

func Compile(rules []config.ClassificationRule) (*Engine, error) {
	compiled := make([]compiledRule, 0, len(rules))
	seenIDs := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if !rule.Enabled {
			continue
		}
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			return nil, fmt.Errorf("classification rule %d is missing id", index+1)
		}
		key := strings.ToLower(id)
		if _, exists := seenIDs[key]; exists {
			return nil, fmt.Errorf("classification rule %d duplicates id %q", index+1, id)
		}
		seenIDs[key] = struct{}{}
		source := strings.ToLower(strings.TrimSpace(rule.Source))
		if source == "" {
			source = "either"
		}
		if source != "hostname" && source != "fqdn" && source != "either" {
			return nil, fmt.Errorf("classification rule %q has invalid source %q", id, rule.Source)
		}
		if strings.TrimSpace(rule.Pattern) == "" {
			return nil, fmt.Errorf("classification rule %q is missing pattern", id)
		}
		expr, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("classification rule %q pattern: %w", id, err)
		}
		if len(rule.Assignments) == 0 {
			return nil, fmt.Errorf("classification rule %q needs at least one field assignment", id)
		}
		for field := range rule.Assignments {
			if _, allowed := allowedAssignmentFields[strings.ToLower(strings.TrimSpace(field))]; !allowed {
				return nil, fmt.Errorf("classification rule %q cannot assign field %q", id, field)
			}
		}
		normalizedAssignments := make(map[string]string, len(rule.Assignments))
		for field, value := range rule.Assignments {
			normalizedAssignments[strings.ToLower(strings.TrimSpace(field))] = strings.TrimSpace(value)
		}
		rule.ID = id
		rule.Source = source
		rule.Assignments = normalizedAssignments
		compiled = append(compiled, compiledRule{config: rule, expr: expr})
	}
	return &Engine{rules: compiled}, nil
}

func (e *Engine) Apply(endpoint models.Endpoint) Result {
	result := Result{Endpoint: endpoint, Changes: make(map[string]string)}
	if e == nil {
		return result
	}
	for _, rule := range e.rules {
		candidate, match := matchingCandidate(rule, result.Endpoint)
		if match == nil {
			continue
		}
		result.MatchedRules = append(result.MatchedRules, rule.config.ID)
		ruleChanged := false
		keys := make([]string, 0, len(rule.config.Assignments))
		for key := range rule.config.Assignments {
			keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
		}
		sort.Strings(keys)
		for _, field := range keys {
			template := rule.config.Assignments[field]
			value := strings.TrimSpace(string(rule.expr.ExpandString(nil, template, candidate, match)))
			if value == "" {
				continue
			}
			current := endpointField(result.Endpoint, field)
			if strings.TrimSpace(current) != "" && !rule.config.Overwrite {
				continue
			}
			setEndpointField(&result.Endpoint, field, value)
			result.Changes[field] = value
			ruleChanged = true
		}
		if ruleChanged {
			result.Endpoint.ClassificationSource = appendSource(result.Endpoint.ClassificationSource, "rule:"+rule.config.ID)
		}
		if rule.config.StopOnMatch {
			break
		}
	}
	return result
}

func matchingCandidate(rule compiledRule, endpoint models.Endpoint) (string, []int) {
	candidates := []string{endpoint.Hostname}
	if rule.config.Source == "fqdn" {
		candidates = []string{endpoint.FQDN}
	} else if rule.config.Source == "either" {
		candidates = []string{endpoint.Hostname, endpoint.FQDN}
	}
	for _, candidate := range candidates {
		if match := rule.expr.FindStringSubmatchIndex(candidate); match != nil {
			return candidate, match
		}
	}
	return "", nil
}

func endpointField(endpoint models.Endpoint, field string) string {
	switch field {
	case "group":
		if strings.EqualFold(strings.TrimSpace(endpoint.Group), "default") {
			return ""
		}
		return endpoint.Group
	case "entitytype":
		return endpoint.EntityType
	case "device":
		return endpoint.Device
	case "vendor":
		return endpoint.Vendor
	default:
		return ""
	}
}

func setEndpointField(endpoint *models.Endpoint, field string, value string) {
	switch field {
	case "group":
		endpoint.Group = value
	case "entitytype":
		endpoint.EntityType = value
	case "device":
		endpoint.Device = value
	case "vendor":
		endpoint.Vendor = value
	}
}

func appendSource(existing string, source string) string {
	for _, item := range strings.Split(existing, ",") {
		if strings.EqualFold(strings.TrimSpace(item), source) {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return source
	}
	return strings.TrimSpace(existing) + "," + source
}
