package proctest

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// waitDelay bounds the reap at cleanup once the held process has exited, for
// output pipes a process it left behind still holds open.
const waitDelay = 5 * time.Second

// pollEvery is how often a watch looks up from the kernel to see whether it
// has been stopped.
const pollEvery = 100 * time.Millisecond

// proveFor bounds how long a process that walks to nothing is read again.
const proveFor = 500 * time.Millisecond

// Held is a process a test started and holds unreaped until the test ends,
// so its pid, and the id of the process group it leads, can't pass to
// another process while the test signals through it. Its exit is watched
// through the kernel, which reaps nothing.
type Held struct {
	cmd   *exec.Cmd
	leads bool
	proof Proof
	exit  *exitWatch

	mu       sync.Mutex
	released bool
}

// Start starts cmd and holds it until the test's cleanup, which sends SIGKILL
// to it, or to every process in the group it leads, and then reaps it. cmd
// leads a group when it starts with Setsid, or with Setpgid and no Pgid.
// marker, when not empty, is an environment entry, KEY=value, unique to the
// test, which binaries of the test's own carry below cmd: one carrying it is
// proved by it, whichever parent it now has.
func Start(t testing.TB, cmd *exec.Cmd, marker string) *Held {
	t.Helper()
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = waitDelay
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("proctest: start %s: %v", cmd.Path, err)
	}
	pid := cmd.Process.Pid
	exit, err := holdExit(pid, watchFailure(t, pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("proctest: hold %s (pid %d): %v", cmd.Path, pid, err)
	}
	attr := cmd.SysProcAttr
	h := &Held{
		cmd:   cmd,
		leads: attr != nil && (attr.Setsid || attr.Setpgid && attr.Pgid == 0),
		proof: PIDProof(pid),
		exit:  exit,
	}
	if marker != "" {
		h.proof = AnyProof(h.proof, EnvProof(marker))
	}
	t.Cleanup(func() { h.release(t) })
	return h
}

// PID is the held process's pid.
func (h *Held) PID() int { return h.cmd.Process.Pid }

// Exited closes once the held process has exited, reaped or not, or once the
// watch on it has failed, which fails the test.
func (h *Held) Exited() <-chan struct{} { return h.exit.done }

// Status is how the held process ended, once Exited has closed. A watch that
// failed before the exit was reported has no status to give, and stops t.
func (h *Held) Status(t testing.TB) syscall.WaitStatus {
	t.Helper()
	<-h.exit.done
	if h.exit.err != nil {
		t.Fatalf("proctest: pid %d: %v", h.PID(), h.exit.err)
	}
	return h.exit.status
}

// Signal sends sig to pid once pid is proved the held process, a process its
// parent chain reaches the held process from, or one carrying the held
// process's marker, proved as the signal is sent, and fails, sending nothing,
// for any other.
func (h *Held) Signal(pid int, sig syscall.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.released {
		return fmt.Errorf("proctest: pid %d is not signalled: the process proving it has been reaped", pid)
	}
	if !h.proves(pid) {
		return fmt.Errorf("proctest: pid %d is no process of the test's, so it is not signalled", pid)
	}
	return syscall.Kill(pid, sig)
}

// SignalGroup sends sig to every process in the group the held process
// leads. Its leader unreaped, no other group can take the id.
func (h *Held) SignalGroup(sig syscall.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.leads {
		return errors.New("proctest: the held process leads no process group, so none is signalled")
	}
	if h.released {
		return errors.New("proctest: the held process has been reaped, so its group is not signalled")
	}
	return syscall.Kill(-h.PID(), sig)
}

// Watch proves pid the held process's own, as Signal does, and returns a
// channel that closes once pid exits, or once the watch has failed, which fails
// the test. A pid already gone closes it at once. It proves pid after the watch
// is in place, so the process it watches is the one it proved, and stops
// watching at the test's cleanup.
func (h *Held) Watch(t testing.TB, pid int) <-chan struct{} {
	t.Helper()
	exit, err := watchExit(pid, watchFailure(t, pid))
	if errors.Is(err, syscall.ESRCH) {
		gone := make(chan struct{})
		close(gone)
		return gone
	}
	if err != nil {
		t.Fatalf("proctest: watch pid %d: %v", pid, err)
	}
	t.Cleanup(exit.stop)
	if !h.proves(pid) {
		t.Fatalf("proctest: pid %d is no process of the test's, so it is not watched", pid)
	}
	return exit.done
}

