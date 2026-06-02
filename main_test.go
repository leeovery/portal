package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/log"
)

// withSeams installs test doubles for executeFunc/errOut and restores the
// originals via t.Cleanup. The main-package seams are package-level mutable
// state (mirroring the cmd-package DI idiom), so these tests must not use
// t.Parallel().
func withSeams(t *testing.T, execute func() error) *bytes.Buffer {
	t.Helper()

	origExecute := executeFunc
	origErrOut := errOut
	t.Cleanup(func() {
		executeFunc = origExecute
		errOut = origErrOut
	})

	buf := &bytes.Buffer{}
	executeFunc = execute
	errOut = buf
	return buf
}

func TestRun(t *testing.T) {
	t.Run("it returns code 0 and calls Close on a clean Execute", func(t *testing.T) {
		buf := withSeams(t, func() error { return nil })

		code, panicked := run()

		if code != 0 {
			t.Errorf("code = %d, want 0", code)
		}
		if panicked {
			t.Error("panicked = true, want false (Close must run)")
		}
		if got := buf.String(); got != "" {
			t.Errorf("stderr = %q, want empty", got)
		}
	})

	t.Run("it returns code 1 on an ordinary Execute error and prints it to stderr", func(t *testing.T) {
		buf := withSeams(t, func() error { return errors.New("boom") })

		code, panicked := run()

		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		if got := buf.String(); got != "boom\n" {
			t.Errorf("stderr = %q, want %q", got, "boom\n")
		}
	})

	t.Run("it returns code 2 on a UsageError", func(t *testing.T) {
		buf := withSeams(t, func() error { return cmd.NewUsageError("bad usage") })

		code, panicked := run()

		if code != 2 {
			t.Errorf("code = %d, want 2", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		// UsageError is not a silent-exit error: its message is printed.
		if got := buf.String(); got != "bad usage\n" {
			t.Errorf("stderr = %q, want %q", got, "bad usage\n")
		}
	})

	t.Run("it returns code 1 on a FatalError without duplicating stderr", func(t *testing.T) {
		buf := withSeams(t, func() error {
			return bootstrap.NewFatal("Portal failed to start tmux", errors.New("cause"))
		})

		code, panicked := run()

		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		// Execute already wrote the user message; main must not duplicate it.
		if got := buf.String(); got != "" {
			t.Errorf("stderr = %q, want empty (no duplication)", got)
		}
	})

	t.Run("it suppresses stderr for an IsSilentExitError", func(t *testing.T) {
		buf := withSeams(t, func() error { return cmd.ErrStatusUnhealthy })

		code, panicked := run()

		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		if got := buf.String(); got != "" {
			t.Errorf("stderr = %q, want empty (silent exit)", got)
		}
	})

	t.Run("it recovers a panic to code 2 and skips Close", func(t *testing.T) {
		// Capture the process: panic emission so the recover block's new ERROR
		// marker (Task 2-13) does not leak to the default stderr handler. The
		// code=2/panicked=true contract is unchanged.
		log.SetTestHandler(t, &captureHandler{})
		withSeams(t, func() error { panic("kaboom") })

		code, panicked := run()

		if code != 2 {
			t.Errorf("code = %d, want 2", code)
		}
		if !panicked {
			t.Error("panicked = false, want true (Close must be skipped)")
		}
	})
}
