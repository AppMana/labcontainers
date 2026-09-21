package bootstrap

import (
	"strings"
	"testing"
)

func TestValidateFormats(t *testing.T) {
	for _, format := range []string{CloudConfig, Ignition, PowerShell} {
		if err := (Data{Format: format, Value: []byte("value")}).Validate(); err != nil {
			t.Errorf("%s: %v", format, err)
		}
	}
	for _, d := range []Data{{Format: CloudConfig}, {Format: "unknown", Value: []byte("x")}} {
		if err := d.Validate(); err == nil {
			t.Errorf("accepted %#v", d)
		}
	}
}

func TestEC2LaunchUsesASCIIBase64Envelope(t *testing.T) {
	raw, err := (Data{Format: PowerShell, Value: []byte(`Write-Host "<&Ω"`)}).EC2Launch()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Ω") || !strings.Contains(string(raw), "FromBase64String") {
		t.Fatalf("bad envelope: %s", raw)
	}
}
