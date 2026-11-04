package saverhost_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

// tornDown is the Fake whose saver session goes as its kill fails, as zmx's
// does when it tears the session down under the kill's connection.
type tornDown struct{ *backendtest.Fake }

func (b *tornDown) Kill(ctx context.Context, name string) error {
	if err := b.Fake.Kill(ctx, name); err != nil {
		return err
	}
	return syscall.ECONNRESET
}

// changingSaver is the Fake whose saver session changes as change says just
// before the nth read of it.
type changingSaver struct {
	*backendtest.Fake
	n      int
	change func(*backendtest.Fake)
	reads  int
}

func (b *changingSaver) Session(ctx context.Context, name string) (backend.Session, error) {
	if b.reads++; b.reads == b.n {
		b.change(b.Fake)
	}
	return b.Fake.Session(ctx, name)
}

// stop stops h's saver, failing the test unless it reports one stopped.
func stop(t *testing.T, h *saverhost.Host) {
	t.Helper()
	if stopped, err := h.Stop(t.Context()); !stopped || err != nil {
		t.Fatalf("Stop = %v, %v; want true, nil", stopped, err)
	}
}

func TestStop(t *testing.T) {
	t.Run("no saver is nothing to stop, and nothing is read of a process or signalled", func(t *testing.T) {
		b := backendtest.New()
		p := newProcs(nil)

		stopped, err := newHost(t, b, p).Stop(t.Context())

		if stopped || err != nil {
			t.Errorf("Stop = %v, %v; want false, nil", stopped, err)
		}
		if len(p.events) != 0 || len(kills(b)) != 0 {
			t.Errorf("events = %q, kills = %q; want none", p.events, kills(b))
		}
	})

	t.Run("it proves the daemon, sends it SIGTERM, and kills the session once it has exited", func(t *testing.T) {
		sink := logtest.Install(t)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		stop(t, newHost(t, b, p))

		if want := []string{"identify 4242", "args 4242", "SIGTERM 4242"}; !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q: the proof, then the signal", p.events, want)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
		started := sink.Records().Matching("saver", "stop started").Only(t, "the stop's start")
		if pid := started.IntAttr(t, "target_pid"); pid != daemonPID {
			t.Errorf("stop started target_pid = %d, want %d", pid, daemonPID)
		}
		sink.Records().Matching("saver", "daemon exited").Only(t, "the daemon's exit").RequireDuration(t, "took")
	})

	t.Run("it waits out TermWait, proves the daemon again and sends it SIGKILL", func(t *testing.T) {
		sink := logtest.Install(t)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGKILL))})
		start := time.Now()

		stop(t, newHost(t, b, p))

		if took := time.Since(start); took < termWait {
			t.Errorf("Stop took %s, want SIGKILL only once TermWait (%s) passed", took, termWait)
		}
		want := []string{"identify 4242", "args 4242", "SIGTERM 4242", "identify 4242", "args 4242", "SIGKILL 4242"}
		if !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q: a fresh proof right before the SIGKILL", p.events, want)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's, once the daemon is gone", got)
		}
		escalated := sink.Records().Matching("saver", "stop escalated").Only(t, "the escalation")
		if reason := escalated.AttrString(t, "reason"); reason != "term-timeout" {
			t.Errorf("stop escalated reason = %q, want term-timeout", reason)
		}
	})

	for name, change := range map[string]func(p *proc){
		"can't be read":                      func(p *proc) { p.identifyErr = errors.New("ps: malformed output") },
		"is another program":                 func(p *proc) { p.verdict = state.IdentifyNotPortalDaemon },
		"runs another program's argv":        func(p *proc) { p.argv = []string{"/bin/sleep", "1000"} },
		"lacks the saver session's marks":    func(p *proc) { p.env = []string{"PATH=/usr/bin"} },
		"has its environment hidden from it": func(p *proc) { p.argsErr = proctable.ErrUnsupported },
	} {
		t.Run("a daemon that outlives TermWait and then "+name+" is sent no SIGKILL, and its session is left", func(t *testing.T) {
			b := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(func(p *proc, sig syscall.Signal) {
				if sig == syscall.SIGTERM {
					change(p)
				}
			})})

			_, err := newHost(t, b, p).Stop(t.Context())

			if err == nil || !strings.Contains(err.Error(), "sent SIGTERM") || !strings.Contains(err.Error(), "wasn't sent SIGKILL") {
				t.Errorf("Stop = %v, want a failure saying it was sent SIGTERM and not SIGKILL", err)
			}
			if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
				t.Errorf("signals = %q, want the SIGTERM alone", got)
			}
			if got := kills(b); len(got) != 0 {
				t.Errorf("kills = %q, want the session left while its daemon may run", got)
			}
		})
	}

	for name, gone := range map[string]func(p *proc){
		"has exited": func(p *proc) { p.exited = true },
		"is a zombie, which ps shows as no daemon": func(p *proc) { p.zombie = true },
	} {
		t.Run("a daemon that "+name+" by its re-proof is stopped, sent no SIGKILL", func(t *testing.T) {
			b := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
			identified := 0
			cfg := config(b, p, t.TempDir())
			cfg.Identify = func(pid int) (state.IdentifyResult, error) {
				// Gone between the wait's last read and the re-proof's identity.
				if identified++; identified > 1 {
					gone(p.table[daemonPID])
				}
				return p.identify(pid)
			}

			stop(t, hostOf(t, cfg))

			if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
				t.Errorf("signals = %q, want the SIGTERM alone", got)
			}
			if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
				t.Errorf("kills = %q, want the saver session's", got)
			}
		})
	}

	for name, tc := range map[string]struct {
		change func(p *proc)
		want   error
	}{
		"is another program": {func(p *proc) { p.verdict = state.IdentifyNotPortalDaemon }, saverhost.ErrNotSaverDaemon},
		"was reused for another program since its identity's read": {func(p *proc) { p.argv = []string{"/bin/sleep", "1000"} }, saverhost.ErrNotSaverDaemon},
		"lacks the session's name":                                 {func(p *proc) { p.env = []string{marks[1]} }, saverhost.ErrNotSaverDaemon},
		"lacks the backend's directory":                            {func(p *proc) { p.env = []string{marks[0]} }, saverhost.ErrNotSaverDaemon},
		"runs in another zmx directory":                            {func(p *proc) { p.env = []string{marks[0], "ZMX_DIR=/tmp/zx-elsewhere"} }, saverhost.ErrNotSaverDaemon},
		"names the directory twice, differently":                   {func(p *proc) { p.env = append(slices.Clone(marks), "ZMX_DIR=/tmp/zx-elsewhere") }, saverhost.ErrNotSaverDaemon},
		"can't be identified":                                      {func(p *proc) { p.identifyErr = errors.New("ps: malformed output") }, saverhost.ErrUnreadable},
		"has an environment that can't be read":                    {func(p *proc) { p.argsErr = errors.New("sysctl: EPERM") }, saverhost.ErrUnreadable},
		"has its environment unread off darwin":                    {func(p *proc) { p.argsErr = proctable.ErrUnsupported }, saverhost.ErrUnreadable},
		"is a portal daemon with no marks at all":                  {func(p *proc) { p.env = nil }, saverhost.ErrNotSaverDaemon},
	} {
		t.Run("a saver session's process that "+name+" is sent nothing, and its session is left", func(t *testing.T) {
			sink := logtest.Install(t)
			b := withSaver(daemonPID)
			d := daemon(exitsOn(syscall.SIGTERM, syscall.SIGKILL))
			tc.change(d)
			p := newProcs(map[int]*proc{daemonPID: d})

			_, err := newHost(t, b, p).Stop(t.Context())

			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "nothing was sent") {
				t.Errorf("Stop = %v, want %v, saying nothing was sent", err, tc.want)
			}
			if got := p.signals(); len(got) != 0 {
				t.Errorf("signals = %q, want none", got)
			}
			if got := kills(b); len(got) != 0 {
				t.Errorf("kills = %q, want the session left", got)
			}
			refused := sink.Records().Matching("saver", "stop refused").AtExactLevel(slog.LevelWarn).Only(t, "the refusal")
			if pid := refused.IntAttr(t, "target_pid"); pid != daemonPID {
				t.Errorf("stop refused target_pid = %d, want %d", pid, daemonPID)
			}
		})
	}

	for name, d := range map[string]*proc{
		"gone":                                  {exited: true},
		"a zombie, which ps shows as no daemon": {zombie: true},
	} {
		t.Run("a saver session's process "+name+" before its proof is sent nothing, and its session is killed", func(t *testing.T) {
			b := withSaver(daemonPID)
			p := newProcs(map[int]*proc{daemonPID: d})

			stop(t, newHost(t, b, p))

			if got := p.signals(); len(got) != 0 {
				t.Errorf("signals = %q, want none", got)
			}
			if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
				t.Errorf("kills = %q, want the saver session's", got)
			}
		})
	}

	t.Run("a daemon found gone while still exiting is waited for before its session is killed", func(t *testing.T) {
		b := withSaver(daemonPID)
		d := exiting(3)
		p := newProcs(map[int]*proc{daemonPID: d})

		stop(t, newHost(t, b, p))

		if d.lingers != 0 {
			t.Errorf("the daemon had %d reads of its exit left, want the stop to wait them out", d.lingers)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon found gone that doesn't finish exiting fails the stop, and its session is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: exiting(1 << 30)})

		if _, err := newHost(t, b, p).Stop(t.Context()); err == nil {
			t.Error("Stop = nil error, want a failure")
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a daemon gone as its SIGTERM is sent is stopped", func(t *testing.T) {
		b := withSaver(daemonPID)
		d := daemon(nil)
		p := newProcs(map[int]*proc{daemonPID: d})
		cfg := config(b, p, t.TempDir())
		cfg.Signal = func(pid int, sig syscall.Signal) error {
			d.exited = true
			return p.signal(pid, sig)
		}

		stop(t, hostOf(t, cfg))

		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a daemon that outlives its SIGKILL fails the stop, and its session is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})

		if _, err := newHost(t, b, p).Stop(t.Context()); err == nil {
			t.Error("Stop = nil error, want a failure")
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242", "SIGKILL 4242"}) {
			t.Errorf("signals = %q, want SIGTERM then SIGKILL", got)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a signal that fails fails the stop", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		cfg := config(b, p, t.TempDir())
		cfg.Signal = func(int, syscall.Signal) error { return syscall.EPERM }

		if _, err := hostOf(t, cfg).Stop(t.Context()); !errors.Is(err, syscall.EPERM) {
			t.Errorf("Stop = %v, want EPERM", err)
		}
		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the session left", got)
		}
	})

	t.Run("a pid the saver session no longer runs is sent nothing, and its refusal logged", func(t *testing.T) {
		sink := logtest.Install(t)
		fake := withSaver(daemonPID)
		b := &changingSaver{Fake: fake, n: 2, change: func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: saver.SessionName, PID: daemonPID + 1})
		}}
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM)), daemonPID + 1: daemon(exitsOn(syscall.SIGTERM))})

		if _, err := newHost(t, b, p).Stop(t.Context()); !errors.Is(err, saverhost.ErrReplaced) {
			t.Errorf("Stop = %v, want ErrReplaced", err)
		}
		if len(p.events) != 0 || len(kills(fake)) != 0 {
			t.Errorf("events = %q, kills = %q; want none", p.events, kills(fake))
		}
		assertRefused(t, sink, daemonPID, saverhost.ErrReplaced.Error())
	})

	t.Run("this process is sent nothing, and its refusal logged", func(t *testing.T) {
		sink := logtest.Install(t)
		self := os.Getpid()
		b := withSaver(self)
		p := newProcs(map[int]*proc{self: daemon(exitsOn(syscall.SIGTERM))})

		if _, err := newHost(t, b, p).Stop(t.Context()); err == nil {
			t.Error("Stop of this process = nil error, want a refusal")
		}
		if len(p.events) != 0 || len(kills(b)) != 0 {
			t.Errorf("events = %q, kills = %q; want none", p.events, kills(b))
		}
		assertRefused(t, sink, self, "is this process")
	})

	t.Run("a saver session gone since its pid was read leaves the pid to its proof", func(t *testing.T) {
		fake := withSaver(daemonPID)
		b := &changingSaver{Fake: fake, n: 2, change: func(f *backendtest.Fake) { f.Remove(saver.SessionName) }}
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		stop(t, newHost(t, b, p))

		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the proved daemon's SIGTERM", got)
		}
		if got := kills(fake); len(got) != 0 {
			t.Errorf("kills = %q, want none: there is no session", got)
		}
	})

	t.Run("a saver session that doesn't answer is sent nothing", func(t *testing.T) {
		b := withSaver(daemonPID)
		b.StopAnswering(saver.SessionName)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if _, err := newHost(t, b, p).Stop(t.Context()); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Stop = %v, want ErrTimeout", err)
		}
		if len(p.events) != 0 {
			t.Errorf("events = %q, want none", p.events)
		}
	})

	t.Run("a saver session naming no process is sent nothing", func(t *testing.T) {
		p := newProcs(nil)

		if _, err := newHost(t, withSaver(0), p).Stop(t.Context()); !errors.Is(err, saverhost.ErrNoProcess) {
			t.Errorf("Stop = %v, want ErrNoProcess", err)
		}
		if len(p.events) != 0 {
			t.Errorf("events = %q, want none", p.events)
		}
	})

	t.Run("a wait the context ends fails the stop, sending no SIGKILL", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		ctx, cancel := context.WithCancel(t.Context())
		p.table[daemonPID].onSignal = func(*proc, syscall.Signal) { cancel() }

		if _, err := newHost(t, b, p).Stop(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Stop = %v, want the context's cause", err)
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the SIGTERM alone", got)
		}
	})

	t.Run("a kill that fails as zmx tears the session down succeeds", func(t *testing.T) {
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		stop(t, newHost(t, &tornDown{withSaver(daemonPID)}, p))
	})

	t.Run("a kill that fails with the session still there fails the stop", func(t *testing.T) {
		b := withSaver(daemonPID)
		boom := errors.New("write: broken pipe")
		b.Fail(backendtest.OpKill, boom)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		if _, err := newHost(t, b, p).Stop(t.Context()); !errors.Is(err, boom) {
			t.Errorf("Stop = %v, want %v", err, boom)
		}
	})

	t.Run("a saver made since its daemon exited is left", func(t *testing.T) {
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(func(p *proc, sig syscall.Signal) {
			p.exited = true
			b.Add(backend.Session{Name: saver.SessionName, PID: daemonPID + 1})
		})})

		stop(t, newHost(t, b, p))

		if got := kills(b); len(got) != 0 {
			t.Errorf("kills = %q, want the new saver left", got)
		}
	})
}

