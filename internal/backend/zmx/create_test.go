package zmx_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
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
	// signal is the Backend's Signal; nil binds none.
	signal func(pid int, sig syscall.Signal) error
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
		Signal:   c.signal,
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

	t.Run("one zmx kill can't end is refused all the same, saying it is left", func(t *testing.T) {
		dir := socketDir(t)
		kill := "echo 'failed to kill session=work: SessionUnresponsive' >&2; exit 1"

		got := createBringingUpKilling(t, dir, "work", ceiling, kill, func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) {
			t.Fatalf("Create = %+v, %v; want ErrUnsupportedProtocol", got.session, got.err)
		}
		if msg := got.err.Error(); !strings.Contains(msg, "left running") || !strings.Contains(msg, "SessionUnresponsive") {
			t.Errorf("Create = %v, want it to say the session is left, with zmx's output", got.err)
		}
	})

	t.Run("one zmx kill can't end is ended by a SIGTERM to its proved daemon, that one alone", func(t *testing.T) {
		dir := socketDir(t)
		var (
			mu       sync.Mutex
			procs    = []zmx.Process{daemonOf(100, dir, "work"), daemonOf(101, dir, "other")}
			signals  []string
			signalOf = func(pid int, sig syscall.Signal) error {
				mu.Lock()
				defer mu.Unlock()
				signals = append(signals, fmt.Sprintf("%d %s", pid, sig))
				if pid == 100 {
					_ = os.Remove(filepath.Join(dir, "work"))
					procs = procs[1:]
				}
				return nil
			}
		)
		c := creation{
			probe:  ceiling,
			kill:   "echo 'failed to kill session=work: SessionUnresponsive' >&2; exit 1",
			signal: signalOf,
			table: func() ([]zmx.Process, error) {
				mu.Lock()
				defer mu.Unlock()
				return slices.Clone(procs), nil
			},
		}

		got := c.run(t, dir, "work", func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) || got.session != (backend.Session{}) {
			t.Fatalf("Create = %+v, %v; want no session and ErrUnsupportedProtocol", got.session, got.err)
		}
		mu.Lock()
		defer mu.Unlock()
		if want := []string{"100 " + syscall.SIGTERM.String()}; !slices.Equal(signals, want) {
			t.Errorf("signals = %q, want %q: work's daemon alone", signals, want)
		}
		if msg := got.err.Error(); !strings.Contains(msg, "SessionUnresponsive") || !strings.Contains(msg, "a SIGTERM to its daemon ended the session") {
			t.Errorf("Create = %v, want the kill's failure, then the session ended by the signal", got.err)
		}
	})

	t.Run("one neither zmx kill nor the signal ends is refused all the same, carrying both failures", func(t *testing.T) {
		dir := socketDir(t)
		refused := errors.New("signal refused")
		c := creation{
			probe:  ceiling,
			kill:   "echo 'failed to kill session=work: SessionUnresponsive' >&2; exit 1",
			signal: func(int, syscall.Signal) error { return refused },
		}

		got := c.run(t, dir, "work", func() { serve(t, dir, "work", olderDaemon(552)) })

		if !errors.Is(got.err, backend.ErrUnsupportedProtocol) || !errors.Is(got.err, refused) {
			t.Fatalf("Create = %v, want ErrUnsupportedProtocol carrying the signal's failure", got.err)
		}
		if msg := got.err.Error(); !strings.Contains(msg, "left running") || !strings.Contains(msg, "SessionUnresponsive") {
			t.Errorf("Create = %v, want it to say the session is left, with zmx kill's output", got.err)
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
