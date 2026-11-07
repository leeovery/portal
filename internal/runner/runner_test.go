package runner_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"golang.org/x/sys/unix"
)

func TestRun_CopiesBothWaysUnchanged(t *testing.T) {
	h := attach(t, modeBlob)
	h.ready(t)
	h.term.await(t, string(childBlob))

	if during := stateOf(t, h.in); *during == *h.before {
		t.Errorf("the terminal's settings are unchanged during the attach, want raw mode")
	}

	// Every byte value but ctrl+\, which would detach the child.
	typed := string(slices.DeleteFunc(slices.Clone(childBlob), func(b byte) bool { return b == ctrlBackslash }))
	h.term.typeIn(t, typed)
	var got string
	harnesstest.PollUntil(t, settle, 10*time.Millisecond, func() bool {
		got = strings.Join(h.term.reads(t), "")
		return len(got) >= len(typed)
	})
	if got != typed {
		t.Errorf("the child read %q, want every byte typed unchanged: %q", got, typed)
	}
	if bytes.Contains(h.term.output(), []byte(typed)) {
		t.Errorf("the terminal echoed what was typed")
	}

	h.detach(t)
}

func TestRun_LeavesLaterInputForTheNextReader(t *testing.T) {
	h := attach(t, modeZmx)
	h.ready(t)
	h.detach(t)

	h.term.typeIn(t, "next\r")

	if got := h.term.next(t); got != "next\n" {
		t.Errorf("the next reader found %q, want the keys typed after the attach", got)
	}
}

func TestRun_DropsInputTheAttachDidNotRead(t *testing.T) {
	runner.SetGrace(t, shortGrace)
	h := attach(t, modeDeaf)
	h.ready(t)

	h.term.typeIn(t, keyDetach)
	// The child has its ctrl+\, so the runner reads no more, and it ignores
	// it, so the attach is still ending when the terminal answers late.
	h.term.await(t, "in:1c")
	h.term.typeIn(t, "late\r")
	if _, err := h.finish(t); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := h.term.nextRaw(t); got != "" {
		t.Errorf("the next reader found %q, want what reached the terminal during the attach dropped", got)
	}
}

func TestRun_BoundKey(t *testing.T) {
	for _, tc := range []struct {
		name       string
		key        string
		want       attachkeys.Action
		wantPicker bool
		wantAsk    bool
		reason     string
	}{
		{"detach goes to the picker", keyDetach, attachkeys.Detach, true, false, "detach"},
		{"detach to shell goes to the shell", keyDetachToShell, attachkeys.DetachToShell, false, false, "detach_to_shell"},
		{"kill detaches to the picker, asking for the kill", keyKill, attachkeys.Kill, true, true, "kill"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := logtest.Install(t)
			h := attach(t, modeZmx)
			h.ready(t)

			h.term.typeIn(t, "abc"+tc.key+"dropped")
			out, err := h.finish(t)

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if want := (runner.Outcome{Action: tc.want}); out != want {
				t.Errorf("Outcome = %+v, want %+v", out, want)
			}
			if out.Picker() != tc.wantPicker {
				t.Errorf("Picker() = %v, want %v", out.Picker(), tc.wantPicker)
			}
			if out.AsksKill() != tc.wantAsk {
				t.Errorf("AsksKill() = %v, want %v", out.AsksKill(), tc.wantAsk)
			}
			// ctrl+\ in a read of its own, as zmx detaches only on that.
			if got, want := h.term.reads(t), []string{"abc", "\x1c"}; !slices.Equal(got, want) {
				t.Errorf("the child read %q, want the input before the key, then ctrl+\\ alone", got)
			}
			h.term.await(t, "nonblock:true")
			if bytes.Contains(h.term.output(), []byte("\x1bc")) {
				t.Errorf("the terminal was reset after a child that exited by its own hand")
			}
			rec := sink.Records().Matching("attach", "outcome").AtExactLevel(slog.LevelInfo).Only(t, "outcome record")
			if got := rec.AttrString(t, "session"); got != "fake" {
				t.Errorf("outcome session = %q, want fake", got)
			}
			if got := rec.AttrString(t, "reason"); got != tc.reason {
				t.Errorf("outcome reason = %q, want %q", got, tc.reason)
			}
			if got := rec.IntAttr(t, "code"); got != 0 {
				t.Errorf("outcome code = %d, want 0", got)
			}
		})
	}
}

