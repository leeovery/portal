package runnerreg_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/runnertoken"
)

// deadlineOf is the deadline a request line carries.
func deadlineOf(t *testing.T, line string) time.Time {
	t.Helper()
	var q struct {
		Deadline time.Time `json:"deadline"`
	}
	if err := json.Unmarshal([]byte(line), &q); err != nil {
		t.Fatalf("read the request %q: %v", line, err)
	}
	return q.Deadline
}

// reasonsOf is each runner's reason DetachToPicker gives for none accepting.
func reasonsOf(t *testing.T, err error) []error {
	t.Helper()
	joined, ok := errors.Unwrap(err).(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("%v holds no reasons", err)
	}
	return joined.Unwrap()
}

// exitsOnceScanned has the registry judge pid running as it scans, the
// first time it asks, and gone every time after, as the runner exits between
// the scan that finds it and the request that asks it.
func exitsOnceScanned(t *testing.T, pid int) {
	var asked atomic.Int32
	runnerreg.SetRunning(t, func(p int, running func(int) bool) bool {
		if p != pid {
			return running(p)
		}
		return asked.Add(1) == 1
	})
}

// contextWithin is a context of the test's that ends after d.
func contextWithin(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(t.Context(), d)
}

func TestSwitch(t *testing.T) {
	t.Run("it asks the runner the token names, and only it", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		typed, other := &handler{}, &handler{}
		register(t, r, "typed1", "api-x7Kd9a", typed)
		register(t, r, "other1", "api-x7Kd9a", other)

		if err := r.Switch(t.Context(), "typed1", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Fatalf("Switch = %v, want the runner's acceptance", err)
		}

		if switches, _ := typed.asked(); !slices.Equal(switches, []string{"blog-c3d4"}) {
			t.Errorf("the runner named switched to %q, want blog-c3d4", switches)
		}
		if switches, _ := other.asked(); len(switches) != 0 {
			t.Errorf("another runner on the session was asked to switch to %q", switches)
		}
		rec := sink.Records().Matching("attach", "switch").AtExactLevel(slog.LevelInfo).Only(t, "switch record")
		for key, want := range map[string]string{"session": "api-x7Kd9a", "target": "blog-c3d4", "result": "accepted"} {
			if got := rec.AttrString(t, key); got != want {
				t.Errorf("switch %s = %q, want %q", key, got, want)
			}
		}
		if got := rec.IntAttr(t, "target_pid"); got != int64(os.Getpid()) {
			t.Errorf("switch target_pid = %d, want %d", got, os.Getpid())
		}
	})

	t.Run("no token, one the registry doesn't hold, or one no runner could have, is ErrNoRunner, asking no one", func(t *testing.T) {
		for _, token := range []string{"", "nosuch", "../../tok123", "tok123/x"} {
			sink := logtest.Install(t)
			r := runnerreg.New(stateDir(t))
			h := &handler{}
			register(t, r, "tok123", "api-x7Kd9a", h)

			if err := r.Switch(t.Context(), token, "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnertoken.ErrNoRunner) {
				t.Errorf("Switch(%q) = %v, want ErrNoRunner", token, err)
			}
			if switches, _ := h.asked(); len(switches) != 0 {
				t.Errorf("Switch(%q) asked a runner to switch to %q", token, switches)
			}
			rec := sink.Records().Matching("attach", "switch").Only(t, "switch record")
			if got := rec.AttrString(t, "result"); got != "no runner" || rec.HasAttr("target_pid") || rec.HasAttr("error") {
				t.Errorf("switch record for %q = %+v, want result no runner, naming no runner and no error", token, rec)
			}
		}
	})

	t.Run("a runner whose entry names a session other than current is ErrNoRunner, asked nothing", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		h := &handler{}
		register(t, r, "tok123", "docs-e5f6", h)

		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnertoken.ErrNoRunner) {
			t.Errorf("Switch = %v, want ErrNoRunner", err)
		}
		if switches, _ := h.asked(); len(switches) != 0 {
			t.Errorf("the runner on docs-e5f6 was asked to switch to %q", switches)
		}
		if !exists(t, r.EntryPath("tok123")) {
			t.Error("the runner's entry was removed")
		}
		rec := sink.Records().Matching("attach", "switch").Only(t, "switch record")
		if got := rec.AttrString(t, "result"); got != "no runner" || rec.IntAttr(t, "target_pid") != int64(os.Getpid()) {
			t.Errorf("switch record = %+v, want result no runner, naming the runner the token names", rec)
		}
	})

	t.Run("a refusal is returned, and logged as one", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		register(t, r, "tok123", "api-x7Kd9a", &handler{refuse: errors.New("the runner is not attached")})

		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnertoken.ErrRefused) {
			t.Fatalf("Switch = %v, want ErrRefused", err)
		}
		rec := sink.Records().Matching("attach", "switch").Only(t, "switch record")
		if got := rec.AttrString(t, "result"); got != "refused" || !rec.HasAttr("error") {
			t.Errorf("switch record = %+v, want result refused with the error", rec)
		}
	})
}

