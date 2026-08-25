package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

const discoveryReviewStoreSchema = 1

type discoveryReviewRecord struct {
	IdentityKey string          `json:"identity_key"`
	Aliases     []string        `json:"aliases,omitempty"`
	State       string          `json:"state"`
	Note        string          `json:"note,omitempty"`
	ReviewedAt  string          `json:"reviewed_at,omitempty"`
	Endpoint    models.Endpoint `json:"endpoint"`
}

type discoveryReviewStore struct {
	SchemaVersion int                     `json:"schema_version"`
	Records       []discoveryReviewRecord `json:"records"`
}

type discoveryReviewWriteRequest struct {
	State string            `json:"state"`
	Note  string            `json:"note,omitempty"`
	Items []models.Endpoint `json:"items"`
}

type discoveryReviewWriteResponse struct {
	State       string            `json:"state"`
	UpdatedAt   string            `json:"updated_at"`
	Items       []models.Endpoint `json:"items"`
	RecordCount int               `json:"record_count"`
}

type discoveryRegistryEntry struct {
	Endpoint models.Endpoint
	Aliases  []string
	State    string
	Note     string
	At       string
}

func discoveryReviewStorePath(historyPath string) string {
	return filepath.Join(strings.TrimSpace(historyPath), "reviews.json")
}

