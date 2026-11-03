package runner_test

import (
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
)

// screenClear is what zmx's client writes as it begins an attach.
const screenClear = "\x1b[0m\x1b[2J\x1b[H"

func TestScanForSwitch(t *testing.T) {
	t.Run("it finds zmx's reset followed by its screen clear, wherever the reads split them", func(t *testing.T) {
		if got := runner.ScanForSwitch(switchSignature); got != 0 {
			t.Errorf("in one read: found in read %d, want 0", got)
		}
		for i := 1; i < len(switchSignature); i++ {
			if got := runner.ScanForSwitch("before"+switchSignature[:i], switchSignature[i:]+"after"); got != 1 {
				t.Errorf("split at %d: found in read %d, want 1", i, got)
			}
		}
		bytewise := strings.Split(switchSignature, "")
		if got := runner.ScanForSwitch(bytewise...); got != len(bytewise)-1 {
			t.Errorf("a byte a read: found in read %d, want the last, %d", got, len(bytewise)-1)
		}
	})

	t.Run("it finds one that begins inside a false start", func(t *testing.T) {
		for _, before := range []string{"\x1b", "\x1bc", terminalReset[:7], terminalReset, terminalReset + "\x1b"} {
			if got := runner.ScanForSwitch(before + switchSignature); got != 0 {
				t.Errorf("after %q: found in read %d, want 0", before, got)
			}
		}
	})

	t.Run("a detach's reset, an attach's screen clear, or the two apart are no switch", func(t *testing.T) {
		for _, reads := range [][]string{
			{terminalReset},
			{screenClear},
			{screenClear, "output", terminalReset},
			{terminalReset, "output", screenClear},
			{terminalReset + "x" + screenClear},
			{switchSignature[:len(switchSignature)-1], "x"},
		} {
			if got := runner.ScanForSwitch(reads...); got != -1 {
				t.Errorf("%q: found in read %d, want none", reads, got)
			}
		}
	})
}

// onSwitchedAway counts the calls of SwitchedAway, and closes called at the
// first.
func onSwitchedAway(calls *atomic.Int32, called chan struct{}) option {
	return func(_ *harness, cfg *runner.Config) {
		cfg.SwitchedAway = func() {
			if calls.Add(1) == 1 {
				close(called)
			}
		}
	}
}

// switchedAway attaches a child that zmx switches to another session, and
// waits for the runner to see it.
func switchedAway(t *testing.T, opts ...option) (*harness, *atomic.Int32) {
	t.Helper()
	calls, called := new(atomic.Int32), make(chan struct{})
	h := attach(t, modeSwitched, append(opts, onSwitchedAway(calls, called))...)
	h.ready(t)
	select {
	case <-called:
	case <-time.After(settle):
		t.Fatalf("SwitchedAway was never called; the terminal shows %q", h.term.output())
	}
	return h, calls
}

func TestRun_SwitchedAway(t *testing.T) {
	t.Run("it is called once, logged, and the switch reaches the terminal unchanged", func(t *testing.T) {
		sink := logtest.Install(t)
		h, calls := switchedAway(t)

		h.term.await(t, switchSignature+switchSignature)
		h.detach(t)

		if n := calls.Load(); n != 1 {
			t.Errorf("SwitchedAway was called %d times for two switches, want once", n)
		}
		rec := sink.Records().Matching("attach", "switched away").AtExactLevel(slog.LevelInfo).Only(t, "switched away record")
		if got := rec.AttrString(t, "session"); got != "fake" {
			t.Errorf("switched away session = %q, want fake", got)
		}
	})

	t.Run("the kill key kills nothing and ends the attach for the picker, saying why", func(t *testing.T) {
		sink := logtest.Install(t)
		h, _ := switchedAway(t) // the harness's Kill fails the test if called

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || out.Action != attachkeys.Kill || !errors.Is(out.Err, runner.ErrSwitchedAway) {
			t.Fatalf("Run = %+v, %v; want the kill refused for the switch", out, err)
		}
		if !out.Picker() {
			t.Error("Picker() = false, want the picker to show why")
		}
		assertDetachedCleanly(t, h, sink)
	})

	t.Run("every request is refused, and the attach goes on to detach", func(t *testing.T) {
		requests := make(chan runner.Request)
		h, _ := switchedAway(t, withRequests(requests))

		for _, req := range []runner.Request{{Switch: "blog-c3d4"}, {}} {
			if err := ask(t, requests, req); !errors.Is(err, runner.ErrSwitchedAway) {
				t.Errorf("Run answered %+v with %v, want ErrSwitchedAway", req, err)
			}
		}
		h.detach(t)
	})

	t.Run("the detach keys still detach", func(t *testing.T) {
		for key, action := range map[string]attachkeys.Action{keyDetach: attachkeys.Detach, keyDetachToShell: attachkeys.DetachToShell} {
			h, _ := switchedAway(t)

			h.term.typeIn(t, key)
			out, err := h.finish(t)

			if err != nil || out != (runner.Outcome{Action: action}) {
				t.Errorf("Run = %+v, %v; want %v", out, err, action)
			}
		}
	})
}
