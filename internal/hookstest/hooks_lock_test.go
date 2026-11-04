package hookstest_test

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// emitLockWarn writes the line a mutation that could not take the sidecar
// leaves, so a case below can vary one part of it and nothing else.
func emitLockWarn(t *testing.T, extra ...any) *logtest.Sink {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	attrs := []any{
		"op", "set",
		"component", "hooks",
		"hook_key", hookstest.SubjectSeedA,
		"via", "cli",
		"error", errors.New("hooks.json.lock: lock held"),
	}
	logger.Warn("set", append(attrs, extra...)...)
	return sink
}

func TestAssertLockWarn(t *testing.T) {
	t.Run("it passes for the WARN a timed-out mutation leaves", func(t *testing.T) {
		sink := emitLockWarn(t)

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertLockWarn(rec, sink, "set", hookstest.SubjectSeedA, "cli") })

		if rec.Failed() {
			t.Errorf("the lock WARN reported a failure: %s", rec.Report())
		}
	})

	t.Run("it fails when the WARN carries an error_class", func(t *testing.T) {
		sink := emitLockWarn(t, "error_class", "rename")

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertLockWarn(rec, sink, "set", hookstest.SubjectSeedA, "cli") })

		if len(rec.Errors) != 1 {
			t.Fatalf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
		if !strings.Contains(rec.Errors[0], "error_class") {
			t.Errorf("failure message %q does not name the attr no write phase could have set", rec.Errors[0])
		}
	})

	t.Run("it fails when the WARN carries a value", func(t *testing.T) {
		sink := emitLockWarn(t, "value", "npm start")

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertLockWarn(rec, sink, "set", hookstest.SubjectSeedA, "cli") })

		if len(rec.Errors) != 1 {
			t.Fatalf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
		if !strings.Contains(rec.Errors[0], "value") {
			t.Errorf("failure message %q does not name the attr a never-opened file could not carry", rec.Errors[0])
		}
	})

	t.Run("it fails when the hook_key differs", func(t *testing.T) {
		sink := emitLockWarn(t)

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertLockWarn(rec, sink, "set", hookstest.SubjectSeedB, "cli") })

		if len(rec.Errors) != 1 {
			t.Fatalf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
		if !strings.Contains(rec.Errors[0], "hook_key") {
			t.Errorf("failure message %q does not name the key it could not match", rec.Errors[0])
		}
	})

	t.Run("it fails when the error attr is empty", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		logger.Warn("rm",
			"op", "rm",
			"component", "hooks",
			"hook_key", hookstest.SubjectSeedA,
			"via", "internal",
			"error", errors.New(""))

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertLockWarn(rec, sink, "rm", hookstest.SubjectSeedA, "internal") })

		if len(rec.Errors) != 1 {
			t.Fatalf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
	})

	t.Run("it fails when a second record joins the WARN at or above its level", func(t *testing.T) {
		sink := emitLockWarn(t)
		logger := slog.New(sink)
		logger.Error("save", "op", "set", "component", "hooks")

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertLockWarn(rec, sink, "set", hookstest.SubjectSeedA, "cli") })

		if len(rec.Fatals) != 1 {
			t.Fatalf("got %d fatals, want exactly 1: %v", len(rec.Fatals), rec.Fatals)
		}
	})
}

func TestAssertSidecarHeld(t *testing.T) {
	t.Run("it passes while another description holds the sidecar exclusively", func(t *testing.T) {
		_, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
		hookstest.HoldHooksSidecar(t, path)

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertSidecarHeld(rec, path) })

		if rec.Failed() {
			t.Errorf("a held sidecar reported a failure: %s", rec.Report())
		}
	})

	t.Run("it fails when nothing holds the sidecar", func(t *testing.T) {
		_, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		rec := &harnesstest.Recorder{}
		rec.Run(func() { hookstest.AssertSidecarHeld(rec, path) })

		if len(rec.Errors) != 1 {
			t.Errorf("got %d errors, want exactly 1: %v", len(rec.Errors), rec.Errors)
		}
	})
}
