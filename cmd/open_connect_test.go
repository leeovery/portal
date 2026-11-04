package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// attachRecorder makes no attach. It records each run the runner is asked
// for, answering from results, and each exec instead of replacing the test
// process, with whether the exec marker was already logged when it ran. Its
// runners stand in for the runner registry.
type attachRecorder struct {
	sink *logtest.Sink
	runs []runner.Config
	// returned counts the runs that have returned.
	returned int
	// results answers each run in turn; a run past them ends as a session that
	// exits 0 ends it.
	results    []runResult
	execs      []execCall
	markerSeen bool
	runners    runnersFake
}

type runResult struct {
	out runner.Outcome
	err error
	// ended, if set, runs as the run returns.
	ended func()
}

type execCall struct {
	argv0     string
	argv, env []string
}

// Run begins the attach, as runner.Run does, unless it answers with an error.
func (r *attachRecorder) Run(_ context.Context, cfg runner.Config) (runner.Outcome, error) {
	r.runs = append(r.runs, cfg)
	defer func() { r.returned++ }()
	var res runResult
	if len(r.results) > 0 {
		res = r.results[0]
		r.results = r.results[1:]
	}
	if res.err == nil && cfg.Began != nil {
		cfg.Began()
	}
	if res.ended != nil {
		res.ended()
	}
	return res.out, res.err
}

func (r *attachRecorder) Exec(argv0 string, argv, env []string) error {
	r.execs = append(r.execs, execCall{argv0: argv0, argv: argv, env: env})
	if r.sink != nil && len(r.sink.Records().Matching("process", "exec")) > 0 {
		r.markerSeen = true
	}
	return nil
}

// answer queues the outcomes of the next runs.
func (r *attachRecorder) answer(results ...runResult) {
	r.results = append(r.results, results...)
}

// onlyRun is the one run, and no exec, the test expects.
func (r *attachRecorder) onlyRun(t *testing.T) runner.Config {
	t.Helper()
	if len(r.runs) != 1 || len(r.execs) != 0 {
		t.Fatalf("runs = %+v, execs = %+v; want exactly one run and no exec", r.runs, r.execs)
	}
	return r.runs[0]
}

// onlyExec is the one exec, and no run, the test expects.
func (r *attachRecorder) onlyExec(t *testing.T) execCall {
	t.Helper()
	if len(r.execs) != 1 || len(r.runs) != 0 {
		t.Fatalf("execs = %+v, runs = %+v; want exactly one exec and no run", r.execs, r.runs)
	}
	return r.execs[0]
}

func (r *attachRecorder) assertNone(t *testing.T) {
	t.Helper()
	if len(r.runs) != 0 || len(r.execs) != 0 {
		t.Errorf("runs = %+v, execs = %+v; want no attach", r.runs, r.execs)
	}
}

// envBackend is a Fake whose attach command carries an environment, as the
// zmx driver's does.
type envBackend struct {
	*backendtest.Fake
	env []string
	// emptyArgv breaks the attach command, as no driver should.
	emptyArgv bool
}

func (b envBackend) AttachCommand(ctx context.Context, name string) (backend.Command, error) {
	c, err := b.Fake.AttachCommand(ctx, name)
	if err == nil {
		c.Env = b.env
	}
	if b.emptyArgv {
		c.Argv = nil
	}
	return c, err
}

var attachEnv = []string{"ZMX_DIR=/tmp/zx-open-test", "TERM=xterm-256color"}

// stageAttach stages a Fake holding sessions behind an environment-carrying
// attach, and a runner and an exec that record instead of attaching, owing no
// bootstrap.
func stageAttach(t *testing.T, sessions ...string) (*backendtest.Fake, *attachRecorder) {
	t.Helper()
	fake := backendtest.New()
	for _, name := range sessions {
		fake.Add(backend.Session{Name: name})
	}
	withBootstrapDeps(t, BootstrapDeps{})
	withBackendDeps(t, BackendDeps{Backend: envBackend{Fake: fake, env: attachEnv}})
	rec := &attachRecorder{runners: runnersFake{switchErr: runnertoken.ErrNoRunner}}
	withOpenDeps(t, OpenDeps{Execer: rec, Runner: rec.Run, Runners: &rec.runners})
	return fake, rec
}

