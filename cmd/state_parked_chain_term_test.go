package cmd

import (
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
)

// signalPane sends sig to every process in the pane: the parked chain leads the
// pane's process group.
func (p termPane) signalPane(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(-p.parked, sig); err != nil {
		t.Fatalf("send %v to the pane's process group %d: %v", sig, p.parked, err)
	}
}

func (p termPane) assertParkedShellUp(t *testing.T) {
	t.Helper()
	select {
	case <-p.exited:
		t.Fatalf("the parked shell ended; %s", p.diagnostics())
	default:
	}
}

func TestParkedResumeChain_SIGTERM(t *testing.T) {
	t.Run("a waiting pane stays waiting through a SIGTERM to its parked shell", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.signal(t, p.parked, syscall.SIGTERM)

		p.assertStillWaiting(t, waiter)
		if !processAlive(p.parked) {
			t.Errorf("the parked shell (pid %d) ended; %s", p.parked, p.diagnostics())
		}
	})

	t.Run("a waiting pane stays waiting through a SIGTERM to every process in it", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.signalPane(t, syscall.SIGTERM)

		p.assertStillWaiting(t, waiter)
		if !processAlive(p.parked) {
			t.Errorf("the parked shell (pid %d) ended; %s", p.parked, p.diagnostics())
		}
	})

	t.Run("an answered pane keeps its session through a SIGTERM to every process while its hook runs", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)
		p.press(t, "\r")
		p.readPID(t, "hook.pid", "the hook program")

		p.signalPane(t, syscall.SIGTERM)

		shell := p.readPID(t, "shell.pid", "the user's shell")
		if shell != waiter {
			t.Fatalf("the user's shell runs as pid %d, want the pane's own process %d", shell, waiter)
		}
		time.Sleep(termPaneSettle)
		p.assertParkedShellUp(t)
		if !processAlive(shell) {
			t.Errorf("the user's shell (pid %d) ended; %s", shell, p.diagnostics())
		}
		if p.recorded(termEventRecovered) {
			t.Errorf("the recovery tail ran while the user's shell holds the pane; %s", p.diagnostics())
		}
	})

	t.Run("an answered pane whose parked shell caught a SIGTERM hands its hook and shell a default SIGTERM", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)
		p.signal(t, p.parked, syscall.SIGTERM)
		p.assertStillWaiting(t, waiter)

		p.press(t, "\r")

		hook := p.readPID(t, "hook.pid", "the hook program")
		assertEndsOnSIGTERM(t, hook, "the hook program")
		shell := p.readPID(t, "shell.pid", "the user's shell")
		if shell != waiter {
			t.Fatalf("the user's shell runs as pid %d, want the pane's own process %d", shell, waiter)
		}
		assertEndsOnSIGTERM(t, shell, "the user's shell")
	})

	t.Run("a waiting pane's parked shell still ends on SIGHUP", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		p.awaitEvent(t, termEventWait, 1)

		p.signal(t, p.parked, syscall.SIGHUP)

		if !harnesstest.PollUntil(t, termPaneWait, 10*time.Millisecond, func() bool {
			select {
			case <-p.exited:
				return true
			default:
				return false
			}
		}) {
			t.Fatalf("the parked shell survived SIGHUP; %s", p.diagnostics())
		}
	})
}
