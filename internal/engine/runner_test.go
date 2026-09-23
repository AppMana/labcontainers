package engine

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestExecCancellationBoundsInheritedPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Docker host shell regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (ExecRunner{}).Run(ctx, nil, "sh", "-c", "sleep 3 & wait")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("inherited pipes delayed cancellation: %s", elapsed)
	}
}