// runnersFake stands in for the runner registry. It answers each switch with
// switchErr, marks each attach's command with runnerToken through the backend
// its listing was given, and records the switches asked for and, in order,
// what each attach did to the listing: listed or moved to a session as it
// began, ended, and unlisted.
type runnersFake struct {
	switchErr error
	switches  []switchCall
	events    []string
	listed    bool
	backend   runnerreg.Backend
}

// runnerToken is the token runnersFake lists every runner under.
const runnerToken = "tok123"

type switchCall struct{ token, current, target string }

func (f *runnersFake) Listing(b runnerreg.Backend) runnerListing {
	f.backend = b
	return f
}

func (f *runnersFake) Switch(_ context.Context, token, current, target string) error {
	f.switches = append(f.switches, switchCall{token, current, target})
	return f.switchErr
}

func (f *runnersFake) Attach(cfg *runner.Config) {
	cfg.Command = f.backend.Track(cfg.Command, runnertoken.Var, runnerToken)
	session := cfg.Session
	cfg.Began = func() {
		if f.listed {
			f.events = append(f.events, "moved "+session)
		} else {
			f.events = append(f.events, "listed "+session)
		}
		f.listed = true
	}
}

func (f *runnersFake) Ended() { f.events = append(f.events, "ended") }

func (f *runnersFake) Close() {
	if f.listed {
		f.events = append(f.events, "unlisted")
	}
	f.listed = false
}

func contextCommand() *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(context.Background())
	return c
}

// assertAttached checks the one attach ran the session's attach command, its
// environment and all, marked with the runner's token through the backend, on
// Portal's own terminal, watching for keys.
func assertAttached(t *testing.T, rec *attachRecorder, name string) {
	t.Helper()
	cfg := rec.onlyRun(t)
	if cfg.Session != name {
		t.Errorf("session = %q, want %q", cfg.Session, name)
	}
	if want := []string{backendtest.AttachProgram, name}; !slices.Equal(cfg.Command.Argv, want) {
		t.Errorf("argv = %q, want %q", cfg.Command.Argv, want)
	}
	if want := append(slices.Clone(attachEnv), runnertoken.Var+"="+runnerToken); !slices.Equal(cfg.Command.Env, want) {
		t.Errorf("env = %q, want the attach command's own, marked, %q", cfg.Command.Env, want)
	}
	if cfg.In != os.Stdin || cfg.Out != os.Stdout {
		t.Errorf("terminal = %v, %v; want Portal's stdin and stdout", cfg.In, cfg.Out)
	}
	if cfg.Scanner == nil {
		t.Error("no scanner watches for Portal's keys")
	}
}

// assertExeced checks the one attach exec'd the session's attach command with
// its environment, unmarked: an exec'd client is no runner.
func assertExeced(t *testing.T, rec *attachRecorder, name string) {
	t.Helper()
	call := rec.onlyExec(t)
	if call.argv0 != backendtest.AttachProgram {
		t.Errorf("exec'd %q, want the attach command's program %q", call.argv0, backendtest.AttachProgram)
	}
	if want := []string{backendtest.AttachProgram, name}; !slices.Equal(call.argv, want) {
		t.Errorf("argv = %q, want %q", call.argv, want)
	}
	if !slices.Equal(call.env, attachEnv) {
		t.Errorf("env = %q, want the attach command's own %q", call.env, attachEnv)
	}
}

func TestOpenSession_AttachesThroughTheRunner(t *testing.T) {
	t.Run("it runs the attach command, with its environment, on Portal's terminal", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
	})

	t.Run("inside a session with no runner it execs the command, which switches and exits, and never runs the runner", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("blog-c3d4")
		if command, _ := fake.AttachCommand(context.Background(), "api-x7Kd9a"); !command.Switches {
			t.Fatal("the fake's attach command does not switch inside a session; the case is unarmed")
		}

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertExeced(t, rec, "api-x7Kd9a")
	})

	t.Run("a terminal the runner can't attach execs the same command instead", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.answer(runResult{err: fmt.Errorf("attach session: %w", runner.ErrNoTerminal)})

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		if len(rec.runs) != 1 || len(rec.execs) != 1 {
			t.Fatalf("runs = %d, execs = %d; want the run refused, then one exec", len(rec.runs), len(rec.execs))
		}
		rec.runs = nil
		assertExeced(t, rec, "api-x7Kd9a")
	})

	t.Run("inside tmux it still attaches through the runner, never switching a tmux client", func(t *testing.T) {
		// The command carries no tmux client, so any tmux call panics.
		t.Setenv("TMUX", "/tmp/portal-open-inside-tmux-test,1,0")
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
	})

	t.Run("an attach command with no argv is refused, attaching nothing", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a")
		withBackendDeps(t, BackendDeps{Backend: envBackend{Fake: fake, emptyArgv: true}})

		if err := openSession(contextCommand(), "api-x7Kd9a"); err == nil {
			t.Error("openSession = nil, want a refusal of the empty command")
		}
		rec.assertNone(t)
	})

	t.Run("an attach the backend refuses attaches nothing", func(t *testing.T) {
		_, rec := stageAttach(t)

		err := openSession(contextCommand(), "gone-a1b2c3")
		if !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("openSession = %v, want ErrNoSuchSession", err)
		}
		rec.assertNone(t)
	})
}

