//go:build darwin

package cmd

import (
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
)

// drainPTY reads whatever the pane writes, as tmux does, so no write to the
// terminal blocks on a full output queue. The reads poll a non-blocking
// descriptor: a blocking read in flight would hold the master open past its
// Close, and the terminal would never hang up.
func drainPTY(t *testing.T, fd int) (stop func()) {
	t.Helper()
	if err := syscall.SetNonblock(fd, true); err != nil {
		t.Fatalf("make the pty master non-blocking: %v", err)
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
			}
			if n, err := syscall.Read(fd, buf); n <= 0 || err != nil {
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
	t.Cleanup(stop)
	return stop
}

func TestParkedResumeChain_PaneTerminalHangup(t *testing.T) {
	t.Run("a waiting pane whose terminal closes ends on SIGHUP before its recovery tail starts", func(t *testing.T) {
		master, slave := openPTY(t)
		stopDrain := drainPTY(t, int(master.Fd()))
		p := startTermPane(t, termPaneOpts{tty: slave})
		_ = slave.Close()
		waiter := p.awaitEvent(t, termEventWait, 1)

		stopDrain()
		if err := master.Close(); err != nil {
			t.Fatalf("close the pane's terminal: %v", err)
		}

		select {
		case <-p.exited:
		case <-time.After(termPaneWait):
			t.Fatalf("the parked chain outlived its terminal; %s", p.diagnostics())
		}
		ws, ok := p.ended.state.Sys().(syscall.WaitStatus)
		if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGHUP {
			t.Errorf("the parked chain ended %v, want killed by SIGHUP; %s", p.ended.state, p.diagnostics())
		}
		if !harnesstest.PollUntil(t, termPaneWait, 10*time.Millisecond, func() bool { return !processAlive(waiter) }) {
			t.Errorf("the waiter (pid %d) outlived its terminal; %s", waiter, p.diagnostics())
		}
		if p.recorded(termEventRecovered) {
			t.Errorf("the recovery tail ran; %s", p.diagnostics())
		}
		if p.recorded(termEventCleared) {
			t.Errorf("a pending-marker clear was attempted; %s", p.diagnostics())
		}
	})
}
