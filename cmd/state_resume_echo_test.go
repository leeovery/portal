package cmd

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestRunResumeDraw_Echo(t *testing.T) {
	screens := []struct {
		name    string
		payload func() resumeChainPayload
	}{
		{"the first draw", samplePayload},
		{"the discard confirmation", func() resumeChainPayload {
			p := drawScreenPayload(resumeScreenDiscard)
			p.DropInput = true
			return p
		}},
		{"a report", func() resumeChainPayload {
			return reportedOn(samplePayload(), resumeScreenPanel, "could not clear the pending marker")
		}},
		{"a resize redraw", func() resumeChainPayload {
			p := samplePayload()
			p.Width, p.Height = 80, 20
			return p
		}},
	}
	for _, s := range screens {
		t.Run(s.name+" turns echo off before the appearance query", func(t *testing.T) {
			var probe resumeDrawProbe
			cfg := newResumeDrawConfig(t, &probe, s.payload(), fixedSize(100, 30))
			cfg.DropInputQueue = func() error { return nil }

			if err := runResumeDraw(cfg); err != nil {
				t.Fatalf("runResumeDraw() error = %v", err)
			}

			if probe.echoOffCalls != 1 {
				t.Fatalf("echo turned off %d times, want exactly 1", probe.echoOffCalls)
			}
			if probe.echoOffsAtResolve != 1 {
				t.Error("the appearance query ran before echo was turned off, want after")
			}
			if probe.execsAtEchoOff != 0 {
				t.Error("echo turned off after the hand-off exec, want before")
			}
		})
	}

	t.Run("a refused echo step logs one WARN and still paints and hands the pane to the waiter", func(t *testing.T) {
		var probe resumeDrawProbe
		payload := samplePayload()
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := newResumeDrawConfig(t, &probe, payload, fixedSize(100, 30))
		cfg.Logger = logger
		probe.echoOffErr = errors.New("inappropriate ioctl for device")

		if err := runResumeDraw(cfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		warn := sink.Records().WithMessage("disable terminal echo failed").Only(t, "the refused echo step")
		if got := warn.AttrOrEmpty("component"); got != "hydrate" {
			t.Errorf("WARN component = %q, want hydrate", got)
		}
		if got := warn.AttrOrEmpty("pane_key"); got != payload.PaneKey {
			t.Errorf("WARN pane_key = %q, want %q", got, payload.PaneKey)
		}
		if got := warn.AttrOrEmpty("hook_key"); got != payload.HookKey {
			t.Errorf("WARN hook_key = %q, want %q", got, payload.HookKey)
		}
		if probe.stdout.Len() == 0 {
			t.Error("the draw painted nothing after its echo step was refused")
		}
		if probe.execCalls != 1 {
			t.Errorf("ExecSelf called %d times, want the pane handed to the waiter", probe.execCalls)
		}
	})
}

func TestResumeWait_EchoOnHandingThePaneOn(t *testing.T) {
	answers := []struct {
		name    string
		payload resumeChainPayload
		keys    string
	}{
		{"Enter", samplePayload(), "\r"},
		{"the confirmed discard", confirmationPayload(), "y"},
	}
	for _, a := range answers {
		t.Run(a.name+" turns echo back on after the raw restore and before the exec", func(t *testing.T) {
			var probe resumeWaitProbe
			if err := runResumeWait(newResumeWaitConfig(t, &probe, a.payload, keystrokes(t, a.keys))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			if probe.echoOnCalls != 1 {
				t.Fatalf("echo turned on %d times, want exactly 1", probe.echoOnCalls)
			}
			if probe.restoresAtEchoOn != 1 {
				t.Errorf("%d raw restores had run when echo turned on, want 1: the restore puts back the echo-off tty the waiter found", probe.restoresAtEchoOn)
			}
			if probe.execsAtEchoOn != 0 {
				t.Error("echo turned on after the exec, want before")
			}
			if probe.signalsOnCalls != 1 {
				t.Errorf("signal generation turned on %d times, want exactly 1", probe.signalsOnCalls)
			}
		})

		t.Run(a.name+" still hands the pane on when echo cannot be turned on", func(t *testing.T) {
			var probe resumeWaitProbe
			probe.echoOnErr = errors.New("inappropriate ioctl for device")
			if err := runResumeWait(newResumeWaitConfig(t, &probe, a.payload, keystrokes(t, a.keys))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			if probe.execCalls != 1 {
				t.Errorf("ExecSelf called %d times, want the pane handed on", probe.execCalls)
			}
			warn := probe.sink.Records().WithMessage("enable terminal echo failed").Only(t, "the failed enable")
			if got := warn.AttrOrEmpty("pane_key"); got != a.payload.PaneKey {
				t.Errorf("WARN pane_key = %q, want %q", got, a.payload.PaneKey)
			}
			if got := warn.AttrOrEmpty("hook_key"); got != a.payload.HookKey {
				t.Errorf("WARN hook_key = %q, want %q", got, a.payload.HookKey)
			}
		})
	}
}

func TestResumeWait_EchoStaysOffAcrossScreens(t *testing.T) {
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
		t.Run(tc.name+" redraws with echo still off", func(t *testing.T) {
			var probe resumeWaitProbe
			if tc.prepare != nil {
				tc.prepare(&probe)
			}
			if err := runResumeWait(newResumeWaitConfig(t, &probe, tc.payload, keystrokes(t, tc.keys))); err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}

			assertRedrawnWithEchoOff(t, &probe)
		})
	}

	t.Run("Escape on the confirmation redraws with echo still off", func(t *testing.T) {
		var probe resumeWaitProbe
		cfg := newResumeWaitConfig(t, &probe, confirmationPayload(), heldInput(t, "\x1b"))
		cfg.AwaitInput = quietAtOnce

		if err := runResumeWait(cfg); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		assertRedrawnWithEchoOff(t, &probe)
	})

	t.Run("a resize redraws with echo still off", func(t *testing.T) {
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

		assertRedrawnWithEchoOff(t, &probe)
	})
}

func assertRedrawnWithEchoOff(t *testing.T, probe *resumeWaitProbe) {
	t.Helper()
	exe, err := resumeChainExe()
	if err != nil {
		t.Fatalf("resumeChainExe() error = %v", err)
	}
	if probe.execCalls != 1 || probe.execProg != exe {
		t.Fatalf("exec = %d × %q, want one redraw %q", probe.execCalls, probe.execProg, exe)
	}
	if probe.echoOnCalls != 0 {
		t.Errorf("echo turned on %d times on a redraw, want 0", probe.echoOnCalls)
	}
}
