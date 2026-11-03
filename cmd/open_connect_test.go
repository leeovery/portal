package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/spf13/cobra"
)

// recordingExec captures each exec instead of replacing the test process, and
// whether the exec marker was already logged when it ran.
type recordingExec struct {
	sink       *logtest.Sink
	calls      []execCall
	markerSeen bool
}

type execCall struct {
	argv0     string
	argv, env []string
}

func (r *recordingExec) Exec(argv0 string, argv, env []string) error {
	r.calls = append(r.calls, execCall{argv0: argv0, argv: argv, env: env})
	if r.sink != nil && len(r.sink.Records().Matching("process", "exec")) > 0 {
		r.markerSeen = true
	}
	return nil
}

// only is the one exec the test expects.
func (r *recordingExec) only(t *testing.T) execCall {
	t.Helper()
	if len(r.calls) != 1 {
		t.Fatalf("execs = %+v, want exactly one", r.calls)
	}
	return r.calls[0]
}

// envBackend is a Fake whose attach command carries an environment, as the
// zmx driver's does.
type envBackend struct {
	*backendtest.Fake
	env []string
}

func (b envBackend) AttachCommand(ctx context.Context, name string) (backend.Command, error) {
	c, err := b.Fake.AttachCommand(ctx, name)
	if err == nil {
		c.Env = b.env
	}
	return c, err
}

var attachEnv = []string{"ZMX_DIR=/tmp/zx-open-test", "TERM=xterm-256color"}

// stageAttach stages a Fake holding sessions behind an environment-carrying
// attach, and an exec that records instead of replacing the process.
func stageAttach(t *testing.T, sessions ...string) (*backendtest.Fake, *recordingExec) {
	t.Helper()
	fake := backendtest.New()
	for _, name := range sessions {
		fake.Add(backend.Session{Name: name})
	}
	withBackendDeps(t, BackendDeps{Backend: envBackend{Fake: fake, env: attachEnv}})
	rec := &recordingExec{}
	withOpenDeps(t, OpenDeps{Execer: rec})
	return fake, rec
}

func contextCommand() *cobra.Command {
	c := &cobra.Command{}
	c.SetContext(context.Background())
	return c
}

func assertAttached(t *testing.T, rec *recordingExec, name string) {
	t.Helper()
	call := rec.only(t)
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

func TestOpenSession_ExecsTheBackendsAttachCommand(t *testing.T) {
	t.Run("it execs the attach argv with the attach command's environment", func(t *testing.T) {
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
	})

	t.Run("inside a session it execs the same command, which switches and exits", func(t *testing.T) {
		fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
		fake.SetCurrent("blog-c3d4")
		if command, _ := fake.AttachCommand(context.Background(), "api-x7Kd9a"); !command.Switches {
			t.Fatal("the fake's attach command does not switch inside a session; the case is unarmed")
		}

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
	})

	t.Run("inside tmux it still execs the attach, never switching a tmux client", func(t *testing.T) {
		// The command carries no tmux client, so any tmux call panics.
		t.Setenv("TMUX", "/tmp/portal-open-inside-tmux-test,1,0")
		_, rec := stageAttach(t, "api-x7Kd9a")

		if err := openSession(contextCommand(), "api-x7Kd9a"); err != nil {
			t.Fatalf("openSession: %v", err)
		}
		assertAttached(t, rec, "api-x7Kd9a")
	})

	t.Run("an attach the backend refuses execs nothing", func(t *testing.T) {
		_, rec := stageAttach(t)

		err := openSession(contextCommand(), "gone-a1b2c3")
		if !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("openSession = %v, want ErrNoSuchSession", err)
		}
		if len(rec.calls) != 0 {
			t.Errorf("execs = %+v, want none", rec.calls)
		}
	})
}

func TestOpenSession_LogsTheExecHandoffFirst(t *testing.T) {
	t.Run("the marker names zmx and the attach args, before the exec", func(t *testing.T) {
		sink := logtest.Install(t)
		_, rec := stageAttach(t, "api-x7Kd9a")
		rec.sink = sink

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
		if want := (sessionindex.Entry{Name: name, Dir: dir, CreatedAt: at}); !ok || entry.Name != want.Name || entry.Dir != want.Dir || !entry.CreatedAt.Equal(at) {
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

	t.Run("an index that cannot be written warns and the session still opens", func(t *testing.T) {
		sink := logtest.Install(t)
		fake, rec := stageAttach(t)
		stageMint(t, fake, at)
		blockStateDir(t)
		dir := t.TempDir()

		if err := openPath(contextCommand(), dir, nil); err != nil {
			t.Fatalf("openPath = %v, want nil: the session exists whatever the index holds", err)
		}
		name := rec.only(t).argv[1]
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
		if len(rec.calls) != 0 {
			t.Errorf("execs = %+v, want none", rec.calls)
		}
		if idx, _ := sessionindex.Read(stateDir); len(idx.Sessions) != 0 {
			t.Errorf("index = %+v, want empty", idx.Sessions)
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
func stageResolution(t *testing.T, rec *recordingExec) {
	t.Helper()
	withOpenDeps(t, OpenDeps{
		AliasLookup:  &testAliasLookup{aliases: map[string]string{}},
		Zoxide:       &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator: &testDirValidator{existing: map[string]bool{}},
		Execer:       rec,
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
		if len(rec.calls) != 0 {
			t.Errorf("execs = %+v, want none", rec.calls)
		}
	})

	t.Run("Portal's own sessions are not targets", func(t *testing.T) {
		_, rec := stageAttach(t, "_portal-saver")
		stageResolution(t, rec)

		if _, _, err := runRootCmd(t, "open", "-s", "_portal-saver"); err == nil {
			t.Fatal("open -s _portal-saver = nil, want a miss")
		}
		if len(rec.calls) != 0 {
			t.Errorf("execs = %+v, want none", rec.calls)
		}
	})
}

func TestOpenCommand_SeveralSessionsAtOnceRefuse(t *testing.T) {
	for name, args := range map[string][]string{
		"two targets":                  {"api-x7Kd9a", "blog-c3d4"},
		"two session pins":             {"-s", "api-x7Kd9a", "-s", "blog-c3d4"},
		"a glob expanding to two":      {"*-*"},
		"a session and a path to mint": {"api-x7Kd9a", "-p", "/tmp"},
	} {
		t.Run(name, func(t *testing.T) {
			fake, rec := stageAttach(t, "api-x7Kd9a", "blog-c3d4")
			withOpenDeps(t, OpenDeps{
				AliasLookup:  &testAliasLookup{aliases: map[string]string{}},
				Zoxide:       &testZoxideQuerier{err: resolver.ErrNoMatch},
				DirValidator: &testDirValidator{existing: map[string]bool{"/tmp": true}},
				Execer:       rec,
			})
			argv := append([]string{"open"}, args...)
			withFuncSeam(t, &openRawArgs, func() []string { return append([]string{"portal"}, argv...) })

			_, _, err := runRootCmd(t, argv...)
			if err == nil || err.Error() != "opening several sessions at once isn't available yet" {
				t.Fatalf("open %v = %v, want the one-line refusal", args, err)
			}
			if len(rec.calls) != 0 || len(fake.CallsTo(backendtest.OpCreate)) != 0 {
				t.Errorf("execs %+v, creates %+v; want neither", rec.calls, fake.CallsTo(backendtest.OpCreate))
			}
		})
	}
}