func TestOpenSession_LogsTheExecHandoffFirst(t *testing.T) {
	t.Run("the marker names zmx and the attach args, before the exec", func(t *testing.T) {
		sink := logtest.Install(t)
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.sink = sink
		rec.answer(runResult{err: runner.ErrNoTerminal})

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		r := sink.Records().Matching("process", "exec").Only(t, "process exec record")
		if r.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want INFO", r.Level)
		}
		if got := r.AttrString(t, "target"); got != "zmx" {
			t.Errorf("target = %q, want zmx", got)
		}
		if got := r.AttrString(t, "args"); got != "api-x7Kd9a" {
			t.Errorf("args = %q, want the argv after the program", got)
		}
		if !rec.markerSeen {
			t.Error("the exec marker was not logged when the exec ran; it must precede it")
		}
	})

	t.Run("an attach through the runner logs no exec", func(t *testing.T) {
		sink := logtest.Install(t)
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		rec.onlyRun(t)
		if got := sink.Records().Matching("process", "exec"); len(got) != 0 {
			t.Errorf("exec records = %+v, want none: Portal's process carries on", got)
		}
	})

	t.Run("an empty argv logs empty args", func(t *testing.T) {
		sink := logtest.Install(t)

		logExecHandoff(nil)

		if got := sink.Records().Matching("process", "exec").Only(t, "process exec record").AttrString(t, "args"); got != "" {
			t.Errorf("args = %q, want empty", got)
		}
	})
}

// stageMint stages what a path mint needs of its own: a projects file, a
// state directory and a fixed creation clock, returning the state directory.
func stageMint(t *testing.T, fake *backendtest.Fake, at time.Time) string {
	t.Helper()
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(t.TempDir(), "projects.json"))
	t.Setenv("SHELL", "/bin/zsh")
	fake.Now = func() time.Time { return at }
	return isolateStateDir(t)
}

