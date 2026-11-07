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

			if !(Gate{StateDir: dir(t), Running: running, ZmxDir: thisZmxDir, Saver: saver.check}).Owed(t.Context()) {
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
		// A read that fails says nothing of the saver, whatever else it
		// answers.
		"a saver that can't be told there, its read answering there as well": {present: true, err: errBoom},
	} {
		t.Run("a bootstrap is owed with the latch current and "+name, func(t *testing.T) {
			if !(Gate{StateDir: latched(t, running), Running: running, ZmxDir: thisZmxDir, Saver: saver.check}).Owed(t.Context()) {
				t.Error("Owed = false, want a bootstrap owed")
			}
		})
	}

	// Decision 67: restoring covers restore's creates alone. The latch, removed
	// before a bootstrap's first change, is what records one unfinished.
	t.Run("the restoring flag raised owes none, with the latch current", func(t *testing.T) {
		dir := latched(t, running)
		if err := state.Restoring.Set(dir); err != nil {
			t.Fatal(err)
		}

		if (Gate{StateDir: dir, Running: running, ZmxDir: thisZmxDir, Saver: (&presence{present: true}).check}).Owed(t.Context()) {
			t.Error("Owed = true, want the warm path: the flag is no record of a bootstrap")
		}
	})

	t.Run("a warm run owes none, and asks only whether the saver is there, once", func(t *testing.T) {
		saver := &presence{present: true}

		if (Gate{StateDir: latched(t, running), Running: running, ZmxDir: thisZmxDir, Saver: saver.check}).Owed(t.Context()) {
			t.Error("Owed = true, want the warm path")
		}
		if saver.asks != 1 {
			t.Errorf("the saver was asked %d times, want once", saver.asks)
		}
	})

	// decision 76: a warm command against another zmx directory's state would
	// record its mints in that state's index.
	for name, check := range map[string]func() error{
		"a state directory of another zmx directory": func() error { return &OtherZmxDirError{StateDir: "/s", Recorded: "/z", Using: "/y"} },
		"a zmx directory record it can't read":       func() error { return errBoom },
		"no zmx directory recorded":                  func() error { return ErrZmxDirUnrecorded },
	} {
		t.Run("a bootstrap is owed with the latch current and "+name+", the saver unasked", func(t *testing.T) {
			saver := &presence{present: true}

			if !(Gate{StateDir: latched(t, running), Running: running, ZmxDir: check, Saver: saver.check}).Owed(t.Context()) {
				t.Error("Owed = false, want the bootstrap that refuses it owed")
			}
			if saver.asks != 0 {
				t.Errorf("the saver was asked %d times, want none", saver.asks)
			}
		})
	}
}

// thisZmxDir is a state directory that records the zmx directory in use.
func thisZmxDir() error { return nil }
