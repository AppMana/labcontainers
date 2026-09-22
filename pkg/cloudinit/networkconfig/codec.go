package networkconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
)

func unmarshalOpenObject(data []byte, typed any) (map[string]any, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	known := map[string]json.RawMessage{}
	fields := reflect.TypeOf(typed).Elem()
	for i := 0; i < fields.NumField(); i++ {
		key := strings.Split(fields.Field(i).Tag.Get("json"), ",")[0]
		if key == "-" {
			continue
		}
		if value, ok := raw[key]; ok {
			known[key] = value
			delete(raw, key)
		}
	}
	encoded, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(encoded, typed); err != nil {
		return nil, err
	}
	extra := make(map[string]any, len(raw))
	for key, value := range raw {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		extra[key] = decoded
	}
	return extra, nil
}

// marshalOpenObject fills a gap in the upstream generator's JSON codec.
// Native schema types and their field tags remain entirely generated.
func marshalOpenObject(typed any, additional any) ([]byte, error) {
	raw, err := json.Marshal(typed)
	if err != nil || additional == nil {
		return raw, err
	}
	extra, ok := additional.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("additionalProperties must be a map[string]any")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	fields := reflect.TypeOf(typed)
	for key, value := range extra {
		for i := 0; i < fields.NumField(); i++ {
			field := fields.Field(i)
			if field.Name != "AdditionalProperties" && strings.Split(field.Tag.Get("json"), ",")[0] == key {
				return nil, fmt.Errorf("additional property %q conflicts with a generated field", key)
			}
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		object[key] = encoded
	}
	return json.Marshal(object)
}

// WriteFile serializes the schema-generated network object as JSON (accepted
// by cloud-init's YAML reader), without adding DHCP, DNS, gateways, or NICs.
// Bind this file to the VM launcher's native network-config input. Encoding
// happens before writing so serialization failures do not truncate the file.
func WriteFile(filename string, config *NetworkConfigVersion2) error {
	if config == nil {
		return fmt.Errorf("cloud-init network configuration is required")
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	// Exercise generated enum/required-field checks without replacing the
	// caller's object or re-encoding a default-populated object.
	var checked NetworkConfigVersion2
	if err := json.Unmarshal(raw, &checked); err != nil {
		return err
	}
	return os.WriteFile(filename, raw, 0o600)
}
