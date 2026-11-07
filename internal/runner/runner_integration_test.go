//go:build integration

package runner_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/zmxtest"
)

const sessionName = "runner"

func TestRunOverZmx(t *testing.T) {
	t.Run("a bound key detaches, and the session lives on", func(t *testing.T) {
		z := attachZmx(t, nil)

		z.h.detach(t)

		z.assertEndedByZmx(t)
		z.awaitClients(t, 0)
	})

	t.Run("a bound key detaches though the user turned zmx's detach key off, and the session's program is untouched", func(t *testing.T) {
		z := attachZmx(t, []string{"sh", "-c", "cat; echo cat-ended-$?; exec sleep 1000"}, "ZMX_NO_DETACH_KEY=1")

		z.h.detach(t)

		z.assertEndedByZmx(t)
		// ctrl+\ reaching the session would be SIGQUIT for cat and its shell.
		if alive, err := z.b.Exists(t.Context(), sessionName); err != nil || !alive {
			t.Fatalf("Exists = %v, %v after the detach, want the session alive", alive, err)
		}
		history, err := z.b.History(t.Context(), sessionName, 0)
		if err != nil || bytes.Contains(history, []byte("cat-ended")) {
			t.Errorf("the session's history is %q, %v; want cat still running", history, err)
		}
		z.awaitClients(t, 0)
	})

	t.Run("input just before a bound key reaches the session, and the key still detaches", func(t *testing.T) {
		z := attachZmx(t, nil)

		z.h.term.typeIn(t, "echo marker-$((6*7))\r"+keyDetach)
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
			t.Errorf("Run = %+v, %v; want a detach", out, err)
		}
		z.assertEndedByZmx(t)
		var history []byte
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			history, _ = z.b.History(t.Context(), sessionName, 0)
			return bytes.Contains(history, []byte("marker-42"))
		}) {
			t.Errorf("the session's history is %q, want the command typed before the key run", history)
		}
	})

	t.Run("kill detaches as detach does, asking for the kill, and the session lives on", func(t *testing.T) {
		z := attachZmx(t, nil)

		z.h.term.typeIn(t, keyKill)
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Kill}) || !out.AsksKill() {
			t.Errorf("Run = %+v, %v; want the kill asked", out, err)
		}
		z.assertEndedByZmx(t)
		z.awaitClients(t, 0)
		if alive, err := z.b.Exists(t.Context(), sessionName); err != nil || !alive {
			t.Errorf("Exists = %v, %v after the kill key, want the session alive for the picker to ask about", alive, err)
		}
	})

	t.Run("the user's own ctrl+\\ detaches to the shell", func(t *testing.T) {
		z := attachZmx(t, nil)

		z.h.term.typeIn(t, "\x1c")
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{}) {
			t.Errorf("Run = %+v, %v; want the shell with status 0", out, err)
		}
		z.assertEndedByZmx(t)
		z.awaitClients(t, 0)
	})

	t.Run("a zmx attach typed in the session switches the child under the runner, whose kill key then asks for no kill", func(t *testing.T) {
		z := attachZmx(t, nil)
		if _, err := z.b.Create(t.Context(), "other", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		z.h.term.typeIn(t, shellquote.Single(zmxtest.Binary(t))+" attach other\r")
		z.awaitSwitchSeen(t)
		z.h.term.typeIn(t, keyKill)
		out, err := z.h.finish(t)

		if err != nil || out.Action != attachkeys.Kill || !errors.Is(out.Err, runner.ErrUnconfirmed) || out.AsksKill() {
			t.Errorf("Run = %+v, %v; want the kill refused, unconfirmed", out, err)
		}
		z.assertEndedByZmx(t)
		for _, name := range []string{sessionName, "other"} {
			if alive, err := z.b.Exists(t.Context(), name); err != nil || !alive {
				t.Errorf("Exists(%s) = %v, %v after the kill key, want it alive", name, alive, err)
			}
		}
	})

	t.Run("a zmx attach typed in the session switching to a session it makes still leaves the kill key asking for no kill", func(t *testing.T) {
		z := attachZmx(t, nil)

		z.h.term.typeIn(t, shellquote.Single(zmxtest.Binary(t))+" attach scratch\r")
		z.awaitSwitchSeen(t)
		z.h.term.typeIn(t, keyKill)
		out, err := z.h.finish(t)

		if err != nil || out.Action != attachkeys.Kill || !errors.Is(out.Err, runner.ErrUnconfirmed) || out.AsksKill() {
			t.Errorf("Run = %+v, %v; want the kill refused, unconfirmed", out, err)
		}
		if shown := z.h.term.output(); !bytes.Contains(shown, []byte(`session "scratch" created`)) {
			t.Errorf("the terminal shows %q, want zmx's line for the session the switch made", shown)
		}
		z.assertEndedByZmx(t)
		for _, name := range []string{sessionName, "scratch"} {
			if alive, err := z.b.Exists(t.Context(), name); err != nil || !alive {
				t.Errorf("Exists(%s) = %v, %v after the kill key, want it alive", name, alive, err)
			}
		}
	})

	t.Run("a zmx inside the session switching its own client leaves the kill key asking to kill the session", func(t *testing.T) {
		z := attachZmx(t, nil)
		inner := zmxtest.New(t)
		for _, name := range []string{"inner1", "inner2"} {
			if _, err := inner.Create(t.Context(), name, t.TempDir(), nil); err != nil {
				t.Fatalf("Create %s: %v", name, err)
			}
		}
		binary := shellquote.Single(zmxtest.Binary(t))

		z.h.term.typeIn(t, "env -u ZMX_SESSION ZMX_DIR="+shellquote.Single(inner.Dir())+" "+binary+" attach inner1\r")
		awaitClients(t, inner, "inner1", 1)
		z.h.term.typeIn(t, binary+" attach inner2\r")
		awaitClients(t, inner, "inner2", 1)
		z.awaitSwitchSeen(t)
		z.h.term.typeIn(t, keyKill)
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Kill}) || !out.AsksKill() {
			t.Errorf("Run = %+v, %v; want the kill asked", out, err)
		}
		z.assertEndedByZmx(t)
	})

	t.Run("a detach from elsewhere drops the child, and the session lives on", func(t *testing.T) {
		z := attachZmx(t, nil)

		if err := z.b.Detach(t.Context(), sessionName); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Dropped: true}) {
			t.Errorf("Run = %+v, %v; want the child dropped", out, err)
		}
		z.assertEndedByZmx(t)
		if alive, err := z.b.Exists(t.Context(), sessionName); err != nil || !alive {
			t.Errorf("Exists = %v, %v after the detach, want the session alive", alive, err)
		}
	})

	t.Run("the session ending drops the child, gone by the time the attach ends", func(t *testing.T) {
		z := attachZmx(t, nil)

		z.h.term.typeIn(t, "exit\r")
		out, err := z.h.finish(t)

		if err != nil || out != (runner.Outcome{Dropped: true}) {
			t.Errorf("Run = %+v, %v; want the child dropped, status 0", out, err)
		}
		// zmx unlinks the session's socket before it hangs up on its clients,
		// so a caller judging the drop finds the session gone.
		if alive, err := z.b.Exists(t.Context(), sessionName); err != nil || alive {
			t.Errorf("Exists = %v, %v as the attach ends, want the session gone", alive, err)
		}
		z.assertEndedByZmx(t)
	})
}

