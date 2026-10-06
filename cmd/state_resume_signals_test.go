package cmd

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/resumemode"
)

// signalsSeam records a terminal-signal seam: how often it ran, and whether the
// exec it must precede had already happened.
type signalsSeam struct {
	calls     int
	afterExec bool
}

// stubChainExe stands in for the portal binary under a parked chain: its draw
// signals the chain's whole process group and records whether it outlived the
// signal, and its recovery tail records that it ran.
const stubChainExe = `#!/bin/sh
case "$2" in
resume-draw)
	kill -"$CHAIN_SIGNAL" 0
	sleep 1
	echo DRAW-SURVIVED
	;;
resume-recover)
	echo RECOVER-RAN
	;;
esac
`

func TestParkedResumeChain_GroupSignal(t *testing.T) {
	for _, signal := range []string{"INT", "QUIT"} {
		t.Run("a SIG"+signal+" to the whole group while the draw runs ends in the recovery tail", func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "portal")
			if err := os.WriteFile(exe, []byte(stubChainExe), 0o700); err != nil {
				t.Fatalf("stage the stub executable: %v", err)
			}

			parked := exec.Command("/bin/sh", "-c", parkedResumeChain(exe, samplePayload()))
			parked.Env = append(os.Environ(), "CHAIN_SIGNAL="+signal)
			parked.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			out, err := parked.Output()

			if err != nil {
				t.Fatalf("the parked shell died: %v (output %q)", err, out)
			}
			if !strings.Contains(string(out), "RECOVER-RAN") {
				t.Errorf("the recovery tail did not run; output %q", out)
			}
			if strings.Contains(string(out), "DRAW-SURVIVED") {
				t.Errorf("the draw outlived SIG%s: the parked shell's handling reached its child; output %q", signal, out)
			}
		})
	}
}

func TestHydrateLazy_ClearsSignalGenerationBeforeTheFirstDraw(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			stub := &stubExecShell{}
			var seam signalsSeam
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), stub)
			opts.DisableTTYSignals = seam.record(stub)

			lazyRun(t, tail, opts)

			if seam.calls != 1 {
				t.Fatalf("signal generation cleared %d times, want exactly 1", seam.calls)
			}
			if seam.afterExec {
				t.Error("signal generation was cleared after the chain was exec'd, want before")
			}
			if !stub.called || stub.target != "/bin/sh" {
				t.Errorf("exec = %q, want the parked chain", stub.target)
			}
		})
	}
}

func TestHydrateLazy_ParksTheChainBehindACaughtTrap(t *testing.T) {
	stub := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), stub)

	lazyRun(t, lazyTails()[0], opts)

	if len(stub.args) != 3 || !strings.HasPrefix(stub.args[2], "trap : INT QUIT TERM; ") {
		t.Errorf("parked chain = %q, want it to open with a caught trap on INT, QUIT and TERM", stub.args)
	}
}

func TestHydrateLazy_ParksTheChainWhenSignalGenerationCannotBeCleared(t *testing.T) {
	stub := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), stub)
	opts.Logger = logger
	opts.DisableTTYSignals = func() error { return errors.New("inappropriate ioctl for device") }

	paneKey := lazyRun(t, lazyTails()[0], opts)

	if want := lazyChainArgs("echo hi", paneKey); !slices.Equal(stub.args, want) {
		t.Errorf("exec args = %q, want the parked chain %q", stub.args, want)
	}
	warn := execLogLine(t, sink.Body(), "WARN", "disable terminal signals failed")
	if !strings.Contains(warn, "pane_key="+paneKey) || !strings.Contains(warn, "inappropriate ioctl") {
		t.Errorf("WARN = %q, want it to name the pane and the error", warn)
	}
	if !strings.Contains(warn, "hook_key="+lazyHookKey) {
		t.Errorf("WARN = %q, want it to name the pane by its durable token hook_key=%s", warn, lazyHookKey)
	}
}

