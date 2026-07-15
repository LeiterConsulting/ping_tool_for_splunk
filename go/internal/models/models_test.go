package models

import "testing"

func TestStableEndpointID(t *testing.T) {
	first := StableEndpointID("", "192.0.2.10")
	second := StableEndpointID("", " 192.0.2.10 ")
	if first == "" || first != second {
		t.Fatalf("StableEndpointID values = %q and %q", first, second)
	}
	if explicit := StableEndpointID("customer-id", "192.0.2.10"); explicit != "customer-id" {
		t.Fatalf("explicit endpoint ID = %q", explicit)
	}
}