// zmxAttach is a runner attached to a real zmx session.
type zmxAttach struct {
	h    *harness
	b    *zmx.Backend
	sink *logtest.Sink
}

// attachZmx creates a session running command, the shell when nil, in a zmx
// directory of the test's own, and runs the runner on its attach command.
// environ joins the environment the attach
// command is made in, as the user's own does.
func attachZmx(t *testing.T, command []string, environ ...string) *zmxAttach {
	t.Helper()
	return attachZmxWith(t, command, environ)
}

// attachToken is the variable whose value in the attach's client marks the
// session's leader as the runner's own, as Portal's own token does.
const attachToken = "RUNNER_TEST_ATTACH"

// attachZmxWith is attachZmx with each of opts applied to the attach's
// config. The runner confirms its session as Portal does: through the
// session's leader carrying a token its attach command gave its client.
func attachZmxWith(t *testing.T, command, environ []string, opts ...option) *zmxAttach {
	t.Helper()
	sink := logtest.Install(t)
	b := zmxtest.New(t)
	if _, err := b.Create(t.Context(), sessionName, t.TempDir(), command); err != nil {
		t.Fatalf("Create: %v", err)
	}
	const token = "runner-token"
	h := attachOn(t, t.Context(), newTerminal(t), b.Track(attachCommand(t, b, environ), attachToken, token), append([]option{func(_ *harness, cfg *runner.Config) {
		cfg.Session = sessionName
		cfg.Confirm = func(ctx context.Context) error {
			env, err := b.LeaderEnv(ctx, sessionName)
			if err != nil {
				return err
			}
			if got, _ := env.Lookup(attachToken); got != token {
				return fmt.Errorf("the session's leader carries %q, not %q", got, token)
			}
			return nil
		}
	}}, opts...)...)
	z := &zmxAttach{h: h, b: b, sink: sink}
	z.awaitClients(t, 1)
	return z
}

