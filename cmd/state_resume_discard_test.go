package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

func confirmationPayload() resumeChainPayload {
	p := samplePayload()
	p.Width, p.Height = 100, 30
	p.Screen = resumeScreenDiscard
	return p
}

func answerDiscard(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload) {
	t.Helper()
	if err := runResumeWait(newResumeWaitConfig(t, p, payload, keystrokes(t, "y"))); err != nil {
		t.Fatalf("runResumeWait() error = %v", err)
	}
}

func assertOrder(t *testing.T, p *resumeWaitProbe, want ...string) {
	t.Helper()
	if !slices.Equal(p.order, want) {
		t.Errorf("answer sequence = %q, want %q", p.order, want)
	}
}

// reportedOn is the redraw a refused step hands to: screen, carrying the
// refusal's report row and no input drop.
func reportedOn(p resumeChainPayload, screen, row string) resumeChainPayload {
	p.Screen = screen
	p.Report = row
	p.DropInput = false
	return p
}

func TestResumeAnswerDiscard_Removal(t *testing.T) {
	t.Run("it removes the registration before it writes anything", func(t *testing.T) {
		var probe resumeWaitProbe
		payload := confirmationPayload()
		answerDiscard(t, &probe, payload)

		if !slices.Equal(probe.discardKeys, []string{payload.HookKey}) {
			t.Fatalf("discard written for %q, want one write for %q", probe.discardKeys, payload.HookKey)
		}
		if len(probe.order) == 0 || probe.order[0] != "discard" {
			t.Errorf("answer sequence = %q, want the removal before anything reaches the pane", probe.order)
		}
	})

	t.Run("it hands the store the command the confirmation showed", func(t *testing.T) {
		var probe resumeWaitProbe
		payload := confirmationPayload()
		answerDiscard(t, &probe, payload)

		if !slices.Equal(probe.discardShown, []string{payload.Command}) {
			t.Errorf("discard handed %q, want one discard of the shown %q", probe.discardShown, payload.Command)
		}
	})

	t.Run("it treats nothing-to-remove as a discard", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.discardMiss = true
		answerDiscard(t, &probe, confirmationPayload())

		assertOrder(t, &probe, "discard", "stdout", "clear", "confirm", "unpin", "exec")
		if got := probe.stdout.String(); got != hydrateResetPreamble {
			t.Errorf("what the answer wrote to the pane = %q, want the leave sequence %q", got, hydrateResetPreamble)
		}
		assertShellHandOff(t, &probe)
	})
}

func TestResumeAnswerDiscard_RefusedWrite(t *testing.T) {
	writeErr := fmt.Errorf("%w: /x/hooks.json.lock", hooks.ErrLockHeld)
	const writeRow = "can't lock hooks.json: another process holds it"

	t.Run("it reports a refused write on the confirmation", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.discardErr = writeErr
		payload := confirmationPayload()
		answerDiscard(t, &probe, payload)

		assertHandOff(t, &probe, reportedOn(payload, resumeScreenDiscard, writeRow))
		assertArgvCarriesDropInput(t, probe.execArgs, false)
	})

	t.Run("it leaves the marker and the registration standing when the write is refused", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.discardErr = writeErr
		answerDiscard(t, &probe, confirmationPayload())

		assertOrder(t, &probe, "discard", "exec")
		if probe.clearCalls != 0 {
			t.Errorf("ClearMarker called %d times, want 0", probe.clearCalls)
		}
		if probe.stdout.Len() != 0 {
			t.Errorf("the answer wrote %q to the pane, want nothing: the confirmation stays up", probe.stdout.String())
		}
		if probe.execProg == resolveShell() {
			t.Errorf("exec target = %q, want no shell while the registration stands", probe.execProg)
		}
	})

	t.Run("it retries the removal on a second y from the report", func(t *testing.T) {
		var probe resumeWaitProbe
		payload := reportedOn(confirmationPayload(), resumeScreenDiscard, writeRow)
		answerDiscard(t, &probe, payload)

		if !slices.Equal(probe.discardKeys, []string{payload.HookKey}) {
			t.Fatalf("discard written for %q, want one write for %q", probe.discardKeys, payload.HookKey)
		}
		assertOrder(t, &probe, "discard", "stdout", "clear", "confirm", "unpin", "exec")
		assertShellHandOff(t, &probe)
	})
}

