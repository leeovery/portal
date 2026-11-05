package zmx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/shellquote"
)

const createdLine = `printf 'session "%s" created\n' "$2"`

// ran reads what fakeZmx recorded of its run, failing when it never ran.
func ran(t *testing.T, log, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(log, file))
	if err != nil {
		t.Fatalf("zmx was not run: %v", err)
	}
	return b
}

func notRun(t *testing.T, log string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(log, "args")); err == nil {
		t.Error("zmx was run")
	}
}

func TestCreate(t *testing.T) {
	t.Run("it runs zmx attach in the directory, stdin at EOF, with the session's environment", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)
		zmxDir, workDir := socketDir(t), t.TempDir()
		b := newBackendRunning(t, binary, zmxDir,
			"PATH=/usr/bin:/bin", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=live", "ZMX_SESSION_PREFIX=pre-",
			"TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1", "LANG=en_GB.UTF-8")

		if _, err := b.Create(t.Context(), "work", workDir, []string{"sh", "-c", "echo hi"}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if got, want := strings.Split(strings.TrimSuffix(string(ran(t, log, "args")), "\x00"), "\x00"), []string{"attach", "work", "sh", "-c", "echo hi"}; !slices.Equal(got, want) {
			t.Errorf("zmx args = %q, want %q", got, want)
		}
		if got, want := strings.TrimSpace(string(ran(t, log, "cwd"))), realDir(t, workDir); got != want {
			t.Errorf("zmx ran in %q, want %q", got, want)
		}
		if got := ran(t, log, "stdin"); len(got) != 0 {
			t.Errorf("zmx read %q on stdin, want EOF", got)
		}
		env := strings.Split(string(ran(t, log, "env")), "\x00")
		for key, want := range map[string][]string{
			"ZMX_DIR":            {zmxDir},
			"ZMX_SESSION":        nil,
			"ZMX_SESSION_PREFIX": nil,
			"TMUX":               nil,
			"TMUX_PANE":          nil,
			"PATH":               {"/usr/bin:/bin"},
			"LANG":               {"en_GB.UTF-8"},
		} {
			if got := envValues(env, key); !slices.Equal(got, want) {
				t.Errorf("zmx env %s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("it succeeds on zmx's created line whatever the exit code", func(t *testing.T) {
		binary, _ := fakeZmx(t, createdLine+"; echo 'cannot connect' >&2; exit 1")

		if _, err := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin").Create(t.Context(), "brief", t.TempDir(), []string{"true"}); err != nil {
			t.Errorf("Create = %v, want nil once zmx said it created the session", err)
		}
	})

	t.Run("it counts the created line only where zmx prints it, first", func(t *testing.T) {
		binary, _ := fakeZmx(t, `printf '\033[0m'; `+createdLine)

		_, err := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin").Create(t.Context(), "x", t.TempDir(), nil)

		if !errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("Create = %v, want the run read as a join", err)
		}
	})

	t.Run("it reports a run that failed and left nothing live as the failure", func(t *testing.T) {
		binary, _ := fakeZmx(t, "echo 'error: boom' >&2; exit 1")

		_, err := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin").Create(t.Context(), "x", t.TempDir(), nil)

		if err == nil || errors.Is(err, backend.ErrSessionExists) || !strings.Contains(err.Error(), "error: boom") {
			t.Errorf("Create = %v, want zmx's failure with its stderr", err)
		}
	})

	t.Run("it refuses a live name without running zmx, which would join it", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)
		dir := socketDir(t)
		d := serve(t, dir, "taken", silent)

		_, err := newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin").Create(t.Context(), "taken", t.TempDir(), nil)

		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "taken" {
			t.Errorf("Create(taken) = %v, want a SessionExistsError held as taken", err)
		}
		notRun(t, log)
		if reqs := d.received(); len(reqs) != 0 {
			t.Errorf("Create(taken) sent the live session %+v", reqs)
		}
	})

	t.Run("it refuses a name live under another case, naming the session that holds it", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)
		dir := socketDir(t)
		foldsCase(t, dir)
		serve(t, dir, "taken", silent)

		_, err := newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin").Create(t.Context(), "TAKEN", t.TempDir(), nil)

		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "taken" || exists.Name != "TAKEN" {
			t.Errorf("Create(TAKEN) = %v, want a SessionExistsError held as taken", err)
		}
		notRun(t, log)
	})

	t.Run("it leaves a refusing socket from before this boot for zmx to replace", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)
		dir := socketDir(t)
		stale(t, dir, "crashed")

		if _, err := newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin").Create(t.Context(), "crashed", t.TempDir(), nil); err != nil {
			t.Errorf("Create over a stale socket = %v, want nil", err)
		}
		ran(t, log, "args")
	})

	t.Run("it refuses a refusing socket from this boot that a process may hold without running zmx, which would replace it", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)
		dir := socketDir(t)
		refusing(t, dir, "busy")

		_, err := held(newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin"), dir, "busy").Create(t.Context(), "busy", t.TempDir(), nil)

		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "busy" {
			t.Errorf("Create(busy) = %v, want a SessionExistsError held as busy", err)
		}
		notRun(t, log)
	})

	t.Run("it leaves a refusing socket from this boot that no process holds for zmx to replace", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)
		dir := socketDir(t)
		refusing(t, dir, "crashed")

		if _, err := newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin").Create(t.Context(), "crashed", t.TempDir(), nil); err != nil {
			t.Errorf("Create over a socket no process holds = %v, want nil", err)
		}
		ran(t, log, "args")
	})

	t.Run("it gives up on a run past its ceiling", func(t *testing.T) {
		binary, _ := fakeZmx(t, "exec sleep 5")
		b := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin")
		zmx.ShortenCreateCeiling(b, time.Second)

		start := time.Now()
		_, err := b.Create(t.Context(), "slow", t.TempDir(), nil)

		if !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Create of a hung run = %v, want ErrTimeout", err)
		}
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("Create of a run hung past its 1s ceiling took %s", took)
		}
	})

	t.Run("it refuses an invalid name without running zmx", func(t *testing.T) {
		binary, log := fakeZmx(t, createdLine)

		if _, err := newBackendRunning(t, binary, socketDir(t)).Create(t.Context(), "-h", t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Create(-h) = %v, want ErrInvalidName", err)
		}
		notRun(t, log)
	})
}

