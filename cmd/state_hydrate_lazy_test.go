package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/state"
)

const (
	lazyPaneID  = "%7"
	lazyHookKey = "paneToken"
	lazyExe     = "/opt/portal/bin/portal"
)

// lazyTail names one of the three routes runHydrate can end on: the replay it
// takes when the signal arrives and the scrollback is readable, and the two
// degraded tails.
type lazyTail struct {
	name string
	// stage fills the route-deciding seams of opts and returns the pane key the
	// route's FIFO resolves to.
	stage func(t *testing.T, dir string, opts *hydrateCfgOpts) string
}

func lazyTails() []lazyTail {
	return []lazyTail{
		{name: "replay", stage: func(t *testing.T, dir string, opts *hydrateCfgOpts) string {
			t.Helper()
			fifo := makeFIFO(t, dir, "hydrate-replay__0.0.fifo")
			scrollback := filepath.Join(dir, "sb")
			if err := os.WriteFile(scrollback, []byte("OLD"), 0o600); err != nil {
				t.Fatalf("seed scrollback: %v", err)
			}
			signalFIFOAsync(t, fifo)
			opts.FIFO, opts.File, opts.OpenFIFO = fifo, scrollback, openFIFOWithTimeout
			return state.PaneKeyFromFIFOPath(fifo)
		}},
		{name: "signal timeout", stage: func(t *testing.T, dir string, opts *hydrateCfgOpts) string {
			t.Helper()
			fifo := filepath.Join(dir, "hydrate-timeout__0.0.fifo")
			opts.FIFO, opts.File, opts.OpenFIFO = fifo, filepath.Join(dir, "sb"), instantTimeoutOpenFIFO
			opts.HandleTimeout = handleHydrateTimeout
			return state.PaneKeyFromFIFOPath(fifo)
		}},
		{name: "scrollback missing", stage: func(t *testing.T, dir string, opts *hydrateCfgOpts) string {
			t.Helper()
			fifo := makeFIFO(t, dir, "hydrate-missing__0.0.fifo")
			signalFIFOAsync(t, fifo)
			opts.FIFO, opts.File, opts.OpenFIFO = fifo, filepath.Join(dir, "absent-sb"), openFIFOWithTimeout
			opts.HandleFileMissing = handleHydrateFileMissing
			return state.PaneKeyFromFIFOPath(fifo)
		}},
	}
}

// lazyRun stages one tail's seams, runs the helper down it and returns the pane
// key the route's FIFO named.
func lazyRun(t *testing.T, tail lazyTail, opts hydrateCfgOpts) string {
	t.Helper()
	dir := t.TempDir()
	paneKey := tail.stage(t, dir, &opts)
	if err := runHydrate(hydrateCfg(t, opts)); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	return paneKey
}

// lazyPrefs stages a prefs.json naming the install-wide resume mode. An empty
// mode stages no file at all, which is the shipped install.
func lazyPrefs(t *testing.T, mode string) func() (*prefs.Store, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prefs.json")
	if mode != "" {
		if err := os.WriteFile(path, []byte(`{"resume_mode":"`+mode+`"}`), 0o600); err != nil {
			t.Fatalf("seed prefs.json: %v", err)
		}
	}
	return func() (*prefs.Store, error) { return prefs.NewStore(path), nil }
}

// lazyOpts is the config every lazy case starts from: a pane that can be
// marked, an executable that resolves and a commander that accepts both option
// writes.
func lazyOpts(t *testing.T, store *hooks.Store, loadPrefs func() (*prefs.Store, error), exec *stubExecShell) hydrateCfgOpts {
	t.Helper()
	t.Setenv("TMUX_PANE", lazyPaneID)
	t.Setenv("SHELL", "/bin/zsh")
	return hydrateCfgOpts{
		HookKey:        lazyHookKey,
		HookStore:      store,
		LoadPrefsStore: loadPrefs,
		ResolveExe:     func() (string, error) { return lazyExe, nil },
		ExecShell:      exec.fn(),
	}
}

// lazyChainArgs is the argv a parked chain execs for one pane.
func lazyChainArgs(command, paneKey string) []string {
	payload := resumeChainPayload{
		Command: command,
		HookKey: lazyHookKey,
		Pane:    lazyPaneID,
		PaneKey: paneKey,
	}
	draw := shellquote.Join(resumeChainArgv(lazyExe, resumeDrawSubcommand, payload))
	recover := shellquote.Join(resumeChainArgv(lazyExe, resumeRecoverSubcommand, payload))
	return []string{"sh", "-c", parkedChainTrap + draw + "; " + recover + parkedChainBackstop(lazyExe, payload)}
}

