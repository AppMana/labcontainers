package kube

import (
	"context"
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNativeObjectsUseDeclaredBastionAndPreserveFields(t *testing.T) {
	node := &fakeNode{}
	c := &Client{Bastion: node, ControlPlanes: []string{"192.0.2.10"}}
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.appmana.com/v1", "kind": "Scenario",
		"metadata": map[string]any{"name": "network-test"},
		"spec":     map[string]any{"opaqueField": []any{"keep", "unchanged"}},
	}}
	before, err := object.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyObjects(context.Background(), object); err != nil {
		t.Fatal(err)
	}
	var list metav1.List
	if err := json.Unmarshal([]byte(node.stdin), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Kind != "List" {
		t.Fatalf("native list=%+v", list)
	}
	var decoded unstructured.Unstructured
	if err := decoded.UnmarshalJSON(list.Items[0].Raw); err != nil {
		t.Fatal(err)
	}
	after, _ := decoded.MarshalJSON()
	if string(before) != string(after) {
		t.Fatalf("resource fields changed: %s", after)
	}
	unchanged, _ := object.MarshalJSON()
	if string(before) != string(unchanged) {
		t.Fatal("caller object changed")
	}
}

func TestInvalidObjectsFailBeforeAnyNetworkProbe(t *testing.T) {
	for _, objects := range [][]runtime.Object{nil, {nil}, {(*unstructured.Unstructured)(nil)}, {&unstructured.Unstructured{}}} {
		node := &fakeNode{}
		c := &Client{Bastion: node, ControlPlanes: []string{"192.0.2.10"}}
		if err := c.ApplyObjects(context.Background(), objects...); err == nil {
			t.Fatal("invalid objects accepted")
		}
		if len(node.calls) != 0 {
			t.Fatal("invalid request reached bastion")
		}
	}
}