type createResult struct {
	session backend.Session
	err     error
	// afterUp is how long Create took to return once the daemon was up.
	afterUp time.Duration
	// log is what the fake zmx recorded of its last run.
	log string
}

// createBringingUp runs Create of name with a fake zmx that prints its
// created line and then waits, as zmx's attach does while its daemon starts,
// until up has run here and brought the session's daemon up. The daemon is a
// process holding the name throughout.
func createBringingUp(t *testing.T, dir, name string, up func()) createResult {
	t.Helper()
	return createBringingUpWithin(t, dir, name, ceiling, up)
}

// createBringingUpWithin is createBringingUp under the probe ceiling probe.
func createBringingUpWithin(t *testing.T, dir, name string, probe time.Duration, up func()) createResult {
	t.Helper()
	return createBringingUpKilling(t, dir, name, probe, "echo 'zmx kill was not expected' >&2; exit 1", up)
}

// createBringingUpKilling is createBringingUpWithin with a fake zmx whose
// kill runs kill.
func createBringingUpKilling(t *testing.T, dir, name string, probe time.Duration, kill string, up func()) createResult {
	t.Helper()
	return creation{probe: probe, kill: kill}.run(t, dir, name, up)
}

// creation is a Create run with a fake zmx that prints its created line and
// then waits, as zmx's attach does while its daemon starts, until up has run
// here and brought the session's daemon up, and whose kill runs kill.
type creation struct {
	// probe is every socket operation's ceiling.
	probe time.Duration
	// create is Create's own ceiling, zero for its default.
	create time.Duration
	kill   string
	// table is the process table the Backend reads; nil holds a zmx daemon
	// holding the name in dir, and nothing else.
	table func() ([]zmx.Process, error)
	// cancelAtUp cancels Create's context as up runs, after zmx has printed
	// its created line.
	cancelAtUp bool
}