func TestHydrateLazy_NoRegistrationRestoresAsToday(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet()
			store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Body: map[string]map[string]string{}})
			opts := lazyOpts(t, store, lazyPrefs(t, ""), exec)
			opts.Commander = cmder

			lazyRun(t, tail, opts)

			if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
				t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
			}
			assertNoPendingMarker(t, cmder)
			assertNoAltScreenPin(t, cmder)
		})
	}
}

func TestHydrateLazy_EagerRegistrationRestoresAsToday(t *testing.T) {
	cases := []struct {
		name         string
		registration resumemode.Mode
		install      string
	}{
		{name: "pinned eager under the shipped install", registration: resumemode.Eager, install: ""},
		{name: "naming no mode under an eager install", registration: resumemode.Unset, install: "eager"},
	}
	for _, tc := range cases {
		for _, tail := range lazyTails() {
			t.Run(tc.name+"/"+tail.name, func(t *testing.T) {
				exec := &stubExecShell{}
				cmder := commandertest.Quiet()
				opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", tc.registration), lazyPrefs(t, tc.install), exec)
				opts.Commander = cmder

				lazyRun(t, tail, opts)

				want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
				if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
					t.Errorf("exec = %q %v, want /bin/sh %v", exec.target, exec.args, want)
				}
				assertNoPendingMarker(t, cmder)
				assertNoAltScreenPin(t, cmder)
			})
		}
	}
}

func TestHydrateLazy_PinnedLazyBeatsAnEagerInstall(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, "eager"), exec)

	paneKey := lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/sh" {
		t.Fatalf("exec target = %q, want /bin/sh", exec.target)
	}
	if want := lazyChainArgs("echo hi", paneKey); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

func TestHydrateLazy_MarksPendingBeforeClearingMidRestoreMarker(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet()
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Commander = cmder

			paneKey := lazyRun(t, tail, opts)

			pending := cmder.CallsMatching("set-option", "-p", state.ResumePendingOption).FirstIndex()
			cleared := cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex()
			if pending < 0 {
				t.Fatalf("no pending marker write; calls: %v", cmder.Calls())
			}
			if cleared < 0 {
				t.Fatalf("no skeleton marker clear; calls: %v", cmder.Calls())
			}
			if pending > cleared {
				t.Errorf("pending marker written at %d, after the skeleton clear at %d; calls: %v", pending, cleared, cmder.Calls())
			}
		})
	}
}

func TestHydrateLazy_ParksTheDrawAndTheTailInOneShell(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	paneKey := lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/sh" {
		t.Errorf("exec target = %q, want /bin/sh", exec.target)
	}
	if want := lazyChainArgs("echo hi", paneKey); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want %v", exec.args, want)
	}
}

func TestHydrateLazy_QuotesTheCommandIntoTheChain(t *testing.T) {
	commands := []string{
		"claude --resume abc",
		"echo 'hi there'",
		"echo $(whoami)",
		"echo `date`",
		"echo one\necho two",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			exec := &stubExecShell{}
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, command, resumemode.Lazy), lazyPrefs(t, ""), exec)

			lazyRun(t, lazyTails()[0], opts)

			chained := exec.args[2]
			if !strings.Contains(chained, shellquote.Single(command)) {
				t.Errorf("chain %q does not carry the command as one quoted word %q", chained, shellquote.Single(command))
			}
		})
	}
}

func TestHydrateLazy_SeparatesTheDrawAndTheTailWithASemicolon(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, lazyTails()[0], opts)

	chained := strings.TrimPrefix(exec.args[2], parkedChainTrap)
	if strings.Contains(chained, "&&") {
		t.Errorf("chain %q joins its halves with &&; the tail must run whatever the draw did", chained)
	}
	if n := strings.Count(chained, "; "+shellquote.Single(lazyExe)); n != 1 {
		t.Errorf("chain %q does not separate the two halves with exactly one semicolon", chained)
	}
}

