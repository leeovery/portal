package zmx

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/backend"
)

// Runner runs the zmx binary. Run trims surrounding whitespace from stdout;
// RunRaw returns it verbatim. A failed run is returned as a *CommandError.
type Runner interface {
	Run(cmd backend.Command) (string, error)
	RunRaw(cmd backend.Command) (string, error)
}

// ExecRunner runs each command as a child process with stdin from /dev/null.
type ExecRunner struct{}

func (ExecRunner) Run(cmd backend.Command) (string, error) {
	out, err := execute(cmd)
	return strings.TrimSpace(out), err
}

func (ExecRunner) RunRaw(cmd backend.Command) (string, error) {
	return execute(cmd)
}

func execute(c backend.Command) (string, error) {
	cmd := exec.Command(c.Argv[0], c.Argv[1:]...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	// Stdin and Stderr stay nil: stdin reads /dev/null, which Create relies
	// on, and exec captures stderr onto the *exec.ExitError.
	out, err := cmd.Output()
	if err != nil {
		return "", wrapCommandError(err, c.Argv[1:])
	}
	return string(out), nil
}

// CommandError is a failed zmx run, carrying the child's stderr, empty when
// none was captured.
type CommandError struct {
	// Args is the argv after the binary name.
	Args   []string
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

func wrapCommandError(err error, args []string) error {
	var stderr string
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		stderr = string(exitErr.Stderr)
	}
	return &CommandError{Args: args, Stderr: stderr, Err: err}
}