func (c creation) run(t *testing.T, dir, name string, up func()) createResult {
	t.Helper()
	scratch := t.TempDir()
	ready, printed := filepath.Join(scratch, "up"), filepath.Join(scratch, "printed")
	binary, log := fakeZmx(t, `if [ "$1" = kill ]; then `+c.kill+"; exit; fi; "+
		createdLine+"; : > "+shellquote.Single(printed)+"; while [ ! -e "+shellquote.Single(ready)+" ]; do sleep 0.01; done")
	b := mustNew(t, zmx.Config{
		Binary:   binary,
		Dir:      dir,
		Environ:  func() []string { return []string{"PATH=/usr/bin:/bin"} },
		Hostname: func() (string, error) { return hostname, nil },
	})
	zmx.ShortenCeilings(b, c.probe)
	if c.create != 0 {
		zmx.ShortenCreateCeiling(b, c.create)
	}
	if c.table == nil {
		held(b, dir, name)
	} else {
		zmx.FakeProcessesFrom(b, c.table)
	}

	type returned struct {
		result createResult
		at     time.Time
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan returned, 1)
	go func() {
		s, err := b.Create(ctx, name, t.TempDir(), nil)
		done <- returned{createResult{session: s, err: err}, time.Now()}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(printed); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the fake zmx never printed its created line")
		}
		time.Sleep(10 * time.Millisecond)
	}
	up()
	if c.cancelAtUp {
		cancel()
	}
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	upAt := time.Now()
	r := <-done
	r.result.afterUp = r.at.Sub(upAt)
	r.result.log = log
	return r.result
}

// lastRun is the arguments of the fake zmx's last run.
func lastRun(t *testing.T, log string) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(string(ran(t, log, "args")), "\x00"), "\x00")
}

func TestCreateReturnsTheSessionItMade(t *testing.T) {
	t.Run("it reads the session back from its new daemon, as List reports it", func(t *testing.T) {
		dir := socketDir(t)
		got := createBringingUp(t, dir, "work", func() { serve(t, dir, "work", answering(infoFrame(t))) })
		if got.err != nil {
			t.Fatalf("Create = %v", got.err)
		}
		sessions, err := newBackend(t, dir).List(t.Context())
		if err != nil || len(sessions) != 1 {
			t.Fatalf("List = %+v, %v; want the one session", sessions, err)
		}
		if !got.session.Answered || got.session.Created.IsZero() || got.session != sessions[0] {
			t.Errorf("Create returned %+v, want %+v as List reports it", got.session, sessions[0])
		}
	})

	t.Run("it asks again while the new daemon is not yet accepting", func(t *testing.T) {
		dir := socketDir(t)
		// A probe ceiling of several seconds, which the asking ends well
		// inside, so the daemon's coming up late under load can't outlast it.
		got := createBringingUpWithin(t, dir, "work", 50*ceiling, func() {
			refusing(t, dir, "work")
			go func() {
				time.Sleep(50 * time.Millisecond)
				// The listening socket replaces the refusing one in one step, so
				// no ask finds the name missing and takes the session as ended.
				_ = os.Rename(filepath.Join(dir, "work-up"), filepath.Join(dir, "work"))
			}()
			serve(t, dir, "work-up", answering(infoFrame(t)))
		})
		if got.err != nil || !got.session.Answered {
			t.Errorf("Create = %+v, %v; want the session read once its daemon accepted", got.session, got.err)
		}
	})

	t.Run("a session already ended is returned by name, made all the same", func(t *testing.T) {
		binary, _ := fakeZmx(t, createdLine)

		s, err := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin").Create(t.Context(), "brief", t.TempDir(), []string{"true"})
		if err != nil || s != (backend.Session{Name: "brief"}) {
			t.Errorf("Create = %+v, %v; want only the name, and no error", s, err)
		}
	})

	t.Run("a new daemon that does not answer within a probe ceiling is returned by name", func(t *testing.T) {
		dir := socketDir(t)
		got := createBringingUp(t, dir, "work", func() { serve(t, dir, "work", silent) })
		if got.err != nil || got.session != (backend.Session{Name: "work"}) {
			t.Errorf("Create = %+v, %v; want only the name, and no error", got.session, got.err)
		}
		// Far from the two ceilings it takes, the read-back's and the ask in
		// zmx 0.8.1's framing, and far short of the create ceiling it would
		// wait out unbounded.
		if bound := zmx.CreateCeiling / 2; got.afterUp > bound {
			t.Errorf("Create took %s reading back a silent daemon, want about two %s probe ceilings, well within %s", got.afterUp, ceiling, bound)
		}
		if args := lastRun(t, got.log); args[0] != "attach" {
			t.Errorf("zmx last ran %q, want only the attach: a daemon answering neither framing is not ended", args)
		}
	})
}

