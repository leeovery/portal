package saverhost_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

const (
	exe      = "/opt/homebrew/Cellar/portal/0.13.0/bin/portal"
	release  = "0.13.0"
	older    = "0.12.6"
	dev      = "dev-754e31b498e0-modified-20261103T231632.195584591Z"
	otherDev = "dev-754e31b498e0-modified-20261103T224015.802113657Z"
)

// record records pid and version in dir as a daemon holding the lock does.
func record(dir string, pid int, version string) error {
	if err := state.WritePIDFile(dir, pid); err != nil {
		return err
	}
	return state.WriteVersionFile(dir, version, nil)
}

// writeDaemon is record, failing the test on an error.
func writeDaemon(t *testing.T, dir string, pid int, version string) {
	t.Helper()
	if err := record(dir, pid, version); err != nil {
		t.Fatal(err)
	}
}

// startingDaemon is the Fake whose saver session's process comes up as
// `portal state daemon` does, holding the lock in dir and recording version.
type startingDaemon struct {
	*backendtest.Fake
	dir     string
	version string
}

func (b *startingDaemon) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err != nil {
		return s, err
	}
	return s, record(b.dir, s.PID, b.version)
}

// versionOnly is the Fake whose saver session's daemon records version, and
// leaves daemon.pid as it found it.
type versionOnly struct {
	*backendtest.Fake
	dir     string
	version string
}

func (b *versionOnly) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err != nil {
		return s, err
	}
	return s, state.WriteVersionFile(b.dir, b.version, nil)
}

// slowToRecord is the Fake whose saver session's daemon holds the lock as it
// is made and records version only after a while.
type slowToRecord struct {
	*backendtest.Fake
	t       *testing.T
	dir     string
	version string
}

func (b *slowToRecord) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err != nil {
		return s, err
	}
	if err := state.WritePIDFile(b.dir, s.PID); err != nil {
		return s, err
	}
	recorded := make(chan struct{})
	b.t.Cleanup(func() { <-recorded })
	time.AfterFunc(readyWait/3, func() {
		defer close(recorded)
		if err := state.WriteVersionFile(b.dir, b.version, nil); err != nil {
			b.t.Error(err)
		}
	})
	return s, nil
}

// beatenToIt is the Fake where another process creates the saver, its daemon
// coming up as version, just before this one's create.
type beatenToIt struct {
	*backendtest.Fake
	dir     string
	version string
}

func (b *beatenToIt) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	theirs, err := b.Fake.Create(ctx, name, dir, []string{"/usr/local/bin/portal", "state", "daemon"})
	if err != nil {
		return theirs, err
	}
	if err := record(b.dir, theirs.PID, b.version); err != nil {
		return theirs, err
	}
	return b.Fake.Create(ctx, name, dir, command)
}

// refusingDaemon is the Fake whose saver session ends as it is made, its
// daemon refusing to run.
type refusingDaemon struct{ *backendtest.Fake }

func (b *refusingDaemon) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err == nil {
		b.Remove(name)
	}
	return s, err
}

// heldUp is the Fake whose saver session, once made, holds its creator until
// open closes, its daemon only then coming up at version: an ensure caught
// between its create and its daemon's readiness, as another run reads the
// saver.
type heldUp struct {
	*backendtest.Fake
	dir     string
	version string
	created chan struct{}
	open    chan struct{}
}

func newHeldUp(fake *backendtest.Fake, dir, version string) *heldUp {
	return &heldUp{Fake: fake, dir: dir, version: version, created: make(chan struct{}), open: make(chan struct{})}
}

func (b *heldUp) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err != nil {
		return s, err
	}
	close(b.created)
	<-b.open
	return s, record(b.dir, s.PID, b.version)
}

// replacedBeforeItsStop is the Fake where the saver is replaced, its new
// daemon up at this version, as the recycle's stop reads the session.
type replacedBeforeItsStop struct {
	*backendtest.Fake
	t     *testing.T
	dir   string
	reads int
}

