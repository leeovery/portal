package saverhost_test

import (
	"fmt"
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
// kernel: every Stopper a test makes signals through procs.
const daemonPID = 4242

var marks = []string{"ZMX_SESSION=_portal-saver", "ZMX_DIR=/tmp/zx-saverhost"}

// daemonArgv is the argv the saver's daemon runs.
var daemonArgv = []string{"/opt/homebrew/bin/portal", "state", "daemon"}

// The bounds every Stopper a test makes waits by.
const (
	termWait = 40 * time.Millisecond
	killWait = 40 * time.Millisecond
	poll     = 2 * time.Millisecond
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
	return pr, ok && !pr.exited
}

func (p *procs) identify(pid int) (state.IdentifyResult, error) {
	p.events = append(p.events, fmt.Sprintf("identify %d", pid))
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
	case !ok:
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

// config is a Config over b that reads and signals p alone, at the tests'
// bounds.
func config(b backend.Backend, p *procs) saverhost.Config {
	return saverhost.Config{
		Backend:  b,
		Marks:    marks,
		Signal:   p.signal,
		Identify: p.identify,
		Args:     p.args,
		Running:  p.running,
		TermWait: termWait,
		KillWait: killWait,
		Poll:     poll,
	}
}

// newStopper is a Stopper over b that reads and signals p alone.
func newStopper(t *testing.T, b backend.Backend, p *procs) *saverhost.Stopper {
	t.Helper()
	return stopperOf(t, config(b, p))
}

// stopperOf is the Stopper cfg binds, failing the test on a refusal.
func stopperOf(t *testing.T, cfg saverhost.Config) *saverhost.Stopper {
	t.Helper()
	s, err := saverhost.NewStopper(cfg)
	if err != nil {
		t.Fatalf("NewStopper: %v", err)
	}
	return s
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
