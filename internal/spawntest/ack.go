// FakeAckChannel is the in-memory test double for the spawn ack channel,
// mirroring the FakeAdapter conventions in this test-only package. It lets the
// burst-pipeline tests confirm/timeout windows without a real tmux server: a
// test seeds "this token arrived" via Ack/Write, the burster polls Collect, and
// Clean records the swept batches — all in memory.
//
// Test-only: production code MUST NOT import this package.

package spawntest

import (
	"maps"
	"sync"

	"github.com/leeovery/portal/internal/spawn"
)

// FakeAckChannel is an in-memory batch→token-set store satisfying
// spawn.AckChannelFull (Collect + Clean) and spawn.AckWriter (Write). It must be
// used by pointer — the mutex makes it non-copyable once used.
type FakeAckChannel struct {
	// Cleaned records each batch passed to Clean, in call order.
	Cleaned []string
	// FailCollect, when non-nil, makes Collect return it (and a nil map) so a
	// test can script the enumeration-failure case.
	FailCollect error

	mu    sync.Mutex
	store map[string]map[string]struct{}
}

// Write records token as arrived under batch.
func (f *FakeAckChannel) Write(batch, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store == nil {
		f.store = map[string]map[string]struct{}{}
	}
	set := f.store[batch]
	if set == nil {
		set = map[string]struct{}{}
		f.store[batch] = set
	}
	set[token] = struct{}{}
	return nil
}

// Ack seeds "this token arrived" for a batch — an alias of Write for tests that
// do not care about the (always-nil) error.
func (f *FakeAckChannel) Ack(batch, token string) { _ = f.Write(batch, token) }

// Collect returns a copy of the batch's token set (a non-nil empty map when the
// batch has none), or (nil, FailCollect) when FailCollect is set.
func (f *FakeAckChannel) Collect(batch string) (map[string]struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailCollect != nil {
		return nil, f.FailCollect
	}
	out := map[string]struct{}{}
	maps.Copy(out, f.store[batch])
	return out, nil
}

// Clean appends batch to Cleaned and deletes the batch's token set.
func (f *FakeAckChannel) Clean(batch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Cleaned = append(f.Cleaned, batch)
	delete(f.store, batch)
	return nil
}

// Compile-time guards: *FakeAckChannel satisfies the burst orchestrators' seam
// and the write seam.
var (
	_ spawn.AckChannelFull = (*FakeAckChannel)(nil)
	_ spawn.AckWriter      = (*FakeAckChannel)(nil)
)