func TestRun_ChildEndingOnItsOwnExitsToTheShell(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		typed     string
		wantCode  int
		wantReset bool
	}{
		{"the user's own ctrl+\\", modeZmx, "\x1c", 0, false},
		{"an error exit", modeExit, "", 3, false},
		{"a death by signal, which leaves the reset to the runner", modeSelfKill, "", 128 + int(syscall.SIGKILL), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := attach(t, tc.mode)
			h.ready(t)

			h.term.typeIn(t, tc.typed)
			out, err := h.finish(t)

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if want := (runner.Outcome{ExitCode: tc.wantCode}); out != want {
				t.Errorf("Outcome = %+v, want %+v", out, want)
			}
			if out.Picker() {
				t.Errorf("Picker() = true when the child ended on its own, want the shell")
			}
			if tc.wantReset {
				h.term.await(t, terminalReset)
			} else if bytes.Contains(h.term.output(), []byte("\x1bc")) {
				t.Errorf("the terminal was reset after a child that exited by its own hand")
			}
		})
	}

	t.Run("zmx dropping the child, as a session's end or a detach from elsewhere does, is a drop", func(t *testing.T) {
		h := attach(t, modeZmx)
		h.ready(t)

		_ = syscall.Kill(h.child(), syscall.SIGUSR1)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Dropped: true}) {
			t.Errorf("Run = %+v, %v; want the child dropped", out, err)
		}
		if out.Picker() {
			t.Error("Picker() = true for a drop, want the caller to judge it")
		}
	})

	t.Run("a drop once zmx's detach key has been passed on, in any form, is no drop: the user may have detached", func(t *testing.T) {
		h := attach(t, modeZmx)
		h.ready(t)

		h.term.typeIn(t, "\x1b[92;5u")
		h.term.await(t, "in:1b5b39323b3575")
		_ = syscall.Kill(h.child(), syscall.SIGUSR1)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{}) {
			t.Errorf("Run = %+v, %v; want the shell with status 0, no drop", out, err)
		}
	})

	t.Run("the child's last output reaches the terminal", func(t *testing.T) {
		h := attach(t, modeExit)
		h.ready(t)
		if _, err := h.finish(t); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !bytes.Contains(h.term.output(), []byte("bye\r\n")) {
			t.Errorf("the terminal shows %q, want the child's last line", h.term.output())
		}
	})
}

func TestRun_BoundsTheWaitForTheChild(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    string
		signals []string
	}{
		{"a child that ignores ctrl+\\ is sent SIGHUP", modeDeaf, []string{"SIGHUP"}},
		{"a child that ignores SIGHUP too is sent SIGKILL", modeStubborn, []string{"SIGHUP", "SIGKILL"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner.SetGrace(t, shortGrace)
			sink := logtest.Install(t)
			h := attach(t, tc.mode)
			h.ready(t)

			h.term.typeIn(t, keyDetach)
			out, err := h.finish(t)

			if err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
				t.Errorf("Run = %+v, %v; want the detach", out, err)
			}
			if !slices.Contains(h.term.reads(t), "\x1c") {
				t.Errorf("the child was never sent ctrl+\\")
			}
			var sent []string
			for _, rec := range sink.Records().Matching("attach", "child did not exit").AtExactLevel(slog.LevelWarn) {
				sent = append(sent, rec.AttrString(t, "detail"))
			}
			if !slices.Equal(sent, tc.signals) {
				t.Errorf("signals sent = %q, want %q", sent, tc.signals)
			}
			if err := syscall.Kill(h.child(), 0); !errors.Is(err, syscall.ESRCH) {
				t.Errorf("the child %d is still running: %v", h.child(), err)
			}
			// zmx resets the terminal on its way out, which a signal stops.
			h.term.await(t, terminalReset)
		})
	}
}