func TestHydrateLazy_LeavesSignalGenerationAloneOnAPaneThatNeverWaits(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, stub *stubExecShell) hydrateCfgOpts
	}{
		{name: "an eager registration", stage: func(t *testing.T, stub *stubExecShell) hydrateCfgOpts {
			return lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Eager), lazyPrefs(t, ""), stub)
		}},
		{name: "no registration", stage: func(t *testing.T, stub *stubExecShell) hydrateCfgOpts {
			store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Body: map[string]map[string]string{}})
			return lazyOpts(t, store, lazyPrefs(t, ""), stub)
		}},
		{name: "a pending marker that could not be written", stage: func(t *testing.T, stub *stubExecShell) hydrateCfgOpts {
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), stub)
			opts.Commander = commandertest.Quiet(commandertest.Fails(errors.New("tmux refused"), "set-option", "-p"))
			return opts
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubExecShell{}
			var seam signalsSeam
			opts := tc.stage(t, stub)
			opts.DisableTTYSignals = seam.record(stub)

			lazyRun(t, lazyTails()[0], opts)

			if seam.calls != 0 {
				t.Errorf("signal generation cleared %d times on a pane that never waits, want 0", seam.calls)
			}
		})
	}
}

func (s *signalsSeam) record(stub *stubExecShell) func() error {
	return func() error {
		s.calls++
		s.afterExec = stub.called
		return nil
	}
}

