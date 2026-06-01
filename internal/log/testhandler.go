package log

import (
	"log/slog"
	"testing"
)

// SetTestHandler swaps h into the shared handler indirection for the duration
// of the test t and registers a t.Cleanup that restores the handler that was
// pinned at call time. After this call, every For-created logger — including
// those cached at package init — routes its records to h, so a test can capture
// or silence log output in-process without spawning a subprocess.
//
// This is the ONLY sanctioned way to replace the handler outside Init.
// Production code must never call it: the *testing.T-first parameter
// structurally marks it test-only (it cannot be referenced from non-test code),
// mirroring portaltest.IsolateStateForTest.
//
// Nested swaps restore in LIFO order. Each call captures the handler present at
// its call time, and t.Cleanup runs in reverse registration order, so inner
// swaps unwind before outer ones and the original handler is restored last.
// Restoration is unconditional and never depends on a record having been
// emitted — a test that swaps but never logs still restores cleanly.
func SetTestHandler(t *testing.T, h slog.Handler) {
	t.Helper()
	prev := currentHandler()
	setHandler(h)
	t.Cleanup(func() { setHandler(prev) })
}
