package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/state"
)

const lazyExe = "/opt/portal/bin/portal"

var eagerHookArgs = []string{"sh", "-c", "trap : TERM; echo hi; exec '/bin/zsh' -l"}

// hydrateTail is one route the helper takes to its hand-off: the replay of a
// readable transcript, a transcript it can't read, and none saved.
type hydrateTail struct {
	name  string
	stage func(t *testing.T, cfg *hydrateConfig)
}

func hydrateTails() []hydrateTail {
	return []hydrateTail{
		{name: "replay", stage: func(t *testing.T, cfg *hydrateConfig) {
			cfg.File = transcript(t, "OLD")
		}},
		{name: "scrollback missing", stage: func(t *testing.T, cfg *hydrateConfig) {
			cfg.File = filepath.Join(t.TempDir(), "absent.bin")
		}},
		{name: "no scrollback saved", stage: func(*testing.T, *hydrateConfig) {}},
	}
}

// lazyRun runs the helper down tail.
func lazyRun(t *testing.T, tail hydrateTail, cfg hydrateConfig) {
	t.Helper()
	tail.stage(t, &cfg)
	runHydrate(t.Context(), cfg)
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

// lazyConfig is the config every lazy case starts from: the world's session,
// registered in store, under the install loadPrefs reads, with an executable
// that resolves.
func lazyConfig(t *testing.T, store *hooks.Store, loadPrefs func() (*prefs.Store, error), exec *stubExecShell) hydrateConfig {
	t.Helper()
	t.Setenv("SHELL", "/bin/zsh")
	cfg := newHydrateWorld(t).config(exec)
	cfg.HookStore = store
	cfg.LoadPrefsStore = loadPrefs
	cfg.ResolveExe = func() (string, error) { return lazyExe, nil }
	return cfg
}

// lazyFlag is the session's flag in the state directory the test runs in.
func lazyFlag(t *testing.T) string {
	t.Helper()
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	path, err := state.ResumePending.Path(dir, hydrateSession)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// lazyChainArgs is the argv a parked chain execs for the session.
func lazyChainArgs(t *testing.T, command string) []string {
	t.Helper()
	parked := parkedChain{Exe: lazyExe, Flag: lazyFlag(t)}
	return []string{"sh", "-c", parkedResumeChain(parked, resumeChainPayload{Command: command, Session: hydrateSession})}
}

func flagRaisedAt(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("look for the flag: %v", err)
	}
	return err == nil
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
	for _, tail := range hydrateTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cfg := lazyConfig(t, emptyHookStore(t), lazyPrefs(t, ""), exec)

			lazyRun(t, tail, cfg)

			if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
				t.Errorf("exec = %q %v, want the login shell -zsh", exec.target, exec.args)
			}
			assertNoFlag(t)
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
		for _, tail := range hydrateTails() {
			t.Run(tc.name+"/"+tail.name, func(t *testing.T) {
				exec := &stubExecShell{}
				cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", tc.registration), lazyPrefs(t, tc.install), exec)

				lazyRun(t, tail, cfg)

				if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
					t.Errorf("exec = %q %v, want /bin/sh %v", exec.target, exec.args, eagerHookArgs)
				}
				assertNoFlag(t)
			})
		}
	}
}

func TestHydrateLazy_PinnedLazyBeatsAnEagerInstall(t *testing.T) {
	exec := &stubExecShell{}
	cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, "eager"), exec)

	lazyRun(t, hydrateTails()[0], cfg)

	if exec.target != "/bin/sh" {
		t.Fatalf("exec target = %q, want /bin/sh", exec.target)
	}
	if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
	}
}

// decision 6: a session portal migrate made waits on its panel at its first
// restore whatever its hook's mode, since the tmux session it came from may
// still run the program its hook resumes.
func TestHydrateLazy_ARestoreAskingLazyParksWhateverTheMode(t *testing.T) {
	cases := []struct {
		name         string
		registration resumemode.Mode
		install      string
	}{
		{name: "pinned eager", registration: resumemode.Eager, install: ""},
		{name: "naming no mode under an eager install", registration: resumemode.Unset, install: "eager"},
		{name: "pinned lazy", registration: resumemode.Lazy, install: "eager"},
	}
	for _, tc := range cases {
		for _, tail := range hydrateTails() {
			t.Run(tc.name+"/"+tail.name, func(t *testing.T) {
				exec := &stubExecShell{}
				cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", tc.registration), lazyPrefs(t, tc.install), exec)
				cfg.Lazy = true

				lazyRun(t, tail, cfg)

				if exec.target != "/bin/sh" {
					t.Fatalf("exec target = %q, want /bin/sh", exec.target)
				}
				if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
					t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
				}
				if !flagRaisedAt(t, lazyFlag(t)) {
					t.Error("the parked session's flag is not raised")
				}
			})
		}
	}
}

