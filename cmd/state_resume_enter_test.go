package cmd

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

func foundHook(command string) func(string) (hooks.OnResume, error) {
	return func(string) (hooks.OnResume, error) {
		return hooks.OnResume{Command: command, Found: true}, nil
	}
}

func answerEnter(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload) {
	t.Helper()
	if err := runResumeWait(newResumeWaitConfig(t, p, payload, keystrokes(t, "\r"))); err != nil {
		t.Fatalf("runResumeWait() error = %v", err)
	}
}

// assertHookHandOff pins the resume exec against the shape the hydrate helper
// already uses, so the pane still closes on the first exit.
func assertHookHandOff(t *testing.T, p *resumeWaitProbe, command string) {
	t.Helper()
	wantProg, wantArgs := hookExecArgs(command, resolveShell())
	assertExec(t, p, wantProg, wantArgs, true)
}

func assertShellHandOff(t *testing.T, p *resumeWaitProbe) {
	t.Helper()
	shell := resolveShell()
	assertExec(t, p, shell, []string{shell}, false)
}

func assertExec(t *testing.T, p *resumeWaitProbe, wantProg string, wantArgs []string, wantHook bool) {
	t.Helper()
	if p.execCalls != 1 {
		t.Fatalf("ExecSelf called %d times, want exactly 1", p.execCalls)
	}
	if p.execProg != wantProg {
		t.Errorf("exec target = %q, want %q", p.execProg, wantProg)
	}
	if !slices.Equal(p.execArgs, wantArgs) {
		t.Errorf("exec argv = %q, want %q", p.execArgs, wantArgs)
	}

	rec := p.sink.Records().WithMessage("exec").Only(t, "the resume exec marker")
	if got := rec.AttrOrEmpty("target"); got != wantProg {
		t.Errorf("exec marker target = %q, want %q", got, wantProg)
	}
	if got := rec.AttrOrEmpty("args"); got != strings.Join(wantArgs, " ") {
		t.Errorf("exec marker args = %q, want %q", got, strings.Join(wantArgs, " "))
	}
	if got := rec.AttrOrEmpty("hook_present"); got != strconv.FormatBool(wantHook) {
		t.Errorf("exec marker hook_present = %q, want %v", got, wantHook)
	}
}

func TestResumeAnswerEnter_Order(t *testing.T) {
	t.Run("it leaves the panel's screen before it clears the marker", func(t *testing.T) {
		var probe resumeWaitProbe
		answerEnter(t, &probe, samplePayload())

		if len(probe.order) < 2 || probe.order[0] != "stdout" || probe.order[1] != "clear" {
			t.Fatalf("answer sequence = %q, want the leave sequence written before the clear", probe.order)
		}
		if got := probe.stdout.String(); got != hydrateResetPreamble {
			t.Errorf("what the answer wrote to the pane = %q, want the leave sequence %q", got, hydrateResetPreamble)
		}
	})

	t.Run("it clears the marker before it reads the store", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.lookup = foundHook("make deploy")
		answerEnter(t, &probe, samplePayload())

		clearAt := slices.Index(probe.order, "clear")
		lookupAt := slices.Index(probe.order, "lookup")
		if clearAt < 0 || lookupAt < 0 {
			t.Fatalf("answer sequence = %q, want both a clear and a store read", probe.order)
		}
		if clearAt > lookupAt {
			t.Errorf("answer sequence = %q, want the clear before the store read", probe.order)
		}
	})
}

