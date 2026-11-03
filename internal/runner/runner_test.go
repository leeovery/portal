package runner_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
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

func TestRun_BoundKey(t *testing.T) {
	for _, tc := range []struct {
		name       string
		key        string
		want       attachkeys.Action
		wantPicker bool
		reason     string
	}{
		{"detach goes to the picker", keyDetach, attachkeys.Detach, true, "detach"},
		{"detach to shell goes to the shell", keyDetachToShell, attachkeys.DetachToShell, false, "detach_to_shell"},
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

func TestRun_Kill(t *testing.T) {
	t.Run("a kill that succeeds waits for the session's end to end the child", func(t *testing.T) {
		var killed []string
		h := attach(t, modeZmx, func(h *harness, cfg *runner.Config) {
			cfg.Kill = func(_ context.Context, name string) error {
				killed = append(killed, name)
				return syscall.Kill(h.child(), syscall.SIGUSR1)
			}
		})
		h.ready(t)

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Kill}) {
			t.Errorf("Run = %+v, %v; want a kill with no error", out, err)
		}
		if !out.Picker() {
			t.Errorf("Picker() = false after a kill, want the picker")
		}
		if !slices.Equal(killed, []string{"fake"}) {
			t.Errorf("killed %q, want the attached session once", killed)
		}
		if slices.Contains(h.term.reads(t), "\x1c") {
			t.Errorf("the child was sent ctrl+\\ after a kill that succeeded")
		}
		if err := syscall.Kill(h.child(), 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("the child %d outlived Run: %v", h.child(), err)
		}
	})

	t.Run("a kill that fails detaches the child and carries the error", func(t *testing.T) {
		refused := errors.New("kill refused")
		sink := logtest.Install(t)
		h := attach(t, modeZmx, func(_ *harness, cfg *runner.Config) {
			cfg.Kill = func(context.Context, string) error { return refused }
		})
		h.ready(t)

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || out.Action != attachkeys.Kill || !errors.Is(out.Err, refused) {
			t.Errorf("Run = %+v, %v; want a kill carrying %v", out, err, refused)
		}
		if !out.Picker() {
			t.Errorf("Picker() = false after a failed kill, want the picker to show the error")
		}
		if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
			t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
		}
		rec := sink.Records().Matching("attach", "outcome").Only(t, "outcome record")
		if got := rec.ErrorAttr(t, "error"); !errors.Is(got, refused) {
			t.Errorf("outcome error = %v, want %v", got, refused)
		}
	})

	t.Run("a signal to Portal during the kill ends the attach as that signal", func(t *testing.T) {
		h := attach(t, modeZmx, func(h *harness, cfg *runner.Config) {
			cfg.Kill = func(context.Context, string) error {
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
				time.Sleep(100 * time.Millisecond) // the signal lands while the kill runs
				return syscall.Kill(h.child(), syscall.SIGUSR1)
			}
		})
		h.ready(t)

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{ExitCode: 128 + int(syscall.SIGTERM)}) {
			t.Errorf("Run = %+v, %v; want the SIGTERM's outcome", out, err)
		}
	})

	t.Run("a context that ends during the kill ends the attach with its error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		h := attachWithin(t, ctx, modeZmx, func(_ *harness, cfg *runner.Config) {
			cfg.Kill = func(ctx context.Context, _ string) error {
				cancel()
				return ctx.Err()
			}
		})
		h.ready(t)

		h.term.typeIn(t, keyKill)
		out, err := h.finish(t)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %+v, %v; want the context's error", out, err)
		}
		if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
			t.Errorf("the child read %q, want ctrl+\\ to detach it", got)
		}
	})
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
	refused := errors.New("kill refused")
	var killReturned atomic.Int64
	h := attach(t, modeFrozen, func(h *harness, cfg *runner.Config) {
		// The kill ends the child, input still unread in its terminal, and
		// fails, so the child is detached after its end.
		cfg.Kill = func(context.Context, string) error {
			_ = syscall.Kill(h.child(), syscall.SIGUSR1)
			harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
				return errors.Is(syscall.Kill(h.child(), 0), syscall.ESRCH)
			})
			killReturned.Store(time.Now().UnixNano())
			return refused
		}
	})
	h.ready(t)

	h.term.typeIn(t, "abc"+keyKill)
	out, err := h.finish(t)

	if err != nil || !errors.Is(out.Err, refused) {
		t.Errorf("Run = %+v, %v; want the failed kill", out, err)
	}
	if took := time.Since(time.Unix(0, killReturned.Load())); took > time.Second {
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
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("find the test binary: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*settle)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), afterRunEnv+"=1")

	out, _ := cmd.CombinedOutput()

	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || !status.Signaled() || status.Signal() != syscall.SIGINT {
		t.Errorf("a SIGINT after Run returned left the process %v, want it ended: %s", cmd.ProcessState, out)
	}
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
	sink.Records().Matching("attach", "panic").AtExactLevel(slog.LevelError).Only(t, "panic record")
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
}

func TestRun_StalledTerminalLeavesNoCopyLoop(t *testing.T) {
	runner.SetGrace(t, shortGrace)
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
	_, err := h.finish(t)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error = %v, want the context's", err)
	}
}

func TestRun_FailedStartRestoresTheTerminal(t *testing.T) {
	tm := newTerminal(t)
	before := stateOf(t, tm.tty)

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
	if nonblockingOf(t, tm.tty) {
		t.Errorf("the terminal is non-blocking after a failed start, want it blocking again")
	}
}

func TestRun_RefusesWithoutStarting(t *testing.T) {
	in, typed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = in.Close(), typed.Close() })
	tm := newTerminal(t)
	// A start would fail on this command, with an error of its own.
	missing := backend.Command{Argv: []string{"/nonexistent/zmx", "attach", "fake"}}

	for _, tc := range []struct {
		name    string
		in      *os.File
		command backend.Command
		want    func(error) bool
	}{
		{"input that is not a terminal", in, missing, func(err error) bool { return errors.Is(err, runner.ErrNoTerminal) }},
		{"a command that switches sessions", tm.tty, backend.Command{Argv: missing.Argv, Switches: true}, func(err error) bool {
			return err != nil && strings.Contains(err.Error(), "switches sessions")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runner.Run(t.Context(), runner.Config{Session: "fake", Command: tc.command, In: tc.in, Out: tm.tty, Scanner: keys})

			if !tc.want(err) || errors.Is(err, os.ErrNotExist) {
				t.Errorf("Run error = %v, want the refusal before any start", err)
			}
		})
	}
	if got := tm.output(); len(got) > 0 {
		t.Errorf("Run wrote %q to its output", got)
	}
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