func TestHydrateLazy_ComposesTheTailWithThePaneFlagsAlone(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	paneKey := lazyRun(t, lazyTails()[0], opts)

	_, tail, found := strings.Cut(strings.TrimPrefix(exec.args[2], parkedChainTrap), "; ")
	if !found {
		t.Fatalf("chain %q has no tail", exec.args[2])
	}
	payload := resumeChainPayload{Command: "echo hi", HookKey: lazyHookKey, Pane: lazyPaneID, PaneKey: paneKey}
	want := shellquote.Join([]string{lazyExe, "state", resumeRecoverSubcommand, "--pane", lazyPaneID, "--pane-key", paneKey}) + parkedChainBackstop(lazyExe, payload)
	if tail != want {
		t.Errorf("tail = %q, want %q", tail, want)
	}
}

func TestHydrateLazy_FiresTheHookWhenThePaneCannotBeMarked(t *testing.T) {
	refusals := []struct {
		name    string
		prepare func(t *testing.T, opts *hydrateCfgOpts)
		// wantNoPin marks a refusal taken before the pin: a pin written ahead
		// of resolution would outlive the eager hook.
		wantNoWrite, wantNoPin bool
	}{
		{name: "the marker write fails", prepare: func(t *testing.T, opts *hydrateCfgOpts) {
			t.Helper()
			opts.Commander = commandertest.Quiet(
				commandertest.Fails(errors.New("tmux refused"), markerWriteArgv()...),
			)
		}},
		{name: "the alternate-screen pin fails", wantNoWrite: true, prepare: func(t *testing.T, opts *hydrateCfgOpts) {
			t.Helper()
			opts.Commander = commandertest.Quiet(
				commandertest.Fails(errors.New("tmux refused"), pinArgv()...),
			)
		}},
		{name: "TMUX_PANE is absent", wantNoWrite: true, wantNoPin: true, prepare: func(t *testing.T, opts *hydrateCfgOpts) {
			t.Helper()
			t.Setenv("TMUX_PANE", "")
		}},
		{name: "the executable cannot be resolved", wantNoWrite: true, wantNoPin: true, prepare: func(t *testing.T, opts *hydrateCfgOpts) {
			t.Helper()
			opts.ResolveExe = func() (string, error) { return "", errors.New("no executable") }
		}},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			exec := &stubExecShell{}
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Logger = logger
			opts.Commander = commandertest.Quiet()
			refusal.prepare(t, &opts)
			cmder := opts.Commander.(*commandertest.Scripted)

			paneKey := lazyRun(t, lazyTails()[0], opts)

			want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
			if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, want)
			}
			assertOneMarkerRefusalWarn(t, sink, paneKey)
			if refusal.wantNoWrite {
				assertNoPendingMarker(t, cmder)
			}
			if refusal.wantNoPin {
				assertNoAltScreenPin(t, cmder)
			}
		})
	}
}

func TestHydrateLazy_NamesThePinsErrorWhenTheAlternateScreenCannotBePinned(t *testing.T) {
	pinErr := errors.New("invalid option: alternate-screen")
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
	opts.Logger = logger
	opts.Commander = commandertest.Quiet(commandertest.Fails(pinErr, pinArgv()...))

	lazyRun(t, lazyTails()[0], opts)

	warn := execLogLine(t, sink.Body(), "WARN", "set resume pending marker failed")
	if !strings.Contains(warn, pinErr.Error()) || !strings.Contains(warn, altScreenOption) {
		t.Errorf("refusal WARN = %q, want it to name the pin's error %q", warn, pinErr)
	}
}

