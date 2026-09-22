package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	"github.com/appmana/labcontainers/pkg/rig"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
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
		copy, err := copyObject(i, object)
		if err != nil {
			return err
		}
		list.Items = append(list.Items, runtime.RawExtension{Object: copy})
	}
	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("encode native Kubernetes objects: %w", err)
	}
	return c.Apply(ctx, data)
}

// WriteObjects writes native objects as a YAML document stream to an explicit
// guest path, for consumers such as kubeadm and kubelet's static-pod directory.
// It does not apply objects to an API, create directories, infer GVKs, or add
// defaults. All objects are validated and serialized before the node is touched.
func WriteObjects(ctx context.Context, node rig.Node, filename string, mode fs.FileMode, objects ...runtime.Object) error {
	if !path.IsAbs(filename) || path.Clean(filename) == "/" {
		return fmt.Errorf("Kubernetes object destination must be an absolute file path")
	}
	if len(objects) == 0 {
		return fmt.Errorf("at least one Kubernetes object is required")
	}
	var data bytes.Buffer
	for i, object := range objects {
		copy, err := copyObject(i, object)
		if err != nil {
			return err
		}
		encoded, err := yaml.Marshal(copy)
		if err != nil {
			return fmt.Errorf("encode Kubernetes object %d: %w", i, err)
		}
		if i > 0 {
			data.WriteString("---\n")
		}
		data.Write(encoded)
	}
	return node.Put(ctx, &data, filename, mode)
}

func copyObject(index int, object runtime.Object) (runtime.Object, error) {
	if object == nil {
		return nil, fmt.Errorf("Kubernetes object %d is nil", index)
	}
	copy := object.DeepCopyObject()
	if copy == nil {
		return nil, fmt.Errorf("Kubernetes object %d is nil", index)
	}
	gvk := copy.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" || gvk.Version == "" {
		return nil, fmt.Errorf("Kubernetes object %d requires native apiVersion and kind", index)
	}
	return copy, nil
}
