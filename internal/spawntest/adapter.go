// Package spawntest provides the fakes for unit-testing Portal's host-terminal
// spawn pipeline without touching a real terminal.
//
// Test-only: production code must not import this package.
package spawntest

import (
	"slices"
	"sync"

	"github.com/leeovery/portal/internal/spawn"
)

// FakeAdapter is a test double for spawn.Adapter, recording every OpenWindow
// argv and replaying scripted Results. Use it by pointer — the mutex makes it
// non-copyable once used.
type FakeAdapter struct {
	// Calls holds a defensive copy of each argv OpenWindow was handed, in
	// call order.
	Calls [][]string
	// Results scripts the outcome of each call: call i returns Results[i], and
	// calls beyond it default to spawn.Success("").
	Results []spawn.Result
	// Ack, when set, receives the parsed token of each confirmed success call,
	// simulating the spawned window's ack write.
	Ack *FakeAckChannel
	// Confirm gates the ack write per window: Confirm[i] false suppresses
	// window i's write (→ ack timeout). A nil slice confirms every window.
	Confirm []bool

	mu sync.Mutex
}

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

func (f *FakeAdapter) confirmed(i int) bool {
	if i < len(f.Confirm) {
		return f.Confirm[i]
	}
	return true
}

// Splitting through the real spawn.ParseSpawnAckFlag keeps the fake honest to
// the wire format composeOpenArgv produces.
func parseSpawnAck(argv []string) (batch, token string, ok bool) {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--ack" {
			return spawn.ParseSpawnAckFlag(argv[i+1])
		}
	}
	return "", "", false
}

var _ spawn.Adapter = (*FakeAdapter)(nil)
