package runnerreg_test

import (
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/runnertoken"
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

			if pid, err := r.ConfirmSession(t.Context(), token, session); !errors.Is(err, runnertoken.ErrNoRunner) || pid != 0 {
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

		if pid, err := r.ConfirmSession(t.Context(), "tok123", "api-x7Kd9a"); !errors.Is(err, runnertoken.ErrRefused) || pid != 0 {
			t.Errorf("ConfirmSession = %d, %v; want the refusal", pid, err)
		}
	})

	t.Run("a runner whose pid isn't running is ErrNoRunner, and its entry goes", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "tok123", entryFor(deadPID(t), "api-x7Kd9a"))

		if _, err := r.ConfirmSession(t.Context(), "tok123", "api-x7Kd9a"); !errors.Is(err, runnertoken.ErrNoRunner) {
			t.Errorf("ConfirmSession = %v, want ErrNoRunner", err)
		}
		if exists(t, r.EntryPath("tok123")) {
			t.Error("the dead runner's entry is still there")
		}
	})

	t.Run("a read-only registry finds a dead runner's entry no runner, and leaves it", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "tok123", entryFor(deadPID(t), "api-x7Kd9a"))
		refusingSocket(t, r.SocketPath("tok123"))

		if _, err := r.ReadOnly().ConfirmSession(t.Context(), "tok123", "api-x7Kd9a"); !errors.Is(err, runnertoken.ErrNoRunner) {
			t.Errorf("ConfirmSession = %v, want ErrNoRunner", err)
		}
		if got, want := files(t, r.Dir()), []string{"tok123.json", "tok123.sock"}; !slices.Equal(got, want) {
			t.Errorf("the registry holds %q, want the dead runner's %q left", got, want)
		}
	})
}

func TestListing_ConfirmSession(t *testing.T) {
	t.Run("a request to confirm reaches the attach naming the session asked of, carrying the caller's deadline", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		before := time.Now()
		confirmed := make(chan error, 1)
		go func() {
			_, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a")
			confirmed <- err
		}()

		req := a.take(t, nil)
		if req.Confirm != "api-x7Kd9a" || req.Switch != "" || req.Deadline.Before(before.Add(runnerreg.Timeout())) {
			t.Errorf("the attach took %+v, want a request to confirm api-x7Kd9a due %v from the ask", req, runnerreg.Timeout())
		}
		if err := <-confirmed; err != nil {
			t.Errorf("ConfirmSession = %v, want the attach's confirmation", err)
		}
	})

	t.Run("the attach's refusal is the runner's", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		a := begun(listing(t, r), "api-x7Kd9a")

		confirmed := make(chan error, 1)
		go func() {
			_, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a")
			confirmed <- err
		}()
		a.take(t, runner.ErrUnconfirmed)

		if err := <-confirmed; !errors.Is(err, runnertoken.ErrRefused) || !strings.Contains(err.Error(), runner.ErrUnconfirmed.Error()) {
			t.Errorf("ConfirmSession = %v, want the attach's refusal", err)
		}
	})

	t.Run("an attach that has ended refuses", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		l := listing(t, r)
		a := begun(l, "api-x7Kd9a")

		l.Ended()

		if _, err := r.ConfirmSession(t.Context(), a.token(), "api-x7Kd9a"); !errors.Is(err, runnertoken.ErrRefused) {
			t.Errorf("ConfirmSession = %v, want the ended attach's refusal", err)
		}
	})
}
