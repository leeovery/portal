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
// Set Ack (plus, optionally, Confirm) to simulate the spawned window's marker
// write: on a success Result for window i whose Confirm[i] is true (a nil Confirm
// slice — and any index beyond it — confirms), the fake parses
// --ack <batch>:<token> out of the argv it was handed and calls
// Ack.Write(batch, token). A false Confirm[i] writes nothing, so the burster
// times that window out. Leave Ack nil to record calls only.
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
	// Ack, when set, receives the parsed token of each confirmed success call,
	// simulating the spawned `portal open`'s @portal-spawn marker write.
	Ack *FakeAckChannel
	// Confirm gates the marker write per window: Confirm[i] false suppresses
	// window i's write (→ ack timeout). A nil slice confirms every window.
	Confirm []bool

	mu sync.Mutex
}

// OpenWindow records a defensive copy of command into Calls, returns the scripted
// Result for this call index (defaulting to spawn.Success("") once Results is
// exhausted or empty), and — on a confirmed success — writes the argv's parsed
// token to Ack. It satisfies spawn.Adapter.
func (f *FakeAdapter) OpenWindow(command []string) spawn.Result {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, slices.Clone(command))

	i := len(f.Calls) - 1
	result := spawn.Success("")
	if i < len(f.Results) {
		result = f.Results[i]
	}
	if result.OK() && f.Ack != nil && f.confirmed(i) {
		if batch, token, ok := parseSpawnAck(command); ok {
			_ = f.Ack.Write(batch, token)
		}
	}
	return result
}

// confirmed reports whether window i's token marker should be written. A nil
// Confirm slice (and any index beyond it) confirms; only an explicit false
// suppresses the write.
func (f *FakeAdapter) confirmed(i int) bool {
	if i < len(f.Confirm) {
		return f.Confirm[i]
	}
	return true
}

// parseSpawnAck finds the --ack <value> pair in an argv and splits its value
// back into (batch, token) via the real spawn.ParseSpawnAckFlag, so the fake
// stays honest to the exact wire format composeOpenArgv produces.
func parseSpawnAck(argv []string) (batch, token string, ok bool) {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--ack" {
			return spawn.ParseSpawnAckFlag(argv[i+1])
		}
	}
	return "", "", false
}

// Compile-time guard: FakeAdapter must satisfy spawn.Adapter.
var _ spawn.Adapter = (*FakeAdapter)(nil)