// attachCommand is the session's attach command, made by a Backend on b's
// directory in b's environment and environ.
func attachCommand(t *testing.T, b *zmx.Backend, environ []string) backend.Command {
	t.Helper()
	command, err := b.AttachCommand(t.Context(), sessionName)
	if err != nil {
		t.Fatalf("AttachCommand: %v", err)
	}
	if len(environ) == 0 {
		return command
	}
	env := append(slices.Clone(command.Env), environ...)
	in, err := zmx.New(zmx.Config{Binary: command.Argv[0], Dir: b.Dir(), Environ: func() []string { return slices.Clone(env) }})
	if err != nil {
		t.Fatalf("bind a Backend to %s: %v", b.Dir(), err)
	}
	if command, err = in.AttachCommand(t.Context(), sessionName); err != nil {
		t.Fatalf("AttachCommand: %v", err)
	}
	return command
}

// awaitClients waits for the session to have want clients attached.
func (z *zmxAttach) awaitClients(t *testing.T, want int) {
	t.Helper()
	awaitClients(t, z.b, sessionName, want)
}

// awaitClients waits for b's session name to have want clients attached.
func awaitClients(t *testing.T, b *zmx.Backend, name string, want int) {
	t.Helper()
	var got int
	var err error
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		var s backend.Session
		s, err = b.Session(t.Context(), name)
		got = s.Clients
		return err == nil && got == want
	}) {
		t.Fatalf("session %s has %d clients (%v), want %d", name, got, err, want)
	}
}

// awaitSwitchSeen waits for the runner to see a switch in the child's
// output.
func (z *zmxAttach) awaitSwitchSeen(t *testing.T) {
	t.Helper()
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		return len(z.sink.Records().Matching("attach", "switch seen")) > 0
	}) {
		t.Fatalf("the runner never saw the switch; the terminal shows %q", z.h.term.output())
	}
}

// assertEndedByZmx checks zmx attach ended by its own hand and reset the
// terminal itself: the runner neither signalled it nor wrote a reset, and
// zmx's reset is the last the terminal shows. The runner leaves the reset to
// zmx on that strength alone.
func (z *zmxAttach) assertEndedByZmx(t *testing.T) {
	t.Helper()
	if signalled := z.sink.Records().Matching("attach", "child did not exit").AtExactLevel(slog.LevelWarn); len(signalled) > 0 {
		t.Errorf("zmx attach had to be signalled: %+v", signalled)
	}
	if reset := z.sink.Records().Matching("attach", "terminal reset"); len(reset) > 0 {
		t.Errorf("the runner reset the terminal in zmx's place: %+v", reset)
	}
	var shown []byte
	if !harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
		shown = z.h.term.output()
		return bytes.HasSuffix(shown, []byte(terminalReset))
	}) {
		t.Errorf("the terminal shows %q last, want zmx's own reset", shown[max(len(shown)-64, 0):])
	}
}
