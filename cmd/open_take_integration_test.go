//go:build integration

package cmd_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
)

// takenTitle and takenBy are on the screen an attach another terminal took
// lands on: its title, and the line saying where the session went.
const (
	takenTitle = "Open on another device"
	takenBy    = "Taken by "
)

// The environments a take reads where the session went from: over SSH, and
// in Ghostty on this Mac, an SSH_CONNECTION the developer's own shell may
// export blanked.
var (
	overSSH   = []string{"SSH_CONNECTION=192.0.2.20 52144 192.0.2.5 22"}
	inGhostty = []string{"SSH_CONNECTION=", "TERM_PROGRAM=ghostty"}
)

// sizeReporter is a session's program that prints the size of its terminal
// as it starts and at each change, as size-<rows>x<cols>, and reads nothing.
var sizeReporter = []string{"/bin/sh", "-c", `report() { echo "size-$(stty size | tr ' ' x)"; }
trap report WINCH
report
while :; do sleep 1 & wait $!; done`}

// portalWith runs the portal binary with args on a terminal of its own, size,
// in the world's environment with extra over it.
func (w *portalWorld) portalWith(t *testing.T, size pty.Winsize, extra []string, args ...string) *portalTerm {
	t.Helper()
	return w.startSized(t, size, append(slices.Clone(w.env), extra...), append([]string{w.binary}, args...)...)
}

// awaitReportedSize waits for the size session's sizeReporter last printed,
// as its history holds it, to be want.
func (w *portalWorld) awaitReportedSize(t *testing.T, session, want string) {
	t.Helper()
	var last string
	if !harnesstest.PollUntil(t, attachSettle, 50*time.Millisecond, func() bool {
		history, err := w.b.History(t.Context(), session, 0)
		if err != nil {
			return false
		}
		for line := range strings.SplitSeq(ansi.Strip(string(history)), "\n") {
			if line = strings.TrimSpace(line); strings.HasPrefix(line, "size-") {
				last = line
			}
		}
		return last == "size-"+want
	}) {
		t.Fatalf("session %s last reported %q, want size-%s", session, last, want)
	}
}

// awaitTaken waits for p's terminal to show, after mark, the screen saying
// another terminal took session, and where it went when by is not empty.
func (p *portalTerm) awaitTaken(t *testing.T, mark int, session, by string) {
	t.Helper()
	p.awaitAfter(t, mark, takenTitle)
	p.awaitAfter(t, mark, session)
	if by != "" {
		p.awaitAfter(t, mark, takenBy+by+".")
	}
}

// opener is a Portal opening a session, the size its terminal is and where
// a take reads it is.
type opener struct {
	p     *portalTerm
	size  string
	where string
}

func TestPortalOpenTakesTheSessionAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)

	t.Run("a second Portal takes the session, sized for it from its first frame; the first's screen says where, watches it live, and takes it back", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		w.createRunning(t, "alpha", sizeReporter)
		first := w.portalWith(t, pty.Winsize{Rows: 40, Cols: 120}, inGhostty, "open", "alpha")
		first.awaitLeading(t, "alpha")
		w.awaitReportedSize(t, "alpha", "40x120")
		mark := first.mark()

		second := w.portalWith(t, pty.Winsize{Rows: 30, Cols: 100}, overSSH, "open", "alpha")

		// Nothing is typed in the second terminal: zmx sizes the session for
		// it only once it leads, which it does from its attach only when the
		// take has left no other client there.
		second.awaitAfter(t, 0, "size-30x100")
		w.awaitReportedSize(t, "alpha", "30x100")
		first.awaitTaken(t, mark, "alpha", "SSH from 192.0.2.20")
		second.awaitLeading(t, "alpha")
		w.awaitClients(t, "alpha", 1)

		mark = first.mark()
		first.typeIn(t, " ")
		first.awaitAfter(t, mark, "◉ preview")
		if err := pty.Setsize(second.tty, &pty.Winsize{Rows: 25, Cols: 90}); err != nil {
			t.Fatalf("resize the second terminal: %v", err)
		}
		first.awaitAfter(t, mark, "size-25x90")
		w.awaitClients(t, "alpha", 1)

		mark = first.mark()
		first.typeIn(t, "\x1b")
		first.awaitTaken(t, mark, "alpha", "SSH from 192.0.2.20")
		mark = second.mark()
		first.typeIn(t, "\r")

		second.awaitTaken(t, mark, "alpha", "Ghostty on this Mac")
		first.awaitLeading(t, "alpha")
		w.awaitReportedSize(t, "alpha", "40x120")

		second.typeIn(t, "\x1b")
		second.awaitAfter(t, mark, pickerMark)
		second.typeIn(t, "q")
		if code := second.exitCode(t); code != 0 {
			t.Errorf("quitting the picker exited %d, want 0", code)
		}
		if !w.alive(t, "alpha") {
			t.Error("the session ended")
		}
	})

	t.Run("two Portals opening a session at once end with one attached, the session sized for it, and the other on the screen saying where it went", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		w.createRunning(t, "alpha", sizeReporter)
		if code := w.portal(t, "list").exitCode(t); code != 0 {
			t.Fatalf("the bootstrap's list exited %d, want 0", code)
		}

		a := opener{w.portalWith(t, pty.Winsize{Rows: 40, Cols: 120}, inGhostty, "open", "alpha"), "40x120", "Ghostty on this Mac"}
		b := opener{w.portalWith(t, pty.Winsize{Rows: 30, Cols: 100}, overSSH, "open", "alpha"), "30x100", "SSH from 192.0.2.20"}

		winner, loser := a, b
		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
			if strings.Contains(a.p.shownSince(0), takenTitle) {
				winner, loser = b, a
				return true
			}
			return strings.Contains(b.p.shownSince(0), takenTitle)
		}) {
			t.Fatalf("neither terminal shows the taken screen; the first shows:\n%s\nthe second:\n%s", a.p.shownSince(0), b.p.shownSince(0))
		}
		loser.p.awaitTaken(t, 0, "alpha", winner.where)
		winner.p.awaitLeading(t, "alpha")
		w.awaitClients(t, "alpha", 1)
		w.awaitReportedSize(t, "alpha", winner.size)
		if shown := winner.p.shownSince(0); strings.Contains(shown, takenTitle) {
			t.Errorf("the attached Portal was taken from too:\n%s", shown)
		}
	})

	t.Run("a raw zmx client is detached by the take, back at its shell", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		w.create(t, "alpha")
		raw := w.start(t, zmxtest.Binary(t), "attach", "alpha")
		w.awaitClients(t, "alpha", 1)

		p := w.portal(t, "open", "alpha")

		if code := raw.exitCode(t); code != 0 {
			t.Errorf("the raw client exited %d, want a detach's 0", code)
		}
		p.awaitLeading(t, "alpha")
		w.awaitClients(t, "alpha", 1)
		p.echoes(t, 6, 7)
	})

	t.Run("a runner that won't let go, as another version's, is detached by the backend, and its screen says where from the take's record", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		first, name := w.openProject(t)
		first.refusingAsAnotherVersion(t)
		mark := first.mark()

		second := w.portalWith(t, pty.Winsize{Rows: portalTermRows, Cols: portalTermCols}, overSSH, "open", name)

		first.awaitTaken(t, mark, name, "SSH from 192.0.2.20")
		w.awaitClients(t, name, 1)
		second.echoes(t, 6, 7)
	})

	t.Run("a detach from elsewhere with no take, leaving no terminal on the session, exits to the shell", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, name := w.openProject(t)
		mark := p.mark()

		if err := w.b.Detach(t.Context(), name); err != nil {
			t.Fatalf("detach %s: %v", name, err)
		}

		if code := p.exitCode(t); code != 0 {
			t.Errorf("portal exited %d, want 0", code)
		}
		if shown := p.shownSince(mark); strings.Contains(shown, takenTitle) || strings.Contains(shown, pickerMark) {
			t.Errorf("a detach no terminal took landed on a screen:\n%s", shown)
		}
		if !w.alive(t, name) {
			t.Error("the session ended")
		}
	})
}
