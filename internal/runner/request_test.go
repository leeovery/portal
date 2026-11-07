package runner_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
)

// untaken bounds a test's wait for a request the runner must not take.
const untaken = 300 * time.Millisecond

// withRequests hands the attach requests, unbuffered, so a send completes
// only once Run takes it.
func withRequests(requests chan runner.Request) option {
	return func(_ *harness, cfg *runner.Config) { cfg.Requests = requests }
}

// ask sends req, failing the test unless Run reads it, and returns Run's
// answer.
func ask(t *testing.T, requests chan<- runner.Request, req runner.Request) error {
	t.Helper()
	answer := make(chan error, 1)
	req.Answer = answer
	select {
	case requests <- req:
	case <-time.After(settle):
		t.Fatalf("Run never read %+v", req)
	}
	select {
	case err := <-answer:
		return err
	case <-time.After(settle):
		t.Fatalf("Run never answered %+v", req)
		return nil
	}
}

// send sends req, failing the test unless Run takes it.
func send(t *testing.T, requests chan<- runner.Request, req runner.Request) {
	t.Helper()
	if err := ask(t, requests, req); err != nil {
		t.Fatalf("Run refused %+v: %v", req, err)
	}
}

// goesOn checks the attach is still under way: Run takes a switch, and ends
// the attach for it.
func goesOn(t *testing.T, h *harness, requests chan<- runner.Request) {
	t.Helper()
	send(t, requests, runner.Request{Switch: "blog-c3d4"})
	if out, err := h.finish(t); err != nil || out != (runner.Outcome{Switch: "blog-c3d4"}) {
		t.Errorf("Run = %+v, %v; want the attach still under way, ended by the switch sent after", out, err)
	}
}

// taken reports whether Run reads req within the wait.
func taken(requests chan<- runner.Request, req runner.Request, within time.Duration) bool {
	req.Answer = make(chan error, 1)
	select {
	case requests <- req:
		return true
	case <-time.After(within):
		return false
	}
}

func TestRun_Requests(t *testing.T) {
	t.Run("a switch detaches with ctrl+\\ and names its session in the outcome", func(t *testing.T) {
		sink := logtest.Install(t)
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests))
		h.ready(t)

		send(t, requests, runner.Request{Switch: "blog-c3d4"})
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Switch: "blog-c3d4"}) {
			t.Fatalf("Run = %+v, %v; want a switch to blog-c3d4", out, err)
		}
		if out.Picker() {
			t.Error("Picker() = true after a switch, want the switch's session attached instead")
		}
		assertDetachedCleanly(t, h, sink)
		rec := sink.Records().Matching("attach", "outcome").AtExactLevel(slog.LevelInfo).Only(t, "outcome record")
		if got := rec.AttrString(t, "reason"); got != "switch" {
			t.Errorf("outcome reason = %q, want switch", got)
		}
		if got := rec.AttrString(t, "target"); got != "blog-c3d4" {
			t.Errorf("outcome target = %q, want blog-c3d4", got)
		}
		if got := rec.IntAttr(t, "code"); got != 0 {
			t.Errorf("outcome code = %d, want 0", got)
		}
	})

	t.Run("a request naming no session detaches to the picker, as the detach key does", func(t *testing.T) {
		sink := logtest.Install(t)
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests))
		h.ready(t)

		send(t, requests, runner.Request{})
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
			t.Fatalf("Run = %+v, %v; want a detach", out, err)
		}
		if !out.Picker() {
			t.Error("Picker() = false, want the picker to follow")
		}
		assertDetachedCleanly(t, h, sink)
	})

	t.Run("a request to confirm the attach's session is accepted without asking Confirm, ending nothing", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests), func(_ *harness, cfg *runner.Config) {
			cfg.Confirm = func(context.Context) error {
				t.Error("Confirm was asked with no switch seen")
				return nil
			}
		})
		h.ready(t)

		send(t, requests, runner.Request{Confirm: "fake"})

		goesOn(t, h, requests)
		if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
			t.Errorf("the child read %q, want only the switch's ctrl+\\", got)
		}
	})

	t.Run("a request to confirm another session is refused, and the attach goes on", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests))
		h.ready(t)

		if err := ask(t, requests, runner.Request{Confirm: "blog-c3d4"}); err == nil || errors.Is(err, runner.ErrUnconfirmed) || !strings.Contains(err.Error(), "shows fake") {
			t.Errorf("Run answered %v, want a refusal naming the session it shows", err)
		}
		h.detach(t)
	})

	t.Run("a request past its deadline is refused, and the attach goes on", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests))
		h.ready(t)

		err := ask(t, requests, runner.Request{Switch: "blog-c3d4", Deadline: time.Now().Add(-time.Millisecond)})

		if !errors.Is(err, runner.ErrPastDeadline) {
			t.Errorf("Run answered %v, want ErrPastDeadline", err)
		}
		h.detach(t)
		if got := h.term.reads(t); !slices.Equal(got, []string{"\x1c"}) {
			t.Errorf("the child read %q, want only the detach key's ctrl+\\", got)
		}
	})

	t.Run("a request before its deadline is taken", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests))
		h.ready(t)

		send(t, requests, runner.Request{Switch: "blog-c3d4", Deadline: time.Now().Add(settle)})
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Switch: "blog-c3d4"}) {
			t.Errorf("Run = %+v, %v; want the switch", out, err)
		}
	})

	t.Run("input typed before a request reaches the session first", func(t *testing.T) {
		requests := make(chan runner.Request)
		h := attach(t, modeZmx, withRequests(requests))
		h.ready(t)

		h.term.typeIn(t, "abc")
		h.term.await(t, "in:616263")
		send(t, requests, runner.Request{Switch: "blog-c3d4"})
		if _, err := h.finish(t); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if got, want := h.term.reads(t), []string{"abc", "\x1c"}; !slices.Equal(got, want) {
			t.Errorf("the child read %q, want the input, then ctrl+\\ alone", got)
		}
	})
}

