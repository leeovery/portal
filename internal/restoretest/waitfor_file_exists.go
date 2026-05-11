package restoretest

import (
	"os"
	"testing"
	"time"
)

// fataller is the minimal subset of testing.TB that WaitForFileExists
// needs. *testing.T satisfies it. Tests can pass an in-memory fake
// to exercise the timeout branch without aborting the test process.
type fataller interface {
	Helper()
	Name() string
	Fatalf(format string, args ...any)
}

// WaitForFileExists polls os.Stat(path) every tick until the file
// exists or budget elapses. On timeout it calls t.Fatalf with a
// diagnostic that includes the supplied path and the elapsed budget
// so the failing test surfaces both the missing artifact and the
// poll window that was honoured.
//
// tick is mandatory — original call sites disagreed on cadence
// (one parameterised, one hardcoded 50ms) so the consolidated
// helper requires the caller to be explicit.
//
// Callers wanting a context-specific failure message should wrap
// this helper with their own t.Fatalf at the call site rather than
// extending this signature.
func WaitForFileExists(t *testing.T, path string, budget, tick time.Duration) {
	t.Helper()
	waitForFileExists(t, path, budget, tick)
}

// waitForFileExists is the unexported core, taking the fataller
// interface so unit tests can drive the timeout path with a fake
// without the test process aborting via real t.Fatalf.
func waitForFileExists(t fataller, path string, budget, tick time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(tick)
	}
	t.Fatalf("WaitForFileExists(%s): %s did not appear within %v", t.Name(), path, budget)
}