// A session the create made that answers only zmx 0.8.1's framing was made by
// a zmx Portal can't talk to, as every session that zmx makes would be: it is
// ended by that zmx's own kill and refused, so no caller keeps it.
func TestCreateRefusesASessionOfAnOlderZmx(t *testing.T) {
	t.Run("it ends the session with zmx kill and refuses it", func(t *testing.T) {
		dir := socketDir(t)
		kill := "rm -f " + shellquote.Single(filepath.Join(dir, "work"))

		got := createBringingUpKilling(t, dir, "work", ceiling, kill, func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) || got.session != (backend.Session{}) {
			t.Fatalf("Create = %+v, %v; want no session and ErrUnsupportedProtocol", got.session, got.err)
		}
		if args := lastRun(t, got.log); !slices.Equal(args, []string{"kill", "work"}) {
			t.Errorf("zmx last ran %q, want kill work", args)
		}
		if !strings.Contains(got.err.Error(), "zmx kill ended the session") {
			t.Errorf("Create = %v, want it to say the session was ended", got.err)
		}
	})

	// decision 80: zmx kill is the refusal's only end. A signal's proof of
	// which daemon this Create made can be fooled into reaching a session
	// Portal didn't make, so one the kill can't end is left, and no process
	// is even looked for.
	t.Run("one zmx kill can't end is left, nothing signalled, the refusal saying so", func(t *testing.T) {
		dir := socketDir(t)
		killed := filepath.Join(t.TempDir(), "killed")
		var looked atomic.Int32
		c := creation{
			probe: ceiling,
			kill:  ": > " + shellquote.Single(killed) + "; echo 'failed to kill session=work: SessionUnresponsive' >&2; exit 1",
			table: func() ([]zmx.Process, error) {
				if _, err := os.Stat(killed); err == nil {
					looked.Add(1)
				}
				return []zmx.Process{daemonOf(100, dir, "work")}, nil
			},
		}

		got := c.run(t, dir, "work", func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) || got.session != (backend.Session{}) {
			t.Fatalf("Create = %+v, %v; want no session and ErrUnsupportedProtocol", got.session, got.err)
		}
		if msg := got.err.Error(); !strings.Contains(msg, "left running") || !strings.Contains(msg, "SessionUnresponsive") {
			t.Errorf("Create = %v, want it to say the session is left, with zmx's output", got.err)
		}
		if n := looked.Load(); n != 0 {
			t.Errorf("the process table was read %d times after the kill failed, want none: nothing is proved or signalled", n)
		}
		if _, err := os.Stat(filepath.Join(dir, "work")); err != nil {
			t.Errorf("stat the session's socket = %v, want it left", err)
		}
	})

	// The caller giving up after zmx made the session must not leave a session
	// of 0.8.1's made: Ctrl+C on the loading page, or the bootstrap's end.
	t.Run("one made as the caller's context ends is judged and refused all the same", func(t *testing.T) {
		dir := socketDir(t)
		c := creation{probe: ceiling, kill: "rm -f " + shellquote.Single(filepath.Join(dir, "work")), cancelAtUp: true}

		got := c.run(t, dir, "work", func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) || !strings.Contains(got.err.Error(), "zmx kill ended the session") {
			t.Errorf("Create = %+v, %v; want ErrUnsupportedProtocol, the session ended by zmx kill", got.session, got.err)
		}
	})

	// The read-back spends the create's ceiling on a daemon that never
	// answers Portal's framing, so the judgement and its kill start with none
	// of it left; a kill outlasting all of it still lands.
	t.Run("its judgement and kill are not cut short by the create's ceiling", func(t *testing.T) {
		dir := socketDir(t)
		const create = 2 * time.Second
		kill := "sleep 2.2; rm -f " + shellquote.Single(filepath.Join(dir, "work"))
		c := creation{probe: 3 * time.Second, create: create, kill: kill}

		got := c.run(t, dir, "work", func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) || !strings.Contains(got.err.Error(), "zmx kill ended the session") {
			t.Errorf("Create = %+v, %v; want ErrUnsupportedProtocol, the session ended by a zmx kill longer than the create's %s ceiling", got.session, got.err, create)
		}
	})
}