func TestOpenPath_MintsThroughTheBackend(t *testing.T) {
	at := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)

	t.Run("it creates the session in the directory, records it in the index and attaches it", func(t *testing.T) {
		fake, rec := stageAttach(t)
		stateDir := stageMint(t, fake, at)
		dir := filepath.Join(t.TempDir(), "myproj")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := openPath(contextCommand(), dir, nil); err != nil {
			t.Fatalf("openPath: %v", err)
		}
		creates := fake.CallsTo(backendtest.OpCreate)
		if len(creates) != 1 || creates[0].Dir != dir || creates[0].Command != nil {
			t.Fatalf("Create calls = %+v, want one at %s with no command", creates, dir)
		}
		name := creates[0].Name
		if !strings.HasPrefix(name, "myproj-") {
			t.Errorf("session name = %q, want myproj-<nanoid>", name)
		}
		assertAttached(t, rec, name)

		idx, err := sessionindex.Read(stateDir)
		if err != nil {
			t.Fatalf("read index: %v", err)
		}
		entry, ok := idx.Lookup(name)
		held, _ := fake.Held(name)
		if want := (sessionindex.Entry{Name: name, Dir: dir, CreatedAt: at, PID: held.PID}); !ok || entry.Name != want.Name || entry.Dir != want.Dir || !entry.CreatedAt.Equal(at) || entry.PID == 0 || entry.PID != want.PID {
			t.Errorf("index entry = %+v, %v; want %+v", entry, ok, want)
		}
	})

	t.Run("a command runs in the session's shell", func(t *testing.T) {
		fake, _ := stageAttach(t)
		stageMint(t, fake, at)
		dir := t.TempDir()

		if err := openPath(contextCommand(), dir, []string{"claude", "--resume"}); err != nil {
			t.Fatalf("openPath: %v", err)
		}
		want := session.ShellCommand([]string{"claude", "--resume"}, "/bin/zsh")
		if creates := fake.CallsTo(backendtest.OpCreate); len(creates) != 1 || !slices.Equal(creates[0].Command, want) {
			t.Errorf("Create calls = %+v, want command %q", creates, want)
		}
	})

	t.Run("a failed index write is warned of once, by the store, and the session still opens", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, rec := stageAttach(t)
		stateDir := stageMint(t, fake, at)
		// Everything the write takes before the write itself is in place, so
		// only the temp file sessions.json is written through can fail.
		if _, err := state.EnsureDir(); err != nil {
			t.Fatal(err)
		}
		lock, err := commitlock.Acquire(stateDir)
		if err != nil {
			t.Fatalf("seed commit lock: %v", err)
		}
		_ = lock.Close()
		if err := os.Chmod(stateDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(stateDir, 0o700) })

		if err := openPath(contextCommand(), t.TempDir(), nil); err != nil {
			t.Fatalf("openPath = %v, want nil: the session exists whatever the index holds", err)
		}
		rec.onlyRun(t)
		if n := len(sink.Records().AtOrAboveLevel(slog.LevelWarn)); n != 1 {
			t.Errorf("WARN records = %d, want the store's one", n)
		}
		sink.Records().Matching("sessions", "claim").AtExactLevel(slog.LevelWarn).Only(t, "the store's failed-write warning")
	})

	t.Run("an index that cannot be locked warns and the session still opens", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, rec := stageAttach(t)
		stageMint(t, fake, at)
		blockStateDir(t)
		dir := t.TempDir()

		if err := openPath(contextCommand(), dir, nil); err != nil {
			t.Fatalf("openPath = %v, want nil: the session exists whatever the index holds", err)
		}
		name := rec.onlyRun(t).Session
		r := sink.Records().Matching("sessions", "unrecorded").Only(t, "unrecorded warning")
		if r.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", r.Level)
		}
		if got := r.AttrString(t, "session"); got != name {
			t.Errorf("session = %q, want %q", got, name)
		}
		if got := r.AttrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want cli", got)
		}
	})

	t.Run("a failed create attaches nothing and records nothing", func(t *testing.T) {
		fake, rec := stageAttach(t)
		stateDir := stageMint(t, fake, at)
		fake.Fail(backendtest.OpCreate, errors.New("zmx attach: boom"))

		if err := openPath(contextCommand(), t.TempDir(), nil); err == nil {
			t.Fatal("openPath = nil, want the create failure")
		}
		rec.assertNone(t)
		if idx, _ := sessionindex.Read(stateDir); idx.Len() != 0 {
			t.Errorf("index = %+v, want empty", idx.Entries())
		}
	})
}

func TestIndexingCreator_RecordsThePickersMintsViaTheTUI(t *testing.T) {
	sink := logtest.Install(t)
	fake := backendtest.New()
	stageMint(t, fake, time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC))
	store := &recordingProjectStore{}

	name, err := newIndexingCreator(context.Background(), store, fake, "tui").CreateFromDir(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("CreateFromDir: %v", err)
	}
	r := sink.Records().Matching("sessions", "claim").Only(t, "index claim record")
	if got := r.AttrString(t, "session"); got != name {
		t.Errorf("session = %q, want %q", got, name)
	}
	if got := r.AttrString(t, "via"); got != "tui" {
		t.Errorf("via = %q, want tui", got)
	}
	if store.upserts != 1 {
		t.Errorf("project upserts = %d, want 1", store.upserts)
	}
}

type recordingProjectStore struct{ upserts int }

func (s *recordingProjectStore) Upsert(string, string, string) error {
	s.upserts++
	return nil
}

// stageResolution stages the resolver's alias, zoxide and directory seams as
// empty, leaving its session names to the backend.
func stageResolution(t *testing.T, rec *attachRecorder) {
	t.Helper()
	withOpenDeps(t, OpenDeps{
		AliasLookup:  &testAliasLookup{aliases: map[string]string{}},
		Zoxide:       &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator: &testDirValidator{existing: map[string]bool{}},
		Execer:       rec,
		Runner:       rec.Run,
		Runners:      &rec.runners,
	})
}

