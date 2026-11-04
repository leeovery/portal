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
	lazySession = "proj-a1b2"
	lazyExe     = "/opt/portal/bin/portal"
)

var eagerHookArgs = []string{"sh", "-c", "trap : TERM; echo hi; exec /bin/zsh"}

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

// lazyOpts is the config every lazy case starts from: a state directory of the
// test's own to raise the session's flag in, and an executable that resolves.
func lazyOpts(t *testing.T, store *hooks.Store, loadPrefs func() (*prefs.Store, error), exec *stubExecShell) hydrateCfgOpts {
	t.Helper()
	resumeStateDir(t)
	t.Setenv("SHELL", "/bin/zsh")
	return hydrateCfgOpts{
		HookKey:        lazySession,
		HookStore:      store,
		LoadPrefsStore: loadPrefs,
		ResolveExe:     func() (string, error) { return lazyExe, nil },
		ExecShell:      exec.fn(),
	}
}

// lazyFlag is the session's flag in the state directory the test runs in.
func lazyFlag(t *testing.T) string {
	t.Helper()
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	path, err := state.ResumePending.Path(dir, lazySession)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// lazyChainArgs is the argv a parked chain execs for the session.
func lazyChainArgs(t *testing.T, command string) []string {
	t.Helper()
	parked := parkedChain{Exe: lazyExe, Flag: lazyFlag(t)}
	return []string{"sh", "-c", parkedResumeChain(parked, resumeChainPayload{Command: command, Session: lazySession})}
}

func flagRaisedAt(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("look for the flag: %v", err)
	}
	return err == nil
}

// flagAtSkeletonClear scripts the skeleton marker's clear to record whether
// the session's flag was raised as it ran.
func flagAtSkeletonClear(t *testing.T, raised *bool) *commandertest.Scripted {
	t.Helper()
	flag := lazyFlag(t)
	return commandertest.Quiet(
		commandertest.When(commandertest.ArgvPrefix("set-option", "-su"), "", nil).
			Doing(func([]string) { *raised = flagRaisedAt(t, flag) }),
	)
}

// unwritableStateDir points PORTAL_STATE_DIR under a regular file, so nothing
// can be made in it.
func unwritableStateDir(t *testing.T) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORTAL_STATE_DIR", filepath.Join(file, "state"))
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
			assertNoFlag(t)
			assertNoPaneOptionWrite(t, cmder)
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
				opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", tc.registration), lazyPrefs(t, tc.install), exec)
				opts.Commander = cmder

				lazyRun(t, tail, opts)

				if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
					t.Errorf("exec = %q %v, want /bin/sh %v", exec.target, exec.args, eagerHookArgs)
				}
				assertNoFlag(t)
				assertNoPaneOptionWrite(t, cmder)
			})
		}
	}
}

func TestHydrateLazy_PinnedLazyBeatsAnEagerInstall(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, "eager"), exec)

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/sh" {
		t.Fatalf("exec target = %q, want /bin/sh", exec.target)
	}
	if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

func TestHydrateLazy_RaisesTheFlagBeforeClearingTheMidRestoreMarker(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			var raisedAtClear bool
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			cmder := flagAtSkeletonClear(t, &raisedAtClear)
			opts.Commander = cmder

			paneKey := lazyRun(t, tail, opts)

			if cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex() < 0 {
				t.Fatalf("no skeleton marker clear; calls: %v", cmder.Calls())
			}
			if !raisedAtClear {
				t.Error("the skeleton marker cleared before the session's flag was raised")
			}
			if !flagRaisedAt(t, lazyFlag(t)) {
				t.Error("the parked session's flag is not raised")
			}
			assertNoPaneOptionWrite(t, cmder)
		})
	}
}

func TestHydrateLazy_ParksTheDrawAndTheTailInOneShell(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/sh" {
		t.Errorf("exec target = %q, want /bin/sh", exec.target)
	}
	if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
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
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, command, resumemode.Lazy), lazyPrefs(t, ""), exec)

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
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, lazyTails()[0], opts)

	parked := parkedChain{Exe: lazyExe, Flag: lazyFlag(t)}
	payload := resumeChainPayload{Command: "echo hi", Session: lazySession}
	tail, found := strings.CutPrefix(exec.args[2], parkedChainTrap+parkedChainDraw(parked, payload))
	if !found {
		t.Fatalf("chain %q does not open on the draw", exec.args[2])
	}
	if !strings.HasPrefix(tail, "; "+shellquote.Single(lazyExe)) {
		t.Errorf("tail %q is not separated from the draw by a semicolon", tail)
	}
}

