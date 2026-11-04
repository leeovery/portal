package saverhost_test

import (
	"context"
	"errors"
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
	exe       = "/opt/homebrew/Cellar/portal/0.13.0/bin/portal"
	release   = "0.13.0"
	readyWait = 150 * time.Millisecond
)

// writeDaemon records pid and version in dir as a daemon holding the lock
// does.
func writeDaemon(t *testing.T, dir string, pid int, version string) {
	t.Helper()
	if err := state.WritePIDFile(dir, pid); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteVersionFile(dir, version, nil); err != nil {
		t.Fatal(err)
	}
}

// startingDaemon is the Fake whose saver session's process comes up as
// `portal state daemon` does, holding the lock in dir as version.
type startingDaemon struct {
	*backendtest.Fake
	t       *testing.T
	dir     string
	version string
}

func (b *startingDaemon) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err == nil {
		writeDaemon(b.t, b.dir, s.PID, b.version)
	}
	return s, err
}

// beatenToIt is the Fake where another process creates the saver, its daemon
// coming up, just before this one's create.
type beatenToIt struct {
	*backendtest.Fake
	t   *testing.T
	dir string
}

func (b *beatenToIt) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	theirs, err := b.Fake.Create(ctx, name, dir, []string{"/usr/local/bin/portal", "state", "daemon"})
	if err != nil {
		b.t.Fatal(err)
	}
	writeDaemon(b.t, b.dir, theirs.PID, release)
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

func newEnsurer(t *testing.T, b backend.Backend, p *procs, dir, version string) *saverhost.Ensurer {
	t.Helper()
	e, err := saverhost.NewEnsurer(newStopper(t, b, p), saverhost.Daemon{Exe: exe, Version: version, StateDir: dir, ReadyWait: readyWait})
	if err != nil {
		t.Fatalf("NewEnsurer: %v", err)
	}
	return e
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

		if err := newEnsurer(t, &startingDaemon{fake, t, dir, release}, p, dir, release).EnsureSaver(t.Context()); err != nil {
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

	t.Run("the argv it runs is one the identity check and the pgrep pattern both name", func(t *testing.T) {
		pattern := regexp.MustCompile(state.PortalDaemonArgvPattern)
		for _, path := range []string{exe, "/opt/homebrew/bin/portal", "/Users/someone/go/bin/portal", "/var/folders/x/T/go-build1/b001/exe/portal"} {
			dir := t.TempDir()
			fake := backendtest.New()
			e, err := saverhost.NewEnsurer(newStopper(t, &startingDaemon{fake, t, dir, release}, newProcs(nil)),
				saverhost.Daemon{Exe: path, Version: release, StateDir: dir, ReadyWait: readyWait})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.EnsureSaver(t.Context()); err != nil {
				t.Fatalf("EnsureSaver: %v", err)
			}

			argv := strings.Join(fake.CallsTo(backendtest.OpCreate)[0].Command, " ")
			if !pattern.MatchString(argv) {
				t.Errorf("PortalDaemonArgvPattern does not match the saver's argv %q", argv)
			}
		}
	})

	t.Run("a live saver whose daemon holds the lock at this version is kept", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, release)
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if err := newEnsurer(t, fake, p, dir, release).EnsureSaver(t.Context()); err != nil {
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

	for name, tc := range map[string]struct {
		running string
		seed    func(t *testing.T, dir string)
		reason  string
	}{
		"its daemon records another version": {release, func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID, "0.12.6") }, "version"},
		"this is a dev build, of its version too": {
			"dev-754e31b498e0-modified-20261103T231632.195584591Z",
			func(t *testing.T, dir string) {
				writeDaemon(t, dir, daemonPID, "dev-754e31b498e0-modified-20261103T231632.195584591Z")
			},
			"dev",
		},
		"daemon.pid names another process": {release, func(t *testing.T, dir string) { writeDaemon(t, dir, daemonPID+1, release) }, "unknown"},
		"its daemon records no version": {release, func(t *testing.T, dir string) {
			if err := state.WritePIDFile(dir, daemonPID); err != nil {
				t.Fatal(err)
			}
		}, "unknown"},
		"nothing holds the lock": {release, func(*testing.T, string) {}, "unknown"},
	} {
		t.Run("a live saver is stopped and replaced when "+name, func(t *testing.T) {
			sink := logtest.Install(t)
			dir := t.TempDir()
			tc.seed(t, dir)
			fake := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

			if err := newEnsurer(t, &startingDaemon{fake, t, dir, tc.running}, p, dir, tc.running).EnsureSaver(t.Context()); err != nil {
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
		writeDaemon(t, dir, daemonPID, "0.12.6")
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

	t.Run("a saver another process creates first counts, once its daemon holds the lock", func(t *testing.T) {
		dir := t.TempDir()
		fake := backendtest.New()
		p := newProcs(nil)

		if err := newEnsurer(t, &beatenToIt{fake, t, dir}, p, dir, release).EnsureSaver(t.Context()); err != nil {
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

func TestNewEnsurer(t *testing.T) {
	stopper := newStopper(t, backendtest.New(), newProcs(nil))
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		stopper *saverhost.Stopper
		daemon  saverhost.Daemon
	}{
		"no stopper":           {nil, saverhost.Daemon{Exe: exe, Version: release, StateDir: dir}},
		"no exe":               {stopper, saverhost.Daemon{Version: release, StateDir: dir}},
		"an exe found on PATH": {stopper, saverhost.Daemon{Exe: "portal", Version: release, StateDir: dir}},
		"a relative exe":       {stopper, saverhost.Daemon{Exe: "bin/portal", Version: release, StateDir: dir}},
		"no version":           {stopper, saverhost.Daemon{Exe: exe, StateDir: dir}},
		"no state directory":   {stopper, saverhost.Daemon{Exe: exe, Version: release}},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if e, err := saverhost.NewEnsurer(tc.stopper, tc.daemon); err == nil {
				t.Errorf("NewEnsurer = %v, want a refusal", e)
			}
		})
	}
}
