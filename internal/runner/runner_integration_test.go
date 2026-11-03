//go:build integration

package runner_test

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/zmxtest"
)

const sessionName = "runner"

func TestRunOverZmx(t *testing.T) {
	t.Run("a bound key detaches, and the session lives on", func(t *testing.T) {
		z := attachZmx(t)

		z.h.detach(t)

		z.assertDetachedByZmx(t)
		z.awaitClients(t, 0)
	})

	t.Run("input just before a bound key reaches the session, and the key still detaches", func(t *testing.T) {
		z := attachZmx(t)

		z.h.term.typeIn(t, "echo marker-$((6*7))\r"+keyDetach)
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
			t.Errorf("Run = %+v, %v; want a detach", out, err)
		}
		z.assertDetachedByZmx(t)
		var history []byte
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			history, _ = z.b.History(t.Context(), sessionName, 0)
			return bytes.Contains(history, []byte("marker-42"))
		}) {
			t.Errorf("the session's history is %q, want the command typed before the key run", history)
		}
	})

	t.Run("kill ends the session", func(t *testing.T) {
		z := attachZmx(t)

		z.h.term.typeIn(t, keyKill)
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Kill}) {
			t.Errorf("Run = %+v, %v; want a kill with no error", out, err)
		}
		z.assertDetachedByZmx(t)
		if alive, err := z.b.Exists(t.Context(), sessionName); err != nil || alive {
			t.Errorf("Exists = %v, %v after the kill, want the session gone", alive, err)
		}
	})

	t.Run("the user's own ctrl+\\ detaches to the shell", func(t *testing.T) {
		z := attachZmx(t)

		z.h.term.typeIn(t, "\x1c")
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{}) {
			t.Errorf("Run = %+v, %v; want the shell with status 0", out, err)
		}
		z.awaitClients(t, 0)
	})

	t.Run("the session ending exits to the shell", func(t *testing.T) {
		z := attachZmx(t)

		z.h.term.typeIn(t, "exit\r")
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{}) {
			t.Errorf("Run = %+v, %v; want the shell with status 0", out, err)
		}
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			alive, err := z.b.Exists(t.Context(), sessionName)
			return err == nil && !alive
		}) {
			t.Errorf("the session outlived its shell's exit")
		}
	})
}

// zmxAttach is a runner attached to a real zmx session.
type zmxAttach struct {
	h    *harness
	b    *zmx.Backend
	sink *logtest.Sink
}

// attachZmx creates a session in a zmx directory of the test's own and runs
// the runner on its attach command, killing through the backend.
func attachZmx(t *testing.T) *zmxAttach {
	t.Helper()
	sink := logtest.Install(t)
	b := zmxtest.New(t)
	if _, err := b.Create(t.Context(), sessionName, t.TempDir(), nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	command, err := b.AttachCommand(t.Context(), sessionName)
	if err != nil {
		t.Fatalf("AttachCommand: %v", err)
	}
	h := attachCommand(t, t.Context(), command, func(cfg *runner.Config) {
		cfg.Session = sessionName
		cfg.Kill = b.Kill
	})
	z := &zmxAttach{h: h, b: b, sink: sink}
	z.awaitClients(t, 1)
	return z
}

// awaitClients waits for the session to have want clients attached.
func (z *zmxAttach) awaitClients(t *testing.T, want int) {
	t.Helper()
	var got int
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		s, err := z.b.Session(t.Context(), sessionName)
		got = s.Clients
		return err == nil && got == want
	}) {
		t.Fatalf("the session has %d clients, want %d", got, want)
	}
}

// assertDetachedByZmx checks the attach ended without signalling the child:
// zmx itself ended it.
func (z *zmxAttach) assertDetachedByZmx(t *testing.T) {
	t.Helper()
	if signalled := z.sink.Records().Matching("attach", "child did not exit").AtExactLevel(slog.LevelWarn); len(signalled) > 0 {
		t.Errorf("zmx attach had to be signalled: %+v", signalled)
	}
}
