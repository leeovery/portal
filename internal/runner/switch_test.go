package runner_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

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

// confirming counts the calls of Confirm, answering each with answer.
func confirming(calls *atomic.Int32, answer error) option {
	return func(_ *harness, cfg *runner.Config) {
		cfg.Confirm = func(context.Context) error {
			calls.Add(1)
			return answer
		}
	}
}

// unsure attaches a child whose output shows a switch twice, and waits for
// the terminal to show both, by when the runner has seen them.
func unsure(t *testing.T, opts ...option) *harness {
	t.Helper()
	h := attach(t, modeSwitched, opts...)
	h.ready(t)
	h.term.await(t, switchSignature+switchSignature)
	return h
}

func TestRun_Unsure(t *testing.T) {
	t.Run("a switch reaches the terminal unchanged and is logged once", func(t *testing.T) {
		sink := logtest.Install(t)
		h := unsure(t)

		h.detach(t)

		rec := sink.Records().Matching("attach", "switch seen").AtExactLevel(slog.LevelDebug).Only(t, "switch seen record")
		if got := rec.AttrString(t, "session"); got != "fake" {
			t.Errorf("switch seen session = %q, want fake", got)
		}
	})

	t.Run("Unsure is called once, for the first switch seen", func(t *testing.T) {
		calls := new(atomic.Int32)
		h := unsure(t, func(_ *harness, cfg *runner.Config) {
			cfg.Unsure = func() { calls.Add(1) }
		})

		h.detach(t)

		if n := calls.Load(); n != 1 {
			t.Errorf("Unsure was called %d times for two switches, want once", n)
		}
	})

	t.Run("the kill key asks Confirm as it is pressed, and kills once it confirms", func(t *testing.T) {
		calls := new(atomic.Int32)
		h := unsure(t, confirming(calls, nil), func(h *harness, cfg *runner.Config) {
			cfg.Kill = func(context.Context, string) error {
				if calls.Load() != 1 {
					t.Error("the session was killed before Confirm confirmed it")
				}
				return syscall.Kill(h.child(), syscall.SIGUSR1)
			}
		})
		if n := calls.Load(); n != 0 {
			t.Errorf("Confirm was asked %d times as the switch was seen, want only as the kill key is pressed", n)
		}

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Kill}) {
			t.Errorf("Run = %+v, %v; want a kill with no error", out, err)
		}
		if n := calls.Load(); n != 1 {
			t.Errorf("Confirm was asked %d times, want once", n)
		}
	})

	t.Run("a kill Confirm refuses kills nothing and ends the attach for the picker, saying why", func(t *testing.T) {
		sink := logtest.Install(t)
		doubt := errors.New("another client leads the session")
		h := unsure(t, confirming(new(atomic.Int32), doubt)) // the harness's Kill fails the test if called

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || out.Action != attachkeys.Kill || !errors.Is(out.Err, runner.ErrUnconfirmed) {
			t.Fatalf("Run = %+v, %v; want the kill refused, unconfirmed", out, err)
		}
		if !out.Picker() {
			t.Error("Picker() = false, want the picker to show why")
		}
		rec := sink.Records().Matching("attach", "kill unconfirmed").AtExactLevel(slog.LevelDebug).Only(t, "kill unconfirmed record")
		if got := rec.AttrString(t, "error"); got != doubt.Error() {
			t.Errorf("kill unconfirmed error = %q, want Confirm's %q", got, doubt)
		}
		assertDetachedCleanly(t, h, sink)
	})

	t.Run("with no Confirm, nothing confirms a kill", func(t *testing.T) {
		h := unsure(t)

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || !errors.Is(out.Err, runner.ErrUnconfirmed) {
			t.Errorf("Run = %+v, %v; want the kill refused, unconfirmed", out, err)
		}
	})

	t.Run("a context that ends while Confirm is asked ends the attach with its error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		h := attachWithin(t, ctx, modeSwitched, func(_ *harness, cfg *runner.Config) {
			cfg.Confirm = func(ctx context.Context) error {
				cancel()
				return ctx.Err()
			}
		})
		h.ready(t)
		h.term.await(t, switchSignature+switchSignature)

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %+v, %v; want the context's error", out, err)
		}
	})

	t.Run("a switch request is taken", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := unsure(t, withRequests(requests))

		send(t, requests, runner.Request{Switch: "blog-c3d4"})
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Switch: "blog-c3d4"}) {
			t.Errorf("Run = %+v, %v; want the switch", out, err)
		}
	})

	t.Run("a request for the picker is refused, and the attach goes on to detach", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := unsure(t, withRequests(requests))

		if err := ask(t, requests, runner.Request{}); !errors.Is(err, runner.ErrUnconfirmed) {
			t.Errorf("Run answered the picker's request with %v, want ErrUnconfirmed", err)
		}
		h.detach(t)
	})

	t.Run("the detach keys still detach", func(t *testing.T) {
		for key, action := range map[string]attachkeys.Action{keyDetach: attachkeys.Detach, keyDetachToShell: attachkeys.DetachToShell} {
			h := unsure(t)

			h.term.typeIn(t, key)
			out, err := h.finish(t)

			if err != nil || out != (runner.Outcome{Action: action}) {
				t.Errorf("Run = %+v, %v; want %v", out, err, action)
			}
		}
	})
}

func TestRun_KillWithNoSwitchSeenNeverAsksConfirm(t *testing.T) {
	h := attach(t, modeZmx, func(h *harness, cfg *runner.Config) {
		cfg.Confirm = func(context.Context) error {
			t.Error("Confirm was asked with no switch seen")
			return nil
		}
		cfg.Unsure = func() { t.Error("Unsure was called with no switch seen") }
		cfg.Kill = func(context.Context, string) error { return syscall.Kill(h.child(), syscall.SIGUSR1) }
	})
	h.ready(t)

	h.term.typeIn(t, keyKill)
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Action: attachkeys.Kill}) {
		t.Errorf("Run = %+v, %v; want a kill with no error", out, err)
	}
}
