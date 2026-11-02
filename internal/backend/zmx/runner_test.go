package zmx_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func sh(script string) backend.Command {
	return backend.Command{Argv: []string{"/bin/sh", "-c", script}}
}

func TestExecRunner(t *testing.T) {
	t.Run("it trims Run output", func(t *testing.T) {
		got, err := zmx.ExecRunner{}.Run(context.Background(), sh(`printf '  body \n\n'`))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got != "body" {
			t.Errorf("Run = %q, want %q", got, "body")
		}
	})

	t.Run("it returns RunRaw output verbatim", func(t *testing.T) {
		got, err := zmx.ExecRunner{}.RunRaw(context.Background(), sh(`printf '  \033[1mbody\033[0m \n\n'`))
		if err != nil {
			t.Fatalf("RunRaw: %v", err)
		}
		if want := "  \x1b[1mbody\x1b[0m \n\n"; got != want {
			t.Errorf("RunRaw = %q, want %q", got, want)
		}
	})

	t.Run("it runs with exactly the command's environment and directory", func(t *testing.T) {
		dir := t.TempDir()
		cmd := sh(`printf '%s|%s|%s' "$ONLY" "${HOME-unset}" "$(pwd -P)"`)
		cmd.Env = []string{"ONLY=this"}
		cmd.Dir = dir

		got, err := zmx.ExecRunner{}.Run(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if want := "this|unset|" + real; got != want {
			t.Errorf("Run = %q, want %q", got, want)
		}
	})

	t.Run("it gives the child stdin at EOF", func(t *testing.T) {
		got, err := zmx.ExecRunner{}.Run(context.Background(), sh(`cat; echo done`))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got != "done" {
			t.Errorf("Run = %q, want done", got)
		}
	})

	t.Run("it returns a failure as a CommandError carrying stderr and the argv", func(t *testing.T) {
		for _, run := range []func(context.Context, backend.Command) (string, error){zmx.ExecRunner{}.Run, zmx.ExecRunner{}.RunRaw} {
			out, err := run(context.Background(), sh(`echo partial; echo 'error: session "x" does not exist' >&2; exit 1`))
			ce, ok := errors.AsType[*zmx.CommandError](err)
			if !ok {
				t.Fatalf("error = %v, want a *CommandError", err)
			}
			if ce.Stderr != "error: session \"x\" does not exist\n" {
				t.Errorf("Stderr = %q", ce.Stderr)
			}
			if _, ok := errors.AsType[*exec.ExitError](err); !ok {
				t.Errorf("error %v does not unwrap to the *exec.ExitError", err)
			}
			if out != "" {
				t.Errorf("output = %q alongside the error, want empty", out)
			}
		}
	})

	t.Run("it keeps what the child wrote to stdout before failing", func(t *testing.T) {
		_, err := zmx.ExecRunner{}.Run(context.Background(), sh(`printf 'session "x" created\n'; exit 1`))
		ce, ok := errors.AsType[*zmx.CommandError](err)
		if !ok || ce.Stdout != "session \"x\" created\n" {
			t.Errorf("error = %#v, want a *CommandError carrying the stdout", err)
		}
	})

	t.Run("it stops a run when its context ends, failing with the context's cause", func(t *testing.T) {
		cause := errors.New("too slow")
		ctx, cancel := context.WithTimeoutCause(context.Background(), 100*time.Millisecond, cause)
		defer cancel()
		start := time.Now()

		_, err := zmx.ExecRunner{}.Run(ctx, sh(`echo started >&2; exec sleep 30`))

		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("Run took %s past a 100ms bound", took)
		}
		ce, ok := errors.AsType[*zmx.CommandError](err)
		if !ok || !errors.Is(err, cause) {
			t.Fatalf("error = %v, want a *CommandError wrapping the cause", err)
		}
		if ce.Stderr != "started\n" {
			t.Errorf("Stderr = %q, want what the child wrote before it was stopped", ce.Stderr)
		}
	})

	t.Run("it does not wait past its grace on a pipe a grandchild holds", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()

		_, err := zmx.ExecRunner{}.Run(ctx, sh(`sleep 3 & exec sleep 30`))

		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("Run took %s, want the pipe abandoned after its grace", took)
		}
		if err == nil {
			t.Error("Run = nil, want the timeout")
		}
	})

	t.Run("it returns a missing binary as a CommandError with no stderr", func(t *testing.T) {
		_, err := zmx.ExecRunner{}.Run(context.Background(), backend.Command{Argv: []string{filepath.Join(t.TempDir(), "no-zmx")}})
		ce, ok := errors.AsType[*zmx.CommandError](err)
		if !ok {
			t.Fatalf("error = %v, want a *CommandError", err)
		}
		if ce.Stderr != "" || !errors.Is(err, os.ErrNotExist) {
			t.Errorf("CommandError = %+v, want no stderr and the not-exist cause", ce)
		}
	})
}

func TestCommandError(t *testing.T) {
	t.Run("it renders the argv, exit status and stderr", func(t *testing.T) {
		_, err := zmx.ExecRunner{}.Run(context.Background(), backend.Command{Argv: []string{"/bin/sh", "-c", "echo 'error: boom' >&2; exit 3"}})

		got := err.Error()
		for _, part := range []string{"zmx -c echo 'error: boom' >&2; exit 3", "exit 3", "error: boom"} {
			if !strings.Contains(got, part) {
				t.Errorf("Error() = %q, want it to contain %q", got, part)
			}
		}
	})

	t.Run("it renders a cause that is not an exit", func(t *testing.T) {
		err := &zmx.CommandError{Args: []string{"list"}, Err: errors.New("fork failed")}
		if got, want := err.Error(), "zmx list: fork failed"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}