func TestHydrateLazy_ComposesTheTailWithTheSessionAlone(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, lazyTails()[0], opts)

	parked := parkedChain{Exe: lazyExe, Flag: lazyFlag(t)}
	payload := resumeChainPayload{Command: "echo hi", Session: lazySession}
	tail, found := strings.CutPrefix(exec.args[2], parkedChainTrap+parkedChainDraw(parked, payload)+"; ")
	if !found {
		t.Fatalf("chain %q has no tail", exec.args[2])
	}
	want := shellquote.Join([]string{lazyExe, "state", resumeRecoverSubcommand, "--session", lazySession}) + parkedChainBackstop(parked)
	if tail != want {
		t.Errorf("tail = %q, want %q", tail, want)
	}
}

func TestHydrateLazy_FiresTheHookWhenTheFlagCannotBeRaised(t *testing.T) {
	refusals := []struct {
		name    string
		session string
		prepare func(t *testing.T, opts *hydrateCfgOpts)
	}{
		{name: "the flag's directory can't be made", session: lazySession, prepare: func(t *testing.T, _ *hydrateCfgOpts) {
			t.Helper()
			unwritableStateDir(t)
		}},
		{name: "the session's name can't name a file", session: "..", prepare: func(t *testing.T, opts *hydrateCfgOpts) {
			t.Helper()
			opts.HookKey = ".."
			opts.HookStore = hydrateStoreWithMode(t, "..", "echo hi", resumemode.Lazy)
		}},
		{name: "the state directory can't be resolved", session: lazySession, prepare: func(t *testing.T, _ *hydrateCfgOpts) {
			t.Helper()
			t.Setenv("PORTAL_STATE_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("HOME", "")
		}},
		{name: "the executable can't be resolved", session: lazySession, prepare: func(t *testing.T, opts *hydrateCfgOpts) {
			t.Helper()
			opts.ResolveExe = func() (string, error) { return "", errors.New("no executable") }
		}},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet()
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Commander, opts.Logger = cmder, logger
			assertNoFlagLater := noFlagIn(t)
			refusal.prepare(t, &opts)

			paneKey := lazyRun(t, lazyTails()[0], opts)

			if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
				t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
			}
			assertOneFlagRefusalWarn(t, sink, refusal.session)
			if n := strings.Count(sink.Body(), "WARN"); n != 1 {
				t.Errorf("got %d WARNs, want the refusal alone: %q", n, sink.Body())
			}
			assertNoFlagLater()
			if cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex() < 0 {
				t.Errorf("a refused flag must still clear the skeleton marker; calls: %v", cmder.Calls())
			}
		})
	}
}

func TestHydrateLazy_FiresTheHookOnEveryTailWhenTheFlagCannotBeRaised(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cmder := commandertest.Quiet()
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			unwritableStateDir(t)
			opts.Commander, opts.Logger = cmder, logger

			paneKey := lazyRun(t, tail, opts)

			if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
				t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
			}
			assertOneFlagRefusalWarn(t, sink, lazySession)
			if cmder.CallsMatching("set-option", "-su", state.SkeletonMarkerPrefix+paneKey).FirstIndex() < 0 {
				t.Errorf("a refused flag must still clear the skeleton marker; calls: %v", cmder.Calls())
			}
		})
	}
}

func TestHydrateLazy_ResolvesTheShippedDefaultWhenThePrefsReadFails(t *testing.T) {
	exec := &stubExecShell{}
	unreadable := t.TempDir()
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Unset), func() (*prefs.Store, error) {
		return prefs.NewStore(unreadable), nil
	}, exec)

	lazyRun(t, lazyTails()[0], opts)

	if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

func TestHydrateLazy_ResolvesTheShippedDefaultWhenThePrefsStoreCannotBeBuilt(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Unset), func() (*prefs.Store, error) {
		return nil, errors.New("no config base")
	}, exec)

	lazyRun(t, lazyTails()[0], opts)

	if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

func TestHydrateLazy_AbsentHookStoreIsNoRegistration(t *testing.T) {
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	opts := lazyOpts(t, nil, lazyPrefs(t, ""), exec)
	opts.Logger = logger

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
	}
	assertNoFlag(t)
	if dbg := execLogLine(t, sink.Body(), "DEBUG", "hook lookup"); !strings.Contains(dbg, "result=miss") {
		t.Errorf("absent store must record a miss: %q", dbg)
	}
}