func TestRun_LeavesAChildThatReadsNothingToItsSignals(t *testing.T) {
	runner.SetGrace(t, shortGrace)
	ctx, cancel := context.WithCancel(t.Context())
	h := attachWithin(t, ctx, modeFrozen)
	h.ready(t)

	// More than the child's terminal holds, so forwarding it blocks.
	go func() { _, _ = h.term.user.Write(bytes.Repeat([]byte("x"), 8<<10)) }()
	h.term.awaitBacklog(t)
	cancel()
	_, err := h.finish(t)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want the context's", err)
	}
	reads := h.term.reads(t)
	if len(reads) != 1 || reads[0] == "" || strings.Trim(reads[0], "x") != "" {
		t.Errorf("the child found %q waiting, want only what was typed: no ctrl+\\ behind it", reads)
	}
}

func TestRun_DetachingAChildThatHasExitedIsQuick(t *testing.T) {
	runner.SetGrace(t, 5*time.Second)
	var confirmReturned atomic.Int64
	h := attach(t, modeFrozenSwitched, func(h *harness, cfg *runner.Config) {
		// Confirm, asked as the kill key is pressed, ends the child with input
		// still unread in its terminal, so the child is detached after its end.
		cfg.Confirm = func(context.Context) error {
			_ = syscall.Kill(h.child(), syscall.SIGUSR1)
			harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
				return errors.Is(syscall.Kill(h.child(), 0), syscall.ESRCH)
			})
			confirmReturned.Store(time.Now().UnixNano())
			return nil
		}
	})
	h.ready(t)
	h.term.await(t, switchSignature+switchSignature)

	h.term.typeIn(t, "abc"+keyKill)
	r := h.end(t)

	if r.err != nil || !r.out.AsksKill() {
		t.Errorf("Run = %+v, %v; want the kill asked", r.out, r.err)
	}
	if took := r.returned.Sub(time.Unix(0, confirmReturned.Load())); took > time.Second {
		t.Errorf("Run took %v after the child had gone, want no wait for it to read its input", took)
	}
}

func TestRun_CtrlBackslashIsAByteBeforeTheChildMakesItsTerminalRaw(t *testing.T) {
	sink := logtest.Install(t)
	h := attach(t, modeCooked)
	h.ready(t)

	h.term.typeIn(t, "abc"+keyDetach)
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
		t.Errorf("Run = %+v, %v; want a detach", out, err)
	}
	if got, want := h.term.reads(t), []string{"abc", "\x1c"}; !slices.Equal(got, want) {
		t.Errorf("the child read %q, want the input before the key, then ctrl+\\ alone", got)
	}
	if signalled := sink.Records().Matching("attach", "child did not exit"); len(signalled) > 0 {
		t.Errorf("the child had to be signalled: %+v", signalled)
	}
}

func TestRun_ForwardsResizes(t *testing.T) {
	h := attach(t, modeZmx)
	if got := h.ready(t); got != "80x24" {
		t.Errorf("the child's terminal started at %s, want the user's 80x24", got)
	}

	if err := pty.Setsize(h.term.user, &pty.Winsize{Cols: 132, Rows: 50}); err != nil {
		t.Fatalf("resize the user's terminal: %v", err)
	}
	// The user's terminal is not this process's, so its resize signals
	// nobody: send what the terminal would.
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("signal the resize: %v", err)
	}
	h.term.await(t, "size:132x50\r\n")

	h.detach(t)
}

func TestRun_SignalToPortalDetachesAndRestores(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			h := attach(t, modeZmx)
			// Ready means the child started, after the runner caught the
			// signals: one sent now cannot end the test binary.
			h.ready(t)

			if err := syscall.Kill(os.Getpid(), sig); err != nil {
				t.Fatalf("signal Portal: %v", err)
			}
			out, err := h.finish(t)

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if want := (runner.Outcome{ExitCode: 128 + int(sig)}); out != want {
				t.Errorf("Outcome = %+v, want %+v", out, want)
			}
			if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
				t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
			}
		})
	}
}

func TestRun_StopsCatchingSignalsOnReturn(t *testing.T) {
	if state, out := runSelf(t, afterRunEnv); !endedBy(state, syscall.SIGINT) {
		t.Errorf("a SIGINT after Run returned left the process %v, want it ended: %s", state, out)
	}
}

