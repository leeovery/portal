package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

// helperCommand is the command a flag sweep's RunsHelper counts as restore's
// helper.
const helperCommand = "/opt/portal/bin/portal state hydrate --session"

// flagWorld is a state directory of the test's own and the sessions a sweep
// over it finds live.
type flagWorld struct {
	dir  string
	live []backend.Session
}

func newFlagWorld(t *testing.T, live ...backend.Session) *flagWorld {
	t.Helper()
	return &flagWorld{dir: t.TempDir(), live: live}
}

func (w *flagWorld) raise(t *testing.T, flag state.SessionFlag, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := flag.Set(w.dir, name); err != nil {
			t.Fatal(err)
		}
	}
}

func (w *flagWorld) raised(t *testing.T, flag state.SessionFlag) []string {
	t.Helper()
	names, err := flag.List(w.dir)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func (w *flagWorld) sweeper() *FlagSweepCore {
	return &FlagSweepCore{
		Dir:        w.dir,
		List:       func(context.Context) ([]backend.Session, error) { return w.live, nil },
		RunsHelper: func(s backend.Session) bool { return s.Command == helperCommand },
	}
}

func TestSweepStaleFlags(t *testing.T) {
	t.Run("it lowers both flags of a session that isn't live", func(t *testing.T) {
		w := newFlagWorld(t)
		w.raise(t, state.Hydrating, "gone")
		w.raise(t, state.ResumePending, "gone")

		if err := w.sweeper().SweepStaleFlags(t.Context()); err != nil {
			t.Fatalf("SweepStaleFlags: %v", err)
		}

		for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
			if got := w.raised(t, flag); len(got) != 0 {
				t.Errorf("%s flags = %v, want the gone session's lowered", flag, got)
			}
		}
	})

	t.Run("it lowers a hydrating flag on a live session that isn't restore's helper", func(t *testing.T) {
		w := newFlagWorld(t, backend.Session{Name: "made", Answered: true, Command: "vim notes.md"})
		w.raise(t, state.Hydrating, "made")

		if err := w.sweeper().SweepStaleFlags(t.Context()); err != nil {
			t.Fatalf("SweepStaleFlags: %v", err)
		}

		if got := w.raised(t, state.Hydrating); len(got) != 0 {
			t.Errorf("hydrating flags = %v, want the flag no helper will lower lowered", got)
		}
	})

	t.Run("it keeps what a live session's own helper or panel lowers, and a session it can't read", func(t *testing.T) {
		w := newFlagWorld(t,
			backend.Session{Name: "restoring", Answered: true, Command: helperCommand},
			backend.Session{Name: "waiting", Answered: true, Command: "vim notes.md"},
			backend.Session{Name: "silent"},
		)
		w.raise(t, state.Hydrating, "restoring", "silent")
		w.raise(t, state.ResumePending, "waiting", "silent")

		if err := w.sweeper().SweepStaleFlags(t.Context()); err != nil {
			t.Fatalf("SweepStaleFlags: %v", err)
		}

		if got, want := w.raised(t, state.Hydrating), []string{"restoring", "silent"}; !slices.Equal(got, want) {
			t.Errorf("hydrating flags = %v, want %v kept", got, want)
		}
		if got, want := w.raised(t, state.ResumePending), []string{"silent", "waiting"}; !slices.Equal(got, want) {
			t.Errorf("resume-pending flags = %v, want %v kept", got, want)
		}
	})

	t.Run("a listing that fails lowers nothing, and is the error", func(t *testing.T) {
		w := newFlagWorld(t)
		w.raise(t, state.Hydrating, "gone")
		sweeper := w.sweeper()
		sweeper.List = func(context.Context) ([]backend.Session, error) { return nil, errBoom }

		if err := sweeper.SweepStaleFlags(t.Context()); !errors.Is(err, errBoom) {
			t.Errorf("SweepStaleFlags = %v, want the listing's failure", err)
		}
		if got := w.raised(t, state.Hydrating); !slices.Equal(got, []string{"gone"}) {
			t.Errorf("hydrating flags = %v, want none lowered", got)
		}
	})

	t.Run("a sweep missing a seam lowers nothing, and says so", func(t *testing.T) {
		w := newFlagWorld(t)
		w.raise(t, state.Hydrating, "gone")
		sweeper := w.sweeper()
		sweeper.RunsHelper = nil

		if err := sweeper.SweepStaleFlags(t.Context()); err == nil {
			t.Error("SweepStaleFlags = nil, want it refused")
		}
		if got := w.raised(t, state.Hydrating); !slices.Equal(got, []string{"gone"}) {
			t.Errorf("hydrating flags = %v, want none lowered", got)
		}
	})

	t.Run("it logs each flag it lowered, and one summary, under clean", func(t *testing.T) {
		w := newFlagWorld(t, backend.Session{Name: "made", Answered: true})
		w.raise(t, state.Hydrating, "gone", "made")
		w.raise(t, state.ResumePending, "gone")
		sink := logtest.Install(t)

		if err := w.sweeper().SweepStaleFlags(t.Context()); err != nil {
			t.Fatalf("SweepStaleFlags: %v", err)
		}

		if got := sink.Records().Matching("clean", "stale flag lowered").AtExactLevel(slog.LevelDebug); len(got) != 3 {
			t.Errorf("lowered DEBUGs = %+v, want one per flag", got)
		}
		summary := sink.Records().Matching("clean", "stale-flag sweep complete").AtExactLevel(slog.LevelInfo).Only(t, "the summary")
		if got := summary.IntAttr(t, "unset"); got != 3 {
			t.Errorf("unset = %d, want 3", got)
		}
	})
}