func TestHydrateLazy_UnreadableStoreGivesABareShellAndNeverAWait(t *testing.T) {
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Unreadable: true})
	opts := lazyOpts(t, store, lazyPrefs(t, ""), exec)
	opts.Logger = logger

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
	}
	assertNoFlag(t)
	body := sink.Body()
	if dbg := execLogLine(t, body, "DEBUG", "hook lookup"); !strings.Contains(dbg, "result=error") {
		t.Errorf("an unreadable store must record result=error: %q", dbg)
	}
	if n := strings.Count(body, "lookup on-resume hook failed"); n != 1 {
		t.Errorf("want exactly one lookup-failure WARN, got %d: %q", n, body)
	}
}

func TestHydrateLazy_ReadsTheStoreOnceAndThePrefsOncePerSession(t *testing.T) {
	for _, tail := range lazyTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			loads := 0
			seam := lazyPrefs(t, "")
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Eager), func() (*prefs.Store, error) {
				loads++
				return seam()
			}, exec)
			opts.Logger = logger

			lazyRun(t, tail, opts)

			if n := strings.Count(sink.Body(), "DEBUG hook lookup"); n != 1 {
				t.Errorf("hook lookup recorded %d times, want exactly one per session: %q", n, sink.Body())
			}
			if loads != 1 {
				t.Errorf("prefs loaded %d times, want exactly one per session", loads)
			}
		})
	}
}

func TestHydrateLazy_ComposesTheChainFromTheValuesTheMarkStepResolved(t *testing.T) {
	exec := &stubExecShell{}
	exes := 0
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
	want := lazyChainArgs(t, "echo hi")
	// The state directory moves the moment the flag is raised, so a chain
	// composed from a second resolution would carry another flag.
	moved := t.TempDir()
	opts.Commander = commandertest.Quiet(
		commandertest.When(commandertest.ArgvPrefix("set-option", "-su"), "", nil).
			Doing(func([]string) { t.Setenv("PORTAL_STATE_DIR", moved) }),
	)
	opts.ResolveExe = func() (string, error) { exes++; return lazyExe, nil }

	lazyRun(t, lazyTails()[0], opts)

	if exes != 1 {
		t.Errorf("executable resolved %d times, want exactly one per session", exes)
	}
	if !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the chain composed from the raised flag %v", exec.args, want)
	}
}

func TestHydrateLazy_EmptyStoredCommandIsNoRegistration(t *testing.T) {
	exec := &stubExecShell{}
	opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, lazyTails()[0], opts)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"/bin/zsh"}) {
		t.Errorf("exec = %q %v, want a bare /bin/zsh", exec.target, exec.args)
	}
	assertNoFlag(t)
}

func TestHydrateLazy_RaisesAndParksWhateverTheHandlerSeamDoes(t *testing.T) {
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
			var raisedAtClear bool
			opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			opts.Commander = flagAtSkeletonClear(t, &raisedAtClear)
			stage := tail.stage
			tail.stage = func(t *testing.T, dir string, opts *hydrateCfgOpts) string {
				t.Helper()
				paneKey := stage(t, dir, opts)
				replace(opts)
				return paneKey
			}

			lazyRun(t, tail, opts)

			if !raisedAtClear {
				t.Error("the skeleton marker cleared before the session's flag was raised")
			}
			if want := lazyChainArgs(t, "echo hi"); exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec = %q %v, want the parked chain %v", exec.target, exec.args, want)
			}
		})
	}
}