func TestRun_RaisesAgainASignalCaughtByAnAttachThatFailedToBegin(t *testing.T) {
	if state, out := runSelf(t, abandonEnv); !endedBy(state, syscall.SIGTERM) {
		t.Errorf("a SIGTERM caught while the attach began left the process %v, want it ended: %s", state, out)
	}
}

// runSelf runs the test binary in the mode env switches on, and returns how
// it ended and what it wrote.
func runSelf(t *testing.T, env string) (*os.ProcessState, []byte) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("find the test binary: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*settle)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), env+"=1")
	out, _ := cmd.CombinedOutput()
	return cmd.ProcessState, out
}

func endedBy(state *os.ProcessState, sig syscall.Signal) bool {
	status, ok := state.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == sig
}

func TestRun_PanicInACopyLoopDetachesAndRestores(t *testing.T) {
	sink := logtest.Install(t)
	h := attach(t, modeZmx, func(_ *harness, cfg *runner.Config) { cfg.Scanner = panicking{} })
	h.ready(t)

	h.term.typeIn(t, "x")
	out, err := h.finish(t)

	if err == nil || !strings.Contains(err.Error(), "scanner exploded") {
		t.Errorf("Run = %+v, %v; want the panic as an error", out, err)
	}
	if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
		t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
	}
	assertPanicLogged(t, sink, "scanner exploded")
}

func TestRun_PanicEndOutranksASignal(t *testing.T) {
	t.Run("a copy loop's panic after a signal ended the attach", func(t *testing.T) {
		sink := logtest.Install(t)
		scanning, explode := make(chan struct{}), make(chan struct{})
		h := attach(t, modeZmx, func(_ *harness, cfg *runner.Config) {
			cfg.Scanner = holdThenPanic{scanning, explode}
		})
		h.ready(t)

		h.term.typeIn(t, "x")
		<-scanning
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatalf("signal Portal: %v", err)
		}
		// The signal ends the attach, which waits for the scan to finish.
		time.Sleep(100 * time.Millisecond)
		close(explode)
		out, err := h.finish(t)

		if err == nil || !strings.Contains(err.Error(), "scanner exploded") {
			t.Errorf("Run = %+v, %v; want the panic as an error", out, err)
		}
		assertPanicLogged(t, sink, "scanner exploded")
	})

	t.Run("a signal while a copy loop's panic tears the attach down", func(t *testing.T) {
		runner.SetGrace(t, shortGrace)
		sink := logtest.Install(t)
		h := attach(t, modeDeaf, func(_ *harness, cfg *runner.Config) { cfg.Scanner = panicking{} })
		h.ready(t)

		h.term.typeIn(t, "x")
		// The deaf child keeps the teardown waiting while Portal is signalled.
		h.term.await(t, "in:1c")
		signalDuringTeardown(t)
		out, err := h.finish(t)

		if err == nil || !strings.Contains(err.Error(), "scanner exploded") {
			t.Errorf("Run = %+v, %v; want the panic as an error, not the signal's outcome", out, err)
		}
		assertPanicLogged(t, sink, "scanner exploded")
	})

	t.Run("a signal while Began's panic tears the attach down", func(t *testing.T) {
		runner.SetGrace(t, shortGrace)
		h := attach(t, modeDeaf, func(_ *harness, cfg *runner.Config) {
			cfg.Began = func() { panic("began exploded") }
		})
		h.ready(t)

		h.term.await(t, "in:1c")
		signalDuringTeardown(t)
		r := h.end(t)

		if r.panicked != "began exploded" {
			t.Errorf("Run = %+v, %v, panicking with %v; want Began's panic raised again", r.out, r.err, r.panicked)
		}
	})
}

// signalDuringTeardown signals Portal while an attach is torn down.
func signalDuringTeardown(t *testing.T) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal Portal: %v", err)
	}
}

// holdThenPanic is a Scanner that says when it is scanning, then panics once
// told to.
type holdThenPanic struct{ scanning, explode chan struct{} }