func TestResumeWait_SignalGenerationOnHandingThePaneOn(t *testing.T) {
	answers := []struct {
		name    string
		payload resumeChainPayload
		keys    string
	}{
		{"Enter", samplePayload(), "\r"},
		{"the confirmed discard", confirmationPayload(), "y"},
	}
	for _, a := range answers {
		t.Run(a.name+" turns signal generation back on after the raw restore and before the exec", func(t *testing.T) {
			var probe resumeWaitProbe
			if err := runResumeWait(newResumeWaitConfig(t, &probe, a.payload, keystrokes(t, a.keys))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			if probe.signalsOnCalls != 1 {
				t.Fatalf("signal generation turned on %d times, want exactly 1", probe.signalsOnCalls)
			}
			if probe.restoresAtSignalsOn != 1 {
				t.Errorf("%d raw restores had run when signal generation turned on, want 1: the restore puts back the state the waiter found", probe.restoresAtSignalsOn)
			}
			if probe.execsAtSignalsOn != 0 {
				t.Error("signal generation turned on after the exec, want before")
			}
			if probe.execCalls != 1 {
				t.Errorf("ExecSelf called %d times, want 1", probe.execCalls)
			}
		})

		t.Run(a.name+" still hands the pane on when signal generation cannot be turned on", func(t *testing.T) {
			var probe resumeWaitProbe
			probe.signalsOnErr = errors.New("inappropriate ioctl for device")
			if err := runResumeWait(newResumeWaitConfig(t, &probe, a.payload, keystrokes(t, a.keys))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			if probe.execCalls != 1 {
				t.Errorf("ExecSelf called %d times, want the pane handed on", probe.execCalls)
			}
			warn := probe.sink.Records().WithMessage("enable terminal signals failed").Only(t, "the failed enable")
			if got := warn.AttrOrEmpty("pane_key"); got != a.payload.PaneKey {
				t.Errorf("WARN pane_key = %q, want %q", got, a.payload.PaneKey)
			}
			if got := warn.AttrOrEmpty("hook_key"); got != a.payload.HookKey {
				t.Errorf("WARN hook_key = %q, want %q", got, a.payload.HookKey)
			}
		})
	}
}

func TestResumeWait_SignalGenerationStaysOffAcrossScreens(t *testing.T) {
	cases := []struct {
		name    string
		payload resumeChainPayload
		keys    string
		prepare func(p *resumeWaitProbe)
	}{
		{name: "d opening the confirmation", payload: samplePayload(), keys: "d"},
		{name: "a refused clear on Enter", payload: samplePayload(), keys: "\r", prepare: func(p *resumeWaitProbe) {
			p.clearErr = errors.New("can't find pane: %7")
		}},
		{name: "a refused clear on a confirmed discard", payload: confirmationPayload(), keys: "y", prepare: func(p *resumeWaitProbe) {
			p.clearErr = errors.New("can't find pane: %7")
		}},
		{name: "a refused discard", payload: confirmationPayload(), keys: "y", prepare: func(p *resumeWaitProbe) {
			p.discardErr = errors.New("hooks.json: permission denied")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" redraws with signal generation still off", func(t *testing.T) {
			var probe resumeWaitProbe
			if tc.prepare != nil {
				tc.prepare(&probe)
			}
			if err := runResumeWait(newResumeWaitConfig(t, &probe, tc.payload, keystrokes(t, tc.keys))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			assertRedrawnWithSignalsOff(t, &probe)
		})
	}

	t.Run("Escape on the confirmation redraws with signal generation still off", func(t *testing.T) {
		var probe resumeWaitProbe
		cfg := newResumeWaitConfig(t, &probe, confirmationPayload(), heldInput(t, "\x1b"))
		cfg.AwaitInput = quietAtOnce

		if err := runResumeWait(cfg); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		assertRedrawnWithSignalsOff(t, &probe)
	})

	t.Run("a resize redraws with signal generation still off", func(t *testing.T) {
		var probe resumeWaitProbe
		cfg := newResumeWaitConfig(t, &probe, confirmationPayload(), heldInput(t, ""))
		winch := make(chan os.Signal, 1)
		winch <- syscall.SIGWINCH
		cfg.Winch = winch
		cfg.Settle = elapsedAtOnce
		cfg.Size = fixedSize(80, 20)

		if err := runResumeWait(cfg); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		assertRedrawnWithSignalsOff(t, &probe)
	})
}

func assertRedrawnWithSignalsOff(t *testing.T, probe *resumeWaitProbe) {
	t.Helper()
	exe, err := resumeChainExe()
	if err != nil {
		t.Fatalf("resumeChainExe() error = %v", err)
	}
	if probe.execCalls != 1 || probe.execProg != exe {
		t.Fatalf("exec = %d × %q, want one redraw %q", probe.execCalls, probe.execProg, exe)
	}
	if probe.signalsOnCalls != 0 {
		t.Errorf("signal generation turned on %d times on a redraw, want 0", probe.signalsOnCalls)
	}
}

// heldInput delivers typed and then holds the input open, so the wait ends on
// whatever it is waiting for rather than on end of input.
func heldInput(t *testing.T, typed string) io.Reader {
	t.Helper()
	r, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	go func() { _, _ = w.Write([]byte(typed)) }()
	return r
}

// quietAtOnce is a pane on which nothing follows the key just read.
func quietAtOnce(time.Duration) (bool, error) { return false, nil }

func elapsedAtOnce(time.Duration) <-chan time.Time {
	c := make(chan time.Time, 1)
	c <- time.Now()
	return c
}

func TestRunResumeRecover_SignalGeneration(t *testing.T) {
	t.Run("it turns signal generation back on before it hands the pane its shell", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		if probe.signalsOnCalls != 1 {
			t.Fatalf("signal generation turned on %d times, want exactly 1", probe.signalsOnCalls)
		}
		if probe.execsAtSignalsOn != 0 {
			t.Error("signal generation turned on after the exec, want before")
		}
		if probe.execCalls != 1 {
			t.Errorf("ExecShell called %d times, want 1", probe.execCalls)
		}
	})

	t.Run("it still hands the pane its shell when signal generation cannot be turned on", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"
		probe.signalsOnErr = errors.New("inappropriate ioctl for device")
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := newResumeRecoverConfig(t, &probe)
		cfg.Logger = logger

		if err := runResumeRecover(cfg); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		if probe.execCalls != 1 {
			t.Errorf("ExecShell called %d times, want 1", probe.execCalls)
		}
		warn := sink.Records().WithMessage("enable terminal signals failed").Only(t, "the failed enable")
		if got := warn.AttrOrEmpty("pane_key"); got != cfg.PaneKey {
			t.Errorf("WARN pane_key = %q, want %q", got, cfg.PaneKey)
		}
		if got := warn.AttrOrEmpty("hook_key"); got != cfg.HookKey {
			t.Errorf("WARN hook_key = %q, want %q", got, cfg.HookKey)
		}
	})

	t.Run("it leaves signal generation alone on a pane already answered", func(t *testing.T) {
		var probe resumeRecoverProbe

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		if probe.signalsOnCalls != 0 {
			t.Errorf("signal generation turned on %d times, want 0", probe.signalsOnCalls)
		}
	})
}