func TestDetachToPicker(t *testing.T) {
	t.Run("it asks every runner on the session, and no other", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		first, second, elsewhere := &handler{}, &handler{}, &handler{}
		register(t, r, "first1", "api-x7Kd9a", first)
		register(t, r, "second", "api-x7Kd9a", second)
		register(t, r, "elsewh", "blog-c3d4", elsewhere)

		accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

		if err != nil || accepted != 2 {
			t.Errorf("DetachToPicker = %d, %v; want both runners on the session", accepted, err)
		}
		for name, h := range map[string]*handler{"first": first, "second": second} {
			if _, detaches := h.asked(); detaches != 1 {
				t.Errorf("the %s runner detached %d times, want once", name, detaches)
			}
		}
		if _, detaches := elsewhere.asked(); detaches != 0 {
			t.Errorf("a runner on another session detached %d times", detaches)
		}
	})

	t.Run("it counts only acceptances", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		register(t, r, "first1", "api-x7Kd9a", &handler{})
		register(t, r, "second", "api-x7Kd9a", &handler{})
		register(t, r, "third1", "api-x7Kd9a", &handler{refuse: errors.New("the runner is not attached")})

		if accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err != nil || accepted != 2 {
			t.Errorf("DetachToPicker = %d, %v; want the two acceptances", accepted, err)
		}
	})

	t.Run("none accepting is every runner's refusal or failure, and not ErrNoRunner", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		register(t, r, "first1", "api-x7Kd9a", &handler{refuse: errors.New("the runner is not attached")})
		register(t, r, "second", "api-x7Kd9a", &handler{refuse: errors.New("the attach is ending")})
		writeEntry(t, r, "hangup", entryFor(os.Getpid(), "api-x7Kd9a"))
		hangingUpSocket(t, r.SocketPath("hangup"))

		accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

		if accepted != 0 || !errors.Is(err, runnertoken.ErrRefused) || errors.Is(err, runnertoken.ErrNoRunner) {
			t.Fatalf("DetachToPicker = %d, %v; want none accepted, refused, and not ErrNoRunner", accepted, err)
		}
		if reasons := reasonsOf(t, err); len(reasons) != 3 {
			t.Errorf("DetachToPicker gave %d reasons, %q; want one for each runner", len(reasons), reasons)
		}
		for _, why := range []string{"the runner is not attached", "the attach is ending"} {
			if !strings.Contains(err.Error(), why) {
				t.Errorf("DetachToPicker = %q, want it to say %q", err, why)
			}
		}
	})

	t.Run("a runner yet to answer costs the others nothing: each is asked at once, under its own deadline", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		accepting := &handler{}
		register(t, r, "accept", "api-x7Kd9a", accepting)
		// Each gated runner answers only once all have been asked, which
		// none would be if the asking waited for each answer in turn.
		tokens := []string{"gated1", "gated2", "gated3"}
		all, arrivals := newBarrier(len(tokens)), make(chan arrival, len(tokens))
		for _, token := range tokens {
			writeEntry(t, r, token, entryFor(os.Getpid(), "api-x7Kd9a"))
			gatedSocket(t, r.SocketPath(token), all, arrivals)
		}
		start := time.Now()

		accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

		if err != nil || accepted != 4 {
			t.Errorf("DetachToPicker = %d, %v; want every runner's acceptance", accepted, err)
		}
		for range tokens {
			a := <-arrivals
			d := deadlineOf(t, a.line)
			if d.Before(start.Add(runnerreg.Timeout())) || d.After(a.at.Add(runnerreg.Timeout())) {
				t.Errorf("a request read %v into the asking has a deadline %v into it, want %v from when it was sent", a.at.Sub(start), d.Sub(start), runnerreg.Timeout())
			}
		}
	})

	t.Run("a runner gone as it is asked is no runner", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			beside *handler
			check  func(t *testing.T, accepted int, err error)
		}{
			{"alone: ErrNoRunner, for the backend's detach", nil, func(t *testing.T, accepted int, err error) {
				if accepted != 0 || !errors.Is(err, runnertoken.ErrNoRunner) {
					t.Errorf("DetachToPicker = %d, %v; want ErrNoRunner", accepted, err)
				}
			}},
			{"beside one accepting: the acceptance", &handler{}, func(t *testing.T, accepted int, err error) {
				if accepted != 1 || err != nil {
					t.Errorf("DetachToPicker = %d, %v; want the one acceptance", accepted, err)
				}
			}},
			{"beside one refusing: the refusal alone, and not ErrNoRunner", &handler{refuse: errors.New("the runner is not attached")}, func(t *testing.T, accepted int, err error) {
				if accepted != 0 || !errors.Is(err, runnertoken.ErrRefused) || errors.Is(err, runnertoken.ErrNoRunner) {
					t.Errorf("DetachToPicker = %d, %v; want the refusal, and not ErrNoRunner", accepted, err)
				}
				if reasons := reasonsOf(t, err); len(reasons) != 1 {
					t.Errorf("DetachToPicker gave reasons %q, want the refusal alone", reasons)
				}
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				r := runnerreg.New(stateDir(t))
				if tc.beside != nil {
					register(t, r, "beside", "api-x7Kd9a", tc.beside)
				}
				gone := otherPID(t)
				writeEntry(t, r, "gone01", entryFor(gone, "api-x7Kd9a"))
				exitsOnceScanned(t, gone)

				accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

				tc.check(t, accepted, err)
				if exists(t, r.EntryPath("gone01")) {
					t.Error("the gone runner's entry was left")
				}
			})
		}
	})

	t.Run("the caller's deadline caps every runner's", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		first, second := &handler{}, &handler{}
		register(t, r, "first1", "api-x7Kd9a", first)
		register(t, r, "second", "api-x7Kd9a", second)
		ctx, cancel := contextWithin(t, runnerreg.Timeout())
		defer cancel()
		end, _ := ctx.Deadline()

		if accepted, err := r.DetachToPicker(ctx, "api-x7Kd9a"); err != nil || accepted != 2 {
			t.Fatalf("DetachToPicker = %d, %v; want both runners' acceptances", accepted, err)
		}
		for name, h := range map[string]*handler{"first": first, "second": second} {
			if got, want := h.askedDeadlines(), end.Add(-runnerreg.AnswerGrace); len(got) != 1 || !got[0].Equal(want) {
				t.Errorf("the %s runner was given deadlines %v, want %v, the caller's less the answer's grace", name, got, want)
			}
		}
	})

	t.Run("no runner on the session is ErrNoRunner, and says so", func(t *testing.T) {
		sink := logtest.Install(t)

		if accepted, err := runnerreg.New(stateDir(t)).DetachToPicker(t.Context(), "api-x7Kd9a"); !errors.Is(err, runnertoken.ErrNoRunner) || accepted != 0 {
			t.Errorf("DetachToPicker = %d, %v; want none, ErrNoRunner, from a registry never made", accepted, err)
		}
		rec := sink.Records().Matching("attach", "detach to picker").Only(t, "detach to picker record")
		if got := rec.AttrString(t, "result"); got != "no runner" {
			t.Errorf("result = %q, want no runner", got)
		}
	})

	t.Run("a registry it can't read fails", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if err := os.WriteFile(r.Dir(), nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); err == nil {
			t.Error("DetachToPicker = nil, want the failed read")
		}
	})
}
