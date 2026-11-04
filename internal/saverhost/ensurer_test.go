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
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/devversion"
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

// lateDaemon is the Fake whose saver session's create returns at once, its
// daemon recording itself at version only once open closes: an ensure
// waiting on its daemon's readiness, as another run reads the saver.
type lateDaemon struct {
	*backendtest.Fake
	dir      string
	version  string
	created  chan struct{}
	open     chan struct{}
	recorded chan error
}

func newLateDaemon(fake *backendtest.Fake, dir, version string) *lateDaemon {
	return &lateDaemon{Fake: fake, dir: dir, version: version,
		created: make(chan struct{}), open: make(chan struct{}), recorded: make(chan error, 1)}
}

func (b *lateDaemon) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	s, err := b.Fake.Create(ctx, name, dir, command)
	if err != nil {
		return s, err
	}
	close(b.created)
	go func() {
		<-b.open
		b.recorded <- record(b.dir, s.PID, b.version)
	}()
	return s, nil
}

// counted is a backend whose reads of a session it counts, as one run makes
// them.
type counted struct {
	backend.Backend
	reads atomic.Int32
}

func (b *counted) Session(ctx context.Context, name string) (backend.Session, error) {
	b.reads.Add(1)
	return b.Backend.Session(ctx, name)
}

