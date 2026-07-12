// Package spawntest provides the primary DI seam for unit-testing Portal's
// host-terminal spawn pipeline without touching a real terminal.
//
// FakeAdapter satisfies spawn.Adapter: it records the exact composed argv each
// OpenWindow call is handed (a defensive copy, in call order) and returns a
// scripted spawn.Result per call — so a test can drive the whole spawn pipeline
// ("would open a window running command X", "second window fails") with
// fabricated inputs, opening no real window and running no osascript.
//
// Test-only: production code MUST NOT import this package (mirroring the
// precedent for transienttest / restoretest / portalbintest). Enforcement is
// contributor discipline plus the package's confinement to test consumers.
package spawntest

import (
	"slices"
	"sync"

	"github.com/leeovery/portal/internal/spawn"
)

// FakeAdapter is a test double for spawn.Adapter. It records every OpenWindow
// argv in Calls (in call order) and replays scripted Results.
//
// Set Results to script per-call outcomes: call i returns Results[i], and once
// Results is exhausted (or empty) every further call defaults to
// spawn.Success(""). This lets a test express "second window fails" as
// Results = []spawn.Result{spawn.Success(""), spawn.SpawnFailed("…")}.
//
// The mutex guards the recording so the fake is safe to use even though the
// production spawn pipeline calls OpenWindow sequentially. FakeAdapter must be
// used by pointer (the mutex makes it non-copyable once used).
type FakeAdapter struct {
	// Calls holds a defensive copy of each argv OpenWindow was handed, in
	// call order.
	Calls [][]string
	// Results scripts the outcome of each call: call i returns Results[i].
	Results []spawn.Result

	mu sync.Mutex
}

// OpenWindow records a defensive copy of command into Calls, then returns the
// scripted Result for this call index — defaulting to spawn.Success("") once
// Results is exhausted or empty. It satisfies spawn.Adapter.
func (f *FakeAdapter) OpenWindow(command []string) spawn.Result {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, slices.Clone(command))

	i := len(f.Calls) - 1
	if i < len(f.Results) {
		return f.Results[i]
	}
	return spawn.Success("")
}

// Compile-time guard: FakeAdapter must satisfy spawn.Adapter.
var _ spawn.Adapter = (*FakeAdapter)(nil)