func TestResumeAnswerDiscard_Order(t *testing.T) {
	t.Run("it leaves the panel's screen before it clears the marker", func(t *testing.T) {
		var probe resumeWaitProbe
		answerDiscard(t, &probe, confirmationPayload())

		stdoutAt := slices.Index(probe.order, "stdout")
		clearAt := slices.Index(probe.order, "clear")
		if stdoutAt < 0 || clearAt < 0 || stdoutAt > clearAt {
			t.Errorf("answer sequence = %q, want the leave sequence written before the clear", probe.order)
		}
		if got := probe.stdout.String(); got != hydrateResetPreamble {
			t.Errorf("what the answer wrote to the pane = %q, want the leave sequence %q", got, hydrateResetPreamble)
		}
	})

	t.Run("it clears the marker and lifts the pin before it runs the shell", func(t *testing.T) {
		var probe resumeWaitProbe
		answerDiscard(t, &probe, confirmationPayload())

		assertOrder(t, &probe, "discard", "stdout", "clear", "confirm", "unpin", "exec")
	})
}

func TestResumeAnswerDiscard_FailedClear(t *testing.T) {
	clearErr := tmuxRefusal(t)
	const clearRow = "can't unpause this pane: can't find pane: %7"

	t.Run("it redraws the waiting panel with the reason when the clear fails", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.clearErr = clearErr
		payload := confirmationPayload()
		answerDiscard(t, &probe, payload)

		assertHandOff(t, &probe, reportedOn(payload, resumeScreenPanel, clearRow))
		assertArgvLacks(t, probe.execArgs, resumeFlagScreen)
		assertArgvCarriesDropInput(t, probe.execArgs, false)
	})

	t.Run("it names the removed command on that redraw", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.clearErr = clearErr
		payload := confirmationPayload()
		answerDiscard(t, &probe, payload)

		i := slices.Index(probe.execArgs, flagArg(resumeFlagCommand))
		if i < 0 || i+1 >= len(probe.execArgs) || probe.execArgs[i+1] != payload.Command {
			t.Fatalf("redraw argv = %q, want %s %q", probe.execArgs, flagArg(resumeFlagCommand), payload.Command)
		}

		var drawProbe resumeDrawProbe
		drawCfg := newResumeDrawConfig(t, &drawProbe, reportedOn(payload, resumeScreenPanel, clearRow), fixedSize(100, 30))
		drawCfg.Colourless = true
		if err := runResumeDraw(drawCfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}
		painted := drawProbe.stdout.String()
		for _, want := range []string{payload.Command, clearRow, "resume", "discard"} {
			if !strings.Contains(painted, want) {
				t.Errorf("the redrawn panel does not carry %q:\n%s", want, painted)
			}
		}
	})

	t.Run("it runs neither the shell nor anything else, and leaves the pin, when the clear fails", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.clearErr = clearErr
		answerDiscard(t, &probe, confirmationPayload())

		assertOrder(t, &probe, "discard", "stdout", "clear", "exec")
		if len(probe.lookupKeys) != 0 {
			t.Errorf("the store was read for %q, want no read deciding whether to paint", probe.lookupKeys)
		}
		exe, err := resumeChainExe()
		if err != nil {
			t.Fatalf("resumeChainExe() error = %v", err)
		}
		if probe.execProg != exe {
			t.Errorf("exec target = %q, want the redraw %q", probe.execProg, exe)
		}
	})
}