func TestHydrateLazy_ARestoreAskingLazyWithNoHookRestoresAsToday(t *testing.T) {
	exec := &stubExecShell{}
	cfg := lazyConfig(t, emptyHookStore(t), lazyPrefs(t, ""), exec)
	cfg.Lazy = true

	lazyRun(t, hydrateTails()[0], cfg)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
		t.Errorf("exec = %q %v, want the login shell -zsh: there is nothing to wait for", exec.target, exec.args)
	}
	assertNoFlag(t)
}

func TestHydrateLazy_TheLazyFlagReachesTheHelper(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{args: []string{"--session", hydrateSession}, want: false},
		{args: []string{"--session", hydrateSession, "--lazy"}, want: true},
		{args: []string{"--session", hydrateSession, "--lazy", "--file", "/tmp/s.bin"}, want: true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			newHydrateWorld(t)
			var got hydrateConfig
			withFuncSeam(t, &hydrateRunFunc, func(_ context.Context, cfg hydrateConfig) { got = cfg })

			if _, _, err := runRootCmd(t, append([]string{"state", "hydrate"}, tc.args...)...); err != nil {
				t.Fatalf("state hydrate: %v", err)
			}

			if got.Lazy != tc.want {
				t.Errorf("Lazy = %v, want %v", got.Lazy, tc.want)
			}
		})
	}
}

func TestHydrateLazy_RaisesTheFlagBeforeTheRestoreEnds(t *testing.T) {
	for _, tail := range hydrateTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			lowered := false
			var raisedAtLowering bool
			cfg.ClearHydrating = func(dir, name string) (bool, error) {
				lowered, raisedAtLowering = true, flagRaisedAt(t, lazyFlag(t))
				return state.Hydrating.Clear(dir, name)
			}

			lazyRun(t, tail, cfg)

			if !lowered {
				t.Fatal("the hydrating flag was never lowered")
			}
			if !raisedAtLowering {
				t.Error("the hydrating flag came down before the session's resume-pending flag was raised")
			}
			if !flagRaisedAt(t, lazyFlag(t)) {
				t.Error("the parked session's flag is not raised")
			}
			if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
			}
		})
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
			cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, command, resumemode.Lazy), lazyPrefs(t, ""), exec)

			lazyRun(t, hydrateTails()[0], cfg)

			if chained := exec.args[2]; !strings.Contains(chained, shellquote.Single(command)) {
				t.Errorf("chain %q does not carry the command as one quoted word %q", chained, shellquote.Single(command))
			}
		})
	}
}

func TestHydrateLazy_ParksTheDrawAndThenTheTailWithTheSessionAlone(t *testing.T) {
	exec := &stubExecShell{}
	cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, hydrateTails()[0], cfg)

	parked := parkedChain{Exe: lazyExe, Flag: lazyFlag(t)}
	payload := resumeChainPayload{Command: "echo hi", Session: hydrateSession}
	tail, found := strings.CutPrefix(exec.args[2], parkedChainTrap+parkedChainDraw(parked, payload)+"; ")
	if !found {
		t.Fatalf("chain %q does not open on the draw, a semicolon after it", exec.args[2])
	}
	want := shellquote.Join([]string{lazyExe, "state", resumeRecoverSubcommand, "--session", hydrateSession}) + parkedChainBackstop(parked)
	if tail != want {
		t.Errorf("tail = %q, want %q", tail, want)
	}
}

func TestHydrateLazy_FiresTheHookWhenTheFlagCannotBeRaised(t *testing.T) {
	refusals := []struct {
		name    string
		session string
		prepare func(t *testing.T, cfg *hydrateConfig)
	}{
		{name: "the flag's directory can't be made", session: hydrateSession, prepare: func(t *testing.T, _ *hydrateConfig) {
			unwritableStateDir(t)
		}},
		{name: "the session's name can't name a file", session: "..", prepare: func(t *testing.T, cfg *hydrateConfig) {
			cfg.Session, cfg.Current = "..", ".."
			cfg.HookStore = hydrateStoreWithMode(t, "..", "echo hi", resumemode.Lazy)
		}},
		{name: "the state directory can't be resolved", session: hydrateSession, prepare: func(t *testing.T, _ *hydrateConfig) {
			t.Setenv("PORTAL_STATE_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("HOME", "")
		}},
		{name: "the executable can't be resolved", session: hydrateSession, prepare: func(_ *testing.T, cfg *hydrateConfig) {
			cfg.ResolveExe = func() (string, error) { return "", errors.New("no executable") }
		}},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			exec := &stubExecShell{}
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			cfg.Logger = logger
			assertNoFlagLater := noFlagIn(t)
			refusal.prepare(t, &cfg)

			lazyRun(t, hydrateTails()[0], cfg)

			if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
				t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
			}
			assertOneFlagRefusalWarn(t, sink, refusal.session)
			assertNoFlagLater()
		})
	}
}