func TestStopHoldsSaverLock(t *testing.T) {
	t.Run("it waits for saver.lock before it reads the saver", func(t *testing.T) {
		dir := t.TempDir()
		release := holdLock(t, dir)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		h := hostOf(t, config(b, p, dir))
		done := make(chan error, 1)

		go func() {
			_, err := h.Stop(context.Background())
			done <- err
		}()

		stillWaiting(t, done, "Stop")
		if calls := b.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v while saver.lock was held, want none", calls)
		}
		release()
		if err := <-done; err != nil {
			t.Errorf("Stop: %v", err)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a saver.lock held past LockWait is an error, and nothing is read or sent", func(t *testing.T) {
		dir := t.TempDir()
		holdLock(t, dir)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		cfg := config(b, p, dir)
		cfg.LockWait = 30 * time.Millisecond
		h := hostOf(t, cfg)
		before := openFDs(t)
		start := time.Now()

		if stopped, err := h.Stop(t.Context()); stopped || !errors.Is(err, saverhost.ErrLockHeld) {
			t.Errorf("Stop = %v, %v; want false and ErrLockHeld", stopped, err)
		}
		if took := time.Since(start); took < cfg.LockWait {
			t.Errorf("Stop gave up after %s, want LockWait (%s)", took, cfg.LockWait)
		}
		if after := openFDs(t); after != before {
			t.Errorf("open descriptors = %d after the stop, want %d: saver.lock's left open", after, before)
		}
		if len(b.Calls()) != 0 || len(p.events) != 0 {
			t.Errorf("backend calls = %+v, events = %q; want none", b.Calls(), p.events)
		}
	})

	t.Run("two stops send the daemon one SIGTERM", func(t *testing.T) {
		dir := t.TempDir()
		b := withSaver(daemonPID)
		signalled, gate := make(chan struct{}), make(chan struct{})
		first := newProcs(map[int]*proc{daemonPID: daemon(func(p *proc, sig syscall.Signal) {
			close(signalled)
			<-gate
			p.exited = true
		})})
		second := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		firstHost, secondHost := hostOf(t, config(b, first, dir)), hostOf(t, config(b, second, dir))
		firstDone, secondDone := make(chan error, 1), make(chan error, 1)

		go func() {
			_, err := firstHost.Stop(context.Background())
			firstDone <- err
		}()
		<-signalled
		go func() {
			stopped, err := secondHost.Stop(context.Background())
			if stopped {
				err = errors.Join(err, errors.New("the second stop found a saver to stop"))
			}
			secondDone <- err
		}()
		stillWaiting(t, secondDone, "the second Stop")
		close(gate)

		if err := <-firstDone; err != nil {
			t.Errorf("the first Stop: %v", err)
		}
		if err := <-secondDone; err != nil {
			t.Errorf("the second Stop: %v", err)
		}
		if got := append(first.signals(), second.signals()...); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want one SIGTERM", got)
		}
	})

	t.Run("with no state directory and no saver it stops nothing, and creates nothing", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		p := newProcs(nil)

		stopped, err := hostOf(t, config(backendtest.New(), p, dir)).Stop(t.Context())

		if stopped || err != nil {
			t.Errorf("Stop = %v, %v; want false, nil", stopped, err)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s = %v, want it still absent", dir, err)
		}
	})

	t.Run("with no state directory but a saver, it creates the directory for its lock and stops the saver", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})

		stop(t, hostOf(t, config(b, p, dir)))

		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("stat %s = %v, %v; want a directory, mode 0700", dir, info, err)
		}
		if _, err := os.Stat(saverhost.LockPath(dir)); err != nil {
			t.Errorf("stat saver.lock: %v, want it made", err)
		}
		if got := p.signals(); !slices.Equal(got, []string{"SIGTERM 4242"}) {
			t.Errorf("signals = %q, want the daemon's SIGTERM", got)
		}
		if got := kills(b); !slices.Equal(got, []string{saver.SessionName}) {
			t.Errorf("kills = %q, want the saver session's", got)
		}
	})

	t.Run("a context that ends while saver.lock is held fails the stop, leaving no descriptor open", func(t *testing.T) {
		dir := t.TempDir()
		holdLock(t, dir)
		b := withSaver(daemonPID)
		p := newProcs(map[int]*proc{daemonPID: daemon(exitsOn(syscall.SIGTERM))})
		h := hostOf(t, config(b, p, dir))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		before := openFDs(t)

		if _, err := h.Stop(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Stop = %v, want the context's cause", err)
		}
		if after := openFDs(t); after != before {
			t.Errorf("open descriptors = %d after the stop, want %d: saver.lock's left open", after, before)
		}
		if len(b.Calls()) != 0 || len(p.events) != 0 {
			t.Errorf("backend calls = %+v, events = %q; want none", b.Calls(), p.events)
		}
	})
}

