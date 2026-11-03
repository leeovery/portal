package runnerreg_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
)

// attachment is one attach a Listing readied, standing in for runner.Run:
// the test begins it and answers its requests as Run would.
type attachment struct {
	cfg runner.Config
}

func readied(l *runnerreg.Listing, session string) *attachment {
	a := &attachment{cfg: runner.Config{Session: session, Command: backend.Command{Argv: []string{"zmx", "attach", session}}}}
	l.Attach(&a.cfg)
	return a
}

// begun lists the runner as an attach that has begun.
func begun(l *runnerreg.Listing, session string) *attachment {
	a := readied(l, session)
	a.cfg.Began()
	return a
}

// token is the token the attach's command carries, "" for none.
func (a *attachment) token() string {
	for _, kv := range a.cfg.Command.Env {
		if v, ok := strings.CutPrefix(kv, runnerreg.TokenVar+"="); ok {
			return v
		}
	}
	return ""
}

// take waits for the attach's next request, as Run's wait for its end does,
// and answers it with answer.
func (a *attachment) take(t *testing.T, answer error) runner.Request {
	t.Helper()
	select {
	case req := <-a.cfg.Requests:
		req.Answer <- answer
		return req
	case <-time.After(5 * time.Second):
		t.Fatal("no request reached the attach")
		return runner.Request{}
	}
}

// untaken asserts no request reaches the attach within the wait. One that
// does is refused, so its asker is not left waiting.
func (a *attachment) untaken(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case req := <-a.cfg.Requests:
		req.Answer <- errors.New("refused by the test")
		t.Errorf("the attach was handed %+v", req)
	case <-time.After(within):
	}
}

// switchVia asks the runner the token names, on current, to switch to
// target, in the background.
func switchVia(t *testing.T, r runnerreg.Registry, token, current, target string) <-chan error {
	done := make(chan error, 1)
	go func() { done <- r.Switch(t.Context(), token, current, target) }()
	return done
}

func listing(t *testing.T, r runnerreg.Registry) *runnerreg.Listing {
	t.Helper()
	return listingOn(t, r, sessions())
}

// sessions is a backend holding the sessions the tests attach.
func sessions() *backendtest.Fake {
	return backendtest.New(backend.Session{Name: "api-x7Kd9a"}, backend.Session{Name: "blog-c3d4"})
}

// listingOn is a listing whose attaches b marks and confirms.
func listingOn(t *testing.T, r runnerreg.Registry, b *backendtest.Fake) *runnerreg.Listing {
	t.Helper()
	l := r.Listing(b)
	t.Cleanup(l.Close)
	return l
}

// leading makes the client carrying token lead session, as far as b says.
func leading(b *backendtest.Fake, session, token string) {
	b.SetLeaderEnv(session, backend.ClientEnv{{Name: "DISPLAY", Value: ":0"}, {Name: runnerreg.TokenVar, Value: token}})
}

