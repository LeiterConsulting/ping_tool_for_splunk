package advisor

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

var endpointIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func inspectInventory(path string) (Inventory, []Finding) {
	result := Inventory{Path: path}
	findings := []Finding{}
	file, err := os.Open(path)
	if err != nil {
		return result, []Finding{blocker("INV_FILE", "Endpoint inventory cannot be opened", err.Error(), "Confirm the endpoints path and service-account permissions.")}
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = false
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		return result, []Finding{blocker("INV_HEADER", "Endpoint CSV header cannot be read", err.Error(), "Repair the CSV header before applying any fixes.")}
	}
	index := map[string]int{}
	for column, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header))
		if name == "" {
			continue
		}
		if first, exists := index[name]; exists {
			findings = append(findings, blocker("INV_DUPLICATE_HEADER", "Duplicate CSV header", fmt.Sprintf("Header %q appears in columns %d and %d.", name, first+1, column+1), "Keep one unambiguous column for each field."))
			continue
		}
		index[name] = column
	}
	for _, required := range []string{"ip", "hostname"} {
		if _, ok := index[required]; !ok {
			findings = append(findings, blocker("INV_MISSING_HEADER", "Required CSV header is missing", fmt.Sprintf("The inventory does not contain a %q column.", required), "Add the required column without changing existing endpoint meanings."))
		}
	}
	if countSeverity(findings, SeverityBlocker) > 0 {
		return result, findings
	}

	get := func(row []string, name string) string {
		position, ok := index[name]
		if !ok || position >= len(row) {
			return ""
		}
		return row[position]
	}
	seenTargets := map[string]struct {
		endpoint models.Endpoint
		row      int
	}{}
	seenIDs := map[string]int{}
	rowNumber := 1
	for {
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		rowNumber++
		result.Rows++
		if readErr != nil {
			findings = append(findings, blocker("INV_CSV_ROW", "CSV row cannot be parsed", fmt.Sprintf("CSV line %d: %v", rowNumber, readErr), "Repair quoting or delimiters on this row."))
			continue
		}
		rawIP := get(row, "ip")
		rawHostname := get(row, "hostname")
		ipText := strings.TrimSpace(rawIP)
		hostname := strings.TrimSpace(rawHostname)
		dev, devErr := parseBool(get(row, "dev"))
		endpoint := models.Endpoint{
			IP: ipText, Hostname: hostname, Dev: dev, Group: first(get(row, "group"), "default"),
			Description: strings.TrimSpace(get(row, "description")), EntityType: strings.TrimSpace(get(row, "entitytype")),
			Device: strings.TrimSpace(get(row, "device")), Vendor: strings.TrimSpace(get(row, "vendor")),
			AdditionalNotes: strings.TrimSpace(get(row, "additional_notes")), EndpointID: strings.TrimSpace(get(row, "endpoint_id")),
		}
		if strings.TrimSpace(rawIP) != rawIP || strings.TrimSpace(rawHostname) != rawHostname {
			findings = append(findings, Finding{Code: "INV_WHITESPACE", Severity: SeverityOpportunity, Category: "inventory", Title: "Endpoint fields contain surrounding whitespace", Message: fmt.Sprintf("CSV line %d contains whitespace that can be normalized safely.", rowNumber), Recommendation: "Apply safe inventory normalization.", FixID: "normalize_inventory"})
		}
		if devErr != nil {
			findings = append(findings, blocker("INV_DEV", "Invalid dev flag", fmt.Sprintf("CSV line %d: %v", rowNumber, devErr), "Use true/false, 1/0, yes/no, or leave the field blank."))
		}
		if ipText == "" {
			findings = append(findings, blocker("INV_MISSING_IP", "Endpoint IP is missing", fmt.Sprintf("CSV line %d has no target address.", rowNumber), "Enter the intended literal IPv4 or IPv6 address."))
			result.InvalidAddresses++
			continue
		}
		parsed := net.ParseIP(ipText)
		if parsed == nil {
			result.InvalidAddresses++
			recommendation := "Enter a complete literal IPv4 or IPv6 address; the advisor will not guess a target."
			title := "Endpoint address is invalid"
			if looksLikeMissingOctet(ipText) {
				title = "Endpoint address may be missing an octet"
			}
			findings = append(findings, blocker("INV_INVALID_IP", title, fmt.Sprintf("CSV line %d contains %q.", rowNumber, ipText), recommendation))
			continue
		}
		canonical := strings.ToLower(parsed.String())
		endpoint.IP = canonical
		endpoint.EndpointID = models.StableEndpointID(endpoint.EndpointID, canonical)
		duplicateTargetSafe := false
		if hostname == "" {
			result.MissingHostnames++
			findings = append(findings, blocker("INV_MISSING_HOSTNAME", "Endpoint hostname is missing", fmt.Sprintf("CSV line %d (%s) has no hostname.", rowNumber, canonical), "Enter an operator-approved hostname; reverse DNS may be used as a suggestion but is not applied automatically."))
		}
		if devErr == nil && hostname != "" {
			result.NormalizedRows = append(result.NormalizedRows, endpoint)
		}
		if previous, exists := seenTargets[canonical]; exists {
			result.DuplicateTargets++
			safe := endpointsEquivalent(previous.endpoint, endpoint)
			finding := blocker("INV_DUPLICATE_IP", "Duplicate target IP", fmt.Sprintf("CSV line %d (%s) duplicates CSV line %d (%s) for %s.", rowNumber, endpoint.Hostname, previous.row, previous.endpoint.Hostname, canonical), "Remove or correct the duplicate so one target contributes one SLA observation.")
			if safe {
				duplicateTargetSafe = true
				result.SafeDuplicateRows = append(result.SafeDuplicateRows, rowNumber)
				finding.Severity = SeverityWarning
				finding.Message += " The normalized metadata is identical."
				finding.Recommendation = "The later byte-equivalent row can be removed safely after preview."
				finding.FixID = "remove_identical_duplicates"
			}
			findings = append(findings, finding)
		} else {
			seenTargets[canonical] = struct {
				endpoint models.Endpoint
				row      int
			}{endpoint, rowNumber}
			result.SchedulableEndpoints++
		}
		if !endpointIDPattern.MatchString(endpoint.EndpointID) {
			findings = append(findings, blocker("INV_ENDPOINT_ID", "Endpoint ID is invalid", fmt.Sprintf("CSV line %d has endpoint_id %q.", rowNumber, endpoint.EndpointID), "Use letters, digits, dots, underscores, colons, or hyphens."))
		} else if previousRow, exists := seenIDs[strings.ToLower(endpoint.EndpointID)]; exists && !duplicateTargetSafe {
			result.DuplicateIDs++
			findings = append(findings, blocker("INV_DUPLICATE_ID", "Duplicate endpoint ID", fmt.Sprintf("CSV line %d duplicates endpoint_id %q from CSV line %d.", rowNumber, endpoint.EndpointID, previousRow), "Assign a unique stable endpoint ID."))
		} else if !duplicateTargetSafe {
			seenIDs[strings.ToLower(endpoint.EndpointID)] = rowNumber
		}
	}
	return result, findings
}

func parseBool(raw string) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return false, nil
	}
	switch value {
	case "true", "1", "yes", "y":
		return true, nil
	case "false", "0", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}

func endpointsEquivalent(a models.Endpoint, b models.Endpoint) bool {
	a.EndpointID, b.EndpointID = "", ""
	return a == b
}

func looksLikeMissingOctet(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

func first(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