func TestResumeAnswerEnter_FailedClear(t *testing.T) {
	clearErr := errors.New("can't find pane: %7")

	t.Run("it redraws the panel with the reason when the clear fails", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.clearErr = clearErr
		payload := samplePayload()
		answerEnter(t, &probe, payload)

		exe, err := resumeChainExe()
		if err != nil {
			t.Fatalf("resumeChainExe() error = %v", err)
		}
		reported := payload
		reported.Report = clearErr.Error()
		want := resumeChainArgv(exe, resumeDrawSubcommand, reported)

		if probe.execCalls != 1 {
			t.Fatalf("ExecSelf called %d times, want exactly 1", probe.execCalls)
		}
		if probe.execProg != exe {
			t.Errorf("exec target = %q, want %q", probe.execProg, exe)
		}
		if !slices.Equal(probe.execArgs, want) {
			t.Errorf("exec argv = %q, want %q", probe.execArgs, want)
		}
	})

	t.Run("it runs neither the hook nor the shell when the clear fails", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.clearErr = clearErr
		probe.lookup = foundHook("make deploy")
		answerEnter(t, &probe, samplePayload())

		if len(probe.lookupKeys) != 0 {
			t.Errorf("the store was read %d times after a failed clear, want 0", len(probe.lookupKeys))
		}
		if slices.Contains(probe.execArgs, "sh") || probe.execProg == resolveShell() {
			t.Errorf("exec argv = %q, want no shell or hook while the marker stands", probe.execArgs)
		}
	})

	t.Run("it keeps both key hints live on the redraw after a failed clear", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.clearErr = clearErr
		payload := samplePayload()
		answerEnter(t, &probe, payload)

		reported := payload
		reported.Report = clearErr.Error()

		var drawProbe resumeDrawProbe
		drawCfg := newResumeDrawConfig(t, &drawProbe, reported, fixedSize(100, 30))
		drawCfg.Colourless = true
		if err := runResumeDraw(drawCfg); err != nil {
			t.Fatalf("runResumeDraw() error = %v", err)
		}

		painted := drawProbe.stdout.String()
		for _, want := range []string{payload.Command, clearErr.Error(), "resume", "discard"} {
			if !strings.Contains(painted, want) {
				t.Errorf("the redrawn panel does not carry %q:\n%s", want, painted)
			}
		}
	})
}

func TestResumeAnswerEnter_ReadsTheStoreAtTheAnswer(t *testing.T) {
	t.Run("it runs the command the store holds at the moment of the answer", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.lookup = foundHook("make deploy-rewritten")
		payload := samplePayload()
		answerEnter(t, &probe, payload)

		if !slices.Equal(probe.lookupKeys, []string{payload.HookKey}) {
			t.Errorf("store read with %q, want one read of %q", probe.lookupKeys, payload.HookKey)
		}
		assertHookHandOff(t, &probe, "make deploy-rewritten")
		if slices.Contains(probe.execArgs, payload.Command+"; exec "+resolveShell()) {
			t.Errorf("exec argv = %q, want the command the store holds rather than the one the panel displayed", probe.execArgs)
		}
	})

	t.Run("it drops the pane to a plain shell when the entry has gone", func(t *testing.T) {
		var probe resumeWaitProbe
		answerEnter(t, &probe, samplePayload())

		assertShellHandOff(t, &probe)
		if probe.clearCalls != 1 {
			t.Errorf("ClearMarker called %d times, want exactly 1", probe.clearCalls)
		}
	})

	t.Run("it drops the pane to a plain shell when the command is empty", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.lookup = func(string) (hooks.OnResume, error) {
			return hooks.OnResume{Found: true}, nil
		}
		answerEnter(t, &probe, samplePayload())

		assertShellHandOff(t, &probe)
		if probe.clearCalls != 1 {
			t.Errorf("ClearMarker called %d times, want exactly 1", probe.clearCalls)
		}
	})

	t.Run("it drops the pane to a plain shell when the store is unreadable", func(t *testing.T) {
		var probe resumeWaitProbe
		lookupErr := errors.New("is a directory")
		probe.lookup = func(string) (hooks.OnResume, error) { return hooks.OnResume{}, lookupErr }
		payload := samplePayload()
		answerEnter(t, &probe, payload)

		assertShellHandOff(t, &probe)
		if probe.clearCalls != 1 {
			t.Errorf("ClearMarker called %d times, want exactly 1: the pane is no longer waiting whatever the read returned", probe.clearCalls)
		}
		warn := probe.sink.Records().WithMessage("lookup on-resume hook failed").Only(t, "the failed lookup's WARN")
		if got := warn.AttrOrEmpty("hook_key"); got != payload.HookKey {
			t.Errorf("WARN hook_key = %q, want %q", got, payload.HookKey)
		}
		debug := probe.sink.Records().WithMessage("hook lookup").Only(t, "the lookup's DEBUG")
		if got := debug.AttrOrEmpty("result"); got != "error" {
			t.Errorf("hook lookup result = %q, want %q", got, "error")
		}
	})

	t.Run("it resolves /bin/sh when SHELL is unset", func(t *testing.T) {
		t.Setenv("SHELL", "")
		var probe resumeWaitProbe
		answerEnter(t, &probe, samplePayload())

		assertExec(t, &probe, "/bin/sh", []string{"/bin/sh"}, false)
	})

	t.Run("it treats an already-absent marker as cleared", func(t *testing.T) {
		var probe resumeWaitProbe
		probe.lookup = foundHook("make deploy")
		answerEnter(t, &probe, samplePayload())

		if probe.clearCalls != 1 {
			t.Errorf("ClearMarker called %d times, want exactly 1", probe.clearCalls)
		}
		assertHookHandOff(t, &probe, "make deploy")
	})
}