// assertRefused asserts the one WARN stop refused, for pid, its error saying
// why.
func assertRefused(t *testing.T, sink *logtest.Sink, pid int, why string) {
	t.Helper()
	refused := sink.Records().Matching("saver", "stop refused").AtExactLevel(slog.LevelWarn).Only(t, "the refusal")
	if got := refused.IntAttr(t, "target_pid"); got != int64(pid) {
		t.Errorf("stop refused target_pid = %d, want %d", got, pid)
	}
	if err := refused.ErrorAttr(t, "error"); !strings.Contains(err.Error(), why) {
		t.Errorf("stop refused error = %v, want it saying %q", err, why)
	}
}

// The kernel's own reads, as a Host makes them by default, of the test's own
// child: a zombie has finished exiting, and is gone.
func TestTheDefaultReadsCountAZombieExited(t *testing.T) {
	child := exec.Command("/usr/bin/true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Wait() })
	h := hostOf(t, saverhost.Config{
		Backend:  backendtest.New(),
		StateDir: t.TempDir(),
		Marks:    marks,
		Signal: func(pid int, sig syscall.Signal) error {
			t.Errorf("signal %v to %d, want none", sig, pid)
			return nil
		},
		KillWait: 2 * time.Second,
		Poll:     5 * time.Millisecond,
	})

	if err := h.AwaitExit(t.Context(), child.Process.Pid); err != nil {
		t.Errorf("AwaitExit(an unreaped child) = %v, want it exited once a zombie", err)
	}
	if err := h.Prove(child.Process.Pid); !errors.Is(err, saverhost.ErrGone) {
		t.Errorf("Prove(a zombie) = %v, want ErrGone", err)
	}
	quick := hostOf(t, saverhost.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Marks: marks,
		Signal: func(int, syscall.Signal) error { return nil }, KillWait: 30 * time.Millisecond})
	if err := quick.AwaitExit(t.Context(), os.Getpid()); err == nil {
		t.Error("AwaitExit(this process) = nil, want it still running")
	}
}

