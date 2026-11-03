package runnerreg_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
)

// attachment is one attach a Listing readied, standing in for runner.Run:
// the test begins it and takes its requests as Run would.
type attachment struct {
	cfg runner.Config
}

func readied(l *runnerreg.Listing, session string) *attachment {
	a := &attachment{cfg: runner.Config{Session: session}}
	l.Attach(&a.cfg)
	return a
}

// take waits for the attach's next request, as Run's wait for its end does.
func (a *attachment) take(t *testing.T) runner.Request {
	t.Helper()
	select {
	case req := <-a.cfg.Requests:
		return req
	case <-time.After(5 * time.Second):
		t.Fatal("no request reached the attach")
		return runner.Request{}
	}
}

// begun lists the runner as an attach that has begun, typed in, as one whose
// terminal a command came from.
func begun(l *runnerreg.Listing, session string) *attachment {
	a := readied(l, session)
	a.cfg.Began()
	a.cfg.Forwarded()
	return a
}

func TestListing(t *testing.T) {
	t.Run("an attach is listed once it begins, and not before", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)

		a := readied(l, "api-x7Kd9a")
		if runners := live(t, r); len(runners) != 0 {
			t.Fatalf("Live = %+v before the attach began, want none", runners)
		}
		a.cfg.Began()

		if runners := live(t, r); len(runners) != 1 || runners[0].Session != "api-x7Kd9a" || runners[0].PID != os.Getpid() {
			t.Errorf("Live = %+v, want this runner on api-x7Kd9a", runners)
		}
	})

	t.Run("it keeps when the runner last forwarded input", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		a := readied(l, "api-x7Kd9a")
		a.cfg.Began()
		if runners := live(t, r); len(runners) != 1 || !runners[0].LastInput.IsZero() {
			t.Fatalf("Live = %+v, want no input yet", runners)
		}

		before := time.Now()
		a.cfg.Forwarded()

		runners := live(t, r)
		if len(runners) != 1 || runners[0].LastInput.Before(before) || runners[0].LastInput.After(time.Now()) {
			t.Errorf("Live = %+v, want input forwarded since %v", runners, before)
		}
	})

	t.Run("a switch is accepted once the attach takes it", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		a := begun(l, "api-x7Kd9a")

		switched := make(chan error, 1)
		go func() { switched <- r.SwitchFrom(t.Context(), "api-x7Kd9a", "blog-c3d4") }()

		if req := a.take(t); req != (runner.Request{Switch: "blog-c3d4"}) {
			t.Errorf("the attach took %+v, want a switch to blog-c3d4", req)
		}
		if err := <-switched; err != nil {
			t.Errorf("SwitchFrom = %v, want the attach's acceptance", err)
		}
	})

	t.Run("a detach to the picker reaches the attach as a request naming no session", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		a := begun(l, "api-x7Kd9a")

		detached := make(chan int, 1)
		go func() {
			n, _ := r.DetachToPicker(t.Context(), "api-x7Kd9a")
			detached <- n
		}()

		if req := a.take(t); req != (runner.Request{}) {
			t.Errorf("the attach took %+v, want a request for the picker", req)
		}
		if n := <-detached; n != 1 {
			t.Errorf("DetachToPicker = %d, want the attach's acceptance", n)
		}
	})

	t.Run("a request the attach never takes is refused once it ends", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		begun(l, "api-x7Kd9a")

		switched := make(chan error, 1)
		go func() { switched <- r.SwitchFrom(t.Context(), "api-x7Kd9a", "blog-c3d4") }()
		// The request waits on the attach, which never takes it.
		time.Sleep(100 * time.Millisecond)
		l.Ended()

		if err := <-switched; !errors.Is(err, runnerreg.ErrRefused) || !strings.Contains(err.Error(), "not attached") {
			t.Errorf("SwitchFrom = %v, want a refusal: the attach ended", err)
		}
	})

	t.Run("between a switch's attaches the entry stays and refuses requests", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		begun(l, "api-x7Kd9a")
		l.Ended()

		if runners := live(t, r); len(runners) != 1 {
			t.Fatalf("Live = %+v, want the runner still listed", runners)
		}
		if err := r.SwitchFrom(t.Context(), "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("SwitchFrom = %v, want a refusal between attaches", err)
		}
	})

	t.Run("the next attach after a switch moves the same entry to its session", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		begun(l, "api-x7Kd9a")
		entries := files(t, r.Dir())
		l.Ended()

		b := begun(l, "blog-c3d4")

		runners := live(t, r)
		if len(runners) != 1 || runners[0].Session != "blog-c3d4" {
			t.Fatalf("Live = %+v, want the runner moved to blog-c3d4", runners)
		}
		if after := files(t, r.Dir()); strings.Join(after, " ") != strings.Join(entries, " ") {
			t.Errorf("the registry holds %q after the switch, want the same entry as before, %q", after, entries)
		}
		go func() { _ = r.SwitchFrom(t.Context(), "blog-c3d4", "docs-e5f6") }()
		if req := b.take(t); req.Switch != "docs-e5f6" {
			t.Errorf("the next attach took %+v, want the switch sent it", req)
		}
	})

	t.Run("Close unlists the runner, and the next attach lists it again", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := r.Listing()
		t.Cleanup(l.Close)
		begun(l, "api-x7Kd9a")
		l.Ended()

		l.Close()
		if names := files(t, r.Dir()); len(names) != 0 {
			t.Fatalf("the registry holds %q after Close, want nothing", names)
		}
		begun(l, "blog-c3d4")

		if runners := live(t, r); len(runners) != 1 || runners[0].Session != "blog-c3d4" {
			t.Errorf("Live = %+v, want the runner listed on blog-c3d4", runners)
		}
	})

	t.Run("an attach the registry can't list goes on unlisted, with a WARN", func(t *testing.T) {
		sink := logtest.Install(t)
		long := filepath.Join(stateDir(t), strings.Repeat("s", runnerreg.MaxSocketPath()))
		if err := os.MkdirAll(long, 0o700); err != nil {
			t.Fatal(err)
		}
		r := runnerreg.New(long)
		l := r.Listing()
		t.Cleanup(l.Close)

		begun(l, "api-x7Kd9a")
		l.Ended()
		l.Close()

		rec := sink.Records().Matching("attach", "register failed").AtExactLevel(slog.LevelWarn).Only(t, "register failed record")
		if got := rec.AttrString(t, "session"); got != "api-x7Kd9a" {
			t.Errorf("session = %q, want api-x7Kd9a", got)
		}
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, runnerreg.ErrSocketPathTooLong) {
			t.Errorf("error = %v, want ErrSocketPathTooLong", err)
		}
	})
}