// proves reports whether pid is the held process or proved its own. The held
// process needs no reading: unreaped, its pid can't pass to another. Any
// other is read again for a while after a walk that proved nothing, since the
// kernel refuses to read a process's args while it execs; each walk is whole
// and fresh, so a retry proves nothing a single walk wouldn't.
func (h *Held) proves(pid int) bool {
	if pid == h.PID() {
		return true
	}
	deadline := time.Now().Add(proveFor)
	for !Proved(pid, h.proof, Kernel{}) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

// release kills what the test left running, the group while its leader is
// still unreaped, and then reaps the held process and ends its watch, so no
// watch reports into a test that has ended.
func (h *Held) release(t testing.TB) {
	h.mu.Lock()
	defer h.mu.Unlock()
	target := h.PID()
	if h.leads {
		target = -target
	}
	_ = syscall.Kill(target, syscall.SIGKILL)
	_ = h.cmd.Wait()
	h.exit.stop()
	h.released = true
	if h.leads && !groupGoneWithin(h.PID(), waitDelay) {
		t.Errorf("proctest: process group %d outlived its SIGKILL", h.PID())
	}
}

// groupGoneWithin waits for every process in group pgid to exit, so none is
// still writing when the test's temp dirs are removed. A group's id can't
// pass to another process while any process is in it, so asking after its
// leader is reaped asks of the same group, and a signal 0 sends nothing.
func groupGoneWithin(pgid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for syscall.Kill(-pgid, 0) != syscall.ESRCH {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

// exitWatch is one process's exit as the kernel reports it. done closes once
// the exit is reported, with status, or once the watch has failed, with err
// and after fail has reported it; a stopped watch never closes it. finished
// closes however the watch ends.
type exitWatch struct {
	done   chan struct{}
	status syscall.WaitStatus
	err    error

	fail     func(error)
	finished chan struct{}
	stopOnce sync.Once
	stopped  chan struct{}
}

func newExitWatch(fail func(error)) *exitWatch {
	return &exitWatch{
		done:     make(chan struct{}),
		fail:     fail,
		finished: make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// watchFailure fails t from the goroutine a watch waits in, which only Errorf
// is safe from. The watch's own stop, at the test's cleanup, waits for that
// goroutine, so it never reports into a test that has ended.
func watchFailure(t testing.TB, pid int) func(error) {
	return func(err error) { t.Errorf("proctest: watch pid %d: %v", pid, err) }
}

// stop ends the watch and waits until it has.
func (w *exitWatch) stop() {
	w.stopOnce.Do(func() { close(w.stopped) })
	<-w.finished
}

func (w *exitWatch) exited(status syscall.WaitStatus) {
	w.status = status
	close(w.done)
}

func (w *exitWatch) failed(err error) {
	w.err = err
	w.fail(err)
	close(w.done)
}

// szomb is xnu's SZOMB, a process's state once it has exited and before it
// is reaped.
const szomb = 5

// zombieFor bounds how long a child kevent finds gone is read again before
// the process table shows it a zombie.
const zombieFor = time.Second

// holdExit watches pid, a child of the test's not yet reaped, for its exit. A
// child already exited is one the kernel won't watch, and its status is read
// from the process table, which reaps nothing either.
func holdExit(pid int, fail func(error)) (*exitWatch, error) {
	w, err := watchExit(pid, fail)
	if !errors.Is(err, syscall.ESRCH) {
		return w, err
	}
	status, err := zombieStatus(pid)
	if err != nil {
		return nil, err
	}
	w = newExitWatch(fail)
	w.exited(status)
	close(w.finished)
	return w, nil
}

// zombieStatus reads how pid, a child kevent finds gone, ended. kevent finds
// a child gone as it starts to exit, while the process table still shows it
// running, so it is read again until it is a zombie; unreaped, its pid can't
// pass to another process meanwhile.
func zombieStatus(pid int) (syscall.WaitStatus, error) {
	deadline := time.Now().Add(zombieFor)
	for {
		p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil {
			return 0, fmt.Errorf("read exited pid %d: %w", pid, err)
		}
		if p.Proc.P_stat == szomb {
			return syscall.WaitStatus(p.Proc.P_xstat), nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("pid %d can't be watched and is no zombie after %s (state %d)", pid, zombieFor, p.Proc.P_stat)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// watchExit asks the kernel to report pid's exit and its status, with
// EVFILT_PROC's NOTE_EXIT, which reaps nothing, reporting a failed watch to
// fail. A process already exited, or exiting, is ESRCH.
func watchExit(pid int, fail func(error)) (*exitWatch, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("kqueue: %w", err)
	}
	change := unix.Kevent_t{
		Ident:  uint64(pid),
		Filter: unix.EVFILT_PROC,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT | unix.EV_RECEIPT,
		Fflags: unix.NOTE_EXIT | unix.NOTE_EXITSTATUS,
	}
	receipt := make([]unix.Kevent_t, 1)
	if _, err := unix.Kevent(kq, []unix.Kevent_t{change}, receipt, nil); err != nil {
		_ = unix.Close(kq)
		return nil, fmt.Errorf("watch pid %d: %w", pid, err)
	}
	if errno := syscall.Errno(receipt[0].Data); errno != 0 {
		_ = unix.Close(kq)
		return nil, fmt.Errorf("watch pid %d: %w", pid, errno)
	}
	w := newExitWatch(fail)
	go w.await(kq, unix.Kevent)
	return w, nil
}

// keventFunc is unix.Kevent's shape, which await waits through.
type keventFunc func(kq int, changes, events []unix.Kevent_t, timeout *unix.Timespec) (int, error)

// await waits for the exit through kevent, looking up between waits for a
// stop.
func (w *exitWatch) await(kq int, kevent keventFunc) {
	defer close(w.finished)
	defer func() { _ = unix.Close(kq) }()
	events := make([]unix.Kevent_t, 1)
	timeout := unix.NsecToTimespec(pollEvery.Nanoseconds())
	for {
		select {
		case <-w.stopped:
			return
		default:
		}
		n, err := kevent(kq, nil, events, &timeout)
		if err != nil && !errors.Is(err, syscall.EINTR) {
			w.failed(fmt.Errorf("wait for the exit: %w", err))
			return
		}
		if n == 1 {
			w.exited(syscall.WaitStatus(events[0].Data))
			return
		}
	}
}