// recordsLater has the daemon of the saver session's pid record itself at
// version after a while, as a daemon still starting does, its pid first.
func recordsLater(t *testing.T, dir string, pid int, version string) {
	t.Helper()
	recorded := make(chan struct{})
	t.Cleanup(func() { <-recorded })
	time.AfterFunc(readyWait/3, func() {
		defer close(recorded)
		if err := record(dir, pid, version); err != nil {
			t.Error(err)
		}
	})
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

// patientEnsurer is newEnsurer at this release, waiting on its daemon's
// readiness long enough for a test to hold it there.
func patientEnsurer(t *testing.T, b backend.Backend, p *procs, dir string) *saverhost.Ensurer {
	t.Helper()
	cfg := config(b, p, dir)
	cfg.ReadyWait = lockWait
	return ensurerOf(t, hostOf(t, cfg), release)
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
		start := time.Now()

		if err := newEnsurer(t, &startingDaemon{fake, dir, release}, p, dir, release).EnsureSaver(t.Context(), nil); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		if took := time.Since(start); took >= readyWait {
			t.Errorf("EnsureSaver took %s, want no wait on a saver there isn't", took)
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

		if err := newEnsurer(t, &startingDaemon{backendtest.New(), dir, release}, newProcs(nil), dir, release).EnsureSaver(t.Context(), nil); err != nil {
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
			if err := e.EnsureSaver(t.Context(), nil); err != nil {
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

			if err := newEnsurer(t, fake, p, dir, version).EnsureSaver(t.Context(), nil); err != nil {
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
		"this build is unidentified, though its daemon records the same": {devversion.Unidentified, func(t *testing.T, dir string) {
			writeDaemon(t, dir, daemonPID, devversion.Unidentified)
		}, "unknown"},
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

			if err := newEnsurer(t, &startingDaemon{fake, dir, tc.running}, p, dir, tc.running).EnsureSaver(t.Context(), nil); err != nil {
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

	t.Run("a saver still starting is waited for, and kept once its daemon records this version", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		recordsLater(t, dir, daemonPID, release)

		if err := newEnsurer(t, fake, p, dir, release).EnsureSaver(t.Context(), nil); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		if got := ops(fake); len(got) != 0 || len(p.signals()) != 0 {
			t.Errorf("calls = %q, signals = %q; want the starting saver left be", got, p.signals())
		}
		sink.Records().Matching("saver", "daemon kept").Only(t, "the kept saver")
	})

	t.Run("a saver still starting that records another version is replaced for its version", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		recordsLater(t, dir, daemonPID, older)

		if err := newEnsurer(t, &startingDaemon{fake, dir, release}, p, dir, release).EnsureSaver(t.Context(), nil); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		recycled := sink.Records().Matching("saver", "recycle").Only(t, "the recycle")
		if got := recycled.AttrString(t, "reason"); got != "version" {
			t.Errorf("recycle reason = %q, want version", got)
		}
	})

	t.Run("a saver whose daemon never records itself is judged unknown only once ReadyWait has passed", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		start := time.Now()

		if err := newEnsurer(t, &startingDaemon{fake, dir, release}, p, dir, release).EnsureSaver(t.Context(), nil); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		recycled := sink.Records().Matching("saver", "recycle").Only(t, "the recycle")
		if got := recycled.AttrString(t, "reason"); got != "unknown" {
			t.Errorf("recycle reason = %q, want unknown", got)
		}
		if took := time.Since(start); took < readyWait {
			t.Errorf("EnsureSaver recycled after %s, want ReadyWait (%s) waited first", took, readyWait)
		}
	})

	t.Run("a recycle whose stop is refused creates nothing", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, older)
		fake := withSaver(daemonPID)
		foreign := daemon(nil)
		foreign.env = nil
		p := newProcs(map[int]*proc{daemonPID: foreign})

		err := newEnsurer(t, fake, p, dir, release).EnsureSaver(t.Context(), nil)

		if !errors.Is(err, saverhost.ErrNotSaverDaemon) || !errors.Is(err, saverhost.ErrNotReplaced) {
			t.Errorf("EnsureSaver = %v, want the stop's refusal, ErrNotReplaced while its daemon runs", err)
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

		err := newEnsurer(t, &replacedBeforeItsStop{Fake: fake, t: t, dir: dir}, p, dir, release).EnsureSaver(t.Context(), nil)

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

	t.Run("a recycle whose stop fails once the old daemon has exited is no ErrNotReplaced", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, older)
		fake := withSaver(daemonPID)
		boom := errors.New("zmx kill: no answer")
		fake.Fail(backendtest.OpKill, boom)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		err := newEnsurer(t, fake, p, dir, release).EnsureSaver(t.Context(), nil)

		if !errors.Is(err, boom) || errors.Is(err, saverhost.ErrNotReplaced) {
			t.Errorf("EnsureSaver = %v, want the kill's failure, and no ErrNotReplaced: the old daemon is gone", err)
		}
	})

	t.Run("it judges the saver it creates before waiting for its daemon, and the judge's error ends it there", func(t *testing.T) {
		dir := t.TempDir()
		fake := backendtest.New()
		refused := errors.New("an older zmx")
		judged := 0
		start := time.Now()

		err := newEnsurer(t, fake, newProcs(nil), dir, release).EnsureSaver(t.Context(), func(context.Context) error {
			judged++
			if _, held := fake.Held(saver.SessionName); !held {
				t.Error("the judge ran before the saver was created")
			}
			return refused
		})

		if !errors.Is(err, refused) || judged != 1 {
			t.Errorf("EnsureSaver = %v after %d judgements, want the judge's error after one", err, judged)
		}
		if took := time.Since(start); took >= readyWait {
			t.Errorf("EnsureSaver took %s, want no wait for a daemon the judge refused", took)
		}
	})

	t.Run("a saver the judge passes is waited for as any other", func(t *testing.T) {
		dir := t.TempDir()
		judged := 0

		err := newEnsurer(t, &startingDaemon{backendtest.New(), dir, release}, newProcs(nil), dir, release).EnsureSaver(t.Context(), func(context.Context) error {
			judged++
			return nil
		})

		if err != nil || judged != 1 {
			t.Errorf("EnsureSaver = %v after %d judgements, want nil after one", err, judged)
		}
	})

	t.Run("it judges the saver a recycle creates, and never one it keeps", func(t *testing.T) {
		for _, tc := range []struct {
			recorded string
			judged   int
		}{{release, 0}, {older, 1}} {
			dir := t.TempDir()
			writeDaemon(t, dir, daemonPID, tc.recorded)
			p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
			judged := 0

			err := newEnsurer(t, &startingDaemon{withSaver(daemonPID), dir, release}, p, dir, release).EnsureSaver(t.Context(), func(context.Context) error {
				judged++
				return nil
			})

			if err != nil || judged != tc.judged {
				t.Errorf("over a saver recording %s: EnsureSaver = %v after %d judgements, want nil after %d", tc.recorded, err, judged, tc.judged)
			}
		}
	})

	t.Run("a saver another process creates first counts, once its daemon holds the lock at this version", func(t *testing.T) {
		dir := t.TempDir()
		fake := backendtest.New()
		p := newProcs(nil)

		if err := newEnsurer(t, &beatenToIt{fake, dir, release}, p, dir, release).EnsureSaver(t.Context(), nil); err != nil {
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

		err := newEnsurer(t, &beatenToIt{backendtest.New(), dir, older}, newProcs(nil), dir, release).EnsureSaver(t.Context(), nil)

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady", err)
		}
	})

	t.Run("it waits for its daemon to record its version as well as hold the lock", func(t *testing.T) {
		dir := t.TempDir()

		if err := newEnsurer(t, &slowToRecord{backendtest.New(), t, dir, release}, newProcs(nil), dir, release).EnsureSaver(t.Context(), nil); err != nil {
			t.Fatalf("EnsureSaver: %v", err)
		}

		if v, err := state.ReadVersionFile(dir); err != nil || v != release {
			t.Errorf("daemon.version = %q, %v at return; want %s recorded first", v, err, release)
		}
	})

	t.Run("a daemon that records another version fails once ReadyWait has passed", func(t *testing.T) {
		dir := t.TempDir()

		err := newEnsurer(t, &startingDaemon{backendtest.New(), dir, older}, newProcs(nil), dir, release).EnsureSaver(t.Context(), nil)

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady", err)
		}
	})

	t.Run("a daemon.pid still naming the daemon it replaced is no readiness", func(t *testing.T) {
		dir := t.TempDir()
		writeDaemon(t, dir, daemonPID, older)
		fake := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		err := newEnsurer(t, &versionOnly{fake, dir, release}, p, dir, release).EnsureSaver(t.Context(), nil)

		if !errors.Is(err, saverhost.ErrNotReady) {
			t.Errorf("EnsureSaver = %v, want ErrNotReady: daemon.version is this build's, but daemon.pid the old daemon's", err)
		}
	})

	t.Run("a daemon that never holds its lock fails once ReadyWait has passed", func(t *testing.T) {
		dir := t.TempDir()
		start := time.Now()

		err := newEnsurer(t, backendtest.New(), newProcs(nil), dir, release).EnsureSaver(t.Context(), nil)

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

		err := newEnsurer(t, &refusingDaemon{backendtest.New()}, newProcs(nil), dir, release).EnsureSaver(t.Context(), nil)

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

		if err := newEnsurer(t, fake, newProcs(nil), t.TempDir(), release).EnsureSaver(t.Context(), nil); !errors.Is(err, boom) {
			t.Errorf("EnsureSaver = %v, want %v", err, boom)
		}
	})

	t.Run("a saver that doesn't answer is an error, and nothing is created or signalled", func(t *testing.T) {
		fake := withSaver(daemonPID)
		fake.StopAnswering(saver.SessionName)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		start := time.Now()

		err := newEnsurer(t, fake, p, t.TempDir(), release).EnsureSaver(t.Context(), nil)

		if !errors.Is(err, backend.ErrTimeout) || !errors.Is(err, saverhost.ErrUnread) {
			t.Errorf("EnsureSaver = %v, want ErrUnread wrapping ErrTimeout", err)
		}
		if took := time.Since(start); took >= readyWait {
			t.Errorf("EnsureSaver took %s, want it failing at the read, not waiting", took)
		}
		if got := ops(fake); len(got) != 0 || len(p.events) != 0 {
			t.Errorf("calls = %q, events = %q; want none", got, p.events)
		}
	})
}

func TestEnsureSaverHoldsSaverLock(t *testing.T) {
	t.Run("a second ensure waits for the first's saver to come up, reading nothing until then, and keeps it", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := t.TempDir()
		fake := backendtest.New()
		late := newLateDaemon(fake, dir, release)
		first := patientEnsurer(t, late, newProcs(nil), dir)
		secondProcs := newProcs(nil)
		secondReads := &counted{Backend: &startingDaemon{fake, dir, release}}
		second := newEnsurer(t, secondReads, secondProcs, dir, release)
		firstDone, secondDone := make(chan error, 1), make(chan error, 1)

		go func() { firstDone <- first.EnsureSaver(context.Background(), nil) }()
		<-late.created
		go func() { secondDone <- second.EnsureSaver(context.Background(), nil) }()
		stillWaiting(t, secondDone, "the second EnsureSaver")
		if n := secondReads.reads.Load(); n != 0 {
			t.Errorf("the second EnsureSaver read the saver %d times while the first awaited its daemon, want none", n)
		}
		close(late.open)

		if err := <-late.recorded; err != nil {
			t.Error(err)
		}
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
		late := newLateDaemon(fake, dir, release)
		ensure := patientEnsurer(t, late, newProcs(nil), dir)
		ensureDone, stopDone := make(chan error, 1), make(chan error, 1)

		go func() { ensureDone <- ensure.EnsureSaver(context.Background(), nil) }()
		<-late.created
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
		close(late.open)

		if err := <-late.recorded; err != nil {
			t.Error(err)
		}
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

		if err := ensurerOf(t, hostOf(t, cfg), release).EnsureSaver(t.Context(), nil); !errors.Is(err, saverhost.ErrLockHeld) {
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
