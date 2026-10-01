package windows

import (
	"context"
	"testing"

	labv1 "github.com/appmana/labcontainers/api/v1"
)

func TestRecoveryRequiresSuccessfulPolicyReadback(t *testing.T) {
	for _, result := range []*labv1.ExecResponse{
		reply("UNATTENDED_RECOVERY_CONFIGURED"), reply(""), {ExitCode: 1, Stdout: []byte("UNATTENDED_RECOVERY_CONFIGURED")}, nil,
	} {
		g := &scriptedGuest{replies: []*labv1.ExecResponse{result}}
		err := ConfigureUnattendedRecovery(context.Background(), g)
		want := result != nil && result.ExitCode == 0 && len(result.Stdout) > 0
		if (err == nil) != want {
			t.Fatalf("unexpected success for %+v: %v", result, err)
		}
		if len(g.calls) != 1 {
			t.Fatal("policy failure was retried")
		}
	}
}