func TestHydrateLazy_LiftsThePinWhenTheMarkerWriteIsRefused(t *testing.T) {
	markerErr := errors.New("marker refused")

	t.Run("it lifts the pin before the eager hook runs and names the marker's error", func(t *testing.T) {
		exec := &stubExecShell{}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		cmder := commandertest.Quiet(commandertest.Fails(markerErr, markerWriteArgv()...))
		opts.Logger, opts.Commander = logger, cmder

		paneKey := lazyRun(t, lazyTails()[0], opts)

		pin, marker, lift := callIndex(cmder, pinArgv()), callIndex(cmder, markerWriteArgv()), callIndex(cmder, liftArgv())
		if pin < 0 || marker < 0 || lift < 0 || pin > marker || marker > lift {
			t.Errorf("pin at %d, marker at %d, lift at %d; want pin, then the refused marker, then the lift; calls: %v",
				pin, marker, lift, cmder.Calls())
		}
		want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, want)
		}
		assertOneMarkerRefusalWarn(t, sink, paneKey)
		if warn := execLogLine(t, sink.Body(), "WARN", "set resume pending marker failed"); !strings.Contains(warn, markerErr.Error()) {
			t.Errorf("refusal WARN = %q, want it to name the marker's error", warn)
		}
		if n := strings.Count(sink.Body(), "WARN"); n != 1 {
			t.Errorf("got %d WARNs, want the refusal alone after a lift that landed: %q", n, sink.Body())
		}
	})

	t.Run("it records a refused lift apart from the refusal and still runs the hook", func(t *testing.T) {
		liftErr := errors.New("lift refused")
		exec := &stubExecShell{}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		opts.Logger = logger
		opts.Commander = commandertest.Quiet(
			commandertest.Fails(markerErr, markerWriteArgv()...),
			commandertest.Fails(liftErr, liftArgv()...),
		)

		paneKey := lazyRun(t, lazyTails()[0], opts)

		want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, want)
		}
		assertOneMarkerRefusalWarn(t, sink, paneKey)
		lift := execLogLine(t, sink.Body(), "WARN", "lift alternate-screen pin failed")
		if !strings.Contains(lift, "pane_key="+paneKey) || !strings.Contains(lift, liftErr.Error()) {
			t.Errorf("lift WARN = %q, want it to name the pane and the lift's error", lift)
		}
	})
}

func TestHydrateLazy_PinsTheAlternateScreenBeforeMarkingThePane(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet()
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Commander = cmder

			paneKey := lazyRun(t, tail, opts)

			pin := callIndex(cmder, pinArgv())
			marker := callIndex(cmder, markerWriteArgv())
			cleared := cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex()
			if pin < 0 || marker < 0 || cleared < 0 || pin > marker || marker > cleared {
				t.Errorf("pin at %d, marker at %d, skeleton clear at %d; want them in that order; calls: %v",
					pin, marker, cleared, cmder.Calls())
			}
			for i, call := range cmder.Calls()[:max(pin, 0)] {
				if call[0] != "set-option" && strings.Contains(strings.Join(call, " "), "alternate") {
					t.Errorf("call %d %v reads the alternate screen ahead of the pin", i, call)
				}
			}
			if want := lazyChainArgs("echo hi", paneKey); exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec = %q %v, want the parked chain %v", exec.target, exec.args, want)
			}
		})
	}
}

func TestHydrateLazy_ResolvesTheShippedDefaultWhenThePrefsReadFails(t *testing.T) {
	exec := &stubExecShell{}
	unreadable := t.TempDir()
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Unset), func() (*prefs.Store, error) {
		return prefs.NewStore(unreadable), nil
	}, exec)

	paneKey := lazyRun(t, lazyTails()[0], opts)

	if want := lazyChainArgs("echo hi", paneKey); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

func TestHydrateLazy_ResolvesTheShippedDefaultWhenThePrefsStoreCannotBeBuilt(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Unset), func() (*prefs.Store, error) {
		return nil, errors.New("no config base")
	}, exec)

	paneKey := lazyRun(t, lazyTails()[0], opts)

	if want := lazyChainArgs("echo hi", paneKey); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

func TestHydrateLazy_AbsentHookStoreIsNoRegistration(t *testing.T) {
	exec := &stubExecShell{}
	cmder := commandertest.Quiet()
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	opts := lazyOpts(t, nil, lazyPrefs(t, ""), exec)
	opts.Commander, opts.Logger = cmder, logger

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
	}
	assertNoPendingMarker(t, cmder)
	if dbg := execLogLine(t, sink.Body(), "DEBUG", "hook lookup"); !strings.Contains(dbg, "result=miss") {
		t.Errorf("absent store must record a miss: %q", dbg)
	}
}

func TestHydrateLazy_UnreadableStoreGivesABareShellAndNeverAWait(t *testing.T) {
	exec := &stubExecShell{}
	cmder := commandertest.Quiet()
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Unreadable: true})
	opts := lazyOpts(t, store, lazyPrefs(t, ""), exec)
	opts.Commander, opts.Logger = cmder, logger

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
	}
	assertNoPendingMarker(t, cmder)
	body := sink.Body()
	if dbg := execLogLine(t, body, "DEBUG", "hook lookup"); !strings.Contains(dbg, "result=error") {
		t.Errorf("an unreadable store must record result=error: %q", dbg)
	}
	if n := strings.Count(body, "lookup on-resume hook failed"); n != 1 {
		t.Errorf("want exactly one lookup-failure WARN, got %d: %q", n, body)
	}
}

