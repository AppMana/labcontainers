package engine

import (
	"encoding/json"
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestEndpointParkingRejectsForeignOrStoppedContainer(t *testing.T) {
	const fixture = `{"Id":"exact-id","Name":"/clab-session-windows","State":{"Running":true},"Config":{"Labels":{"clab-topo-file":"/owned/topology.yml","clab-node-name":"windows","labcontainers.appmana.com/managed":"true"}}}`
	for _, change := range []string{"valid", "id", "stopped", "topology", "node", "unmanaged", "missing-base", "missing-state", "missing-config"} {
		t.Run(change, func(t *testing.T) {
			var target container.InspectResponse
			if err := json.Unmarshal([]byte(fixture), &target); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "id":
				target.ID = "other-id"
			case "stopped":
				target.State.Running = false
			case "topology":
				target.Config.Labels["clab-topo-file"] = "/foreign/topology.yml"
			case "node":
				target.Config.Labels["clab-node-name"] = "linux"
			case "unmanaged":
				delete(target.Config.Labels, "labcontainers.appmana.com/managed")
			case "missing-base":
				target.ContainerJSONBase = nil
			case "missing-state":
				target.State = nil
			case "missing-config":
				target.Config = nil
			}
			err := validateEndpointTarget(target, "/owned/topology.yml", "windows", "exact-id")
			if (err == nil) != (change == "valid") {
				t.Fatalf("scope validation: %v", err)
			}
		})
	}
}
