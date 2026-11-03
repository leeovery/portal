package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/logtest"
)

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
		if got := buf.String(); got != "" {
			t.Errorf("stderr = %q, want empty (no duplication)", got)
		}
	})

	t.Run("it suppresses stderr for an IsSilentExitError", func(t *testing.T) {
		buf := withSeams(t, func() error { return cmd.ErrDoctorUnhealthy })

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

	t.Run("it exits with an attach's code, silently", func(t *testing.T) {
		buf := withSeams(t, func() error {
			return fmt.Errorf("open: %w", &cmd.ExitCodeError{Code: 130})
		})

		code, panicked := run()

		if code != 130 {
			t.Errorf("code = %d, want the attach's 130", code)
		}
		if panicked {
			t.Error("panicked = true, want false")
		}
		if got := buf.String(); got != "" {
			t.Errorf("stderr = %q, want empty (silent exit)", got)
		}
	})

	t.Run("it recovers a panic to code 2 and skips Close", func(t *testing.T) {
		// Captures the recover block's ERROR marker so it does not reach the
		// default stderr handler.
		logtest.Install(t)
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