// The wait path and the hydrate helper must compose one argv for one command:
// the pane closes on the first exit only because the hook's own shell is exec'd
// over it.
func TestResumeAnswerEnter_HookShapeMatchesTheHelper(t *testing.T) {
	t.Run("it runs the hook in the shape the helper already uses", func(t *testing.T) {
		const command = "make deploy && echo 'done'"
		payload := samplePayload()
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Entries: map[string]string{payload.HookKey: command},
		})

		var helperProg string
		var helperArgs []string
		logger, _ := logtest.NewCaptureLogger(t)
		execShellOrHookAndExit(hydrateConfig{
			HookKey:   payload.HookKey,
			Logger:    logger,
			HookStore: store,
			ExecShell: func(prog string, args []string) {
				helperProg, helperArgs = prog, args
			},
		})

		var probe resumeWaitProbe
		probe.lookup = foundHook(command)
		answerEnter(t, &probe, payload)

		if probe.execProg != helperProg {
			t.Errorf("resume exec target = %q, helper's = %q", probe.execProg, helperProg)
		}
		if !slices.Equal(probe.execArgs, helperArgs) {
			t.Errorf("resume exec argv = %q, helper's = %q", probe.execArgs, helperArgs)
		}
	})
}

func TestResumeAnswerEnter_TerminalAndExecFailure(t *testing.T) {
	t.Run("it restores the terminal before every exec", func(t *testing.T) {
		cases := []struct {
			name    string
			arrange func(p *resumeWaitProbe)
		}{
			{"a registered command", func(p *resumeWaitProbe) { p.lookup = foundHook("make deploy") }},
			{"an entry that has gone", func(*resumeWaitProbe) {}},
			{"an unreadable store", func(p *resumeWaitProbe) {
				p.lookup = func(string) (hooks.OnResume, error) { return hooks.OnResume{}, errors.New("is a directory") }
			}},
			{"a clear that failed", func(p *resumeWaitProbe) { p.clearErr = errors.New("can't find pane: %7") }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var probe resumeWaitProbe
				tc.arrange(&probe)
				answerEnter(t, &probe, samplePayload())

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
		cfg := newResumeWaitConfig(t, &probe, samplePayload(), keystrokes(t, "\r"))
		cfg.Logger = nil
		cfg.ExecSelf = func(_ string, args []string) {
			defaultExecShell("/nonexistent/portal-resume-enter-probe", args)
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

func TestLookupResumeRegistration(t *testing.T) {
	t.Run("it reads the registration the store holds", func(t *testing.T) {
		hooksFileInTempDir(t, map[string]map[string]string{"tok123": {"on-resume": "make deploy"}})

		got, err := lookupResumeRegistration("tok123")
		if err != nil {
			t.Fatalf("lookupResumeRegistration() error = %v", err)
		}
		if want := (hooks.OnResume{Command: "make deploy", Found: true}); got != want {
			t.Errorf("lookupResumeRegistration() = %+v, want %+v", got, want)
		}
	})

	t.Run("it reports no registration when the store cannot be resolved", func(t *testing.T) {
		t.Setenv("PORTAL_HOOKS_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")

		if store, err := loadHookStore(); err == nil || store != nil {
			t.Fatalf("loadHookStore() = %v, %v; the fixture must leave no store to read", store, err)
		}

		got, err := lookupResumeRegistration("tok123")
		if err != nil {
			t.Fatalf("lookupResumeRegistration() error = %v", err)
		}
		if got != (hooks.OnResume{}) {
			t.Errorf("lookupResumeRegistration() = %+v, want the zero registration", got)
		}
	})
}