func TestProve(t *testing.T) {
	t.Run("it proves a portal state daemon carrying every mark", func(t *testing.T) {
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		if err := newHost(t, backendtest.New(), p).Prove(daemonPID); err != nil {
			t.Errorf("Prove = %v, want proved", err)
		}
	})

	t.Run("it reads the identity first, and the argv and environment only of a portal state daemon", func(t *testing.T) {
		other := daemon(nil)
		other.verdict = state.IdentifyNotPortalDaemon
		p := newProcs(map[int]*proc{daemonPID: other})

		_ = newHost(t, backendtest.New(), p).Prove(daemonPID)

		if want := []string{"identify 4242"}; !slices.Equal(p.events, want) {
			t.Errorf("events = %q, want %q", p.events, want)
		}
	})

	t.Run("a zombie is gone before it is identified", func(t *testing.T) {
		p := newProcs(map[int]*proc{daemonPID: {zombie: true}})

		if err := newHost(t, backendtest.New(), p).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove = %v, want ErrGone", err)
		}
		if len(p.events) != 0 {
			t.Errorf("events = %q, want nothing read of it", p.events)
		}
	})

	t.Run("a process that becomes a zombie as ps reads it is gone, not another program", func(t *testing.T) {
		d := daemon(nil)
		p := newProcs(map[int]*proc{daemonPID: d})
		cfg := config(backendtest.New(), p, t.TempDir())
		cfg.Identify = func(pid int) (state.IdentifyResult, error) {
			d.zombie = true
			return p.identify(pid)
		}

		if err := hostOf(t, cfg).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove = %v, want ErrGone", err)
		}
	})

	t.Run("the environment's argv must name a daemon too, since the pid may be reused between the reads", func(t *testing.T) {
		for _, argv := range [][]string{
			{"/bin/sleep", "1000"},
			{"/bin/sh", "-c", "/opt/homebrew/bin/portal state daemon"},
			{"/opt/homebrew/bin/portal", "state", "hydrate"},
			{"/opt/homebrew/bin/portal", "state daemon"},
			nil,
		} {
			d := daemon(nil)
			d.argv = argv
			p := newProcs(map[int]*proc{daemonPID: d})

			if err := newHost(t, backendtest.New(), p).Prove(daemonPID); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
				t.Errorf("Prove of argv %q = %v, want ErrNotSaverDaemon", argv, err)
			}
		}
	})

	t.Run("a process gone at either read is ErrGone", func(t *testing.T) {
		if err := newHost(t, backendtest.New(), newProcs(nil)).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove of no process = %v, want ErrGone", err)
		}
		p := newProcs(map[int]*proc{daemonPID: daemon(nil)})
		cfg := config(backendtest.New(), p, t.TempDir())
		cfg.Args = func(pid int) (proctable.Args, error) {
			p.table[pid].exited = true
			return p.args(pid)
		}
		if err := hostOf(t, cfg).Prove(daemonPID); !errors.Is(err, saverhost.ErrGone) {
			t.Errorf("Prove of a process gone before its arguments' read = %v, want ErrGone", err)
		}
	})

	t.Run("a mark is matched whole, never as a prefix", func(t *testing.T) {
		d := daemon(nil)
		d.env = []string{marks[0], marks[1] + "0"}
		p := newProcs(map[int]*proc{daemonPID: d})

		if err := newHost(t, backendtest.New(), p).Prove(daemonPID); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
			t.Errorf("Prove = %v, want ErrNotSaverDaemon for %s", err, d.env[1])
		}
	})
}