func TestRun_RequestsFollowThePrecedenceOfEnds(t *testing.T) {
	t.Run("a request is not taken once a key has ended the attach", func(t *testing.T) {
		runner.SetGrace(t, shortGrace)
		requests := make(chan runner.Request)
		h := attach(t, modeDeaf, withRequests(requests))
		h.ready(t)

		h.term.typeIn(t, keyDetachToShell)
		// The deaf child keeps the teardown waiting while the request comes.
		h.term.await(t, "in:1c")
		if taken(requests, runner.Request{Switch: "blog-c3d4"}, untaken) {
			t.Error("Run took a switch after the key ended the attach")
		}
		out, err := h.finish(t)

		if err != nil || out.Action != attachkeys.DetachToShell || out.Switch != "" {
			t.Errorf("Run = %+v, %v; want the key's detach to the shell", out, err)
		}
	})

	t.Run("a key once a request has ended the attach is left for the next reader", func(t *testing.T) {
		runner.SetGrace(t, shortGrace)
		requests := make(chan runner.Request)
		h := attach(t, modeDeaf, withRequests(requests))
		h.ready(t)

		send(t, requests, runner.Request{Switch: "blog-c3d4"})
		h.term.await(t, "in:1c")
		h.term.typeIn(t, keyDetachToShell)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{Switch: "blog-c3d4"}) {
			t.Errorf("Run = %+v, %v; want the switch", out, err)
		}
	})

	t.Run("a signal while a request tears the attach down outranks it", func(t *testing.T) {
		runner.SetGrace(t, shortGrace)
		requests := make(chan runner.Request)
		h := attach(t, modeDeaf, withRequests(requests))
		h.ready(t)

		send(t, requests, runner.Request{Switch: "blog-c3d4"})
		h.term.await(t, "in:1c")
		signalDuringTeardown(t)
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{ExitCode: 128 + int(syscall.SIGTERM)}) {
			t.Errorf("Run = %+v, %v; want SIGTERM's outcome, the switch dropped", out, err)
		}
	})

	t.Run("a request is not taken once a signal has ended the attach", func(t *testing.T) {
		runner.SetGrace(t, shortGrace)
		requests := make(chan runner.Request)
		h := attach(t, modeDeaf, withRequests(requests))
		h.ready(t)

		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatalf("signal Portal: %v", err)
		}
		h.term.await(t, "in:1c")
		if taken(requests, runner.Request{Switch: "blog-c3d4"}, untaken) {
			t.Error("Run took a switch after a signal ended the attach")
		}
		out, err := h.finish(t)

		if err != nil || out != (runner.Outcome{ExitCode: 128 + int(syscall.SIGTERM)}) {
			t.Errorf("Run = %+v, %v; want SIGTERM's outcome", out, err)
		}
	})
}

func TestRun_Began(t *testing.T) {
	t.Run("it is called once, and no request is taken before it returns", func(t *testing.T) {
		requests := make(chan runner.Request)
		entered, release := make(chan struct{}, 1), make(chan struct{})
		var calls atomic.Int32
		h := attach(t, modeZmx, withRequests(requests), func(_ *harness, cfg *runner.Config) {
			cfg.Began = func() {
				calls.Add(1)
				entered <- struct{}{}
				<-release
			}
		})
		h.ready(t)

		select {
		case <-entered:
		case <-time.After(settle):
			t.Fatal("Began was never called")
		}
		if taken(requests, runner.Request{}, untaken) {
			t.Fatal("Run took a request before Began returned")
		}
		close(release)
		send(t, requests, runner.Request{})
		if _, err := h.finish(t); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if n := calls.Load(); n != 1 {
			t.Errorf("Began was called %d times, want once", n)
		}
	})

	t.Run("an attach that fails to begin never calls it", func(t *testing.T) {
		tm := newTerminal(t)
		var called atomic.Bool

		_, err := runner.Run(t.Context(), runner.Config{
			Session: "fake",
			Command: backend.Command{Argv: []string{"/nonexistent/zmx", "attach", "fake"}},
			In:      tm.tty,
			Out:     tm.tty,
			Scanner: keys,
			Began:   func() { called.Store(true) },
		})

		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Run error = %v, want the failed start", err)
		}
		if called.Load() {
			t.Error("Began was called for an attach that never began")
		}
	})

	t.Run("a panic in it detaches, restores the terminal, then propagates", func(t *testing.T) {
		sink := logtest.Install(t)
		h := attach(t, modeZmx, func(_ *harness, cfg *runner.Config) {
			cfg.Began = func() { panic("began exploded") }
		})

		r := h.end(t) // the terminal's settings and flags are checked restored

		if r.panicked != "began exploded" {
			t.Errorf("Run = %+v, %v, panicking with %v; want Began's panic raised again", r.out, r.err, r.panicked)
		}
		assertPanicLogged(t, sink, "began exploded")
	})
}