func (s holdThenPanic) Scan([]byte) ([]byte, attachkeys.Action, bool) {
	close(s.scanning)
	<-s.explode
	panic("scanner exploded")
}

func TestRun_PanicInConfirmDetachesThenPropagates(t *testing.T) {
	sink := logtest.Install(t)
	h := unsure(t, func(_ *harness, cfg *runner.Config) {
		cfg.Confirm = func(context.Context) error { panic("confirm exploded") }
	})

	h.term.typeIn(t, keyKill)
	r := h.end(t) // the terminal's settings and flags are checked restored

	if r.panicked != "confirm exploded" {
		t.Errorf("Run = %+v, %v, panicking with %v; want Confirm's panic raised again", r.out, r.err, r.panicked)
	}
	assertDetachedCleanly(t, h, sink)
	assertPanicLogged(t, sink, "confirm exploded")
}

func TestRun_GoexitInConfirmDetachesAndRestores(t *testing.T) {
	sink := logtest.Install(t)
	h := unsure(t, func(_ *harness, cfg *runner.Config) {
		cfg.Confirm = func(context.Context) error {
			runtime.Goexit() // as a test's fake Confirm calling t.FailNow does
			return nil
		}
	})

	h.term.typeIn(t, keyKill)
	r := h.end(t) // the terminal's settings and flags are checked restored

	if !r.goexited {
		t.Errorf("Run = %+v, %v, panicking with %v; want its goroutine to have exited", r.out, r.err, r.panicked)
	}
	assertDetachedCleanly(t, h, sink)
	assertErrorOutcome(t, sink, "goexit", "goroutine exited")
}

// assertDetachedCleanly checks the attach detached the child with ctrl+\,
// which ended it by its own hand: never signalled, and with no reset of the
// runner's.
func assertDetachedCleanly(t *testing.T, h *harness, sink *logtest.Sink) {
	t.Helper()
	if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
		t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
	}
	if err := syscall.Kill(h.child(), 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("the child %d outlived Run: %v", h.child(), err)
	}
	for _, msg := range []string{"child did not exit", "terminal reset"} {
		if recs := sink.Records().Matching("attach", msg); len(recs) > 0 {
			t.Errorf("the attach logged %q: %+v", msg, recs)
		}
	}
}

// assertPanicLogged checks the attach logged a panic with value, and its end.
func assertPanicLogged(t *testing.T, sink *logtest.Sink, value string) {
	t.Helper()
	rec := sink.Records().Matching("attach", "panic").AtExactLevel(slog.LevelWarn).Only(t, "panic record")
	if got := rec.AttrString(t, "error"); got != value {
		t.Errorf("panic error = %q, want %q", got, value)
	}
	if got := rec.AttrString(t, "detail"); !strings.Contains(got, "goroutine") {
		t.Errorf("panic detail = %q, want the stack", got)
	}
	assertErrorOutcome(t, sink, "panic", value)
}

// assertErrorOutcome checks the attach logged an outcome for an end that
// made Run return an error.
func assertErrorOutcome(t *testing.T, sink *logtest.Sink, reason, errorText string) {
	t.Helper()
	rec := sink.Records().Matching("attach", "outcome").AtExactLevel(slog.LevelInfo).Only(t, "outcome record")
	if got := rec.AttrString(t, "reason"); got != reason {
		t.Errorf("outcome reason = %q, want %q", got, reason)
	}
	if got := rec.ErrorAttr(t, "error"); got == nil || !strings.Contains(got.Error(), errorText) {
		t.Errorf("outcome error = %v, want one saying %q", got, errorText)
	}
	if rec.HasAttr("code") {
		t.Errorf("outcome has a code for an end that returns an error")
	}
}

func TestRun_TerminalHangingUpEndsTheAttachAsSIGHUP(t *testing.T) {
	keyboard, input, err := pty.Open()
	if err != nil {
		t.Fatalf("open the input terminal: %v", err)
	}
	t.Cleanup(func() { _, _ = keyboard.Close(), input.Close() })
	if err := unix.SetNonblock(fdOf(t, input), false); err != nil {
		t.Fatal(err)
	}
	h := attach(t, modeZmx, func(_ *harness, cfg *runner.Config) { cfg.In = input })
	h.ready(t)

	// The input terminal hangs up, as one whose window closed does, with no
	// SIGHUP to Portal, which is not its process.
	_ = keyboard.Close()
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{ExitCode: 128 + int(syscall.SIGHUP)}) {
		t.Errorf("Run = %+v, %v; want SIGHUP's outcome", out, err)
	}
	if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
		t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
	}
}