func TestHydrateLazy_ReadsTheStoreOnceAndThePrefsOncePerPane(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			loads := 0
			seam := lazyPrefs(t, "")
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Eager), func() (*prefs.Store, error) {
				loads++
				return seam()
			}, exec)
			opts.Logger = logger

			lazyRun(t, tail, opts)

			if n := strings.Count(sink.Body(), "DEBUG hook lookup"); n != 1 {
				t.Errorf("hook lookup recorded %d times, want exactly one per pane: %q", n, sink.Body())
			}
			if loads != 1 {
				t.Errorf("prefs loaded %d times, want exactly one per pane", loads)
			}
		})
	}
}

func TestHydrateLazy_ComposesTheChainFromTheValuesTheMarkStepResolved(t *testing.T) {
	exec := &stubExecShell{}
	exes := 0
	t.Setenv("TMUX_PANE", lazyPaneID)
	t.Setenv("SHELL", "/bin/zsh")
	// The pane id moves the moment the marker lands, so a chain composed from a
	// second read would carry the new one.
	cmder := commandertest.Quiet(
		commandertest.When(commandertest.ArgvPrefix("set-option", "-p"), "", nil).
			Doing(func([]string) { t.Setenv("TMUX_PANE", "%99") }),
	)
	opts := hydrateCfgOpts{
		HookKey:        lazyHookKey,
		HookStore:      hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy),
		LoadPrefsStore: lazyPrefs(t, ""),
		ResolveExe:     func() (string, error) { exes++; return lazyExe, nil },
		ExecShell:      exec.fn(),
		Commander:      cmder,
	}

	paneKey := lazyRun(t, lazyTails()[0], opts)

	if exes != 1 {
		t.Errorf("executable resolved %d times, want exactly one per pane", exes)
	}
	if want := lazyChainArgs("echo hi", paneKey); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the chain composed from the marked pane %v", exec.args, want)
	}
}

func TestHydrateLazy_EmptyStoredCommandIsNoRegistration(t *testing.T) {
	exec := &stubExecShell{}
	cmder := commandertest.Quiet()
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "", resumemode.Lazy), lazyPrefs(t, ""), exec)
	opts.Commander = cmder

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
	}
	assertNoPendingMarker(t, cmder)
}

func TestHydrateLazy_MarksAndParksWhateverTheHandlerSeamDoes(t *testing.T) {
	inert := map[string]func(opts *hydrateCfgOpts){
		"signal timeout": func(opts *hydrateCfgOpts) {
			opts.HandleTimeout = func(hydrateConfig) error { return nil }
		},
		"scrollback missing": func(opts *hydrateCfgOpts) {
			opts.HandleFileMissing = func(hydrateConfig, hydrateFileMissingContext) error { return nil }
		},
	}
	for _, tail := range lazyTails() {
		replace, ok := inert[tail.name]
		if !ok {
			continue
		}
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet()
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Commander = cmder
			stage := tail.stage
			tail.stage = func(t *testing.T, dir string, opts *hydrateCfgOpts) string {
				t.Helper()
				paneKey := stage(t, dir, opts)
				replace(opts)
				return paneKey
			}

			paneKey := lazyRun(t, tail, opts)

			pending := cmder.CallsMatching("set-option", "-p", state.ResumePendingOption).FirstIndex()
			cleared := cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex()
			if pending < 0 || cleared < 0 || pending > cleared {
				t.Errorf("pending marker at %d, skeleton clear at %d, want the mark first and both present; calls: %v", pending, cleared, cmder.Calls())
			}
			if want := lazyChainArgs("echo hi", paneKey); exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec = %q %v, want the parked chain %v", exec.target, exec.args, want)
			}
		})
	}
}

func TestHydrateLazy_FiresTheHookOnEveryTailWhenThePaneCannotBeMarked(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet(
				commandertest.Fails(errors.New("tmux refused"), "set-option", "-p"),
			)
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Commander, opts.Logger = cmder, logger

			paneKey := lazyRun(t, tail, opts)

			want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
			if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, want)
			}
			assertOneMarkerRefusalWarn(t, sink, paneKey)
			if cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex() < 0 {
				t.Errorf("a refused mark must still clear the skeleton marker; calls: %v", cmder.Calls())
			}
		})
	}
}