func loadDiscoveryReviewStore(historyPath string) (discoveryReviewStore, error) {
	store := discoveryReviewStore{SchemaVersion: discoveryReviewStoreSchema, Records: []discoveryReviewRecord{}}
	data, err := os.ReadFile(discoveryReviewStorePath(historyPath))
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if err := decodeStrictJSON(data, &store); err != nil {
		return discoveryReviewStore{}, fmt.Errorf("read discovery review registry: %w", err)
	}
	if store.SchemaVersion == 0 {
		store.SchemaVersion = discoveryReviewStoreSchema
	}
	if store.SchemaVersion != discoveryReviewStoreSchema {
		return discoveryReviewStore{}, fmt.Errorf("unsupported discovery review registry schema %d", store.SchemaVersion)
	}
	if store.Records == nil {
		store.Records = []discoveryReviewRecord{}
	}
	return store, nil
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func saveDiscoveryReviewStore(historyPath string, store discoveryReviewStore) error {
	store.SchemaVersion = discoveryReviewStoreSchema
	sort.SliceStable(store.Records, func(i, j int) bool {
		return store.Records[i].IdentityKey < store.Records[j].IdentityKey
	})
	return writeJSONAtomic(discoveryReviewStorePath(historyPath), store)
}

func normalizeDiscoveryReviewState(value string) (string, error) {
	state := strings.ToLower(strings.TrimSpace(value))
	switch state {
	case models.DiscoveryReviewNeedsReview, models.DiscoveryReviewDeferred, models.DiscoveryReviewIgnored:
		return state, nil
	case models.DiscoveryReviewApproved:
		return "", errors.New("approved state is created only by saving the device into the endpoint inventory")
	default:
		return "", fmt.Errorf("invalid discovery review state %q; use needs_review, deferred, or ignored", value)
	}
}

func discoveryIdentityScope(endpoint models.Endpoint) string {
	if value := strings.ToLower(strings.TrimSpace(endpoint.RoutingDomain)); value != "" {
		return "routing:" + value
	}
	if value := strings.ToLower(strings.TrimSpace(endpoint.SubnetID)); value != "" {
		return "subnet:" + value
	}
	return "default"
}

func normalizedDiscoveryFQDN(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func discoveryRegistryAliases(endpoint models.Endpoint) []string {
	aliases := make([]string, 0, 6)
	if assetID := strings.ToLower(strings.TrimSpace(endpoint.AssetID)); assetID != "" {
		aliases = append(aliases, "asset:"+assetID)
	}
	scope := discoveryIdentityScope(endpoint)
	if endpoint.DNSForwardConfirmed {
		if fqdn := normalizedDiscoveryFQDN(endpoint.FQDN); fqdn != "" {
			aliases = append(aliases, "fqdn:"+scope+":"+fqdn, "fqdn:"+fqdn)
		}
	}
	if ip := normalizedDiscoveryIP(endpoint.IP); ip != "" {
		aliases = append(aliases, "ip:"+scope+":"+ip, "ip:"+ip)
	}
	return uniqueStrings(aliases)
}

func stableDiscoveryAliases(endpoint models.Endpoint) []string {
	aliases := make([]string, 0, 5)
	if assetID := strings.ToLower(strings.TrimSpace(endpoint.AssetID)); assetID != "" {
		aliases = append(aliases, "asset:"+assetID)
	}
	scope := discoveryIdentityScope(endpoint)
	if endpoint.DynamicAddress {
		if endpoint.DNSForwardConfirmed {
			if fqdn := normalizedDiscoveryFQDN(endpoint.FQDN); fqdn != "" {
				aliases = append(aliases, "fqdn:"+scope+":"+fqdn, "fqdn:"+fqdn)
			}
		}
	} else if ip := normalizedDiscoveryIP(endpoint.IP); ip != "" {
		aliases = append(aliases, "ip:"+scope+":"+ip, "ip:"+ip)
	}
	return uniqueStrings(aliases)
}

func discoveryReviewIdentityKey(endpoint models.Endpoint) string {
	if aliases := stableDiscoveryAliases(endpoint); len(aliases) > 0 {
		return aliases[0]
	}
	if ip := normalizedDiscoveryIP(endpoint.IP); ip != "" {
		return "observation:" + discoveryIdentityScope(endpoint) + ":" + ip
	}
	return "observation:unknown:" + strings.ToLower(strings.TrimSpace(endpoint.Hostname))
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueDiscoveryRegistryIndex(entries []discoveryRegistryEntry) map[string]int {
	counts := make(map[string]int)
	owners := make(map[string]int)
	for index, entry := range entries {
		aliases := uniqueStrings(append(discoveryRegistryAliases(entry.Endpoint), entry.Aliases...))
		for _, alias := range aliases {
			counts[alias]++
			owners[alias] = index
		}
	}
	result := make(map[string]int)
	for alias, count := range counts {
		if count == 1 {
			result[alias] = owners[alias]
		}
	}
	return result
}

func findDiscoveryRegistryEntry(endpoint models.Endpoint, entries []discoveryRegistryEntry, index map[string]int) (discoveryRegistryEntry, bool) {
	matched := -1
	for _, alias := range discoveryRegistryAliases(endpoint) {
		position, exists := index[alias]
		if !exists {
			continue
		}
		if matched >= 0 && matched != position {
			return discoveryRegistryEntry{}, false
		}
		matched = position
	}
	if matched < 0 {
		return discoveryRegistryEntry{}, false
	}
	return entries[matched], true
}

func mergeDiscoveryRegistryMetadata(observed, known models.Endpoint) models.Endpoint {
	merged := observed
	copyText := func(destination *string, source string, blankDefault bool) {
		current := strings.TrimSpace(*destination)
		if blankDefault && strings.EqualFold(current, "default") {
			current = ""
		}
		if current == "" && strings.TrimSpace(source) != "" {
			*destination = strings.TrimSpace(source)
		}
	}
	copyText(&merged.Hostname, known.Hostname, false)
	copyText(&merged.FQDN, known.FQDN, false)
	copyText(&merged.Group, known.Group, true)
	copyText(&merged.Description, known.Description, false)
	copyText(&merged.EntityType, known.EntityType, false)
	copyText(&merged.Device, known.Device, false)
	copyText(&merged.Vendor, known.Vendor, false)
	copyText(&merged.AssetID, known.AssetID, false)
	copyText(&merged.AdditionalNotes, known.AdditionalNotes, false)
	copyText(&merged.ClassificationSource, known.ClassificationSource, false)
	if strings.TrimSpace(known.DeviceMode) != "" || known.Dev {
		merged.DeviceMode = known.EffectiveDeviceMode()
		merged.Dev = merged.DeviceMode == models.DeviceModeLegacyDev
	}
	merged.MonitoringEnabled = models.Bool(known.IsMonitoringEnabled())
	merged.AlertingEnabled = models.Bool(known.IsAlertingEnabled())
	merged.AlertingReason = known.AlertingReason
	merged.MaintenanceUntil = known.MaintenanceUntil
	merged.MaintenanceReason = known.MaintenanceReason
	merged.DynamicAddress = merged.DynamicAddress || known.DynamicAddress
	return merged
}

func (s *apiServer) reconcileDiscoveryEndpoints(items []models.Endpoint) ([]models.Endpoint, error) {
	var inventory []models.Endpoint
	if _, err := os.Stat(s.opts.EndpointsPath); err == nil {
		inventory, err = config.LoadEditableEndpoints(s.opts.EndpointsPath)
		if err != nil {
			return nil, fmt.Errorf("load endpoint identity registry: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect endpoint identity registry: %w", err)
	}
	cfg, ok := s.effectiveStatusConfig()
	if !ok {
		cfg = config.Defaults(s.opts.RootDir)
	}
	historyPath := discoveryHistoryPath(cfg, s.opts.ConfigPath)
	reviews, err := loadDiscoveryReviewStore(historyPath)
	if err != nil {
		return nil, err
	}

	inventoryEntries := make([]discoveryRegistryEntry, 0, len(inventory))
	for _, endpoint := range inventory {
		inventoryEntries = append(inventoryEntries, discoveryRegistryEntry{Endpoint: endpoint, State: models.DiscoveryReviewApproved})
	}
	reviewEntries := make([]discoveryRegistryEntry, 0, len(reviews.Records))
	for _, record := range reviews.Records {
		reviewEntries = append(reviewEntries, discoveryRegistryEntry{Endpoint: record.Endpoint, Aliases: record.Aliases, State: record.State, Note: record.Note, At: record.ReviewedAt})
	}
	inventoryIndex := uniqueDiscoveryRegistryIndex(inventoryEntries)
	reviewIndex := uniqueDiscoveryRegistryIndex(reviewEntries)

	reconciled := make([]models.Endpoint, len(items))
	for position, item := range items {
		if known, found := findDiscoveryRegistryEntry(item, inventoryEntries, inventoryIndex); found {
			item = mergeDiscoveryRegistryMetadata(item, known.Endpoint)
			item.DiscoveryReviewState = models.DiscoveryReviewApproved
			item.DiscoveryReviewedAt = known.Endpoint.DiscoveryReviewedAt
			item.DiscoveryReviewNote = known.Endpoint.DiscoveryReviewNote
		} else if known, found := findDiscoveryRegistryEntry(item, reviewEntries, reviewIndex); found {
			item = mergeDiscoveryRegistryMetadata(item, known.Endpoint)
			item.DiscoveryReviewState = known.State
			item.DiscoveryReviewedAt = known.At
			item.DiscoveryReviewNote = known.Note
		} else {
			item.DiscoveryReviewState = models.DiscoveryReviewNeedsReview
			item.DiscoveryReviewedAt = ""
			item.DiscoveryReviewNote = ""
		}
		reconciled[position] = item
	}
	return reconciled, nil
}

func (s *apiServer) handleDiscoveryReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request discoveryReviewWriteRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	state, err := normalizeDiscoveryReviewState(request.State)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(request.Items) == 0 || len(request.Items) > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "select between 1 and 1000 discovery results"})
		return
	}
	if len(request.Note) > 2000 || strings.ContainsAny(request.Note, "\r\n") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "review note must be a single line of 2000 characters or fewer"})
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cfg, ok := s.effectiveStatusConfig()
	if !ok {
		cfg = config.Defaults(s.opts.RootDir)
	}
	historyPath := discoveryHistoryPath(cfg, s.opts.ConfigPath)
	store, err := loadDiscoveryReviewStore(historyPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated := make([]models.Endpoint, len(request.Items))
	for itemIndex, item := range request.Items {
		if strings.TrimSpace(item.IP) == "" || strings.TrimSpace(item.Hostname) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("discovery item %d requires ip and hostname", itemIndex+1)})
			return
		}
		item.DiscoveryReviewState = state
		item.DiscoveryReviewNote = strings.TrimSpace(request.Note)
		if state == models.DiscoveryReviewNeedsReview {
			item.DiscoveryReviewedAt = ""
		} else {
			item.DiscoveryReviewedAt = now
		}
		aliases := discoveryRegistryAliases(item)
		identityKey := discoveryReviewIdentityKey(item)
		recordIndex := -1
		for index, record := range store.Records {
			if record.IdentityKey == identityKey || stringSetsIntersect(record.Aliases, aliases) {
				if recordIndex >= 0 && recordIndex != index {
					writeJSON(w, http.StatusConflict, map[string]string{"error": "review identity is ambiguous; assign a unique Asset ID or verified FQDN"})
					return
				}
				recordIndex = index
			}
		}
		record := discoveryReviewRecord{
			IdentityKey: identityKey,
			Aliases:     aliases,
			State:       state,
			Note:        item.DiscoveryReviewNote,
			ReviewedAt:  item.DiscoveryReviewedAt,
			Endpoint:    item,
		}
		if recordIndex >= 0 {
			record.Aliases = uniqueStrings(append(store.Records[recordIndex].Aliases, aliases...))
			store.Records[recordIndex] = record
		} else {
			store.Records = append(store.Records, record)
		}
		updated[itemIndex] = item
	}
	if err := saveDiscoveryReviewStore(historyPath, store); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, discoveryReviewWriteResponse{State: state, UpdatedAt: now, Items: updated, RecordCount: len(store.Records)})
}

func stringSetsIntersect(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := seen[value]; exists {
			return true
		}
	}
	return false
}
