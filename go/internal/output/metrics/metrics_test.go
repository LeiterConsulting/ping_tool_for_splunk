package metrics

import (
	"testing"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/config"
	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

func TestBuildPayloadCarriesAlertAndIdentityPolicy(t *testing.T) {
	payload := buildPayload(models.SummaryEvent{
		SchemaVersion: models.SchemaVersion, Timestamp: "2026-08-24T12:00:00Z",
		Hostname: "host-1", TargetIP: "192.0.2.10", DeviceMode: models.DeviceModeProduction,
		AlertingEnabled: false, AlertingReason: "ticket-123", AssetID: "asset-1",
		DynamicAddress: true, ClassificationSource: "rule:server",
		SubnetID: "users", SubnetName: "User LAN", SubnetVLAN: "230", SubnetLocation: "NYC",
		AddressingMode: "dhcp", RoutingDomain: "corp",
	}, config.Metrics{Index: "metrics", UseMetricsIndex: true}, "collector")

	if payload.Fields["metric_name:ping.alerting_enabled"] != 0 {
		t.Fatalf("alerting metric = %#v", payload.Fields["metric_name:ping.alerting_enabled"])
	}
	if got, ok := payload.Fields["metric_name:ping.observed_at_epoch"].(float64); !ok || got != 1787572800 {
		t.Fatalf("observed_at_epoch = %#v, want 1787572800", payload.Fields["metric_name:ping.observed_at_epoch"])
	}
	for key, want := range map[string]interface{}{
		"device_mode": models.DeviceModeProduction, "alerting_enabled": false,
		"alerting_reason": "ticket-123", "asset_id": "asset-1",
		"dynamic_address": true, "classification_source": "rule:server",
		"subnet_id": "users", "subnet_name": "User LAN", "subnet_vlan": "230",
		"subnet_location": "NYC", "addressing_mode": "dhcp", "routing_domain": "corp",
	} {
		if got := payload.Fields[key]; got != want {
			t.Fatalf("field %s = %#v, want %#v", key, got, want)
		}
	}
}
