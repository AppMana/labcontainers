package engine

import (
	"context"
	"testing"
)

func TestProofIsolationRejectsPortPublishingAndIncompleteState(t *testing.T) {
	for _, inspection := range []string{
		`[{"HostConfig":{"NetworkMode":"none","PublishAllPorts":true},"NetworkSettings":{}}]`,
		`[{"HostConfig":{"NetworkMode":"none","PortBindings":{"80/tcp":[{"HostPort":"8080"}]}},"NetworkSettings":{}}]`,
		`[{"HostConfig":{"NetworkMode":"none"},"NetworkSettings":{"Ports":{"80/tcp":[{"HostPort":"8080"}]}}}]`,
		`[{"HostConfig":{"NetworkMode":"none"}}]`,
		`[{"HostConfig":{"NetworkMode":"none"},"NetworkSettings":{"Networks":{"none":null}}}]`,
	} {
		c := &Containerlab{Runner: &fakeRunner{result: Result{Stdout: []byte(inspection)}}}
		if err := c.ProofIsolation(context.Background(), "lab", []string{"node"}); err == nil {
			t.Fatalf("accepted unsafe or incomplete runtime state: %s", inspection)
		}
	}
}