func realDir(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// A zmx the Backend runs that may make a session, Create's and the attach
// command, leaves behind the terminal identity tmux gives its panes when it is
// begun inside tmux, so zmx sets a TERM of its own; begun outside tmux it
// keeps the terminal's.
func TestZmxRuns_LeaveTmuxsTerminalIdentityBehind(t *testing.T) {
	envs := map[string]func(t *testing.T, environ []string) []string{
		"Create": func(t *testing.T, environ []string) []string {
			binary, log := fakeZmx(t, createdLine)
			if _, err := newBackendRunning(t, binary, socketDir(t), environ...).Create(t.Context(), "work", t.TempDir(), []string{"true"}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			return strings.Split(string(ran(t, log, "env")), "\x00")
		},
		"AttachCommand": func(t *testing.T, environ []string) []string {
			dir := socketDir(t)
			serve(t, dir, "work", answering(infoFrame(t)))
			cmd, err := newBackend(t, dir, environ...).AttachCommand(t.Context(), "work")
			if err != nil {
				t.Fatalf("AttachCommand: %v", err)
			}
			return cmd.Env
		},
	}
	tmux := []string{"TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1"}
	identity := []string{"TERM_PROGRAM=tmux", "TERM_PROGRAM_VERSION=3.8"}
	dropped := map[string][]string{"TERM": nil, "TERM_PROGRAM": nil, "TERM_PROGRAM_VERSION": nil}
	for _, tt := range []struct {
		name    string
		environ []string
		want    map[string][]string
	}{
		{"inside tmux, its TERM naming tmux", slices.Concat(tmux, identity, []string{"TERM=tmux-256color"}), dropped},
		{"inside tmux, its TERM naming screen", slices.Concat(tmux, identity, []string{"TERM=screen-256color"}), dropped},
		{"inside tmux, its TERM tmux bare", slices.Concat(tmux, []string{"TERM=tmux"}), dropped},
		{"inside tmux, its TERM screen bare", slices.Concat(tmux, []string{"TERM=screen"}), dropped},
		{"inside tmux with an empty TMUX", slices.Concat([]string{"TMUX="}, identity, []string{"TERM=tmux-256color"}), dropped},
		{"inside tmux, its TERM given twice", slices.Concat(tmux, []string{"TERM=tmux-256color", "TERM=xterm-256color"}),
			map[string][]string{"TERM": {"xterm-256color"}}},
		{"inside tmux, its TERM naming another terminal", slices.Concat(tmux, []string{"TERM=xterm-256color"}),
			map[string][]string{"TERM": {"xterm-256color"}}},
		{"inside tmux, its TERM only beginning as tmux's does", slices.Concat(tmux, []string{"TERM=tmuxterm"}),
			map[string][]string{"TERM": {"tmuxterm"}}},
		{"outside tmux", slices.Concat(identity, []string{"TERM=tmux-256color"}),
			map[string][]string{"TERM": {"tmux-256color"}, "TERM_PROGRAM": {"tmux"}, "TERM_PROGRAM_VERSION": {"3.8"}}},
	} {
		for run, env := range envs {
			t.Run(tt.name+": "+run, func(t *testing.T) {
				got := env(t, append(slices.Clone(tt.environ), "PATH=/usr/bin:/bin"))

				for key, want := range tt.want {
					if values := envValues(got, key); !slices.Equal(values, want) {
						t.Errorf("env %s = %q, want %q", key, values, want)
					}
				}
				if values := slices.Concat(envValues(got, "TMUX"), envValues(got, "TMUX_PANE")); len(values) != 0 {
					t.Errorf("env carries the caller's tmux: %q", values)
				}
			})
		}
	}
}