func TestListing(t *testing.T) {
	t.Run("an attach's command carries the runner's token, and the attach is listed under it once it begins", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)

		a := readied(l, "api-x7Kd9a")
		token := a.token()
		if token == "" || !slices.Equal(a.cfg.Command.Argv, []string{"zmx", "attach", "api-x7Kd9a"}) {
			t.Fatalf("command = %+v, want the attach marked with %s", a.cfg.Command, runnerreg.TokenVar)
		}
		if names := files(t, r.Dir()); len(names) != 0 {
			t.Fatalf("the registry holds %q before the attach began, want nothing", names)
		}
		a.cfg.Began()

		data, err := os.ReadFile(r.EntryPath(token))
		if err != nil || string(data) != entryFor(os.Getpid(), "api-x7Kd9a") {
			t.Errorf("entry %s = %s, %v; want this runner on api-x7Kd9a", token, data, err)
		}
	})

	t.Run("a switch is accepted once the attach takes it, carrying the caller's deadline", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		before := time.Now()
		switched := switchVia(t, r, a.token(), "api-x7Kd9a", "blog-c3d4")

		req := a.take(t, nil)
		if req.Switch != "blog-c3d4" || req.Deadline.Before(before.Add(runnerreg.Timeout())) || req.Deadline.After(time.Now().Add(runnerreg.Timeout())) {
			t.Errorf("the attach took %+v, want a switch to blog-c3d4 due %v from the ask", req, runnerreg.Timeout())
		}
		if err := <-switched; err != nil {
			t.Errorf("Switch = %v, want the attach's acceptance", err)
		}
	})

	t.Run("the attach's refusal is the runner's, saying why", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		switched := switchVia(t, r, a.token(), "api-x7Kd9a", "blog-c3d4")
		a.take(t, runner.ErrUnconfirmed)

		if err := <-switched; !errors.Is(err, runnerreg.ErrRefused) || !strings.Contains(err.Error(), runner.ErrUnconfirmed.Error()) {
			t.Errorf("Switch = %v, want the attach's refusal", err)
		}
	})

	t.Run("a detach to the picker reaches the attach as a request naming no session", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		detached := make(chan int, 1)
		go func() {
			n, _ := r.DetachToPicker(t.Context(), "api-x7Kd9a")
			detached <- n
		}()

		if req := a.take(t, nil); req.Switch != "" || req.Deadline.IsZero() {
			t.Errorf("the attach took %+v, want a request for the picker, with a deadline", req)
		}
		if n := <-detached; n != 1 {
			t.Errorf("DetachToPicker = %d, want the attach's acceptance", n)
		}
	})

	t.Run("a request the attach never takes is refused once it ends", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")

		switched := switchVia(t, r, a.token(), "api-x7Kd9a", "blog-c3d4")
		// The request waits on the attach, which never takes it.
		time.Sleep(100 * time.Millisecond)
		l.Ended()

		if err := <-switched; !errors.Is(err, runnerreg.ErrRefused) || !strings.Contains(err.Error(), "not attached") {
			t.Errorf("Switch = %v, want a refusal: the attach ended", err)
		}
	})

	t.Run("a request is never handed to the attach once its deadline has passed", func(t *testing.T) {
		runnerreg.SetTimeout(t, 200*time.Millisecond)
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		switched := switchVia(t, r, a.token(), "api-x7Kd9a", "blog-c3d4")

		select {
		case err := <-switched:
			if !errors.Is(err, runnerreg.ErrRefused) {
				t.Errorf("Switch = %v, want the runner's refusal at the deadline", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Switch never returned")
		}
		a.untaken(t, 2*runnerreg.Timeout())
	})

	t.Run("between a switch's attaches the entry stays and refuses requests", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")
		l.Ended()

		if !exists(t, r.EntryPath(a.token())) {
			t.Fatal("the entry went between attaches")
		}
		if err := r.Switch(t.Context(), a.token(), "api-x7Kd9a", "blog-c3d4"); !errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("Switch = %v, want a refusal between attaches", err)
		}
	})

	t.Run("the next attach after a switch carries the same token and moves its entry to its session", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")
		l.Ended()

		b := begun(l, "blog-c3d4")

		if b.token() != a.token() {
			t.Fatalf("the next attach carries %q, want the same token, %q", b.token(), a.token())
		}
		if data, err := os.ReadFile(r.EntryPath(b.token())); err != nil || string(data) != entryFor(os.Getpid(), "blog-c3d4") {
			t.Errorf("entry = %s, %v; want it moved to blog-c3d4", data, err)
		}
		if names := files(t, r.Dir()); len(names) != 2 {
			t.Errorf("the registry holds %q, want the one entry and its socket", names)
		}
		switched := switchVia(t, r, b.token(), "blog-c3d4", "docs-e5f6")
		if req := b.take(t, nil); req.Switch != "docs-e5f6" {
			t.Errorf("the next attach took %+v, want the switch sent it", req)
		}
		if err := <-switched; err != nil {
			t.Errorf("Switch = %v, want the next attach's acceptance", err)
		}
	})

	t.Run("Close unlists the runner, and the next attach lists it again under a token of its own", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")
		l.Ended()

		l.Close()
		if names := files(t, r.Dir()); len(names) != 0 {
			t.Fatalf("the registry holds %q after Close, want nothing", names)
		}
		b := begun(l, "blog-c3d4")

		if b.token() == "" || b.token() == a.token() {
			t.Errorf("the next attach carries %q, want a token other than %q", b.token(), a.token())
		}
		if !exists(t, r.EntryPath(b.token())) {
			t.Error("the next attach was not listed")
		}
	})

	t.Run("an attach the registry can't list goes on unlisted, with a WARN", func(t *testing.T) {
		sink := logtest.Install(t)
		long := filepath.Join(stateDir(t), strings.Repeat("s", runnerreg.MaxSocketPath()))
		if err := os.MkdirAll(long, 0o700); err != nil {
			t.Fatal(err)
		}
		b := sessions()
		l := listingOn(t, runnerreg.New(long), b)

		a := begun(l, "api-x7Kd9a")

		rec := sink.Records().Matching("attach", "register failed").AtExactLevel(slog.LevelWarn).Only(t, "register failed record")
		if got := rec.AttrString(t, "session"); got != "api-x7Kd9a" {
			t.Errorf("session = %q, want api-x7Kd9a", got)
		}
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, runnerreg.ErrSocketPathTooLong) {
			t.Errorf("error = %v, want ErrSocketPathTooLong", err)
		}
		leading(b, "api-x7Kd9a", a.token())
		if err := a.cfg.Confirm(t.Context()); err != nil {
			t.Errorf("Confirm = %v, want the unlisted attach still confirmed by the token its client carries", err)
		}
	})

	t.Run("an attach the registry can't move goes on unlisted, never listed on the session it left", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")
		l.Ended()
		// An entry whose path is a directory can't be written over.
		if err := os.Remove(r.EntryPath(a.token())); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(r.EntryPath(a.token()), 0o700); err != nil {
			t.Fatal(err)
		}

		b := begun(l, "blog-c3d4")

		if names := files(t, r.Dir()); len(names) != 0 {
			t.Errorf("the registry holds %q, want the runner unlisted", names)
		}
		if err := r.Switch(t.Context(), a.token(), "blog-c3d4", "docs-e5f6"); !errors.Is(err, runnerreg.ErrNoRunner) {
			t.Errorf("Switch = %v, want ErrNoRunner", err)
		}
		b.untaken(t, 100*time.Millisecond)
		rec := sink.Records().Matching("attach", "register failed").AtExactLevel(slog.LevelWarn).Only(t, "register failed record")
		if got := rec.AttrString(t, "session"); got != "blog-c3d4" {
			t.Errorf("session = %q, want blog-c3d4", got)
		}
	})
}