func TestHydrateLazy_ExecWithNoParkedSessionRunsTheHookAndLooksNothingUp(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateCfg(t, hydrateCfgOpts{
		HookKey:   lazySession,
		HookStore: hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy),
		OpenFIFO:  unexpectedOpenFIFO(t),
		Logger:    logger,
		ExecShell: exec.fn(),
	})
	registration := hooks.OnResume{Found: true, Command: "echo hi", Mode: resumemode.Lazy}

	execShellOrHookAndExit(cfg, registration, nil)

	if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
		t.Errorf("exec = %q %v, want the hook %v", exec.target, exec.args, eagerHookArgs)
	}
	body := sink.Body()
	if info := execLogLine(t, body, "INFO", "exec"); !strings.Contains(info, "hook_present=true") {
		t.Errorf("exec INFO missing hook_present=true: %q", info)
	}
	if n := strings.Count(body, "hook lookup"); n != 0 {
		t.Errorf("the exec looked the registration up itself %d times: %q", n, body)
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
			resumeStateDir(t)
			cmder := commandertest.Quiet()
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			dir := t.TempDir()
			cfg := hydrateCfg(t, hydrateCfgOpts{
				FIFO:      filepath.Join(dir, "hydrate-alone__0.0.fifo"),
				File:      filepath.Join(dir, "sb"),
				HookKey:   lazySession,
				HookStore: hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy),
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
			assertNoFlag(t)
			body := sink.Body()
			for _, line := range wantLines[name] {
				if n := strings.Count(body, line); n != 1 {
					t.Errorf("want exactly one %q, got %d: %q", line, n, body)
				}
			}
		})
	}
}

// assertNoFlag fails on any resume-pending flag in the state directory: the
// session must not wait.
func assertNoFlag(t *testing.T) {
	t.Helper()
	noFlagIn(t)()
}

// noFlagIn fixes the state directory the test runs in now and returns a check
// that no flag was raised in it, for a case that moves PORTAL_STATE_DIR on.
func noFlagIn(t *testing.T) func() {
	t.Helper()
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if raised, err := state.ResumePending.List(dir); err != nil || len(raised) != 0 {
			t.Errorf("resume-pending flags = %v, %v; want none on a session that must not wait", raised, err)
		}
	}
}

// assertNoPaneOptionWrite fails on any pane option written: the token and the
// alternate-screen pin a tmux pane once carried are gone.
func assertNoPaneOptionWrite(t *testing.T, cmder *commandertest.Scripted) {
	t.Helper()
	for _, call := range cmder.Calls() {
		if len(call) > 1 && call[0] == "set-option" && strings.HasPrefix(call[1], "-p") {
			t.Errorf("wrote a pane option %v, want none", call)
		}
	}
}

func assertOneFlagRefusalWarn(t *testing.T, sink *logtest.Sink, session string) {
	t.Helper()
	if n := strings.Count(sink.Body(), "WARN set resume pending flag failed"); n != 1 {
		t.Errorf("got %d refusal WARNs, want one: %q", n, sink.Body())
	}
	warn := execLogLine(t, sink.Body(), "WARN", "set resume pending flag failed")
	if !strings.Contains(warn, "session="+session) {
		t.Errorf("refusal WARN does not name the session: %q", warn)
	}
	if !strings.Contains(warn, "error=") {
		t.Errorf("refusal WARN carries no error: %q", warn)
	}
}

func TestHydrateLazy_LowersAFlagASetLeftBehindWhenItFailed(t *testing.T) {
	// makesThenFails raises the flag as Set would and then fails, as one whose
	// close failed does.
	makesThenFails := func(t *testing.T, after func(dir string)) func(dir, name string) error {
		return func(dir, name string) error {
			if err := state.ResumePending.Set(dir, name); err != nil {
				t.Fatal(err)
			}
			after(dir)
			return errors.New("set flag: close: input/output error")
		}
	}

	t.Run("it lowers the flag again, with the refusal the only WARN", func(t *testing.T) {
		exec := &stubExecShell{}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		opts.Logger = logger
		opts.SetPending = makesThenFails(t, func(string) {})

		lazyRun(t, lazyTails()[0], opts)

		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
		}
		assertNoFlag(t)
		assertOneFlagRefusalWarn(t, sink, lazySession)
		if n := strings.Count(sink.Body(), "WARN"); n != 1 {
			t.Errorf("got %d WARNs, want the refusal alone: %q", n, sink.Body())
		}
	})

	t.Run("a lowering that fails too is logged beside the refusal", func(t *testing.T) {
		exec := &stubExecShell{}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		opts := lazyOpts(t, hydrateStoreWithMode(t, lazySession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		opts.Logger = logger
		opts.SetPending = makesThenFails(t, func(dir string) {
			unsearchable(t, filepath.Join(dir, string(state.ResumePending)))
		})

		lazyRun(t, lazyTails()[0], opts)

		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
		}
		assertOneFlagRefusalWarn(t, sink, lazySession)
		warn := execLogLine(t, sink.Body(), "WARN", "clear resume pending flag failed")
		if !strings.Contains(warn, "session="+lazySession) {
			t.Errorf("lowering WARN does not name the session: %q", warn)
		}
	})
}
