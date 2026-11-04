package zmx_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
)

// endingWorld is a session, work, whose socket in dir refuses, over a process
// table the test sets, and the signals EndDaemon sends.
type endingWorld struct {
	dir string
	b   *zmx.Backend

	mu    sync.Mutex
	procs []zmx.Process
	sent  []syscall.Signal
}

func newEndingWorld(t *testing.T) *endingWorld {
	t.Helper()
	w := &endingWorld{dir: socketDir(t)}
	refusing(t, w.dir, "work")
	w.b = newBackend(t, w.dir)
	zmx.FakeProcessesFrom(w.b, func() ([]zmx.Process, error) {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.procs, nil
	})
	return w
}

// table sets the processes the next listing shows.
func (w *endingWorld) table(procs ...zmx.Process) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.procs = procs
}

func (w *endingWorld) listed() []zmx.Process {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.procs
}

// unlink removes work's socket, as its daemon does on SIGTERM.
func (w *endingWorld) unlink(t *testing.T) {
	t.Helper()
	if err := os.Remove(filepath.Join(w.dir, "work")); err != nil {
		t.Error(err)
	}
}

// signalled records each signal, then does what the daemon does with it.
func (w *endingWorld) signalled(then func()) func(pid int, sig syscall.Signal) error {
	return func(pid int, sig syscall.Signal) error {
		w.mu.Lock()
		w.sent = append(w.sent, sig)
		w.mu.Unlock()
		if pid != 100 {
			return errors.New("signalled a pid the table never proved")
		}
		then()
		return nil
	}
}

func (w *endingWorld) signals() []syscall.Signal {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sent
}

func TestEndDaemon_SignalsTheProvedDaemonThenWaitsForItsSocketAndItToGo(t *testing.T) {
	w := newEndingWorld(t)
	w.table(daemonOf(100, w.dir, "work"))
	end := w.signalled(func() {
		time.AfterFunc(40*time.Millisecond, func() {
			w.unlink(t)
			time.AfterFunc(40*time.Millisecond, func() { w.table() })
		})
	})

	if err := w.b.EndDaemon(t.Context(), "work", end); err != nil {
		t.Fatalf("EndDaemon(work) = %v", err)
	}
	if got := w.listed(); len(got) != 0 {
		t.Errorf("EndDaemon returned while the table still showed %v: before the daemon exited", got)
	}
	if got := w.signals(); len(got) != 1 || got[0] != syscall.SIGTERM {
		t.Errorf("signals sent = %v, want one SIGTERM", got)
	}
}

func TestEndDaemon_EndsADaemonWhosePidIsReusedOnceItExits(t *testing.T) {
	w := newEndingWorld(t)
	w.table(daemonOf(100, w.dir, "work"))
	reused := daemonOf(100, w.dir, "work")
	reused.Args.Argv = []string{"sleep", "60"}
	end := w.signalled(func() {
		w.unlink(t)
		w.table(reused)
	})

	if err := w.b.EndDaemon(t.Context(), "work", end); err != nil {
		t.Errorf("EndDaemon(work) = %v, want nil: pid 100 runs another program now", err)
	}
}

func TestEndDaemon_SignalsNothingWithoutTheProof(t *testing.T) {
	probe := socketDir(t)
	inDir := func(dir string) string { return "ZMX_DIR=" + dir }
	daemon := func(pid int, mutate func(*zmx.Process)) func(dir string) zmx.Process {
		return func(dir string) zmx.Process {
			p := daemonOf(pid, dir, "work")
			mutate(&p)
			return p
		}
	}
	same := func(*zmx.Process) {}

	for _, tt := range []struct {
		name  string
		procs []func(dir string) zmx.Process
	}{
		{"no process", nil},
		{"two daemons", []func(string) zmx.Process{daemon(100, same), daemon(101, same)}},
		{"a child of another process", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.PPID = 50 })}},
		{"a process on a terminal", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Terminal = true })}},
		{"a daemon in another directory", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Args.Env = []string{inDir(probe)} })}},
		{"a daemon in a relative directory", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Args.Env = []string{"ZMX_DIR=zmx"} })}},
		{"a daemon whose environment the kernel doesn't show", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Args.Env = nil })}},
		{"the daemon of another session", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Args.Argv = []string{"zmx", "attach", "works"} })}},
		{"the daemon of its name under a prefix", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) {
			p.Args.Env = append(p.Args.Env, "ZMX_SESSION_PREFIX=p-")
		})}},
		{"a zmx command that starts no daemon", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Args.Argv = []string{"zmx", "kill", "work"} })}},
		{"a process it can't read", []func(string) zmx.Process{daemon(100, func(p *zmx.Process) { p.Err = errors.New("operation not permitted") })}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newEndingWorld(t)
			procs := make([]zmx.Process, len(tt.procs))
			for i, p := range tt.procs {
				procs[i] = p(w.dir)
			}
			w.table(procs...)

			if err := w.b.EndDaemon(t.Context(), "work", w.signalled(func() {})); err == nil {
				t.Error("EndDaemon(work) = nil, want the proof's failure")
			}
			if got := w.signals(); len(got) != 0 {
				t.Errorf("signals sent = %v, want none", got)
			}
		})
	}
}

