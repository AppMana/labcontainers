// Package artifact verifies caller-prepared files. It never downloads, builds,
// caches, or selects a release. It is independent of Kubernetes and VM APIs.
package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// ReadFile returns the exact nonempty bytes matching expectedSHA256 (64 hex
// digits). Callers upload these returned bytes, rather than reopening the file,
// so a later file replacement cannot change the verified payload. A digest
// establishes content identity, not publisher authenticity or version alignment.
func ReadFile(ctx context.Context, filename, expectedSHA256 string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pin, err := hex.DecodeString(expectedSHA256)
	if filename == "" || err != nil || len(pin) != sha256.Size {
		return nil, fmt.Errorf("prepared artifact path and 64-digit SHA256 are required; no artifact is downloaded")
	}
	// Reject non-files before opening, in particular a caller-supplied FIFO
	// that would otherwise wait indefinitely for a writer. Inspect the opened
	// descriptor again below in case the path was replaced.
	info, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("stat prepared artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("prepared artifact must be a regular file: %s", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open prepared artifact: %w", err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("prepared artifact must be a regular file: %s", filename)
	}
	// Read through the same descriptor that was inspected. Symlinks to regular
	// files are supported; digest verification applies to the resolved bytes.
	body, err := read(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("read prepared artifact: %w", err)
	}
	sum := sha256.Sum256(body)
	if len(body) == 0 || !bytes.Equal(sum[:], pin) {
		return nil, fmt.Errorf("prepared artifact is empty or does not match SHA256 %s", expectedSHA256)
	}
	return body, nil
}

func read(ctx context.Context, f *os.File) ([]byte, error) {
	var body bytes.Buffer
	chunk := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := f.Read(chunk)
		body.Write(chunk[:n])
		if err == io.EOF {
			return body.Bytes(), ctx.Err()
		}
		if err != nil {
			return nil, err
		}
	}
}