func TestResumeAnswerDiscard_HandOff(t *testing.T) {
	t.Run("it drops the pane to a plain shell with no wrapper", func(t *testing.T) {
		var probe resumeWaitProbe
		answerDiscard(t, &probe, confirmationPayload())

		assertShellHandOff(t, &probe)
	})

	t.Run("it makes no tmux call but the marker unset", func(t *testing.T) {
		var probe resumeWaitProbe
		answerDiscard(t, &probe, confirmationPayload())

		if probe.clearCalls != 1 {
			t.Errorf("ClearMarker called %d times, want exactly 1", probe.clearCalls)
		}
		if len(probe.lookupKeys) != 0 {
			t.Errorf("the store was read for %q, want only the removal", probe.lookupKeys)
		}
		if len(probe.discardKeys) != 1 {
			t.Errorf("DiscardRegistration called %d times, want exactly 1", len(probe.discardKeys))
		}
		assertOrder(t, &probe, "discard", "stdout", "clear", "confirm", "unpin", "exec")
	})

	t.Run("it restores the terminal before every exec", func(t *testing.T) {
		cases := []struct {
			name    string
			arrange func(p *resumeWaitProbe)
		}{
			{"a removal", func(*resumeWaitProbe) {}},
			{"a refused write", func(p *resumeWaitProbe) { p.discardErr = errors.New("is a directory") }},
			{"a clear that failed", func(p *resumeWaitProbe) { p.clearErr = errors.New("can't find pane: %7") }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var probe resumeWaitProbe
				tc.arrange(&probe)
				answerDiscard(t, &probe, confirmationPayload())

				if probe.execCalls != 1 {
					t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
				}
				if probe.restoredAtExec != 1 {
					t.Errorf("%d restores had run when the exec happened, want 1: the next process image must inherit a cooked tty", probe.restoredAtExec)
				}
			})
		}
	})

	t.Run("it terminates non-zero when the exec fails", func(t *testing.T) {
		t.Setenv("PORTAL_LOG_LEVEL", "info")
		sink := logtest.Install(t)

		var exitCode atomic.Int32
		exitCode.Store(-1)
		var exitCalls atomic.Int32
		withOsExitFake(t, func(code int) {
			exitCalls.Add(1)
			exitCode.Store(int32(code))
		})

		var probe resumeWaitProbe
		cfg := newResumeWaitConfig(t, &probe, confirmationPayload(), keystrokes(t, "y"))
		cfg.Logger = nil
		cfg.ExecSelf = func(_ string, args []string) {
			defaultExecShell("/nonexistent/portal-resume-discard-probe", args)
		}

		if err := runResumeWait(cfg); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		if got := exitCalls.Load(); got != 1 {
			t.Fatalf("osExit invoked %d times, want exactly 1", got)
		}
		if got := exitCode.Load(); got != 1 {
			t.Errorf("osExit code = %d, want 1", got)
		}
		if got := len(sink.Records().WithMessage("exec handoff failed")); got != 1 {
			t.Errorf("exec-failure WARN recorded %d times, want exactly 1", got)
		}
	})
}

func TestResumeAnswerDiscard_RewrittenEntry(t *testing.T) {
	t.Run("it leaves a registration rewritten while the pane waited and drops the pane to a shell", func(t *testing.T) {
		sink := logtest.Install(t)
		payload := confirmationPayload()
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{payload.HookKey: payload.Command + " --replaced"},
		})
		before := readFileBytes(t, path)

		var probe resumeWaitProbe
		cfg := newResumeWaitConfig(t, &probe, payload, keystrokes(t, "y"))
		cfg.DiscardRegistration = func(hookKey, shown string) (bool, error) {
			probe.order = append(probe.order, "discard")
			return store.Discard(hookKey, hooks.EventOnResume, shown, hooks.ViaPanel)
		}
		if err := runResumeWait(cfg); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a discard of a command it no longer holds")
		if got := sink.Records().Matching("hooks", "discard"); len(got) != 0 {
			t.Errorf("discard records = %+v, want none", got)
		}
		assertOrder(t, &probe, "discard", "stdout", "clear", "confirm", "unpin", "exec")
		assertShellHandOff(t, &probe)
	})
}