func TestEndDaemon_SignalsNothingWhenTheTableCantBeListed(t *testing.T) {
	w := newEndingWorld(t)
	zmx.FakeProcesses(w.b, nil, proctable.ErrUnsupported)

	err := w.b.EndDaemon(t.Context(), "work", w.signalled(func() {}))
	if !errors.Is(err, proctable.ErrUnsupported) {
		t.Errorf("EndDaemon(work) = %v, want ErrUnsupported", err)
	}
	if got := w.signals(); len(got) != 0 {
		t.Errorf("signals sent = %v, want none", got)
	}
}

func TestEndDaemon_ProvesTheDaemonAgainJustBeforeTheSignal(t *testing.T) {
	for _, tt := range []struct {
		name  string
		again func(dir string) []zmx.Process
	}{
		{"its pid reused by another daemon of the session", func(dir string) []zmx.Process { return []zmx.Process{daemonOf(200, dir, "work")} }},
		{"gone", func(string) []zmx.Process { return nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newEndingWorld(t)
			lists := [][]zmx.Process{{daemonOf(100, w.dir, "work")}, tt.again(w.dir)}
			zmx.FakeProcessesFrom(w.b, func() ([]zmx.Process, error) {
				next := lists[0]
				if len(lists) > 1 {
					lists = lists[1:]
				}
				return next, nil
			})

			if err := w.b.EndDaemon(t.Context(), "work", w.signalled(func() {})); err == nil {
				t.Error("EndDaemon(work) = nil, want the second proof's failure")
			}
			if got := w.signals(); len(got) != 0 {
				t.Errorf("signals sent = %v, want none", got)
			}
		})
	}
}

func TestEndDaemon_FailsWhenTheDaemonOutlivesTheSignal(t *testing.T) {
	for _, tt := range []struct {
		name string
		then func(w *endingWorld, t *testing.T)
	}{
		{"its socket and it both stay", func(*endingWorld, *testing.T) {}},
		{"its socket goes and it stays", func(w *endingWorld, t *testing.T) { w.unlink(t) }},
		{"it goes and its socket stays", func(w *endingWorld, _ *testing.T) { w.table() }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newEndingWorld(t)
			w.table(daemonOf(100, w.dir, "work"))

			err := w.b.EndDaemon(t.Context(), "work", w.signalled(func() { tt.then(w, t) }))
			if !errors.Is(err, backend.ErrTimeout) {
				t.Errorf("EndDaemon(work) = %v, want ErrTimeout past the kill ceiling", err)
			}
		})
	}
}

func TestEndDaemon_FailsWhenTheSignalFails(t *testing.T) {
	w := newEndingWorld(t)
	w.table(daemonOf(100, w.dir, "work"))

	err := w.b.EndDaemon(t.Context(), "work", func(int, syscall.Signal) error { return syscall.EPERM })
	if !errors.Is(err, syscall.EPERM) {
		t.Errorf("EndDaemon(work) = %v, want the signal's failure", err)
	}
}

func TestEndDaemon_EndsASessionWhoseSocketHasGone(t *testing.T) {
	b := newBackend(t, socketDir(t))
	zmx.FakeProcessesFrom(b, func() ([]zmx.Process, error) {
		t.Error("the process table was read for a session with no socket")
		return nil, nil
	})

	if err := b.EndDaemon(t.Context(), "work", func(int, syscall.Signal) error {
		t.Error("signalled for a session with no socket")
		return nil
	}); err != nil {
		t.Errorf("EndDaemon(work) = %v, want nil: nothing is left to end", err)
	}
}

func TestEndDaemon_RefusesAnInvalidName(t *testing.T) {
	w := newEndingWorld(t)
	w.table(daemonOf(100, w.dir, "work"))

	if err := w.b.EndDaemon(t.Context(), "../work", w.signalled(func() {})); err == nil {
		t.Error("EndDaemon(../work) = nil, want the name refused")
	}
	if got := w.signals(); len(got) != 0 {
		t.Errorf("signals sent = %v, want none", got)
	}
}