func TestSaverPID(t *testing.T) {
	for name, tc := range map[string]struct {
		b           *backendtest.Fake
		wantPID     int
		wantPresent bool
		wantErr     error
	}{
		"the saver session's process": {withSaver(daemonPID), daemonPID, true, nil},
		"no saver session":            {backendtest.New(), 0, false, nil},
		"a session naming no process": {withSaver(0), 0, false, saverhost.ErrNoProcess},
	} {
		t.Run(name, func(t *testing.T) {
			pid, present, err := newHost(t, tc.b, newProcs(nil)).SaverPID(t.Context())
			if pid != tc.wantPID || present != tc.wantPresent || !errors.Is(err, tc.wantErr) {
				t.Errorf("SaverPID = %d, %v, %v; want %d, %v, %v", pid, present, err, tc.wantPID, tc.wantPresent, tc.wantErr)
			}
		})
	}
}

func TestPresent(t *testing.T) {
	for name, tc := range map[string]struct {
		b    *backendtest.Fake
		want bool
	}{
		"a saver session, whatever it runs": {withSaver(0), true},
		"no saver session":                  {backendtest.New(), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := newHost(t, tc.b, newProcs(nil)).Present(t.Context()); got != tc.want || err != nil {
				t.Errorf("Present = %v, %v; want %v, nil", got, err, tc.want)
			}
		})
	}
}