func TestRun_ContextEndDetachesAndRestores(t *testing.T) {
	sink := logtest.Install(t)
	ctx, cancel := context.WithCancel(t.Context())
	h := attachWithin(t, ctx, modeZmx)
	h.ready(t)

	cancel()
	_, err := h.finish(t)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want the context's", err)
	}
	if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
		t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
	}
	assertErrorOutcome(t, sink, "context", context.Canceled.Error())
}

func TestRun_LeavesTheFlagsOfTheTerminalsDescriptionsAlone(t *testing.T) {
	tm := newTerminal(t)
	// Output on a description apart from the input's, so each is checked on
	// its own.
	out := tm.another(t)
	h := attachOn(t, t.Context(), tm, fakeChildCommand(t, modeZmx), func(_ *harness, cfg *runner.Config) { cfg.Out = out })
	h.ready(t)

	h.assertFlags(t)

	h.detach(t) // and checks them after
}

func TestRun_ResetsATerminalTheChildsLastOutputNeverReached(t *testing.T) {
	runner.SetGrace(t, shortGrace)
	sink := logtest.Install(t)
	tm := openTerminal(t)
	h := attachOn(t, t.Context(), tm, fakeChildCommand(t, modeSpill))
	h.readyUnrecorded(t)

	// The terminal stalls, then the child writes its last output and exits
	// by its own hand, as zmx does with its own reset last.
	tm.fill(t)
	tm.typeIn(t, "x")
	reset := waitForReset(t, sink)
	// Longer than any of the runner's waits: the reset waits for the terminal
	// with none.
	time.Sleep(2 * shortGrace)
	tm.record() // the terminal takes output again
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Dropped: true}) {
		t.Errorf("Run = %+v, %v; want the child's own clean exit", out, err)
	}
	if got := reset.AttrString(t, "reason"); got != "drain" {
		t.Errorf("reset reason = %q, want drain", got)
	}
	tm.await(t, terminalReset)
	shown := tm.output()
	if bytes.Contains(shown, []byte(lastOutput)) {
		t.Errorf("the child's last output reached the terminal, want it cut by the grace")
	}
	if !bytes.HasSuffix(shown, []byte(terminalReset)) {
		t.Errorf("the terminal shows %q last, want the reset", shown[max(len(shown)-64, 0):])
	}
}

func TestRun_StopsWaitingForATerminalThatHangsUp(t *testing.T) {
	runner.SetGrace(t, shortGrace)
	sink := logtest.Install(t)
	tm := openTerminal(t)
	h := attachOn(t, t.Context(), tm, fakeChildCommand(t, modeSpill))
	h.readyUnrecorded(t)
	tm.fill(t)
	tm.typeIn(t, "x")
	waitForReset(t, sink)

	// The terminal's window closes before it takes the reset.
	_ = tm.user.Close()
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Dropped: true}) {
		t.Errorf("Run = %+v, %v; want the child's own clean exit", out, err)
	}
}

func TestRun_DeliversOutputThatOutlivesTheChild(t *testing.T) {
	sink := logtest.Install(t)
	tm := openTerminal(t)
	h := attachOn(t, t.Context(), tm, fakeChildCommand(t, modeSpill))
	h.readyUnrecorded(t)
	tm.fill(t)
	tm.typeIn(t, "x")

	// The child exits by its own hand with its last output still on its way,
	// and the terminal takes it within the grace.
	if !harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
		return errors.Is(syscall.Kill(h.child(), 0), syscall.ESRCH)
	}) {
		t.Fatal("the child never exited")
	}
	tm.record()
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Dropped: true}) {
		t.Errorf("Run = %+v, %v; want the child's own clean exit", out, err)
	}
	tm.await(t, lastOutput)
	if shown := tm.output(); !bytes.HasSuffix(shown, []byte(lastOutput)) {
		t.Errorf("the terminal shows %q last, want the child's last output", shown[max(len(shown)-64, 0):])
	}
	if recs := sink.Records().Matching("attach", "terminal reset"); len(recs) > 0 {
		t.Errorf("the runner reset a terminal the child's last output reached: %+v", recs)
	}
}

