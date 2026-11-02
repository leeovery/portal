package zmx

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

// Runner runs the zmx binary until ctx ends. Run trims surrounding whitespace
// from stdout; RunRaw returns it verbatim. A failed run is returned as a
// *CommandError; one cut short by ctx carries context.Cause(ctx) as its Err.
type Runner interface {
	Run(ctx context.Context, cmd backend.Command) (string, error)
	RunRaw(ctx context.Context, cmd backend.Command) (string, error)
}

// ExecRunner runs each command as a child process with stdin from /dev/null.
type ExecRunner struct{}

// pipeGrace bounds the wait for stdout to close after the child is killed, in
// case something it started still holds the pipe.
const pipeGrace = time.Second

func (ExecRunner) Run(ctx context.Context, cmd backend.Command) (string, error) {
	out, err := execute(ctx, cmd)
	return strings.TrimSpace(out), err
}

func (ExecRunner) RunRaw(ctx context.Context, cmd backend.Command) (string, error) {
	return execute(ctx, cmd)
}

func execute(ctx context.Context, c backend.Command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.WaitDelay = pipeGrace
	// Stdin and Stderr stay nil: stdin reads /dev/null, which Create relies
	// on, and exec captures stderr onto the *exec.ExitError.
	out, err := cmd.Output()
	if err != nil {
		ce := newCommandError(err, c.Argv[1:], string(out))
		if ctx.Err() != nil {
			ce.Err = context.Cause(ctx)
		}
		return "", ce
	}
	return string(out), nil
}

// CommandError is a failed zmx run, carrying what the child wrote before it
// failed. Stderr is empty when none was captured.
type CommandError struct {
	// Args is the argv after the binary name.
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

// Error's rendering is not part of the contract: discriminate on Stderr, or
// recover the value with errors.As.
func (e *CommandError) Error() string {
	var b strings.Builder
	b.WriteString("zmx")
	for _, arg := range e.Args {
		b.WriteString(" ")
		b.WriteString(arg)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](e.Err); ok {
		b.WriteString(": exit ")
		b.WriteString(strconv.Itoa(exitErr.ExitCode()))
	} else if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	if stderr := strings.TrimSpace(e.Stderr); stderr != "" {
		b.WriteString(": ")
		b.WriteString(stderr)
	}
	return b.String()
}

func (e *CommandError) Unwrap() error { return e.Err }

func newCommandError(err error, args []string, stdout string) *CommandError {
	var stderr string
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		stderr = string(exitErr.Stderr)
	}
	return &CommandError{Args: args, Stdout: stdout, Stderr: stderr, Err: err}
}