func TestHydrateLazy_ExecWithNoParkedPaneRunsTheHookAndLooksNothingUp(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   lazyHookKey,
		HookStore: hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy),
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		ExecShell: exec.fn(),
	})
	registration := hooks.OnResume{Found: true, Command: "echo hi", Mode: resumemode.Lazy}

	execShellOrHookAndExit(cfg, registration, nil)

	want := []string{"sh", "-c", "echo hi; exec /bin/zsh"}
	if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec = %q %v, want the hook %v", exec.target, exec.args, want)
	}
	body := sink.Body()
	if info := execLogLine(t, body, "INFO", "exec"); !strings.Contains(info, "hook_present=true") {
		t.Errorf("exec INFO missing hook_present=true: %q", info)
	}
	if n := strings.Count(body, "hook lookup"); n != 0 {
		t.Errorf("the exec looked the registration up itself %d times: %q", n, body)
	}
}

func assertNoPendingMarker(t *testing.T, cmder *commandertest.Scripted) {
	t.Helper()
	if calls := cmder.CallsMatching("set-option", "-p", state.ResumePendingOption); len(calls) != 0 {
		t.Errorf("wrote the pending marker on a pane that must not wait: %v", calls)
	}
}

func assertNoAltScreenPin(t *testing.T, cmder *commandertest.Scripted) {
	t.Helper()
	if calls := cmder.CallsMatching("set-option", "-p", altScreenOption); len(calls) != 0 {
		t.Errorf("pinned the alternate screen on a pane that must not wait: %v", calls)
	}
}

// pinArgv is the pane-scoped write that holds a waiting pane's panel on an
// alternate screen whatever the install sets.
func pinArgv() []string {
	return []string{"set-option", "-p", "-t", lazyPaneID, altScreenOption, "on"}
}

func liftArgv() []string {
	return []string{"set-option", "-pu", "-t", lazyPaneID, altScreenOption}
}

func markerWriteArgv() []string {
	return []string{"set-option", "-p", "-t", lazyPaneID, state.ResumePendingOption, "1"}
}

func callIndex(cmder *commandertest.Scripted, argv []string) int {
	for i, call := range cmder.Calls() {
		if reflect.DeepEqual(call, argv) {
			return i
		}
	}
	return -1
}

func assertOneMarkerRefusalWarn(t *testing.T, sink *logtest.Sink, paneKey string) {
	t.Helper()
	warn := execLogLine(t, sink.Body(), "WARN", "set resume pending marker failed")
	if !strings.Contains(warn, "pane_key="+paneKey) {
		t.Errorf("refusal WARN does not name the pane: %q", warn)
	}
	if !strings.Contains(warn, "error=") {
		t.Errorf("refusal WARN carries no error: %q", warn)
	}
}

func TestHydrateHandlers_ReportWithoutTouchingEitherMarker(t *testing.T) {
	runs := map[string]func(t *testing.T, cfg hydrateConfig) error{
		"signal timeout": func(_ *testing.T, cfg hydrateConfig) error { return handleHydrateTimeout(cfg) },
		"scrollback missing": func(_ *testing.T, cfg hydrateConfig) error {
			return handleHydrateFileMissing(cfg, hydrateFileMissingContext{Cause: os.ErrNotExist})
		},
	}
	wantLines := map[string][]string{
		"signal timeout":     {"WARN timeout waiting for hydrate signal", "INFO signal timeout"},
		"scrollback missing": {"WARN scrollback file not found", "INFO scrollback missing"},
	}
	for name, run := range runs {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TMUX_PANE", lazyPaneID)
			cmder := commandertest.Quiet()
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			dir := t.TempDir()
			cfg := hydrateCfg(t, hydrateCfgOpts{
				FIFO:      filepath.Join(dir, "hydrate-alone__0.0.fifo"),
				File:      filepath.Join(dir, "sb"),
				HookKey:   lazyHookKey,
				HookStore: hydrateStoreWithMode(t, lazyHookKey, "echo hi", resumemode.Lazy),
				OpenFIFO:  unexpectedOpenFIFO(t),
				Commander: cmder,
				Logger:    logger,
			})

			if err := run(t, cfg); err != nil {
				t.Fatalf("handler: %v", err)
			}

			if calls := cmder.Calls(); len(calls) != 0 {
				t.Errorf("handler issued tmux commands %v, want none", calls)
			}
			body := sink.Body()
			for _, line := range wantLines[name] {
				if n := strings.Count(body, line); n != 1 {
					t.Errorf("want exactly one %q, got %d: %q", line, n, body)
				}
			}
		})
	}
}
