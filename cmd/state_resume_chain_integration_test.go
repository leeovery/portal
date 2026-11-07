//go:build integration

package cmd_test

import (
	"os"
	"path/filepath"
	"slices"
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

// awaitDrawIn waits for the log, past the first mark bytes of it, to show a
// draw of the session's panel that loaded the theme slug.
func (w *waitingWorld) awaitDrawIn(t *testing.T, mark int, slug string) {
	t.Helper()
	var logged string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		raw, _ := os.ReadFile(filepath.Join(w.stateDir, "portal.log"))
		logged = string(raw[min(mark, len(raw)):])
		draws := map[string]bool{}
		for line := range strings.Lines(logged) {
			pid := " pid=" + logField(line, "pid")
			switch {
			case strings.Contains(line, "process: start") && strings.Contains(line, "resume-draw"):
				draws[pid] = true
			case strings.Contains(line, "theme: loaded") && strings.Contains(line, " slug="+slug+" ") && draws[pid]:
				return true
			}
		}
		return false
	}) {
		t.Fatalf("no draw of the panel in %s; the log since holds:\n%s", slug, logged)
	}
}

// logField is the value of a key=value field of a log line.
func logField(line, key string) string {
	_, rest, _ := strings.Cut(line, " "+key+"=")
	value, _, _ := strings.Cut(rest, " ")
	return strings.TrimSpace(value)
}

func (w *waitingWorld) logSize() int {
	raw, _ := os.ReadFile(filepath.Join(w.stateDir, "portal.log"))
	return len(raw)
}

// setTheme pins the theme the world's processes read from prefs.json.
func (w *waitingWorld) setTheme(t *testing.T, slug string) {
	t.Helper()
	for _, kv := range slices.Backward(w.env) {
		if path, ok := strings.CutPrefix(kv, "PORTAL_PREFS_FILE="); ok {
			if err := os.WriteFile(path, []byte(`{"theme":"`+slug+`"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("the world names no prefs.json")
}

// leftAtTermSize attaches a terminal to the waiting session and leaves it for
// the picker, so the session's tty stays at the size every test terminal has.
func (w *waitingWorld) leftAtTermSize(t *testing.T) *portalTerm {
	t.Helper()
	p := w.portal(t, "open", waitingSession)
	w.awaitClients(t, waitingSession, 1)
	p.awaitRunner(t, waitingSession)
	w.awaitWaiterAt(t, portalTermCols, portalTermRows)
	mark := p.mark()
	p.typeIn(t, "\x1b")
	p.awaitAfter(t, mark, pickerMark)
	w.awaitClients(t, waitingSession, 0)
	return p
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
		p.awaitRunner(t, waitingSession)
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

	// Portal sends a waiting session nothing as it attaches: zmx's own SIGWINCH
	// on every attach, a switch's included, is the redraw, so these hold zmx to
	// its contract. Each attach is at the size an earlier one left the session
	// at, so only that signal can redraw the panel.
	for _, tc := range []struct {
		name string
		// arrive readies a way to the waiting session, then returns the step
		// that takes it.
		arrive func(t *testing.T, w *waitingWorld, picker *portalTerm) func()
	}{
		{"a pick from the picker", func(t *testing.T, w *waitingWorld, picker *portalTerm) func() {
			return func() { picker.typeIn(t, "\r") }
		}},
		{"portal open typed in another session, switching its runner", func(t *testing.T, w *waitingWorld, _ *portalTerm) func() {
			w.create(t, "other")
			q := w.portal(t, "open", "other")
			w.awaitClients(t, "other", 1)
			q.awaitRunner(t, "other")
			return func() { q.openTyped(t, waitingSession) }
		}},
		{"portal open typed in another session under a plain zmx client, which zmx switches", func(t *testing.T, w *waitingWorld, _ *portalTerm) func() {
			w.create(t, "other")
			raw := w.start(t, zmxtest.Binary(t), "attach", "other")
			w.awaitClients(t, "other", 1)
			return func() { raw.openTyped(t, waitingSession) }
		}},
	} {
		t.Run("zmx redraws a waiting panel in the theme in force on "+tc.name+" at an unchanged size", func(t *testing.T) {
			w := newWaitingWorld(t, binary)
			w.awaitPanel(t)
			picker := w.leftAtTermSize(t)
			attach := tc.arrive(t, w, picker)
			w.setTheme(t, "nord")

			mark := w.logSize()
			attach()
			w.awaitClients(t, waitingSession, 1)

			w.awaitDrawIn(t, mark, "nord")
			if !w.flagRaised(t) {
				t.Error("the redraw lowered the session's flag; it still waits")
			}
		})
	}

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