func TestRun_ResetsTheTerminalForAChildThatExitedOnlyWhenSignalled(t *testing.T) {
	runner.SetGrace(t, shortGrace)
	sink := logtest.Install(t)
	h := attach(t, modeHungUpClean)
	h.ready(t)

	h.term.typeIn(t, keyDetach)
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
		t.Errorf("Run = %+v, %v; want the detach", out, err)
	}
	// Its status is 0, but the runner's SIGHUP ended it, so zmx would have
	// written no reset.
	if got := waitForReset(t, sink).AttrString(t, "reason"); got != "exit" {
		t.Errorf("reset reason = %q, want exit", got)
	}
	h.term.await(t, terminalReset)
}

func TestRun_ResetsATerminalThatRefusedTheChildsOutput(t *testing.T) {
	sink := logtest.Install(t)
	tm, screen := newTerminal(t), openTerminal(t)
	h := attachOn(t, t.Context(), tm, fakeChildCommand(t, modeSpill), func(_ *harness, cfg *runner.Config) { cfg.Out = screen.tty })
	h.started(screen.readUntil(t, readyLine))

	// The output's terminal refuses what it is sent, while the input's lives
	// on.
	_ = screen.user.Close()
	tm.typeIn(t, "x")
	out, err := h.finish(t)

	if err != nil || out != (runner.Outcome{Dropped: true}) {
		t.Errorf("Run = %+v, %v; want the child's own clean exit", out, err)
	}
	if got := waitForReset(t, sink).AttrString(t, "reason"); got != "drain" {
		t.Errorf("reset reason = %q, want drain", got)
	}
}

func TestRun_StalledTerminalLeavesNoCopyLoop(t *testing.T) {
	runner.SetGrace(t, shortGrace)
	sink := logtest.Install(t)
	ctx, cancel := context.WithCancel(t.Context())
	tm := openTerminal(t)
	h := attachOn(t, ctx, tm, fakeChildCommand(t, modeFlood))

	// The terminal takes none of the flood, so the runner's copy waits on it.
	user := fdOf(t, tm.user)
	if !harnesstest.PollUntil(t, settle, 10*time.Millisecond, func() bool { return unread(user) }) {
		t.Fatal("the flood never reached the terminal")
	}
	time.Sleep(200 * time.Millisecond)
	cancel()
	// The runner's last write waits for the terminal to take it.
	waitForReset(t, sink)
	tm.record()
	_, err := h.finish(t)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want the context's", err)
	}
	tm.await(t, terminalReset)
	if shown := tm.output(); !bytes.HasSuffix(shown, []byte(terminalReset)) {
		t.Errorf("the terminal shows %q last, want the reset", shown[max(len(shown)-64, 0):])
	}
}

// waitForReset waits for the runner to start writing its reset to the
// terminal, and returns the record saying so.
func waitForReset(t *testing.T, sink *logtest.Sink) logtest.Record {
	t.Helper()
	if !harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
		return len(sink.Records().Matching("attach", "terminal reset")) > 0
	}) {
		t.Fatal("the runner never reset the terminal")
	}
	return sink.Records().Matching("attach", "terminal reset").AtExactLevel(slog.LevelDebug).Only(t, "reset record")
}

func TestRun_FailedStartRestoresTheTerminal(t *testing.T) {
	tm := newTerminal(t)
	before, flags := stateOf(t, tm.tty), flagsOf(t, tm.tty)

	_, err := runner.Run(t.Context(), runner.Config{
		Session: "fake",
		Command: backend.Command{Argv: []string{"/nonexistent/zmx", "attach", "fake"}},
		In:      tm.tty,
		Out:     tm.tty,
		Scanner: keys,
	})

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Run error = %v, want the failed start", err)
	}
	if after := stateOf(t, tm.tty); *after != *before {
		t.Errorf("the terminal's settings are %+v after a failed start, want %+v", after, before)
	}
	if got := flagsOf(t, tm.tty); got != flags {
		t.Errorf("the terminal's flags are %#x after a failed start, want %#x", got, flags)
	}
}

