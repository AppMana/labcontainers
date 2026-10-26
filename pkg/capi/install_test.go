package capi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Cluster API's published components.yaml is a template, not a
// manifest. clusterctl substitutes its variables at install time; an
// operator runs clusterctl, so a lab that
// applies the file raw is not installing what an operator installs.
//
// Applied raw, the controller receives the literal
// "${CAPI_INSECURE_DIAGNOSTICS:=false}" as a flag value and
// crash-loops on ParseBool — which reads as Cluster API being broken
// rather than as the manifest being a template.
func TestClusterctlVariablesAreResolved(t *testing.T) {
	manifest := []byte(`args:
  - "--insecure-diagnostics=${CAPI_INSECURE_DIAGNOSTICS:=false}"
  - "--diagnostics-address=${CAPI_DIAGNOSTICS_ADDRESS:=:8443}"
  - "--feature-gates=MachinePool=${EXP_MACHINE_POOL:=true}"
`)
	got := string(SubstituteDefaults(manifest))

	if strings.Contains(got, "${") {
		t.Errorf("a variable survived substitution:\n%s", got)
	}
	for _, want := range []string{
		"--insecure-diagnostics=false",
		"--diagnostics-address=:8443",
		"MachinePool=true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from:\n%s", want, got)
		}
	}
}

// A variable with no default is left alone, so something genuinely
// unset fails loudly rather than silently becoming an empty string
// that means something else.
func TestAVariableWithNoDefaultIsLeftAlone(t *testing.T) {
	got := string(SubstituteDefaults([]byte("value: ${SOMETHING_REQUIRED}")))
	if !strings.Contains(got, "${SOMETHING_REQUIRED}") {
		t.Errorf("a variable with no default was substituted away: %s", got)
	}
}

// A release is downloaded once: the host has a route out, the lab may not,
// and a re-run must install the same bytes. A manifest naming no images is
// the wrong file, not one that needs nothing carried in.
func TestInstallFetchesOnceAndRejectsAManifestWithoutImages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte("apiVersion: v1\nkind: List\n"))
	}))
	defer server.Close()
	dir := t.TempDir()
	component := Component{Name: "empty", URL: server.URL}
	load := func(context.Context, string) error {
		t.Fatal("carried an image from an image-less manifest")
		return nil
	}
	for range 2 {
		err := Install(context.Background(), nil, dir, load, component)
		if err == nil || !strings.Contains(err.Error(), "names no images") {
			t.Fatalf("Install = %v, want the image-less manifest rejected", err)
		}
	}
	if requests != 1 {
		t.Fatalf("fetched %d times, want once", requests)
	}
}
