package client

import (
	"context"
	"testing"
)

// Crash recovery is explicit convergence, not a disguised Start. The test
// approves replacement of its VM, but never restart/recreation of other nodes.
func recoverVM(t *testing.T, ctx context.Context, lab *Session, name string) {
	t.Helper()
	plan, err := lab.Plan(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.DeployedLab || len(plan.DeletedNodes) != 0 {
		t.Fatalf("unexpected whole-lab recovery: %+v", plan)
	}
	for _, group := range [][]string{plan.AddedNodes, plan.RecreatedNodes, plan.RestartedNodes, plan.StartedNodes} {
		for _, node := range group {
			if node != name {
				t.Fatalf("recovery would mutate unrelated node %q: %+v", node, plan)
			}
		}
	}
	t.Logf("explicitly approved native VM recovery: %+v", plan)
	if err := lab.Apply(ctx, nil, plan, nil); err != nil {
		t.Fatal(err)
	}
}