func TestListing_Confirm(t *testing.T) {
	t.Run("an attach is confirmed while its client leads its session, carrying the runner's token", func(t *testing.T) {
		b := sessions()
		a := begun(listingOn(t, runnerreg.New(stateDir(t)), b), "api-x7Kd9a")
		leading(b, "api-x7Kd9a", a.token())

		if err := a.cfg.Confirm(t.Context()); err != nil {
			t.Errorf("Confirm = %v, want the attach confirmed", err)
		}
		if calls := b.CallsTo(backendtest.OpLeaderEnv); len(calls) != 1 || calls[0].Name != "api-x7Kd9a" {
			t.Errorf("LeaderEnv calls = %+v, want one for the attach's session", calls)
		}
	})

	t.Run("each attach is confirmed on its own session", func(t *testing.T) {
		b := sessions()
		l := listingOn(t, runnerreg.New(stateDir(t)), b)
		a := begun(l, "api-x7Kd9a")
		l.Ended()
		next := begun(l, "blog-c3d4")
		leading(b, "blog-c3d4", a.token())

		if err := next.cfg.Confirm(t.Context()); err != nil {
			t.Errorf("Confirm of the next attach = %v, want it confirmed on blog-c3d4", err)
		}
		if err := a.cfg.Confirm(t.Context()); err == nil {
			t.Error("Confirm of the attach that left api-x7Kd9a = nil, want it unconfirmed: nothing leads api-x7Kd9a")
		}
	})

	for _, tc := range []struct {
		name   string
		leader backend.ClientEnv
	}{
		{"no client leads the session", nil},
		{"the leader carries another runner's token", backend.ClientEnv{{Name: runnerreg.TokenVar, Value: "other1"}}},
		{"the leader carries no token", backend.ClientEnv{{Name: "DISPLAY", Value: ":0"}}},
		{"the leader unset the token", backend.ClientEnv{{Name: runnerreg.TokenVar, Unset: true}}},
	} {
		t.Run("an attach is unconfirmed when "+tc.name, func(t *testing.T) {
			b := sessions()
			a := begun(listingOn(t, runnerreg.New(stateDir(t)), b), "api-x7Kd9a")
			b.SetLeaderEnv("api-x7Kd9a", tc.leader)

			if err := a.cfg.Confirm(t.Context()); err == nil {
				t.Error("Confirm = nil, want the attach unconfirmed")
			}
		})
	}

	t.Run("an attach is unconfirmed when the backend can't say who leads, saying why", func(t *testing.T) {
		b := sessions()
		a := begun(listingOn(t, runnerreg.New(stateDir(t)), b), "api-x7Kd9a")
		leading(b, "api-x7Kd9a", a.token())
		b.Fail(backendtest.OpLeaderEnv, backend.ErrTimeout)

		if err := a.cfg.Confirm(t.Context()); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Confirm = %v, want the backend's ErrTimeout", err)
		}
	})
}
