package runnerreg_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
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
		runnerreg.SetTimeout(t, 100*time.Millisecond)
		r := runnerreg.New(stateDir(t))
		register(t, r, "first1", "api-x7Kd9a", &handler{refuse: errors.New("the runner is not attached")})
		register(t, r, "second", "api-x7Kd9a", &handler{refuse: errors.New("the attach is ending")})
		writeEntry(t, r, "silent", entryFor(os.Getpid(), "api-x7Kd9a"))
		silentSocket(t, r.SocketPath("silent"))

		accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

		if accepted != 0 || !errors.Is(err, runnertoken.ErrRefused) || errors.Is(err, runnertoken.ErrNoRunner) {
			t.Fatalf("DetachToPicker = %d, %v; want none accepted, refused, and not ErrNoRunner", accepted, err)
		}
		for _, why := range []string{"the runner is not attached", "the attach is ending"} {
			if !strings.Contains(err.Error(), why) {
				t.Errorf("DetachToPicker = %q, want it to say %q", err, why)
			}
		}
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Errorf("DetachToPicker = %q, want the silent runner's timeout among the reasons", err)
		}
	})

	t.Run("a runner that never answers costs the others nothing: each request is sent at once, under its own deadline", func(t *testing.T) {
		runnerreg.SetTimeout(t, 300*time.Millisecond)
		r := runnerreg.New(stateDir(t))
		accepting := &handler{}
		register(t, r, "accept", "api-x7Kd9a", accepting)
		requests := make(chan string, 3)
		for _, token := range []string{"stall1", "stall2", "stall3"} {
			writeEntry(t, r, token, entryFor(os.Getpid(), "api-x7Kd9a"))
			stallingSocket(t, r.SocketPath(token), requests)
		}
		start := time.Now()

		accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a")

		if err != nil || accepted != 1 {
			t.Errorf("DetachToPicker = %d, %v; want the answering runner's acceptance", accepted, err)
		}
		deadlines := accepting.askedDeadlines()
		for range 3 {
			deadlines = append(deadlines, deadlineOf(t, <-requests))
		}
		// Asked in turn, a request sent after a stalled one would carry a
		// deadline past the stalled one's wait for its answer.
		for _, d := range deadlines {
			if !d.Before(start.Add(runnerreg.Timeout() + runnerreg.AnswerGrace)) {
				t.Errorf("a request's deadline is %v after the asking began, want each within its own %v", d.Sub(start), runnerreg.Timeout())
			}
		}
	})

	t.Run("the caller's deadline caps every runner's", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		first, second := &handler{}, &handler{}
		register(t, r, "first1", "api-x7Kd9a", first)
		register(t, r, "second", "api-x7Kd9a", second)
		ctx, cancel := contextWithin(t, runnerreg.Timeout()/2+runnerreg.AnswerGrace)
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
