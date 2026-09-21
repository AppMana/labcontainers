package capi

import "testing"

func TestInfraValuesPreferInternalAddressAndCarryIdentity(t *testing.T) {
	got := InfraValues(map[string]any{
		"spec": map[string]any{"providerID": "labcontainers://s/n/u"},
		"status": map[string]any{"addresses": []any{
			map[string]any{"type": "ExternalIP", "address": "192.0.2.1"},
			map[string]any{"type": "InternalIP", "address": "10.0.0.1"},
		}},
	})
	if got["nodeAddress"] != "10.0.0.1" || got["providerID"] != "labcontainers://s/n/u" {
		t.Fatalf("got %#v", got)
	}
}

func TestProviderIDIsUIDFenced(t *testing.T) {
	if got := ProviderID("lab", "worker", "uid"); got != "labcontainers://lab/worker/uid" {
		t.Fatal(got)
	}
}
