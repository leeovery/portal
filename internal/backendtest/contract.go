package backendtest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

// Subject is one backend under Contract, with the means to put sessions in
// each state the contract covers. Contract makes a fresh one for every case.
type Subject interface {
	// Backend is the backend under test, as the latest SetCurrent left it.
	Backend() backend.Backend
	// Answering holds a session that answers every call, under any name a
	// session can have, whatever Create would make of it.
	Answering(t *testing.T, name string)
	// Silent holds a session that takes every call and never answers, as a
	// stopped or wedged daemon does.
	Silent(t *testing.T, name string)
	// Refusing holds a session that turns every call away, as a daemon with a
	// full listen backlog does.
	Refusing(t *testing.T, name string)
	// SetCurrent makes the backend run inside the named session, "" outside
	// any.
	SetCurrent(name string)
	// Lacks says why the backend cannot show a behaviour, "" when it can, so
	// Contract skips that case by name instead of leaving it out.
	Lacks(b Behaviour) string
}

// Behaviour is one that a backend may be unable to show.
type Behaviour int

const (
	// FoldsCase is holding a name under another case of it.
	FoldsCase Behaviour = iota
	// FoldsNormalisation is holding a name under another Unicode
	// normalisation of it.
	FoldsNormalisation
)

// perSession is each call but Exists that acts on one held session, Kill last
// since it ends the session.
var perSession = []struct {
	name string
	call func(context.Context, backend.Backend, string) error
}{
	{"AttachCommand", func(ctx context.Context, b backend.Backend, name string) error {
		_, err := b.AttachCommand(ctx, name)
		return err
	}},
	{"History", func(ctx context.Context, b backend.Backend, name string) error {
		_, err := b.History(ctx, name, 0)
		return err
	}},
	{"Detach", func(ctx context.Context, b backend.Backend, name string) error { return b.Detach(ctx, name) }},
	{"Kill", func(ctx context.Context, b backend.Backend, name string) error { return b.Kill(ctx, name) }},
}

// Contract checks the behaviours every backend.Backend shares, the driver's
// and the Fake's alike, against fresh subjects from newSubject.
func Contract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("a session that answers", func(t *testing.T) { answeringContract(t, newSubject) })
	t.Run("a session that does not answer", func(t *testing.T) { notAnsweringContract(t, newSubject) })
	t.Run("a session that is gone", func(t *testing.T) { goneContract(t, newSubject) })
	t.Run("a name held only under another spelling", func(t *testing.T) { spellingContract(t, newSubject) })
	t.Run("the name rules", func(t *testing.T) { nameContract(t, newSubject) })
	t.Run("a caller context that has ended", func(t *testing.T) { endedContract(t, newSubject) })
	t.Run("an attach from inside a session", func(t *testing.T) { switchContract(t, newSubject) })
}

func answeringContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("it is listed as answering and reached by every call", func(t *testing.T) {
		s := newSubject(t)
		s.Answering(t, "work")
		b, ctx := s.Backend(), t.Context()

		if got := listed(t, b, "work"); !got.Answered {
			t.Errorf("work listed as %+v, want it answering", got)
		}
		if exists, err := b.Exists(ctx, "work"); err != nil || !exists {
			t.Errorf("Exists = %v, %v; want true", exists, err)
		}
		if cmd, err := b.AttachCommand(ctx, "work"); err != nil || len(cmd.Argv) == 0 || cmd.Switches {
			t.Errorf("AttachCommand outside any session = %+v, %v; want an attach", cmd, err)
		}
		if vt, err := b.History(ctx, "work", 0); err != nil || len(vt) == 0 {
			t.Errorf("History = %q, %v; want the screen", vt, err)
		}
		if err := b.Detach(ctx, "work"); err != nil {
			t.Errorf("Detach = %v", err)
		}
	})

	t.Run("it is refused to Create, which names it", func(t *testing.T) {
		s := newSubject(t)
		s.Answering(t, "work")

		assertExists(t, s.Backend().Create(t.Context(), "work", t.TempDir(), nil), "work")
	})

	t.Run("it ends on Kill, freeing its name", func(t *testing.T) {
		s := newSubject(t)
		s.Answering(t, "work")
		b, ctx := s.Backend(), t.Context()

		if err := b.Kill(ctx, "work"); err != nil {
			t.Fatalf("Kill = %v", err)
		}
		if exists, err := b.Exists(ctx, "work"); err != nil || exists {
			t.Errorf("Exists after Kill = %v, %v; want false", exists, err)
		}
		if err := b.Kill(ctx, "work"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("a second Kill = %v, want ErrNoSuchSession", err)
		}
		if got := names(t, b); len(got) != 0 {
			t.Errorf("List after Kill = %v, want none", got)
		}
	})
}

func notAnsweringContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	for _, state := range []struct {
		name string
		hold func(Subject, *testing.T, string)
	}{
		{"silent", Subject.Silent},
		{"refusing", Subject.Refusing},
	} {
		t.Run(state.name+": it is listed by name only, fails every call with ErrTimeout, and keeps its name from Create", func(t *testing.T) {
			s := newSubject(t)
			state.hold(s, t, "wedged")
			b, ctx := s.Backend(), t.Context()

			if got := listed(t, b, "wedged"); got != (backend.Session{Name: "wedged"}) {
				t.Errorf("wedged listed as %+v, want only its name", got)
			}
			if _, err := b.Exists(ctx, "wedged"); !errors.Is(err, backend.ErrTimeout) {
				t.Errorf("Exists = %v, want ErrTimeout", err)
			}
			for _, op := range perSession {
				if err := op.call(ctx, b, "wedged"); !errors.Is(err, backend.ErrTimeout) || errors.Is(err, backend.ErrNoSuchSession) {
					t.Errorf("%s = %v, want ErrTimeout and not ErrNoSuchSession", op.name, err)
				}
			}
			assertExists(t, b.Create(ctx, "wedged", t.TempDir(), nil), "wedged")
			if got := listed(t, b, "wedged"); got.Answered {
				t.Errorf("wedged after the calls = %+v, want it still held, not answering", got)
			}
		})
	}
}

func goneContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("it is not listed, does not exist, and every call on it is ErrNoSuchSession", func(t *testing.T) {
		s := newSubject(t)
		s.Answering(t, "other")
		b, ctx := s.Backend(), t.Context()

		if got := names(t, b); !slices.Equal(got, []string{"other"}) {
			t.Errorf("List = %v, want only other", got)
		}
		if exists, err := b.Exists(ctx, "gone"); err != nil || exists {
			t.Errorf("Exists = %v, %v; want false", exists, err)
		}
		for _, op := range perSession {
			if err := op.call(ctx, b, "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
				t.Errorf("%s = %v, want ErrNoSuchSession", op.name, err)
			}
		}
	})
}

func spellingContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	for _, spelling := range []struct {
		name        string
		behaviour   Behaviour
		held, asked string
	}{
		{"case", FoldsCase, "work", "WORK"},
		{"normalisation", FoldsNormalisation, "café", "café"},
	} {
		t.Run(spelling.name+": no call reaches it under the other spelling, and Create is refused naming it", func(t *testing.T) {
			s := newSubject(t)
			if why := s.Lacks(spelling.behaviour); why != "" {
				t.Skip(why)
			}
			s.Answering(t, spelling.held)
			b, ctx := s.Backend(), t.Context()

			if exists, err := b.Exists(ctx, spelling.asked); err != nil || exists {
				t.Errorf("Exists(%q) = %v, %v; want false", spelling.asked, exists, err)
			}
			for _, op := range perSession {
				if err := op.call(ctx, b, spelling.asked); !errors.Is(err, backend.ErrNoSuchSession) {
					t.Errorf("%s(%q) = %v, want ErrNoSuchSession", op.name, spelling.asked, err)
				}
			}
			assertExists(t, b.Create(ctx, spelling.asked, t.TempDir(), nil), spelling.held)
			if exists, err := b.Exists(ctx, spelling.held); err != nil || !exists {
				t.Errorf("Exists(%q) after the calls = %v, %v; want it still answering", spelling.held, exists, err)
			}
		})
	}
}

func nameContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("lookup: every call refuses a name no session can have", func(t *testing.T) {
		b, ctx := newSubject(t).Backend(), t.Context()
		for _, name := range []string{"", ".", "..", "a/b", "a\x00b"} {
			if exists, err := b.Exists(ctx, name); err != nil || exists {
				t.Errorf("Exists(%q) = %v, %v; want false", name, exists, err)
			}
			for _, op := range perSession {
				if err := op.call(ctx, b, name); !errors.Is(err, backend.ErrInvalidName) {
					t.Errorf("%s(%q) = %v, want ErrInvalidName", op.name, name, err)
				}
			}
			if err := b.Create(ctx, name, t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("Create(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("attach: a session named as zmx attach's own flag is reached by every call but AttachCommand", func(t *testing.T) {
		s := newSubject(t)
		s.Answering(t, "--labels")
		b, ctx := s.Backend(), t.Context()

		if exists, err := b.Exists(ctx, "--labels"); err != nil || !exists {
			t.Errorf("Exists = %v, %v; want true", exists, err)
		}
		for _, op := range perSession {
			err := op.call(ctx, b, "--labels")
			if refused := errors.Is(err, backend.ErrInvalidName); refused != (op.name == "AttachCommand") || (!refused && err != nil) {
				t.Errorf("%s = %v; want ErrInvalidName from AttachCommand alone", op.name, err)
			}
		}
	})

	t.Run("creation: a session made elsewhere under a name Create refuses is reached by every call", func(t *testing.T) {
		for _, name := range []string{"-x", "x*", "a\nb"} {
			s := newSubject(t)
			s.Answering(t, name)
			b, ctx := s.Backend(), t.Context()

			if exists, err := b.Exists(ctx, name); err != nil || !exists {
				t.Errorf("Exists(%q) = %v, %v; want true", name, exists, err)
			}
			for _, op := range perSession {
				if err := op.call(ctx, b, name); err != nil {
					t.Errorf("%s(%q) = %v", op.name, name, err)
				}
			}
		}
	})

	t.Run("creation: Create and ValidateName refuse what zmx misreads or cannot list", func(t *testing.T) {
		b := newSubject(t).Backend()
		for _, name := range []string{"logs", "-x", "x*", "a\nb", "bad\xffname"} {
			if err := b.ValidateName(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
			}
			if err := b.Create(t.Context(), name, t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("Create(%q) = %v, want ErrInvalidName", name, err)
			}
		}
		for _, name := range []string{"work", "_portal-saver", "a.b", "x*y"} {
			if err := b.ValidateName(name); err != nil {
				t.Errorf("ValidateName(%q) = %v, want nil", name, err)
			}
		}
	})
}

func endedContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("every call fails with the caller's cause, changing nothing", func(t *testing.T) {
		s := newSubject(t)
		s.Answering(t, "work")
		b := s.Backend()
		cause := errors.New("caller gave up")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)

		if _, err := b.List(ctx); !errors.Is(err, cause) {
			t.Errorf("List = %v, want the cause", err)
		}
		if err := b.Create(ctx, "new", t.TempDir(), nil); !errors.Is(err, cause) {
			t.Errorf("Create = %v, want the cause", err)
		}
		if _, err := b.Exists(ctx, "work"); !errors.Is(err, cause) {
			t.Errorf("Exists = %v, want the cause", err)
		}
		for _, op := range perSession {
			if err := op.call(ctx, b, "work"); !errors.Is(err, cause) {
				t.Errorf("%s = %v, want the cause", op.name, err)
			}
		}
		if got := names(t, b); !slices.Equal(got, []string{"work"}) {
			t.Errorf("List after the calls = %v, want work alone, still held", got)
		}
	})

	t.Run("List with no session to ask lists none", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(errors.New("caller gave up"))

		if got, err := newSubject(t).Backend().List(ctx); err != nil || got == nil || len(got) != 0 {
			t.Errorf("List = %#v, %v; want an empty, non-nil list", got, err)
		}
	})
}

func switchContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	for _, tt := range []struct {
		current  string
		switches bool
	}{
		{"home", true},
		{"work", true},
		{"wedged", false},
		{"gone", false},
		{"", false},
	} {
		t.Run("inside "+quoted(tt.current), func(t *testing.T) {
			s := newSubject(t)
			s.Answering(t, "work")
			s.Answering(t, "home")
			s.Silent(t, "wedged")
			s.SetCurrent(tt.current)

			cmd, err := s.Backend().AttachCommand(t.Context(), "work")
			if err != nil || cmd.Switches != tt.switches {
				t.Errorf("AttachCommand(work) = %+v, %v; want Switches %v", cmd, err, tt.switches)
			}
		})
	}
}

func listed(t *testing.T, b backend.Backend, name string) backend.Session {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("List = %v", err)
	}
	i := slices.IndexFunc(sessions, func(s backend.Session) bool { return s.Name == name })
	if i < 0 {
		t.Fatalf("%q not in List %+v", name, sessions)
	}
	return sessions[i]
}

func names(t *testing.T, b backend.Backend) []string {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("List = %v", err)
	}
	out := []string{}
	for _, s := range sessions {
		out = append(out, s.Name)
	}
	return out
}

func assertExists(t *testing.T, err error, held string) {
	t.Helper()
	if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != held {
		t.Errorf("Create = %v, want a SessionExistsError held as %q", err, held)
	}
}

func quoted(name string) string {
	if name == "" {
		return "no session"
	}
	return `"` + name + `"`
}
