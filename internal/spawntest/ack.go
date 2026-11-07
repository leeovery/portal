package spawntest

import (
	"maps"
	"sync"

	"github.com/leeovery/portal/internal/spawn"
)

// FakeAckChannel is an in-memory batch→token-set store satisfying the spawn ack
// seams. Use it by pointer — the mutex makes it non-copyable once used.
type FakeAckChannel struct {
	// Cleaned records each batch passed to Clean, in call order.
	Cleaned []string
	// FailCollect, when non-nil, is returned by Collect with a nil map.
	FailCollect error
	// FailClean, when non-nil, is returned by Clean, which still records the
	// batch and removes nothing.
	FailClean error

	mu    sync.Mutex
	store map[string]map[string]struct{}
}

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

// Ack seeds "this token arrived", discarding Write's always-nil error.
func (f *FakeAckChannel) Ack(batch, token string) { _ = f.Write(batch, token) }

// Collect returns a copy of the batch's token set, non-nil even when empty, or
// (nil, FailCollect) when FailCollect is set.
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

func (f *FakeAckChannel) Clean(batch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Cleaned = append(f.Cleaned, batch)
	if f.FailClean != nil {
		return f.FailClean
	}
	delete(f.store, batch)
	return nil
}

var (
	_ spawn.AckChannelFull = (*FakeAckChannel)(nil)
	_ spawn.AckWriter      = (*FakeAckChannel)(nil)
)
