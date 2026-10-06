package cmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
)

// handOffWindow is one moment a waiting pane's process has started but not yet
// installed its SIGTERM catch.
type handOffWindow struct {
	name string
	hold string

	// reach brings the pane to the held process, answering its pid.
	reach func(t *testing.T, p termPane) int
}

func handOffWindows() []handOffWindow {
	return []handOffWindow{
		{
			name: "as its parked shell starts the draw",
			hold: termEventDraw + " 1",
			reach: func(t *testing.T, p termPane) int {
				return p.awaitEvent(t, termEventPreCatch+termEventDraw, 1)
			},
		},
		{
			name: "as its draw execs into the waiter",
			hold: termEventWait + " 1",
			reach: func(t *testing.T, p termPane) int {
				held := p.awaitEvent(t, termEventPreCatch+termEventWait, 1)
				if draw := p.awaitEvent(t, termEventDraw, 1); draw != held {
					t.Fatalf("the waiter runs as pid %d, want the draw's own process %d", held, draw)
				}
				return held
			},
		},
		{
			name: "as its waiter execs back into the draw after a resize",
			hold: termEventDraw + " 2",
			reach: func(t *testing.T, p termPane) int {
				waiter := p.awaitEvent(t, termEventWait, 1)
				p.resize(t, waiter, 120, 40)
				held := p.awaitEvent(t, termEventPreCatch+termEventDraw, 2)
				if held != waiter {
					t.Fatalf("the redraw runs as pid %d, want the waiter's own process %d", held, waiter)
				}
				return held
			},
		},
	}
}

// awaitEnded waits for a held process the SIGTERM ended.
func (p termPane) awaitEnded(t *testing.T, pid int, what string) {
	t.Helper()
	if !harnesstest.PollUntil(t, termPaneWait, 10*time.Millisecond, func() bool { return !processAlive(pid) }) {
		t.Fatalf("%s (pid %d) outlived a SIGTERM before its catch; the harness did not hold it in the window; %s", what, pid, p.diagnostics())
	}
}

func TestResumeWaitingPane_SIGTERMBeforeTheCatch(t *testing.T) {
	for _, w := range handOffWindows() {
		t.Run("a SIGTERM landing "+w.name+" leaves the pane waiting", func(t *testing.T) {
			p := startTermPane(t, termPaneOpts{holdCatch: w.hold})
			held := w.reach(t, p)
			waits := len(p.pidsOf(termEventWait))

			p.signal(t, held, syscall.SIGTERM)
			p.awaitEnded(t, held, "the held process")

			waiter := p.awaitEvent(t, termEventWait, waits+1)
			p.assertStillWaiting(t, waiter)
			p.assertParkedShellUp(t)
		})

		t.Run("a pane answered after a SIGTERM landed "+w.name+" hands its hook and shell a default SIGTERM", func(t *testing.T) {
			p := startTermPane(t, termPaneOpts{holdCatch: w.hold})
			held := w.reach(t, p)
			waits := len(p.pidsOf(termEventWait))
			p.signal(t, held, syscall.SIGTERM)
			p.awaitEnded(t, held, "the held process")
			waiter := p.awaitEvent(t, termEventWait, waits+1)

			p.press(t, "\r")

			hook := p.readPID(t, "hook.pid", "the hook program")
			assertEndsOnSIGTERM(t, hook, "the hook program")
			shell := p.readPID(t, "shell.pid", "the user's shell")
			if shell != waiter {
				t.Fatalf("the user's shell runs as pid %d, want the pane's own process %d", shell, waiter)
			}
			assertEndsOnSIGTERM(t, shell, "the user's shell")
		})
	}

	t.Run("a SIGTERM ending the shell of an answered pane does not put it back on its panel", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)
		p.press(t, "\r")
		p.readPID(t, "hook.pid", "the hook program")
		if err := os.WriteFile(filepath.Join(p.dir, "release"), nil, 0o600); err != nil {
			t.Fatalf("release the hook program: %v", err)
		}
		shell := p.readPID(t, "shell.pid", "the user's shell")
		if shell != waiter {
			t.Fatalf("the user's shell runs as pid %d, want the pane's own process %d", shell, waiter)
		}

		p.signal(t, shell, syscall.SIGTERM)

		select {
		case <-p.exited:
		case <-time.After(termPaneWait):
			t.Fatalf("the parked chain never reached its end; %s", p.diagnostics())
		}
		if draws := p.pidsOf(termEventDraw); len(draws) != 1 {
			t.Errorf("the pane was drawn %d times, want once: an answered pane went back on its panel; %s", len(draws), p.diagnostics())
		}
	})
}
