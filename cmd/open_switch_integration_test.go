//go:build integration

package cmd_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/zmxtest"
)

// create makes a detached session in the world's zmx directory, its shell
// in the world's environment, so a portal typed there stays in the world.
func (w *portalWorld) create(t *testing.T, name string) {
	t.Helper()
	w.createRunning(t, name, nil)
}

// createRunning makes a detached session in the world's zmx directory
// running command in the world's environment, its shell when command is nil.
func (w *portalWorld) createRunning(t *testing.T, name string, command []string) {
	t.Helper()
	b, err := zmx.New(zmx.Config{Binary: zmxtest.Binary(t), Dir: w.b.Dir(), Environ: func() []string { return slices.Clone(w.env) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Create(t.Context(), name, t.TempDir(), command); err != nil {
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

// listed is a runner's entry in the registry, by its token.
type listed struct {
	PID     int    `json:"pid"`
	Session string `json:"session"`
}

// entries is the registry's entries, as their files hold them, by token.
func (w *portalWorld) entries(t *testing.T) map[string]listed {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(runnerreg.New(w.stateDir).Dir(), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]listed{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		var e listed
		if err == nil {
			err = json.Unmarshal(data, &e)
		}
		if err != nil {
			continue // gone, or half-written, since the glob
		}
		entries[strings.TrimSuffix(filepath.Base(path), ".json")] = e
	}
	return entries
}

// runners is the registry's runners, by session.
func (w *portalWorld) runners(t *testing.T) map[string][]int {
	t.Helper()
	bySession := map[string][]int{}
	for _, e := range w.entries(t) {
		bySession[e.Session] = append(bySession[e.Session], e.PID)
	}
	return bySession
}

// token is p's portal's token in the registry, "" when it is not listed.
func (p *portalTerm) token(t *testing.T) string {
	t.Helper()
	for token, e := range p.entries(t) {
		if e.PID == p.cmd.Process.Pid {
			return token
		}
	}
	return ""
}

// awaitRunner waits for the registry to list p's portal as the one runner on
// session.
func (p *portalTerm) awaitRunner(t *testing.T, session string) {
	t.Helper()
	var got map[string][]int
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		got = p.runners(t)
		return slices.Equal(got[session], []int{p.cmd.Process.Pid})
	}) {
		t.Fatalf("runners = %v, want portal %d alone on %s", got, p.cmd.Process.Pid, session)
	}
}

// zmxTyped types a raw zmx command into the session p's terminal shows,
// putting the pinned zmx first on the login shell's PATH as openTyped does.
func (p *portalTerm) zmxTyped(t *testing.T, args string) {
	t.Helper()
	p.typeIn(t, "PATH="+filepath.Dir(zmxtest.Binary(t))+":$PATH zmx "+args+"\r")
}

// refusingAsAnotherVersion puts a socket answering every request as a
// runner of another protocol version does, from p's portal's pid, where p's
// own socket was: the runner is listed and answers, but refuses.
func (p *portalTerm) refusingAsAnotherVersion(t *testing.T) {
	t.Helper()
	token := p.token(t)
	if token == "" {
		t.Fatal("the portal is not listed")
	}
	socket := filepath.Join(runnerreg.New(p.stateDir).Dir(), token+".sock")
	if err := os.Remove(socket); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("bind %s: %v", socket, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	refusal := `{"version":2,"ok":false,"refusal":"protocol version 1, where this runner speaks 2","pid":` + strconv.Itoa(p.cmd.Process.Pid) + "}\n"
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.ReadAll(conn)
			_, _ = conn.Write([]byte(refusal))
			_ = conn.Close()
		}
	}()
}

// awaitKillRefused presses the kill key in p's terminal, a runner zmx
// switched away, and waits for the picker to show why it asks no kill, with
// no kill prompt.
func (p *portalTerm) awaitKillRefused(t *testing.T) {
	t.Helper()
	mark := p.mark()
	p.typeIn(t, altQ)
	p.awaitAfter(t, mark, pickerMark)
	p.awaitAfter(t, mark, "kill refused")
	if shown := p.shownSince(mark); strings.Contains(shown, killPrompt) {
		t.Errorf("the picker asked to kill a session the terminal may not show:\n%s", shown)
	}
}

func TestPortalOpenInsideASessionSwitchesAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)

	t.Run("a session's shell runs portal against the test's state and zmx directories, whether portal or the test made it", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, _ := w.openProject(t)
		w.create(t, "beta")
		q := w.portal(t, "open", "beta")
		w.awaitClients(t, "beta", 1)

		for _, term := range []*portalTerm{p, q} {
			mark := term.mark()
			term.typeIn(t, `echo "st=$PORTAL_STATE_DIR zd=$ZMX_DIR"`+"\r")
			term.awaitAfter(t, mark, "st="+w.stateDir+" zd="+w.b.Dir())
		}
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

	t.Run("a raw zmx client typing portal open on a runner's session switches itself, and the runner stays", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		raw := w.start(t, zmxtest.Binary(t), "attach", alpha)
		w.awaitClients(t, alpha, 2)
		w.create(t, "beta")
		p.awaitRunner(t, alpha)

		raw.openTyped(t, "beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, alpha, 1)
		raw.showsSession(t, "beta", 21)
		p.showsSession(t, alpha, 22)
		p.awaitRunner(t, alpha)
	})

	t.Run("of a runner and a raw zmx client on a session, the runner typed in last switches through its runner, and the raw client stays", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		raw := w.start(t, zmxtest.Binary(t), "attach", alpha)
		w.awaitClients(t, alpha, 2)
		w.create(t, "beta")
		raw.echoes(t, 6, 7)

		p.openTyped(t, "beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, alpha, 1)
		p.showsSession(t, "beta", 21)
		raw.showsSession(t, alpha, 22)
		p.awaitRunner(t, "beta")
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

	t.Run("a leading runner that refuses, as another version's, is switched by the fallback, stays listed, and kills nothing after", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		w.create(t, "beta")
		p.awaitRunner(t, alpha)
		p.refusingAsAnotherVersion(t)

		p.openTyped(t, "beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, alpha, 0)
		p.showsSession(t, "beta", 21)
		w.awaitExitStatus(t, alpha, 0)
		p.awaitRunner(t, alpha)
		p.awaitKillRefused(t)
		for _, name := range []string{alpha, "beta"} {
			if !w.alive(t, name) {
				t.Errorf("%s ended on the kill key of a runner switched away", name)
			}
		}
	})

	t.Run("a zmx attach typed in a Portal attach switches it under the runner, which stays listed and kills nothing after", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		w.create(t, "beta")
		p.awaitRunner(t, alpha)

		p.zmxTyped(t, "attach beta")

		w.awaitClients(t, "beta", 1)
		w.awaitClients(t, alpha, 0)
		p.showsSession(t, "beta", 21)
		p.awaitRunner(t, alpha)
		p.awaitKillRefused(t)
		for _, name := range []string{alpha, "beta"} {
			if !w.alive(t, name) {
				t.Errorf("%s ended on the kill key of a runner switched away", name)
			}
		}
	})

	t.Run("a zmx inside a Portal attach switching its own client, as one over ssh does, leaves the kill key asking to kill the session", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		p.awaitRunner(t, alpha)
		inner := zmxtest.New(t)
		for _, name := range []string{"inner1", "inner2"} {
			if _, err := inner.Create(t.Context(), name, t.TempDir(), nil); err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
		}
		zmxBinary := shellquote.Single(zmxtest.Binary(t))

		p.typeIn(t, "env -u ZMX_SESSION ZMX_DIR="+shellquote.Single(inner.Dir())+" "+zmxBinary+" attach inner1\r")
		awaitClientsOn(t, inner, "inner1", 1)
		p.typeIn(t, zmxBinary+" attach inner2\r")
		awaitClientsOn(t, inner, "inner2", 1)
		// The runner reads the inner switch before the echo that follows it.
		p.showsSession(t, "inner2", 21)
		p.awaitRunner(t, alpha)
		mark := p.mark()
		p.typeIn(t, altQ)
		p.awaitAfter(t, mark, killPrompt)
		p.typeIn(t, "y")

		p.awaitAfter(t, mark, pickerMark)
		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool { return !w.alive(t, alpha) }) {
			t.Errorf("%s outlived the kill prompt's y", alpha)
		}
		if shown := p.shownSince(mark); strings.Contains(shown, "kill refused") {
			t.Errorf("the kill was refused:\n%s", shown)
		}
	})

	t.Run("detach to the picker brings the picker on the runner on the session, and no other terminal", func(t *testing.T) {
		w := newPortalWorld(t, binary)
		p, alpha := w.openProject(t)
		raw := w.start(t, zmxtest.Binary(t), "attach", alpha)
		w.awaitClients(t, alpha, 2)
		w.create(t, "beta")
		other := w.portal(t, "open", "beta")
		other.awaitLeading(t, "beta")
		marks := []int{p.mark(), raw.mark(), other.mark()}

		accepted, err := runnerreg.New(w.stateDir).DetachToPicker(t.Context(), alpha)

		if err != nil || accepted != 1 {
			t.Fatalf("DetachToPicker = %d, %v; want the runner on %s", accepted, err, alpha)
		}
		w.awaitClients(t, alpha, 1)
		p.awaitAfter(t, marks[0], pickerMark)
		raw.showsSession(t, alpha, 22)
		other.showsSession(t, "beta", 23)
		for i, term := range []*portalTerm{raw, other} {
			if shown := term.shownSince(marks[i+1]); strings.Contains(shown, pickerMark) {
				t.Errorf("a terminal the request didn't reach showed the picker:\n%s", shown)
			}
		}
		if !w.alive(t, alpha) {
			t.Error("the session ended on a detach to the picker")
		}
	})
}
