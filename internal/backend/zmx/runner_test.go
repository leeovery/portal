package zmx_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func sh(script string) backend.Command {
	return backend.Command{Argv: []string{"/bin/sh", "-c", script}}
}

func TestExecRunner(t *testing.T) {
	t.Run("it trims Run output", func(t *testing.T) {
		got, err := zmx.ExecRunner{}.Run(sh(`printf '  body \n\n'`))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got != "body" {
			t.Errorf("Run = %q, want %q", got, "body")
		}
	})

	t.Run("it returns RunRaw output verbatim", func(t *testing.T) {
		got, err := zmx.ExecRunner{}.RunRaw(sh(`printf '  \033[1mbody\033[0m \n\n'`))
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

		got, err := zmx.ExecRunner{}.Run(cmd)
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
		got, err := zmx.ExecRunner{}.Run(sh(`cat; echo done`))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got != "done" {
			t.Errorf("Run = %q, want done", got)
		}
	})

	t.Run("it returns a failure as a CommandError carrying stderr and the argv", func(t *testing.T) {
		for _, run := range []func(backend.Command) (string, error){zmx.ExecRunner{}.Run, zmx.ExecRunner{}.RunRaw} {
			out, err := run(sh(`echo partial; echo 'error: session "x" does not exist' >&2; exit 1`))
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

	t.Run("it returns a missing binary as a CommandError with no stderr", func(t *testing.T) {
		_, err := zmx.ExecRunner{}.Run(backend.Command{Argv: []string{filepath.Join(t.TempDir(), "no-zmx")}})
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
		_, err := zmx.ExecRunner{}.Run(backend.Command{Argv: []string{"/bin/sh", "-c", "echo 'error: boom' >&2; exit 3"}})

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
