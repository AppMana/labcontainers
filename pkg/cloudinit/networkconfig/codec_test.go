package networkconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGeneratedObjectsPreserveNativeNetworkChoices(t *testing.T) {
	name, optional := "en*", true
	config := &NetworkConfigVersion2{Version: 2, Ethernets: NetworkConfigVersion2Ethernets{
		"data": {Match: &MappingPhysicalMatch{Name: &name}, Addresses: []string{"192.0.2.10/24"}, Optional: &optional,
			Dhcp4: &MappingDhcp4{Value: false}, Dhcp6: &MappingDhcp6{Value: "no"},
			AdditionalProperties: map[string]any{"accept-ra": false, "vendor-option": map[string]any{"id": json.Number("9007199254740993")}},
		},
	}}
	filename := filepath.Join(t.TempDir(), "network.json")
	if err := WriteFile(filename, config); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	node := document["ethernets"].(map[string]any)["data"].(map[string]any)
	if node["dhcp4"] != false || node["dhcp6"] != "no" || node["accept-ra"] != false {
		t.Fatalf("changed native scalars: %s", raw)
	}
	for _, key := range []string{"gateway4", "gateway6", "nameservers", "routes", "AdditionalProperties"} {
		if _, ok := node[key]; ok {
			t.Fatalf("invented field %s: %s", key, raw)
		}
	}
	var decoded NetworkConfigVersion2
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Ethernets["data"], decoded.Ethernets["data"]) {
		t.Fatalf("lost native fields during roundtrip: %+v", decoded.Ethernets["data"])
	}
	info, err := os.Stat(filename)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("unsafe file mode: %v %v", info, err)
	}
}

func TestAbsentOptionsDoNotAppear(t *testing.T) {
	raw, err := json.Marshal(&NetworkConfigVersion2{Version: 2, Ethernets: NetworkConfigVersion2Ethernets{"data": {Addresses: []string{"192.0.2.1/24"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var actual any
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"version": float64(2), "ethernets": map[string]any{"data": map[string]any{"addresses": []any{"192.0.2.1/24"}}}}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("inserted network defaults: %s", raw)
	}
}

func TestOpenKeysAreNotGoFieldNames(t *testing.T) {
	original := MappingPhysical{AdditionalProperties: map[string]any{"AdditionalProperties": "native-key", "Addresses": "not-the-lowercase-schema-key", "-": true}}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded MappingPhysical
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("lost open keys: %#v", decoded)
	}
}

func TestAdditionalPropertiesCannotOverrideGeneratedFields(t *testing.T) {
	_, err := json.Marshal(MappingPhysical{AdditionalProperties: map[string]any{"dhcp4": true}})
	if err == nil {
		t.Fatal("extra fields overrode typed field")
	}
}

func TestInvalidGeneratedValuesDoNotTruncateFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(filename, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, config := range []*NetworkConfigVersion2{nil, {}, {Version: 2, Ethernets: NetworkConfigVersion2Ethernets{"data": {Dhcp4: &MappingDhcp4{Value: "unsupported"}}}}} {
		if err := WriteFile(filename, config); err == nil {
			t.Fatal("invalid generated config accepted")
		}
		if raw, err := os.ReadFile(filename); err != nil || string(raw) != "original" {
			t.Fatal("invalid input truncated existing file")
		}
	}
}
