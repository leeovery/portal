//go:build integration

package cmd_test

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/zmxtest"
)

// create makes a detached session in the world's zmx directory.
func (w *portalWorld) create(t *testing.T, name string) {
	t.Helper()
	if _, err := w.b.Create(t.Context(), name, t.TempDir(), nil); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

// openTyped types portal open target into the session p's terminal shows,
// ending the line with the command's exit status, as rc-<status>, for the
// session's history. The session's shell is a login shell, whose /etc/profile
// runs macOS's path_helper, putting /etc/paths.d's /opt/homebrew/bin ahead of
// the pinned zmx, so the line puts the pinned zmx first again.
func (p *portalTerm) openTyped(t *testing.T, target string) {
	t.Helper()
	pinned := filepath.Dir(zmxtest.Binary(t))
	p.typeIn(t, "PATH="+pinned+":$PATH "+p.binary+" open "+target+"; echo rc-$?\r")
}

// showsSession proves which session p's terminal is in: an echo of the
// session's own name, computed in its shell, so the line typed never matches.
func (p *portalTerm) showsSession(t *testing.T, name string, n int) {
	t.Helper()
	mark := p.mark()
	p.typeIn(t, "echo in-$ZMX_SESSION-$(("+strconv.Itoa(n)+"*2))\r")
	p.awaitAfter(t, mark, "in-"+name+"-"+strconv.Itoa(n*2))
}

// awaitExitStatus waits for session's history to show the status the line
// openTyped typed there exited with.
func (w *portalWorld) awaitExitStatus(t *testing.T, session string, want int) {
	t.Helper()
	line := []byte("rc-" + strconv.Itoa(want))
	var history []byte
	if !harnesstest.PollUntil(t, attachSettle, 50*time.Millisecond, func() bool {
		history, _ = w.b.History(t.Context(), session, 0)
		return bytes.Contains(history, line)
	}) {
		t.Fatalf("session %s's history never showed %s:\n%s", session, line, history)
	}
}

// runners is the registry's live runners, by session.
func (w *portalWorld) runners(t *testing.T) map[string][]int {
	t.Helper()
	live, err := runnerreg.New(w.stateDir).Live(t.Context())
	if err != nil {
		t.Fatalf("read the runner registry: %v", err)
	}
	bySession := map[string][]int{}
	for _, rn := range live {
		bySession[rn.Session] = append(bySession[rn.Session], rn.PID)
	}
	return bySession
}

// awaitRunner waits for the registry to list p's portal as the one runner on
// session.
func (p *portalTerm) awaitRunner(t *testing.T, session string) {
	t.Helper()
	var got map[string][]int
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		got = p.runners(t)
		pids := got[session]
		return len(pids) == 1 && pids[0] == p.cmd.Process.Pid
	}) {
		t.Fatalf("runners = %v, want portal %d alone on %s", got, p.cmd.Process.Pid, session)
	}
}

func TestPortalOpenInsideASessionSwitchesAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)

	t.Run("the session's shell runs portal against the test's state and zmx directories", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, _ := w.openProject(t)
		mark := p.mark()

		p.typeIn(t, `echo "st=$PORTAL_STATE_DIR zd=$ZMX_DIR"`+"\r")

		p.awaitAfter(t, mark, "st="+w.stateDir+" zd="+w.b.Dir())
	})

	t.Run("portal open typed in a session moves that terminal through its runner, and exits 0", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		w.create(t, "beta")
		p.awaitRunner(t, alpha)

		p.openTyped(t, "beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, alpha, 0)
		p.showsSession(t, "beta", 21)
		w.awaitExitStatus(t, alpha, 0)
		p.awaitRunner(t, "beta")
	})

	t.Run("of two terminals on a session, the one typed in last switches, and the other stays", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		first, alpha := w.openProject(t)
		second := w.portal(t, "open", alpha)
		w.awaitClients(t, alpha, 2)
		w.create(t, "beta")
		first.echoes(t, 6, 7)

		second.openTyped(t, "beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, alpha, 1)
		second.showsSession(t, "beta", 21)
		first.showsSession(t, alpha, 22)
		second.awaitRunner(t, "beta")
		first.awaitRunner(t, alpha)
	})

	t.Run("with no runner, a plain zmx attach, portal open falls back and zmx switches", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p := w.start(t, zmxtest.Binary(t), "attach", "alpha")
		w.awaitClients(t, "alpha", 1)
		w.create(t, "beta")

		p.openTyped(t, "beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, "alpha", 0)
		p.showsSession(t, "beta", 21)
		w.awaitExitStatus(t, "alpha", 0)
		if got := w.runners(t); len(got) != 0 {
			t.Errorf("runners = %v, want none", got)
		}
	})

	t.Run("detach to the picker brings the picker on every runner on the session, and no other", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		first, alpha := w.openProject(t)
		second := w.portal(t, "open", alpha)
		w.awaitClients(t, alpha, 2)
		w.create(t, "beta")
		other := w.portal(t, "open", "beta")
		w.awaitClients(t, "beta", 1)
		marks := []int{first.mark(), second.mark(), other.mark()}

		accepted, err := runnerreg.New(w.stateDir).DetachToPicker(t.Context(), alpha)

		if err != nil || accepted != 2 {
			t.Fatalf("DetachToPicker = %d, %v; want both runners on %s", accepted, err, alpha)
		}
		w.awaitClients(t, alpha, 0)
		first.awaitAfter(t, marks[0], pickerMark)
		second.awaitAfter(t, marks[1], pickerMark)
		other.showsSession(t, "beta", 23)
		if shown := other.shownSince(marks[2]); strings.Contains(shown, pickerMark) {
			t.Errorf("the terminal on beta showed the picker:\n%s", shown)
		}
		if !w.alive(t, alpha) {
			t.Error("the session ended on a detach to the picker")
		}
	})
}
