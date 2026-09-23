package kube

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/appmana/labcontainers/pkg/rig"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

type writeNode struct {
	rig.Node
	data     []byte
	filename string
	mode     fs.FileMode
	err      error
}

func (n *writeNode) Put(_ context.Context, src io.Reader, filename string, mode fs.FileMode) error {
	n.filename, n.mode = filename, mode
	n.data, _ = io.ReadAll(src)
	return n.err
}

func TestWriteObjectsPreservesDocumentSequence(t *testing.T) {
	objects := []runtime.Object{
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "example/v1", "kind": "First", "spec": map[string]any{"opaque": "a: b\n---\nnot a new document"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "example/v2", "kind": "Second", "enabled": false}},
	}
	before := []runtime.Object{objects[0].DeepCopyObject(), objects[1].DeepCopyObject()}
	n := &writeNode{}
	if err := WriteObjects(context.Background(), n, "/etc/native/config.yaml", 0o600, objects...); err != nil {
		t.Fatal(err)
	}
	if n.filename != "/etc/native/config.yaml" || n.mode != 0o600 {
		t.Fatal("destination or mode changed")
	}
	decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(string(n.data)), 4096)
	for _, want := range objects {
		var got unstructured.Unstructured
		if err := decoder.Decode(&got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&got, want) {
			t.Fatalf("wire fields changed: %#v", got.Object)
		}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("unexpected additional document: %v", err)
	}
	if !reflect.DeepEqual(objects, before) {
		t.Fatal("caller mutated")
	}
}

func TestWriteValidationIsNonMutating(t *testing.T) {
	valid := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod"}}
	for _, objects := range [][]runtime.Object{nil, {nil}, {(*unstructured.Unstructured)(nil)}, {valid, &unstructured.Unstructured{}}} {
		n := &writeNode{}
		if err := WriteObjects(context.Background(), n, "/etc/config", 0o600, objects...); err == nil || n.filename != "" {
			t.Fatal("invalid objects reached node")
		}
	}
	for _, filename := range []string{"", "relative", "/"} {
		n := &writeNode{}
		if err := WriteObjects(context.Background(), n, filename, 0o600, valid); err == nil || n.filename != "" {
			t.Fatal("invalid path reached node")
		}
	}
	failure := errors.New("upload failed")
	if err := WriteObjects(context.Background(), &writeNode{err: failure}, "/etc/config", 0o600, valid); !errors.Is(err, failure) {
		t.Fatal("lost upload failure", err)
	}
}
