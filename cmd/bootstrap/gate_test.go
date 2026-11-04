package bootstrap

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

var running = state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}

// presence answers a Gate's saver check as its fields say, counting the asks.
type presence struct {
	present bool
	err     error
	asks    int
}

func (p *presence) check(context.Context) (bool, error) {
	p.asks++
	return p.present, p.err
}

func TestGateOwed(t *testing.T) {
	latched := func(t *testing.T, latch state.Latch) string {
		t.Helper()
		dir := t.TempDir()
		if err := state.WriteLatch(dir, latch); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	for name, dir := range map[string]func(t *testing.T) string{
		"no latch": func(t *testing.T) string { return t.TempDir() },
		"another boot's latch": func(t *testing.T) string {
			return latched(t, state.Latch{BootID: "F58F3A1E", Version: running.Version})
		},
		"another version's one": func(t *testing.T) string { return latched(t, state.Latch{BootID: running.BootID, Version: "0.12.6"}) },
	} {
		t.Run("a bootstrap is owed with "+name+", the saver unasked", func(t *testing.T) {
			saver := &presence{present: true}

			if !(Gate{StateDir: dir(t), Running: running, Present: saver.check}).Owed(t.Context()) {
				t.Error("Owed = false, want a bootstrap owed")
			}
			if saver.asks != 0 {
				t.Errorf("the saver was asked %d times, want none", saver.asks)
			}
		})
	}

	for name, saver := range map[string]*presence{
		"no saver session":                 {present: false},
		"a saver that can't be told there": {err: errBoom},
	} {
		t.Run("a bootstrap is owed with the latch current and "+name, func(t *testing.T) {
			if !(Gate{StateDir: latched(t, running), Running: running, Present: saver.check}).Owed(t.Context()) {
				t.Error("Owed = false, want a bootstrap owed")
			}
		})
	}

	// decision 61: a raised flag is a bootstrap under way, or one cut short
	// before it finished its restore.
	t.Run("a bootstrap is owed with the latch current and the restoring flag raised, the saver unasked", func(t *testing.T) {
		dir := latched(t, running)
		if err := state.Restoring.Set(dir); err != nil {
			t.Fatal(err)
		}
		saver := &presence{present: true}

		if !(Gate{StateDir: dir, Running: running, Present: saver.check}).Owed(t.Context()) {
			t.Error("Owed = false, want a bootstrap owed")
		}
		if saver.asks != 0 {
			t.Errorf("the saver was asked %d times, want none", saver.asks)
		}
	})

	t.Run("a warm run owes none, and asks only whether the saver is there, once", func(t *testing.T) {
		saver := &presence{present: true}

		if (Gate{StateDir: latched(t, running), Running: running, Present: saver.check}).Owed(t.Context()) {
			t.Error("Owed = true, want the warm path")
		}
		if saver.asks != 1 {
			t.Errorf("the saver was asked %d times, want once", saver.asks)
		}
	})
}
