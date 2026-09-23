package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Runner interface {
	Run(ctx context.Context, stdin io.Reader, argv ...string) (Result, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, stdin io.Reader, argv ...string) (Result, error) {
	if len(argv) == 0 {
		return Result{}, fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// A killed CLI can leave descendants holding its stdout/stderr pipes.
	// Do not let pipe draining defeat the caller's cancellation deadline.
	cmd.WaitDelay = time.Second
	cmd.Stdin = stdin
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	r := Result{Stdout: out.Bytes(), Stderr: errOut.Bytes()}
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		if _, ok := err.(*exec.ExitError); ok {
			return r, nil
		}
		return r, err
	}
	return r, nil
}

type CommandError struct {
	Argv   []string
	Result Result
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("%v exited %d: %s", e.Argv, e.Result.ExitCode, string(e.Result.Stderr))
}

func checked(ctx context.Context, r Runner, stdin io.Reader, argv ...string) (Result, error) {
	result, err := r.Run(ctx, stdin, argv...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, &CommandError{Argv: argv, Result: result}
	}
	return result, nil
}