func TestRun_RefusesWithoutStarting(t *testing.T) {
	notTerminal, typed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = notTerminal.Close(), typed.Close() })
	tm := newTerminal(t)

	for _, tc := range []struct {
		name   string
		change func(*runner.Config)
		want   func(error) bool
	}{
		{"input that is not a terminal", func(cfg *runner.Config) { cfg.In = notTerminal }, isNoTerminal},
		{"output that is not a terminal", func(cfg *runner.Config) { cfg.Out = typed }, isNoTerminal},
		{"a command that switches sessions", func(cfg *runner.Config) { cfg.Command.Switches = true }, says("switches sessions")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := runner.Config{
				Session: "fake",
				// A start would fail on this command, with an error of its own.
				Command: backend.Command{Argv: []string{"/nonexistent/zmx", "attach", "fake"}},
				In:      tm.tty,
				Out:     tm.tty,
				Scanner: keys,
			}
			tc.change(&cfg)

			_, err := runner.Run(t.Context(), cfg)

			if !tc.want(err) || errors.Is(err, os.ErrNotExist) {
				t.Errorf("Run error = %v, want the refusal before any start", err)
			}
		})
	}
	if got := tm.output(); len(got) > 0 {
		t.Errorf("Run wrote %q to its output", got)
	}
}

func TestSameDevice(t *testing.T) {
	tm, other := newTerminal(t), newTerminal(t)
	pipe, _, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipe.Close() })
	tty := fdOf(t, tm.tty)

	for _, tc := range []struct {
		name string
		f    *os.File
		want bool
	}{
		{"another description of the terminal", tm.another(t), true},
		{"another terminal", other.tty, false},
		{"a pipe", pipe, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runner.SameDevice(tty, fdOf(t, tc.f)); got != tc.want {
				t.Errorf("SameDevice = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSameNode(t *testing.T) {
	node := func(change func(*unix.Stat_t)) *unix.Stat_t {
		s := &unix.Stat_t{Mode: unix.S_IFCHR | 0o620, Dev: 7, Ino: 1201, Rdev: 0x10000003}
		change(s)
		return s
	}
	same := node(func(*unix.Stat_t) {})

	for _, tc := range []struct {
		name   string
		change func(*unix.Stat_t)
		want   bool
	}{
		{"the same node", func(*unix.Stat_t) {}, true},
		{"another device number", func(s *unix.Stat_t) { s.Rdev++ }, false},
		{"the same number on another devpts", func(s *unix.Stat_t) { s.Dev++ }, false},
		{"another node of the same number", func(s *unix.Stat_t) { s.Ino++ }, false},
		{"not a character device", func(s *unix.Stat_t) { s.Mode = unix.S_IFREG | 0o620 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runner.SameNode(same, node(tc.change)); got != tc.want {
				t.Errorf("SameNode = %v, want %v", got, tc.want)
			}
		})
	}
}

func isNoTerminal(err error) bool { return errors.Is(err, runner.ErrNoTerminal) }

func says(text string) func(error) bool {
	return func(err error) bool { return err != nil && strings.Contains(err.Error(), text) }
}

func TestRun_LogsTheAttach(t *testing.T) {
	sink := logtest.Install(t)
	h := attach(t, modeZmx)
	h.ready(t)

	h.detach(t)

	started := sink.Records().Matching("attach", "started").AtExactLevel(slog.LevelDebug).Only(t, "started record")
	if got := started.AttrString(t, "session"); got != "fake" {
		t.Errorf("started session = %q, want fake", got)
	}
	if got := started.IntAttr(t, "target_pid"); got != int64(h.child()) {
		t.Errorf("started target_pid = %d, want the child's %d", got, h.child())
	}
	ended := sink.Records().Matching("attach", "ended").AtExactLevel(slog.LevelDebug).Only(t, "ended record")
	ended.RequireDuration(t, "took")
}