func TestResumeAnswerDiscard_Breadcrumb(t *testing.T) {
	t.Run("it emits no removal breadcrumb of its own", func(t *testing.T) {
		sink := logtest.Install(t)
		payload := confirmationPayload()
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{payload.HookKey: payload.Command},
		})

		var probe resumeWaitProbe
		cfg := newResumeWaitConfig(t, &probe, payload, keystrokes(t, "y"))
		cfg.Logger = nil
		cfg.DiscardRegistration = func(hookKey, shown string) (bool, error) {
			return store.Discard(hookKey, hooks.EventOnResume, shown, hooks.ViaPanel)
		}
		if err := runResumeWait(cfg); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		rec := sink.Records().Matching("hooks", "discard").Only(t, "the store's discard breadcrumb")
		if got := rec.AttrOrEmpty("value"); got != payload.Command {
			t.Errorf("discard breadcrumb value = %q, want %q", got, payload.Command)
		}
		for _, r := range sink.Records() {
			if r.AttrOrEmpty("component") == "hydrate" && r.Msg != "exec" {
				t.Errorf("the waiter emitted %q, want no line but the exec marker", r.Msg)
			}
		}
		if got, err := store.LookupOnResume(payload.HookKey, hooks.ViaHydrate); err != nil || got.Found {
			t.Errorf("LookupOnResume() = %+v, %v; want the registration gone", got, err)
		}
	})
}

func TestDiscardResumeRegistration(t *testing.T) {
	t.Run("it removes the registration the store holds", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{"tok123": {"on-resume": "make deploy"}})
		sink := logtest.Install(t)

		removed, err := discardResumeRegistration(sampleDiscardPane, "make deploy")
		if err != nil || !removed {
			t.Fatalf("discardResumeRegistration() = %v, %v; want a removal", removed, err)
		}
		rec := sink.Records().Matching("hooks", "discard").Only(t, "panel discard breadcrumb")
		if got := rec.AttrOrEmpty("via"); got != hooks.ViaPanel.String() {
			t.Errorf("discard breadcrumb via = %q, want %q", got, hooks.ViaPanel.String())
		}
		if got := lookupRegistrationOrFail(t, "tok123"); got.Found {
			t.Errorf("registration after discard = %+v, want it gone", got)
		}
	})

	t.Run("it removes nothing from an entry rewritten to another command", func(t *testing.T) {
		_, path := hooksFileInTempDir(t, map[string]map[string]string{"tok123": {"on-resume": "make test"}})
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		removed, err := discardResumeRegistration(sampleDiscardPane, "make deploy")
		if err != nil || removed {
			t.Fatalf("discardResumeRegistration() = %v, %v; want no removal and no error", removed, err)
		}
		if got := sink.Records().Matching("hooks", "discard"); len(got) != 0 {
			t.Errorf("discard records = %+v, want none", got)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed by a discard of a command it no longer holds")
	})

	t.Run("it reports nothing removed for a key the store does not hold", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{"other": {"on-resume": "make deploy"}})

		removed, err := discardResumeRegistration(sampleDiscardPane, "make deploy")
		if err != nil || removed {
			t.Fatalf("discardResumeRegistration() = %v, %v; want no removal and no error", removed, err)
		}
	})

	t.Run("it returns the error when the store cannot be resolved", func(t *testing.T) {
		t.Setenv("PORTAL_HOOKS_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")

		_, wantErr := loadHookStore()
		if wantErr == nil {
			t.Fatal("loadHookStore() resolved a store; the fixture must leave none")
		}

		removed, err := discardResumeRegistration(sampleDiscardPane, "make deploy")
		if removed {
			t.Error("discardResumeRegistration() reported a removal from a store it could not resolve")
		}
		if err == nil || err.Error() != wantErr.Error() {
			t.Errorf("discardResumeRegistration() error = %v, want %v", err, wantErr)
		}
	})
}

var sampleDiscardPane = resumePaneRef{HookKey: "tok123", PaneKey: "proj-a1b2:0.1"}

func lookupRegistrationOrFail(t *testing.T, hookKey string) hooks.OnResume {
	t.Helper()
	got, err := lookupResumeRegistration(hookKey)
	if err != nil {
		t.Fatalf("lookupResumeRegistration() error = %v", err)
	}
	return got
}
