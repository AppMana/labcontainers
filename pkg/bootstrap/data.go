// Package bootstrap defines first-boot data independently of cloud, OS, or cluster distribution.
package bootstrap

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

const (
	CloudConfig = "cloud-config"
	Ignition    = "ignition"
	PowerShell  = "powershell"
)

// Data is the CAPI bootstrap Secret contract. Value is raw, never transport-encoded.
type Data struct {
	Format string
	Value  []byte
}

func (d Data) Validate() error {
	if len(bytes.TrimSpace(d.Value)) == 0 {
		return fmt.Errorf("empty bootstrap data")
	}
	switch d.Format {
	case CloudConfig, Ignition, PowerShell:
		return nil
	default:
		return fmt.Errorf("unsupported bootstrap format %q", d.Format)
	}
}

// EC2Launch renders the uncompressed, once-only Windows EC2Launch envelope.
func (d Data) EC2Launch() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if d.Format != PowerShell {
		return nil, fmt.Errorf("EC2Launch requires powershell, got %q", d.Format)
	}
	payload := base64.StdEncoding.EncodeToString(d.Value)
	return []byte("<powershell>\n$ErrorActionPreference = 'Stop'\n" +
		"Invoke-Expression ([System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('" + payload + "')))\n" +
		"</powershell>\n<persist>false</persist>\n"), nil
}