func (b *replacedBeforeItsStop) Session(ctx context.Context, name string) (backend.Session, error) {
	// The first read is EnsureSaver's own; the next is its stop's.
	if b.reads++; b.reads == 2 {
		b.Add(backend.Session{Name: saver.SessionName, PID: daemonPID + 1})
		writeDaemon(b.t, b.dir, daemonPID+1, release)
	}
	return b.Fake.Session(ctx, name)
}

// ensurerOf is h's Ensurer of exe at version, failing the test on a refusal.
func ensurerOf(t *testing.T, h *saverhost.Host, version string) *saverhost.Ensurer {
	t.Helper()
	e, err := h.Ensurer(saverhost.Daemon{Exe: exe, Version: version})
	if err != nil {
		t.Fatalf("Ensurer: %v", err)
	}
	return e
}

// newEnsurer is the Ensurer of exe at version over b and the state directory
// dir, reading and signalling p alone.
func newEnsurer(t *testing.T, b backend.Backend, p *procs, dir, version string) *saverhost.Ensurer {
	t.Helper()
	return ensurerOf(t, hostOf(t, config(b, p, dir)), version)
}

// ops is the calls b recorded, by op and name.
func ops(b *backendtest.Fake) []string {
	var got []string
	for _, c := range b.Calls() {
		if c.Op == backendtest.OpCreate || c.Op == backendtest.OpKill {
			got = append(got, string(c.Op)+" "+c.Name)
		}
	}
	return got
}

