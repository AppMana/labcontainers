// Package capi defines Labcontainers' clean Cluster API infrastructure contract.
package capi

import "fmt"

const (
	Group   = "infrastructure.labcontainers.appmana.com"
	Version = "v1alpha1"

	ClusterKind              = "LabCluster"
	MachineKind              = "LabMachine"
	MachineTemplateKind      = "LabMachineTemplate"
	ImportedControlPlaneKind = "LabImportedControlPlane"

	NodeNameAnnotation = Group + "/node-name"
	ProviderScheme     = "labcontainers://"
)

type GVK struct{ Group, Version, Kind string }

func ClusterGVK() GVK { return GVK{Group: Group, Version: Version, Kind: ClusterKind} }
func MachineGVK() GVK { return GVK{Group: Group, Version: Version, Kind: MachineKind} }

func ProviderID(session, node, uid string) string {
	id := ProviderScheme + session + "/" + node
	if uid != "" {
		id += "/" + uid
	}
	return id
}

// InfraValues returns the provider-owned values a bootstrap renderer may safely consume.
func InfraValues(machine map[string]any) map[string]any {
	values := map[string]any{}
	if addr := reportedAddress(machine); addr != "" {
		values["nodeAddress"] = addr
	}
	if id, ok := nestedString(machine, "spec", "providerID"); ok && id != "" {
		values["providerID"] = id
	}
	return values
}

func reportedAddress(machine map[string]any) string {
	status, _ := machine["status"].(map[string]any)
	addresses, _ := status["addresses"].([]any)
	var fallback string
	for _, raw := range addresses {
		item, _ := raw.(map[string]any)
		address, _ := item["address"].(string)
		if address == "" {
			continue
		}
		if item["type"] == "InternalIP" {
			return address
		}
		if fallback == "" {
			fallback = address
		}
	}
	return fallback
}

func nestedString(object map[string]any, fields ...string) (string, bool) {
	var current any = object
	for _, field := range fields {
		m, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = m[field]
		if !ok {
			return "", false
		}
	}
	v, ok := current.(string)
	return v, ok
}

func ValidateNodeName(name string) error {
	if name == "" {
		return fmt.Errorf("nodeName is required")
	}
	return nil
}
