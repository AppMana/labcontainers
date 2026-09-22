package kube

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ApplyObjects sends native Kubernetes objects through the declared bastion.
// It does not connect the host to the cluster, invent resource models, write
// manifest files, or modify caller objects. Supply TypeMeta/GVK explicitly,
// just as required by the Kubernetes wire API, including for unstructured CRDs.
func (c *Client) ApplyObjects(ctx context.Context, objects ...runtime.Object) error {
	if len(objects) == 0 {
		return fmt.Errorf("at least one Kubernetes object is required")
	}
	list := &metav1.List{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "List"}}
	for i, object := range objects {
		if object == nil {
			return fmt.Errorf("Kubernetes object %d is nil", i)
		}
		copy := object.DeepCopyObject()
		if copy == nil {
			return fmt.Errorf("Kubernetes object %d is nil", i)
		}
		gvk := copy.GetObjectKind().GroupVersionKind()
		if gvk.Kind == "" || gvk.Version == "" {
			return fmt.Errorf("Kubernetes object %d requires native apiVersion and kind", i)
		}
		list.Items = append(list.Items, runtime.RawExtension{Object: copy})
	}
	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("encode native Kubernetes objects: %w", err)
	}
	return c.Apply(ctx, data)
}