func TestNewHost(t *testing.T) {
	signal := func(int, syscall.Signal) error { return nil }
	b, dir := backendtest.New(), "/tmp/state"
	for name, cfg := range map[string]saverhost.Config{
		"no backend":                    {StateDir: dir, Marks: marks, Signal: signal},
		"no state directory":            {Backend: b, Marks: marks, Signal: signal},
		"no signal":                     {Backend: b, StateDir: dir, Marks: marks},
		"no marks":                      {Backend: b, StateDir: dir, Signal: signal},
		"marks naming no saver session": {Backend: b, StateDir: dir, Marks: []string{marks[1]}, Signal: signal},
		"marks naming the saver session alone, which every zmx directory's carries": {Backend: b, StateDir: dir, Marks: []string{marks[0]}, Signal: signal},
		"an empty directory":        {Backend: b, StateDir: dir, Marks: []string{marks[0], "ZMX_DIR="}, Signal: signal},
		"a mark naming no variable": {Backend: b, StateDir: dir, Marks: []string{marks[0], "=x"}, Signal: signal},
		"a mark that is no entry":   {Backend: b, StateDir: dir, Marks: []string{marks[0], "ZMX_DIR"}, Signal: signal},
	} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if h, err := saverhost.NewHost(cfg); err == nil {
				t.Errorf("NewHost = %v, want a refusal", h)
			}
		})
	}
}