func TestEnsureSaver(t *testing.T) {
	t.Run("from cold it creates the saver running the absolute exe as state daemon, and returns once its daemon holds the lock", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		fake := backendtest.New()
		p := newProcs(nil)

		if err := newEnsurer(t, &startingDaemon{fake, dir, release}, p, dir, release).EnsureSaver(t.Context()); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		creates := fake.CallsTo(backendtest.OpCreate)
		if len(creates) != 1 {
			t.Fatalf("creates = %+v, want one", creates)
		}
		c := creates[0]
		if want := []string{exe, "state", "daemon"}; c.Name != saver.SessionName || c.Dir != dir || !slices.Equal(c.Command, want) {
			t.Errorf("create = %+v, want %s running %q in %s", c, saver.SessionName, want, dir)
		}
		if got := p.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		ready := sink.Records().Matching("saver", "daemon ready").Only(t, "the daemon's readiness")
		created, _ := fake.Held(saver.SessionName)
		if pid := ready.IntAttr(t, "target_pid"); pid != int64(created.PID) {
			t.Errorf("daemon ready target_pid = %d, want the session's %d", pid, created.PID)
		}
	})

	t.Run("it creates the state directory it holds saver.lock in, for the daemon alone", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")

		if err := newEnsurer(t, &startingDaemon{backendtest.New(), dir, release}, newProcs(nil), dir, release).EnsureSaver(t.Context()); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("stat %s = %v, %v; want a directory, mode 0700", dir, info, err)
		}
	})

	t.Run("the argv it runs is one the identity check and the pgrep pattern both name", func(t *testing.T) {
		pattern := regexp.MustCompile(state.PortalDaemonArgvPattern)
		for _, path := range []string{exe, "/opt/homebrew/bin/portal", "/Users/someone/go/bin/portal", "/var/folders/x/T/go-build1/b001/exe/portal"} {
			dir := t.TempDir()
			fake := backendtest.New()
			e, err := hostOf(t, config(&startingDaemon{fake, dir, release}, newProcs(nil), dir)).
				Ensurer(saverhost.Daemon{Exe: path, Version: release})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.EnsureSaver(t.Context()); err != nil {
				t.Fatalf("EnsureSaver: %v", err)
			}

			argv := fake.CallsTo(backendtest.OpCreate)[0].Command
			if !pattern.MatchString(strings.Join(argv, " ")) || !state.IsDaemonArgv(argv) {
				t.Errorf("PortalDaemonArgvPattern does not match the saver's argv %q", argv)
			}
		}
	})

	for name, version := range map[string]string{
		"a release's": release,
		"a dev build's, since a dev version names its build": dev,
	} {
		t.Run("a live saver whose daemon holds the lock at this version, "+name+", is kept", func(t *testing.T) {
			sink := logtest.Install(t)
			dir := t.TempDir()
			writeDaemon(t, dir, daemonPID, version)
			fake := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

			if err := newEnsurer(t, fake, p, dir, version).EnsureSaver(t.Context()); err != nil {
				t.Fatalf("EnsureSaver: %v", err)
			}

			if got := ops(fake); len(got) != 0 {
				t.Errorf("calls = %q, want no create or kill", got)
			}
			if got := p.signals(); len(got) != 0 {
				t.Errorf("signals = %q, want none", got)
			}
			kept := sink.Records().Matching("saver", "daemon kept").Only(t, "the kept saver")
			if pid := kept.IntAttr(t, "target_pid"); pid != daemonPID {
				t.Errorf("daemon kept target_pid = %d, want %d", pid, daemonPID)
			}
		})
	}

	for name, tc := range map[string]struct {
		running string
		seed    func(t *testing.T, dir string)
		reason  string
	}{
		"its daemon records another version":   {release, func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID, older) }, "version"},
		"its daemon records another dev build": {dev, func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID, otherDev) }, "version"},
		"this release replaces a dev build's":  {release, func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID, dev) }, "version"},
		"daemon.pid names another process":     {release, func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID+1, release) }, "unknown"},
		"nothing holds the lock":               {release, func(*testing.T, string) {}, "unknown"},
		"its daemon records no version": {release, func(t *testing.T, dir string) {
			if err := state.WritePIDFile(dir, daemonPID); err != nil {
				t.Fatal(err)
			}
		}, "unknown"},
	} {
		t.Run("a live saver is stopped and replaced when "+name, func(t *testing.T) {
			sink := logtest.Install(t)
			dir := t.TempDir()
			tc.seed(t, dir)
			fake := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

			if err := newEnsurer(t, &startingDaemon{fake, dir, tc.running}, p, dir, tc.running).EnsureSaver(t.Context()); err != nil {
				t.Fatalf("EnsureSaver: %v", err)
			}

			if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
				t.Errorf("signals = %q, want the old daemon's SIGTERM", got)
			}
			if got, want := ops(fake), []string{"Kill " + saver.SessionName, "Create " + saver.SessionName}; !slices.Equal(got, want) {
				t.Errorf("calls = %q, want %q: the old saver gone before the new one", got, want)
			}
			recycled := sink.Records().Matching("saver", "recycle").Only(t, "the recycle")
			if got := recycled.AttrString(t, "reason"); got != tc.reason {
				t.Errorf("recycle reason = %q, want %q", got, tc.reason)
			}
			if pid, err := state.ReadPIDFile(dir); err != nil || pid == daemonPID {
				t.Errorf("daemon.pid = %d, %v; want the new daemon's", pid, err)
			}
		})
	}

	t.Run("a recycle whose stop is refused creates nothing", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, older)
		fake := withSaver(daemonPID)
		foreign := daemon(nil)
		foreign.env = nil
		p := newProcs(map[int]*proc{daemonPID: foreign})

		err := newEnsurer(t, fake, p, dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrNotSaverDaemon) {
			t.Errorf("EnsureSaver = %v, want the stop's refusal", err)
		}
		if got := ops(fake); len(got) != 0 {
			t.Errorf("calls = %q, want no kill and no create", got)
		}
	})

	t.Run("a recycle whose saver is replaced before its stop sends nothing, and creates nothing", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, older)
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM)), daemonPID + 1: daemon(exitsOn(syscall.SIGTERM))})

		err := newEnsurer(t, &replacedBeforeItsStop{Fake: fake, t: t, dir: dir}, p, dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrReplaced) {
			t.Errorf("EnsureSaver = %v, want ErrReplaced", err)
		}
		if got := p.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		if got := ops(fake); len(got) != 0 {
			t.Errorf("calls = %q, want the saver now running left, and nothing created", got)
		}
	})

	t.Run("a saver another process creates first counts, once its daemon holds the lock at this version", func(t *testing.T) {
		dir := t.TempDir()
		fake := backendtest.New()
		p := newProcs(nil)

		if err := newEnsurer(t, &beatenToIt{fake, dir, release}, p, dir, release).EnsureSaver(t.Context()); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		creates := fake.CallsTo(backendtest.OpCreate)
		if len(creates) != 2 || !slices.Equal(creates[1].Command, []string{exe, "state", "daemon"}) {
			t.Errorf("creates = %+v, want the other creator's, then this one's", creates)
		}
		if got := kills(fake); len(got) != 0 {
			t.Errorf("kills = %q, want none", got)
		}
	})

	t.Run("a saver another process creates first at another version fails once ReadyWait has passed", func(t *testing.T) {
		dir := t.TempDir()

		err := newEnsurer(t, &beatenToIt{backendtest.New(), dir, older}, newProcs(nil), dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady", err)
		}
	})

	t.Run("it waits for its daemon to record its version as well as hold the lock", func(t *testing.T) {
		dir := t.TempDir()

		if err := newEnsurer(t, &slowToRecord{backendtest.New(), t, dir, release}, newProcs(nil), dir, release).EnsureSaver(t.Context()); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		if v, err := state.ReadVersionFile(dir); err != nil || v != release {
			t.Errorf("daemon.version = %q, %v at return; want %s recorded first", v, err, release)
		}
	})

	t.Run("a daemon that records another version fails once ReadyWait has passed", func(t *testing.T) {
		dir := t.TempDir()

		err := newEnsurer(t, &startingDaemon{backendtest.New(), dir, older}, newProcs(nil), dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady", err)
		}
	})

	t.Run("a daemon.pid still naming the daemon it replaced is no readiness", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, older)
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		err := newEnsurer(t, &versionOnly{fake, dir, release}, p, dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady: daemon.version is this build's, but daemon.pid the old daemon's", err)
		}
	})

	t.Run("a daemon that never holds its lock fails once ReadyWait has passed", func(t *testing.T) {
		dir := t.TempDir()
		start := time.Now()

		err := newEnsurer(t, backendtest.New(), newProcs(nil), dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady", err)
		}
		if took := time.Since(start); took < readyWait {
			t.Errorf("EnsureSaver gave up after %s, want ReadyWait (%s)", took, readyWait)
		}
	})

	t.Run("a daemon whose session ends fails at once", func(t *testing.T) {
		dir := t.TempDir()
		start := time.Now()

		err := newEnsurer(t, &refusingDaemon{backendtest.New()}, newProcs(nil), dir, release).EnsureSaver(t.Context())

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady", err)
		}
		if took := time.Since(start); took >= readyWait {
			t.Errorf("EnsureSaver took %s, want no wait for a session already gone", took)
		}
	})

	t.Run("a create that fails is an error", func(t *testing.T) {
		fake := backendtest.New()
		boom := errors.New("zmx attach: exec format error")
		fake.Fail(backendtest.OpCreate, boom)

		if err := newEnsurer(t, fake, newProcs(nil), t.TempDir(), release).EnsureSaver(t.Context()); !errors.Is(err, boom) {
			t.Errorf("EnsureSaver = %v, want %v", err, boom)
		}
	})

	t.Run("a saver that doesn't answer is an error, and nothing is created or signalled", func(t *testing.T) {
		fake := withSaver(daemonPID)
		fake.StopAnswering(saver.SessionName)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		err := newEnsurer(t, fake, p, t.TempDir(), release).EnsureSaver(t.Context())

		if !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("EnsureSaver = %v, want ErrTimeout", err)
		}
		if got := ops(fake); len(got) != 0 || len(p.events) != 0 {
			t.Errorf("calls = %q, events = %q; want none", got, p.events)
		}
	})
}