func TestHydrateLazy_FiresTheHookOnEveryTailWhenTheFlagCannotBeRaised(t *testing.T) {
	for _, tail := range hydrateTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
			cfg.Logger = logger
			cfg.ResolveExe = func() (string, error) { return "", errors.New("no executable") }

			lazyRun(t, tail, cfg)

			if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
				t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
			}
			assertOneFlagRefusalWarn(t, sink, hydrateSession)
			assertNoFlag(t)
		})
	}
}

// A session restore asked to wait came from a tmux session that may still
// run the program its hook resumes: a park that fails gives it its shell,
// never the hook, whatever the hook's mode.
func TestHydrateLazy_ARestoreAskingLazyGivesTheShellWhenTheFlagCannotBeRaised(t *testing.T) {
	refusals := map[string]func(t *testing.T, cfg *hydrateConfig){
		"the executable can't be resolved": func(_ *testing.T, cfg *hydrateConfig) {
			cfg.ResolveExe = func() (string, error) { return "", errors.New("no executable") }
		},
		"the flag can't be raised": func(_ *testing.T, cfg *hydrateConfig) {
			cfg.SetPending = func(string, string) error { return errors.New("disk full") }
		},
	}
	for name, refuse := range refusals {
		for _, mode := range []resumemode.Mode{resumemode.Eager, resumemode.Lazy} {
			for _, tail := range hydrateTails() {
				t.Run(name+"/"+mode.String()+"/"+tail.name, func(t *testing.T) {
					exec := &stubExecShell{}
					logger, sink := newCaptureLoggerForComponent(t, "hydrate")
					cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", mode), lazyPrefs(t, ""), exec)
					cfg.Logger = logger
					cfg.Lazy = true
					refuse(t, &cfg)

					lazyRun(t, tail, cfg)

					if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
						t.Errorf("exec = %q %v, want the login shell -zsh, the hook unfired", exec.target, exec.args)
					}
					assertOneFlagRefusalWarn(t, sink, hydrateSession)
					if warn := execLogLine(t, sink.Body(), "WARN", "set resume pending flag failed"); !strings.Contains(warn, "its hook unfired") {
						t.Errorf("refusal WARN = %q, want it saying the hook goes unfired", warn)
					}
					assertNoFlag(t)
				})
			}
		}
	}
}

func TestHydrateLazy_ResolvesTheShippedDefaultWhenThePrefsCannotBeRead(t *testing.T) {
	unreadable := t.TempDir()
	for name, load := range map[string]func() (*prefs.Store, error){
		"a read that fails":         func() (*prefs.Store, error) { return prefs.NewStore(unreadable), nil },
		"a store that can't be had": func() (*prefs.Store, error) { return nil, errors.New("no config base") },
	} {
		t.Run(name, func(t *testing.T) {
			exec := &stubExecShell{}
			cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Unset), load, exec)

			lazyRun(t, hydrateTails()[0], cfg)

			if want := lazyChainArgs(t, "echo hi"); !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec args = %v, want the parked chain %v", exec.args, want)
			}
		})
	}
}

func TestHydrateLazy_AbsentHookStoreIsNoRegistration(t *testing.T) {
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := lazyConfig(t, nil, lazyPrefs(t, ""), exec)
	cfg.Logger = logger

	lazyRun(t, hydrateTails()[0], cfg)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
		t.Errorf("exec = %q %v, want the login shell -zsh", exec.target, exec.args)
	}
	assertNoFlag(t)
	if dbg := execLogLine(t, sink.Body(), "DEBUG", "hook lookup"); !strings.Contains(dbg, "result=miss") {
		t.Errorf("absent store must record a miss: %q", dbg)
	}
}