func TestOpenCommand_ResolvesSessionNamesFromTheBackend(t *testing.T) {
	for name, args := range map[string][]string{
		"a bare name": {"api-x7Kd9a"},
		"a -s pin":    {"-s", "api-x7Kd9a"},
	} {
		t.Run(name+" attaches the session of that name", func(t *testing.T) {
			_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
			stageResolution(t, rec)

			if _, _, err := runRootCmd(t, append([]string{"open"}, args...)...); err != nil {
				t.Fatalf("open %v: %v", args, err)
			}
			assertAttached(t, rec, "api-x7Kd9a")
		})
	}

	t.Run("a label is not a name", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")
		stageResolution(t, rec)
		seedSessionsJSON(t, `{"version":2,"sessions":[{"name":"api-x7Kd9a","label":"work"}]}`)

		if _, _, err := runRootCmd(t, "open", "work"); err == nil {
			t.Fatal("open work = nil, want a miss: resolution matches names, not labels")
		}
		rec.assertNone(t)
	})

	t.Run("Portal's own sessions are not targets", func(t *testing.T) {
		_, rec := stageAttach(t, "_portal-saver")
		stageResolution(t, rec)

		if _, _, err := runRootCmd(t, "open", "-s", "_portal-saver"); err == nil {
			t.Fatal("open -s _portal-saver = nil, want a miss")
		}
		rec.assertNone(t)
	})
}

// Several targets on one line burst: the invoking terminal takes the first,
// and each of the N−1 others opens in a window of its own, net N windows.
func TestOpenCommand_SeveralSessionsAtOnceBurst(t *testing.T) {
	for name, tc := range map[string]struct {
		args         []string
		wantTrigger  string
		wantMint     string
		wantExternal [][]string
	}{
		"two targets":                  {args: []string{"api-x7Kd9a", "blog-c3d4"}, wantTrigger: "api-x7Kd9a", wantExternal: [][]string{{"--session", "blog-c3d4"}}},
		"two session pins":             {args: []string{"-s", "api-x7Kd9a", "-s", "blog-c3d4"}, wantTrigger: "api-x7Kd9a", wantExternal: [][]string{{"--session", "blog-c3d4"}}},
		"a glob expanding to two":      {args: []string{"*-*"}, wantTrigger: "api-x7Kd9a", wantExternal: [][]string{{"--session", "blog-c3d4"}}},
		"a path to mint and a session": {args: []string{"-p", "/tmp", "api-x7Kd9a"}, wantMint: "/tmp", wantExternal: [][]string{{"--session", "api-x7Kd9a"}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
			withOpenDeps(t, OpenDeps{
				AliasLookup:  &testAliasLookup{aliases: map[string]string{}},
				Zoxide:       &testZoxideQuerier{err: resolver.ErrNoMatch},
				DirValidator: &testDirValidator{existing: map[string]bool{"/tmp": true}},
				Execer:       rec,
				Runner:       rec.Run,
				Runners:      &rec.runners,
			})
			events := &openBurstEvents{}
			ack := &spawntest.FakeAckChannel{}
			adapter := &spawntest.FakeAdapter{Ack: ack}
			conn := &recordingConnector{events: events}
			mint := &recordingMint{events: events}
			deps := openBurstDepsForTest(ghosttyIdentity(), spawn.ResolutionNative, adapter, conn, mint.mint)
			deps.Ack = ack
			withOpenBurstDeps(t, *deps)
			argv := append([]string{"open"}, tc.args...)
			withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal"}, argv...) })

			if _, _, err := runRootCmd(t, argv...); err != nil {
				t.Fatalf("open %v = %v, want the burst", tc.args, err)
			}

			if len(adapter.Calls) != len(tc.wantExternal) {
				t.Fatalf("windows opened = %d, want %d: net N windows", len(adapter.Calls), len(tc.wantExternal))
			}
			for i, target := range tc.wantExternal {
				if got := adapter.Calls[i]; !slices.Contains(got, target[0]) || !slices.Contains(got, target[1]) || !slices.Contains(got, "ZMX_SESSION") {
					t.Errorf("window %d runs %q, want open %s %s with ZMX_SESSION unset", i, got, target[0], target[1])
				}
			}
			wantSeq := []string{"connect:" + tc.wantTrigger}
			if tc.wantMint != "" {
				wantSeq = []string{"mint:" + tc.wantMint}
			}
			if !slices.Equal(events.seq, wantSeq) {
				t.Errorf("this terminal = %q, want %q", events.seq, wantSeq)
			}
			if len(ack.Cleaned) != 1 {
				t.Errorf("batches cleaned = %q, want the burst's one", ack.Cleaned)
			}
			rec.assertNone(t)
		})
	}
}
