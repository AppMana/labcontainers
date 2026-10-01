package windows

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	labv1 "github.com/appmana/labcontainers/api/v1"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

type scriptedGuest struct {
	replies []*labv1.ExecResponse
	calls   [][]string
}

func (g *scriptedGuest) ExecWithTimeout(_ context.Context, _ time.Duration, args ...string) (*labv1.ExecResponse, error) {
	g.calls = append(g.calls, args)
	i := len(g.calls) - 1
	if i >= len(g.replies) {
		i = len(g.replies) - 1
	}
	return g.replies[i], nil
}
func reply(body string) *labv1.ExecResponse { return &labv1.ExecResponse{Stdout: []byte(body)} }
func TestFeaturesAlreadyInstalledDoesNotReboot(t *testing.T) {
	g := &scriptedGuest{replies: []*labv1.ExecResponse{reply(`{"installed":true,"rebootRequired":false,"bootId":"100"}`)}}
	if err := EnsureFeatures(context.Background(), g, FeatureOptions{Names: []string{"Containers"}}); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 1 {
		t.Fatalf("unexpected commands: %v", g.calls)
	}
}
func TestFeaturesRequireExplicitRebootAndChangedBootIdentity(t *testing.T) {
	first := reply(`{"installed":true,"rebootRequired":true,"bootId":"100"}`)
	g := &scriptedGuest{replies: []*labv1.ExecResponse{first}}
	if err := EnsureFeatures(context.Background(), g, FeatureOptions{Names: []string{"Containers"}}); err == nil || len(g.calls) != 1 {
		t.Fatal("implicit reboot accepted")
	}
	g = &scriptedGuest{replies: []*labv1.ExecResponse{first, reply(""), reply(`{"installed":true,"bootId":"100"}`), reply(`{"installed":false,"bootId":"101"}`), reply(`{"installed":true,"bootId":"101"}`)}}
	if err := EnsureFeatures(context.Background(), g, FeatureOptions{Names: []string{"Containers"}, AllowReboot: true, PollInterval: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 5 {
		t.Fatalf("accepted an old boot or missing feature: %d", len(g.calls))
	}
}
func TestFeatureErrorsDoNotBecomeSuccess(t *testing.T) {
	for _, response := range []*labv1.ExecResponse{{ExitCode: 1, Stderr: []byte("native failure")}, reply("bad JSON"), reply(`{"installed":true}`)} {
		g := &scriptedGuest{replies: []*labv1.ExecResponse{response}}
		if err := EnsureFeatures(context.Background(), g, FeatureOptions{Names: []string{"Containers"}, AllowReboot: true}); err == nil {
			t.Fatal("accepted invalid result")
		}
		if len(g.calls) != 1 {
			t.Fatal("retried installation failure")
		}
	}
	g := &scriptedGuest{}
	if err := EnsureFeatures(context.Background(), g, FeatureOptions{Names: []string{"Containers'; shutdown /r"}}); err == nil || len(g.calls) != 0 {
		t.Fatal("invalid feature accepted")
	}
}
func TestPowerShellCommandPreservesExactScript(t *testing.T) {
	script := "& tool.exe @('-test.run','A|B'); $x='quote'"
	args := PowerShellCommand(script)
	if len(args) != 5 || args[3] != "-EncodedCommand" {
		t.Fatal(args)
	}
	body, err := base64.StdEncoding.DecodeString(args[4])
	if err != nil {
		t.Fatal(err)
	}
	words := make([]uint16, len(body)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(body[i*2:])
	}
	if !strings.Contains(string(utf16.Decode(words)), script) {
		t.Fatal("script altered")
	}
}