func TestEnsureSaverHoldsSaverLock(t *testing.T) {
	t.Run("a second ensure waits for the first's saver to come up, and keeps it", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		fake := backendtest.New()
		held := newHeldUp(fake, dir, release)
		first := newEnsurer(t, held, newProcs(nil), dir, release)
		secondProcs := newProcs(nil)
		second := newEnsurer(t, &startingDaemon{fake, dir, release}, secondProcs, dir, release)
		firstDone, secondDone := make(chan error, 1), make(chan error, 1)

		go func() { firstDone <- first.EnsureSaver(context.Background()) }()
		<-held.created
		go func() { secondDone <- second.EnsureSaver(context.Background()) }()
		stillWaiting(t, secondDone, "the second EnsureSaver")
		close(held.open)

		if err := <-firstDone; err != nil {
			t.Errorf("the first EnsureSaver: %v", err)
		}
		if err := <-secondDone; err != nil {
			t.Errorf("the second EnsureSaver: %v", err)
		}
		created, _ := fake.Held(saver.SessionName)
		if got := ops(fake); !slices.Equal(got, []string{"Create " + saver.SessionName}) {
			t.Errorf("calls = %q, want the first's create alone", got)
		}
		if got := secondProcs.signals(); len(got) != 0 {
			t.Errorf("signals = %q, want none", got)
		}
		kept := sink.Records().Matching("saver", "daemon kept").Only(t, "the second's keeping the first's saver")
		if pid := kept.IntAttr(t, "target_pid"); pid != int64(created.PID) {
			t.Errorf("daemon kept target_pid = %d, want the first's %d", pid, created.PID)
		}
	})

	t.Run("a stop waits for a running ensure, then stops the saver it brought up", func(t *testing.T) {
		dir := t.TempDir()
		fake := backendtest.New()
		held := newHeldUp(fake, dir, release)
		ensure := newEnsurer(t, held, newProcs(nil), dir, release)
		ensureDone, stopDone := make(chan error, 1), make(chan error, 1)

		go func() { ensureDone <- ensure.EnsureSaver(context.Background()) }()
		<-held.created
		created, _ := fake.Held(saver.SessionName)
		stopProcs := newProcs(map[int]*proc{created.PID: daemon(exitsOn(syscall.SIGTERM))})
		stopper := hostOf(t, config(fake, stopProcs, dir))
		go func() {
			stopped, err := stopper.Stop(context.Background())
			if err == nil && !stopped {
				err = errors.New("it found no saver to stop")
			}
			stopDone <- err
		}()
		stillWaiting(t, stopDone, "Stop")
		close(held.open)

		if err := <-ensureDone; err != nil {
			t.Errorf("EnsureSaver: %v", err)
		}
		if err := <-stopDone; err != nil {
			t.Errorf("Stop: %v", err)
		}
		if got, want := stopProcs.signals(), []string{fmt.Sprintf("SIGTERM %d", created.PID)}; !slices.Equal(got, want) {
			t.Errorf("signals = %q, want %q", got, want)
		}
		if got := kills(fake); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a saver.lock held past LockWait is an error, and nothing is read, created or sent", func(t *testing.T) {
		dir := t.TempDir()
		holdLock(t, dir)
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		cfg := config(&startingDaemon{fake, dir, release}, p, dir)
		cfg.LockWait = 30 * time.Millisecond

		if err := ensurerOf(t, hostOf(t, cfg), release).EnsureSaver(t.Context()); !errors.Is(err, saverhost.ErrLockHeld) {
			t.Errorf("EnsureSaver = %v, want ErrLockHeld", err)
		}
		if len(fake.Calls()) != 0 || len(p.events) != 0 {
			t.Errorf("backend calls = %+v, events = %q; want none", fake.Calls(), p.events)
		}
	})
}

func TestEnsurer(t *testing.T) {
	h := newHost(t, backendtest.New(), newProcs(nil))
	for name, d := range map[string]saverhost.Daemon{
		"no exe":               {Version: release},
		"an exe found on PATH": {Exe: "portal", Version: release},
		"a relative exe":       {Exe: "bin/portal", Version: release},
		"no version":           {Exe: exe},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if e, err := h.Ensurer(d); err == nil {
				t.Errorf("Ensurer = %v, want a refusal", e)
			}
		})
	}
}
