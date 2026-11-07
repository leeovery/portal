package cmd

import (
	"syscall"
	"testing"
	"time"
)

func (p termPane) assertParkedShellUp(t *testing.T) {
	t.Helper()
	select {
	case <-p.exited:
		t.Fatalf("the parked shell ended; %s", p.diagnostics())
	default:
	}
}

func TestParkedResumeChain_SIGTERM(t *testing.T) {
	t.Run("a waiting session stays waiting through a SIGTERM to its parked shell", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.signal(t, p.parked, syscall.SIGTERM)

		p.assertStillWaiting(t, waiter)
		p.assertParkedShellUp(t)
	})

	t.Run("a waiting session stays waiting through a SIGTERM to every process in it", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.signalSession(t, syscall.SIGTERM)

		p.assertStillWaiting(t, waiter)
		p.assertParkedShellUp(t)
	})

	t.Run("an answered session keeps its shell through a SIGTERM to every process while its hook runs", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)
		p.press(t, "\r")
		p.readPID(t, "hook.pid", "the hook program")

		p.signalSession(t, syscall.SIGTERM)

		shell := p.readPID(t, "shell.pid", "the user's shell")
		if shell != waiter {
			t.Fatalf("the user's shell runs as pid %d, want the session's own process %d", shell, waiter)
		}
		gone := p.held.Watch(t, shell)
		time.Sleep(termPaneSettle)
		p.assertParkedShellUp(t)
		select {
		case <-gone:
			t.Errorf("the user's shell (pid %d) ended; %s", shell, p.diagnostics())
		default:
		}
		if p.recorded(termEventRecovered) {
			t.Errorf("the recovery tail ran while the user's shell holds the session; %s", p.diagnostics())
		}
	})

	t.Run("an answered session whose parked shell caught a SIGTERM hands its hook and shell a default SIGTERM", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)
		p.signal(t, p.parked, syscall.SIGTERM)
		p.assertStillWaiting(t, waiter)

		p.press(t, "\r")

		hook := p.readPID(t, "hook.pid", "the hook program")
		assertEndsOnSIGTERM(t, p.held, hook, "the hook program")
		shell := p.readPID(t, "shell.pid", "the user's shell")
		if shell != waiter {
			t.Fatalf("the user's shell runs as pid %d, want the session's own process %d", shell, waiter)
		}
		assertEndsOnSIGTERM(t, p.held, shell, "the user's shell")
	})

	t.Run("a waiting session's parked shell still ends on SIGHUP", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		p.awaitEvent(t, termEventWait, 1)

		p.signal(t, p.parked, syscall.SIGHUP)

		if !endsWithin(p.exited, termPaneWait) {
			t.Fatalf("the parked shell survived SIGHUP; %s", p.diagnostics())
		}
		if status := p.held.Status(t); !status.Signaled() || status.Signal() != syscall.SIGHUP {
			t.Errorf("the parked shell ended %v, want killed by SIGHUP", status)
		}
	})
}
