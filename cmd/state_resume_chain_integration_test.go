//go:build integration

package cmd_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

const (
	waitingSession = "proj-a1b2"
	// hookMark is the file the session's resume command makes, in its
	// directory, and the word the panel shows the command by.
	hookMark = "hook-ran"
)

// waitingWorld is a world holding one session parked on its resume panel, as
// restore leaves a lazy one: its flag raised and its resume command
// registered.
type waitingWorld struct {
	*portalWorld
	work string
}

func newWaitingWorld(t *testing.T, binary string) *waitingWorld {
	t.Helper()
	w := newPortalWorld(t, binary)
	_, hooksPath := hookstest.StageStore(t, hookstest.Staging{
		SidecarAbsent: true,
		Entries:       map[string]string{waitingSession: "touch " + hookMark},
	})
	w.env = append(w.env, "PORTAL_HOOKS_FILE="+hooksPath)

	if err := state.ResumePending.Set(w.stateDir, waitingSession); err != nil {
		t.Fatalf("raise the session's flag: %v", err)
	}
	flag, err := state.ResumePending.Path(w.stateDir, waitingSession)
	if err != nil {
		t.Fatal(err)
	}
	work := realPath(t, t.TempDir())
	chain := cmd.ParkedResumeChain(binary, flag, "touch "+hookMark, waitingSession)
	// A session passes on only a few of the test's variables, so the chain
	// gets the world's whole environment, isolated, from env -i.
	command := append([]string{"/usr/bin/env", "-i"}, w.env...)
	command = append(command, "ZMX_SESSION="+waitingSession, "/bin/sh", "-c", chain)
	if _, err := w.b.Create(t.Context(), waitingSession, work, command); err != nil {
		t.Fatalf("create the waiting session: %v", err)
	}
	return &waitingWorld{portalWorld: w, work: work}
}

func (w *waitingWorld) flagRaised(t *testing.T) bool {
	t.Helper()
	raised, err := state.ResumePending.Exists(w.stateDir, waitingSession)
	if err != nil {
		t.Fatalf("look for the session's flag: %v", err)
	}
	return raised
}

// awaitPanel waits for the session's screen to show its resume panel.
func (w *waitingWorld) awaitPanel(t *testing.T) {
	t.Helper()
	w.awaitScreen(t, "its panel", func(screen string) bool { return strings.Contains(screen, hookMark) })
}

// awaitWaiterAt waits for a waiter on the session to start at cols by rows,
// as the log's process start records it: a key typed while the redraw a
// resize asks for is pending is dropped with the panel it was typed to.
func (w *waitingWorld) awaitWaiterAt(t *testing.T, cols, rows int) {
	t.Helper()
	size := "--width " + strconv.Itoa(cols) + " --height " + strconv.Itoa(rows)
	var logged string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		raw, _ := os.ReadFile(filepath.Join(w.stateDir, "portal.log"))
		logged = string(raw)
		for line := range strings.Lines(logged) {
			if strings.Contains(line, "process: start") && strings.Contains(line, "resume-wait") && strings.Contains(line, size) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("no waiter started at %s; the log holds:\n%s", size, logged)
	}
}

func (w *waitingWorld) awaitScreen(t *testing.T, what string, shows func(screen string) bool) {
	t.Helper()
	var screen string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		history, err := w.b.History(t.Context(), waitingSession, 0)
		screen = ansi.Strip(string(history))
		return err == nil && shows(screen)
	}) {
		t.Fatalf("the session never showed %s; it shows:\n%s", what, screen)
	}
}

func (w *waitingWorld) hookRan() bool {
	_, err := os.Stat(filepath.Join(w.work, hookMark))
	return err == nil
}

func TestResumeChainAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)

	t.Run("Escape on the panel of a session a runner shows sends that terminal to the picker", func(t *testing.T) {
		w := newWaitingWorld(t, binary)
		w.awaitPanel(t)
		p := w.portal(t, "open", waitingSession)
		w.awaitClients(t, waitingSession, 1)
		w.awaitWaiterAt(t, portalTermCols, portalTermRows)

		mark := p.mark()
		p.typeIn(t, "\x1b")

		p.awaitAfter(t, mark, pickerMark)
		w.awaitClients(t, waitingSession, 0)
		if !w.alive(t, waitingSession) {
			t.Error("the session ended on Escape")
		}
		if !w.flagRaised(t) {
			t.Error("Escape lowered the session's flag; it still waits")
		}
		if w.hookRan() {
			t.Error("Escape ran the resume command")
		}
	})

	t.Run("Escape on the panel of a session only a raw client shows detaches it through the backend", func(t *testing.T) {
		w := newWaitingWorld(t, binary)
		w.awaitPanel(t)
		c := zmxtest.Attach(t, w.b, waitingSession)
		w.awaitClients(t, waitingSession, 1)

		c.Type(t, "\x1b")

		if !c.Exited(attachSettle) {
			t.Fatal("the raw client was never detached")
		}
		if !w.alive(t, waitingSession) {
			t.Error("the session ended on Escape")
		}
		if !w.flagRaised(t) {
			t.Error("Escape lowered the session's flag; it still waits")
		}
	})

	t.Run("Enter on the panel runs the resume command and lowers the session's flag", func(t *testing.T) {
		w := newWaitingWorld(t, binary)
		w.awaitPanel(t)
		c := zmxtest.Attach(t, w.b, waitingSession)
		w.awaitClients(t, waitingSession, 1)

		c.Type(t, "\r")

		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, w.hookRan) {
			t.Fatal("the resume command never ran")
		}
		if w.flagRaised(t) {
			t.Error("the session's flag is still raised after Enter")
		}
		if !w.alive(t, waitingSession) {
			t.Error("the session ended; Enter hands it its shell")
		}
	})
}
