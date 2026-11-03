package runnerreg_test

import (
	"errors"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
)

func TestConfirmSession(t *testing.T) {
	t.Run("it returns the pid of the runner the token names once it confirms it shows the session", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		h := &handler{}
		register(t, r, "tok123", "api-x7Kd9a", h)

		pid, err := r.ConfirmSession(t.Context(), "tok123", "api-x7Kd9a")

		if err != nil || pid != os.Getpid() {
			t.Fatalf("ConfirmSession = %d, %v; want this runner's pid %d", pid, err, os.Getpid())
		}
		if asked := h.askedConfirms(); !slices.Equal(asked, []string{"api-x7Kd9a"}) {
			t.Errorf("the runner was asked whether it shows %q, want api-x7Kd9a", asked)
		}
		rec := sink.Records().Matching("attach", "confirm session").AtExactLevel(slog.LevelInfo).Only(t, "confirm session record")
		if rec.AttrString(t, "session") != "api-x7Kd9a" || rec.AttrString(t, "result") != "accepted" || rec.IntAttr(t, "target_pid") != int64(os.Getpid()) {
			t.Errorf("confirm session record = %+v, want the session, accepted, naming the runner", rec)
		}
	})

	t.Run("a token the registry doesn't hold, or one naming a runner on another session, is ErrNoRunner, asking no one", func(t *testing.T) {
		for token, session := range map[string]string{"": "api-x7Kd9a", "nosuch": "api-x7Kd9a", "../tok123": "api-x7Kd9a", "tok123": "blog-c3d4"} {
			r := runnerreg.New(stateDir(t))
			h := &handler{}
			register(t, r, "tok123", "api-x7Kd9a", h)

			if pid, err := r.ConfirmSession(t.Context(), token, session); !errors.Is(err, runnerreg.ErrNoRunner) || pid != 0 {
				t.Errorf("ConfirmSession(%q, %s) = %d, %v; want ErrNoRunner", token, session, pid, err)
			}
			if asked := h.askedConfirms(); len(asked) != 0 {
				t.Errorf("ConfirmSession(%q, %s) asked the runner about %q", token, session, asked)
			}
		}
	})

	t.Run("a runner's refusal is returned, with no pid", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		register(t, r, "tok123", "api-x7Kd9a", &handler{refuse: runner.ErrUnconfirmed})

		if pid, err := r.ConfirmSession(t.Context(), "tok123", "api-x7Kd9a"); !errors.Is(err, runnerreg.ErrRefused) || pid != 0 {
			t.Errorf("ConfirmSession = %d, %v; want the refusal", pid, err)
		}
	})

	t.Run("a runner whose pid isn't running is ErrNoRunner, and its entry goes", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "tok123", entryFor(deadPID(t), "api-x7Kd9a"))

		if _, err := r.ConfirmSession(t.Context(), "tok123", "api-x7Kd9a"); !errors.Is(err, runnerreg.ErrNoRunner) {
			t.Errorf("ConfirmSession = %v, want ErrNoRunner", err)
		}
		if exists(t, r.EntryPath("tok123")) {
			t.Error("the dead runner's entry is still there")
		}
	})
}

func TestListing_ConfirmSession(t *testing.T) {
	t.Run("a listed attach confirms the session it shows, ending nothing", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		if pid, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a"); err != nil || pid != os.Getpid() {
			t.Errorf("ConfirmSession = %d, %v; want this runner", pid, err)
		}
		a.untaken(t, 0)
	})

	t.Run("an attach that has seen a switch is unsure, and refuses", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		a.cfg.Unsure()

		if _, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a"); !errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("ConfirmSession = %v, want the unsure runner's refusal", err)
		}
	})

	t.Run("the next attach, after a switch, is sure again", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		first := begun(l, "api-x7Kd9a")
		first.cfg.Unsure()
		l.Ended()

		next := begun(l, "blog-c3d4")

		if _, err := r.ConfirmSession(t.Context(), next.token(), "blog-c3d4"); err != nil {
			t.Errorf("ConfirmSession = %v, want the new attach's confirmation", err)
		}
	})

	t.Run("an attach on another session than the one asked of refuses, whatever its entry says", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "blog-c3d4")
		writeEntry(t, r, a.token(), entryFor(os.Getpid(), "api-x7Kd9a"))

		if _, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a"); !errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("ConfirmSession = %v, want the runner's refusal", err)
		}
	})

	t.Run("an attach that has ended refuses", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")

		l.Ended()

		if _, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a"); !errors.Is(err, runnerreg.ErrRefused) {
			t.Errorf("ConfirmSession = %v, want the ended attach's refusal", err)
		}
	})
}