func TestHydrateLazy_UnreadableStoreGivesTheLoginShellAndNeverAWait(t *testing.T) {
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Unreadable: true})
	cfg := lazyConfig(t, store, lazyPrefs(t, ""), exec)
	cfg.Logger = logger

	lazyRun(t, hydrateTails()[0], cfg)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
		t.Errorf("exec = %q %v, want the login shell -zsh", exec.target, exec.args)
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
	for _, tail := range hydrateTails() {
		t.Run(tail.name, func(t *testing.T) {
			exec := &stubExecShell{}
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			loads := 0
			seam := lazyPrefs(t, "")
			cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Eager), func() (*prefs.Store, error) {
				loads++
				return seam()
			}, exec)
			cfg.Logger = logger

			lazyRun(t, tail, cfg)

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
	cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
	want := lazyChainArgs(t, "echo hi")
	// The state directory moves once the flag is raised, so a chain composed
	// from a second resolution would carry another flag.
	moved := t.TempDir()
	cfg.ClearHydrating = func(dir, name string) (bool, error) {
		t.Setenv("PORTAL_STATE_DIR", moved)
		return state.Hydrating.Clear(dir, name)
	}
	cfg.ResolveExe = func() (string, error) { exes++; return lazyExe, nil }

	lazyRun(t, hydrateTails()[0], cfg)

	if exes != 1 {
		t.Errorf("executable resolved %d times, want exactly one per session", exes)
	}
	if !reflect.DeepEqual(exec.args, want) {
		t.Errorf("exec args = %v, want the chain composed from the raised flag %v", exec.args, want)
	}
}

func TestHydrateLazy_EmptyStoredCommandIsNoRegistration(t *testing.T) {
	exec := &stubExecShell{}
	cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "", resumemode.Lazy), lazyPrefs(t, ""), exec)

	lazyRun(t, hydrateTails()[0], cfg)

	if exec.target != "/bin/zsh" || !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
		t.Errorf("exec = %q %v, want the login shell -zsh", exec.target, exec.args)
	}
	assertNoFlag(t)
}

func TestHydrateLazy_ExecWithNoParkedSessionRunsTheHookAndLooksNothingUp(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	exec := &stubExecShell{}
	logger, sink := newCaptureLoggerForComponent(t, "hydrate")
	cfg := hydrateConfig{Session: hydrateSession, Logger: logger, ExecShell: exec.fn()}
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
		cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		cfg.Logger = logger
		cfg.SetPending = makesThenFails(t, func(string) {})

		lazyRun(t, hydrateTails()[0], cfg)

		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
		}
		assertNoFlag(t)
		assertOneFlagRefusalWarn(t, sink, hydrateSession)
		if n := strings.Count(sink.Body(), "WARN"); n != 1 {
			t.Errorf("got %d WARNs, want the refusal alone: %q", n, sink.Body())
		}
	})

	t.Run("a lowering that fails too is logged beside the refusal", func(t *testing.T) {
		exec := &stubExecShell{}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		cfg.Logger = logger
		cfg.SetPending = makesThenFails(t, func(dir string) {
			unsearchable(t, filepath.Join(dir, string(state.ResumePending)))
		})

		lazyRun(t, hydrateTails()[0], cfg)

		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
		}
		assertOneFlagRefusalWarn(t, sink, hydrateSession)
		warn := execLogLine(t, sink.Body(), "WARN", "clear resume pending flag failed")
		if !strings.Contains(warn, "session="+hydrateSession) {
			t.Errorf("lowering WARN does not name the session: %q", warn)
		}
	})

	// Set's mkdir fails with ENOTDIR, and a lowering would too: nothing can
	// stand at the flag's path, so none is owed and no lowering is logged.
	t.Run("a set refused for a file where the flag's directory goes owes no lowering", func(t *testing.T) {
		exec := &stubExecShell{}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := lazyConfig(t, hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Lazy), lazyPrefs(t, ""), exec)
		cfg.Logger = logger
		flags := filepath.Join(os.Getenv("PORTAL_STATE_DIR"), string(state.ResumePending))
		if err := os.WriteFile(flags, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		lazyRun(t, hydrateTails()[0], cfg)

		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, eagerHookArgs) {
			t.Errorf("exec = %q %v, want the eager hook %v", exec.target, exec.args, eagerHookArgs)
		}
		assertOneFlagRefusalWarn(t, sink, hydrateSession)
		if refusal := execLogLine(t, sink.Body(), "WARN", "set resume pending flag failed"); !strings.Contains(refusal, "not a directory") {
			t.Errorf("refusal WARN = %q, want the mkdir's ENOTDIR", refusal)
		}
		if n := strings.Count(sink.Body(), "WARN"); n != 1 {
			t.Errorf("got %d WARNs, want the refusal alone: %q", n, sink.Body())
		}
	})
}
