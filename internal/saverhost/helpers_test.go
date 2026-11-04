package saverhost_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// daemonPID is the saver's daemon in the fakes. No signal here reaches the
// kernel: every Host a test makes signals through procs.
const daemonPID = 4242

var marks = []string{"ZMX_SESSION=_portal-saver", "ZMX_DIR=/tmp/zx-saverhost"}

// daemonArgv is the argv the saver's daemon runs.
var daemonArgv = []string{"/opt/homebrew/bin/portal", "state", "daemon"}

// The bounds every Host a test makes waits by.
const (
	lockWait  = 5 * time.Second
	termWait  = 40 * time.Millisecond
	killWait  = 40 * time.Millisecond
	readyWait = 150 * time.Millisecond
	poll      = 2 * time.Millisecond
)

// proc is a process as the fakes read it, and as it answers a signal.
type proc struct {
	verdict     state.IdentifyResult
	identifyErr error
	argv, env   []string
	argsErr     error
	// exited is a process every read finds gone; it runs on through lingers
	// more reads of whether it is running, still exiting.
	exited  bool
	lingers int
	// zombie is a process that has finished exiting, unreaped: ps shows it
	// as no daemon, and its arguments can't be read.
	zombie bool
	// onSignal is what a signal does to it; nil leaves it running.
	onSignal func(p *proc, sig syscall.Signal)
}

// daemon is the saver's daemon: a portal state daemon carrying the marks.
func daemon(onSignal func(*proc, syscall.Signal)) *proc {
	return &proc{
		verdict:  state.IdentifyIsPortalDaemon,
		argv:     daemonArgv,
		env:      append([]string{"PATH=/usr/bin", "HOME=/nonexistent"}, marks...),
		onSignal: onSignal,
	}
}

// exiting is a process every read finds gone, still exiting for lingers
// reads of whether it is running.
func exiting(lingers int) *proc {
	return &proc{exited: true, lingers: lingers}
}

// exitsOn ends a process at any of sigs.
func exitsOn(sigs ...syscall.Signal) func(*proc, syscall.Signal) {
	return func(p *proc, sig syscall.Signal) {
		if slices.Contains(sigs, sig) {
			p.exited = true
		}
	}
}

// procs is the process table the fakes read and signal, recording each read
// and signal in order.
type procs struct {
	table  map[int]*proc
	events []string
}

func newProcs(table map[int]*proc) *procs { return &procs{table: table} }

func (p *procs) live(pid int) (*proc, bool) {
	pr, ok := p.table[pid]
	return pr, ok && !pr.exited && !pr.zombie
}

func (p *procs) identify(pid int) (state.IdentifyResult, error) {
	p.events = append(p.events, fmt.Sprintf("identify %d", pid))
	if pr, ok := p.table[pid]; ok && pr.zombie {
		return state.IdentifyNotPortalDaemon, nil
	}
	pr, ok := p.live(pid)
	if !ok {
		return state.IdentifyDead, nil
	}
	return pr.verdict, pr.identifyErr
}

func (p *procs) args(pid int) (proctable.Args, error) {
	p.events = append(p.events, fmt.Sprintf("args %d", pid))
	pr, ok := p.live(pid)
	if !ok {
		return proctable.Args{}, fmt.Errorf("read arguments of process %d: %w", pid, proctable.ErrGone)
	}
	return proctable.Args{Argv: pr.argv, Env: pr.env}, pr.argsErr
}

func (p *procs) running(pid int) bool {
	pr, ok := p.table[pid]
	switch {
	case !ok || pr.zombie:
		return false
	case pr.exited && pr.lingers > 0:
		pr.lingers--
		return true
	}
	return !pr.exited
}

func (p *procs) signal(pid int, sig syscall.Signal) error {
	p.events = append(p.events, fmt.Sprintf("%s %d", unix.SignalName(sig), pid))
	pr, ok := p.live(pid)
	if !ok {
		return syscall.ESRCH
	}
	if pr.onSignal != nil {
		pr.onSignal(pr, sig)
	}
	return nil
}

// signals is every signal sent, in order.
func (p *procs) signals() []string {
	var sent []string
	for _, e := range p.events {
		if strings.HasPrefix(e, "SIG") {
			sent = append(sent, e)
		}
	}
	return sent
}

// config is a Config over b and the state directory dir that reads and
// signals p alone, at the tests' bounds.
func config(b backend.Backend, p *procs, dir string) saverhost.Config {
	return saverhost.Config{
		Backend:   b,
		StateDir:  dir,
		Marks:     marks,
		Signal:    p.signal,
		Identify:  p.identify,
		Args:      p.args,
		Running:   p.running,
		LockWait:  lockWait,
		TermWait:  termWait,
		KillWait:  killWait,
		ReadyWait: readyWait,
		Poll:      poll,
	}
}

// newHost is a Host over b and a state directory of its own that reads and
// signals p alone.
func newHost(t *testing.T, b backend.Backend, p *procs) *saverhost.Host {
	t.Helper()
	return hostOf(t, config(b, p, t.TempDir()))
}

// hostOf is the Host cfg binds, failing the test on a refusal.
func hostOf(t *testing.T, cfg saverhost.Config) *saverhost.Host {
	t.Helper()
	h, err := saverhost.NewHost(cfg)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	return h
}

// withSaver is a Fake holding the saver session, running pid.
func withSaver(pid int) *backendtest.Fake {
	return backendtest.New(backend.Session{Name: saver.SessionName, PID: pid})
}

// kills is the names b was asked to kill, in order.
func kills(b *backendtest.Fake) []string {
	var names []string
	for _, c := range b.CallsTo(backendtest.OpKill) {
		names = append(names, c.Name)
	}
	return names
}

// stillWaiting fails the test if done delivers within a moment, as a run
// waiting on saver.lock would not, putting back what it delivered.
func stillWaiting(t *testing.T, done chan error, what string) {
	t.Helper()
	select {
	case err := <-done:
		t.Errorf("%s = %v while saver.lock was held, want it waiting", what, err)
		done <- err
	case <-time.After(100 * time.Millisecond):
	}
}

// openFDs is how many descriptors this process has open.
func openFDs(t *testing.T) int {
	t.Helper()
	fds, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(fds)
}

// holdLock takes saver.lock in dir through an open file description of its
// own, as another process would, until the returned release or the test's
// end.
func holdLock(t *testing.T, dir string) (release func()) {
	t.Helper()
	f, err := os.OpenFile(saverhost.LockPath(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock saver.lock: %v", err)
	}
	return func() { _ = f.Close() }
}
