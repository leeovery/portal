package backendtest

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

// Subject is one backend under Contract, with the means to put sessions in
// each state the contract covers. Contract makes a fresh one for every case.
type Subject interface {
	// Backend is the backend under test, as the latest SetCurrent left it.
	Backend() backend.Backend
	// Hold holds a session in state under name, which may be any name a
	// session can have, whatever Create would make of it, and returns the
	// session as List should report it. A zero PID there stands for any
	// positive one, and a zero Created for any time up to now: the backend
	// chooses those.
	Hold(t *testing.T, name string, state State) backend.Session
	// SetCurrent makes the backend run inside the named session, "" outside
	// any.
	SetCurrent(name string)
	// Lacks says why the backend cannot show a behaviour, "" when it can, so
	// Contract skips that case by name instead of leaving it out.
	Lacks(b Behaviour) string
}

// State is how a held session meets the calls made on it.
type State int

const (
	// Answering answers every call.
	Answering State = iota
	// Silent takes every call and never answers, as a stopped or wedged
	// daemon does.
	Silent
	// Refusing turns every call away, as a daemon with a full listen backlog
	// does.
	Refusing
	// HangingUp takes every call and hangs up on it without a well-formed
	// answer.
	HangingUp
	// Historyless answers every call but History, which it answers with no
	// screen.
	Historyless
)

// Behaviour is one that a backend may be unable to show.
type Behaviour int

const (
	// FoldsCase is holding a name under another case of it.
	FoldsCase Behaviour = iota
	// FoldsNormalisation is holding a name under another Unicode
	// normalisation of it.
	FoldsNormalisation
	// NonUTF8Names is holding a session under a name that is not UTF-8.
	NonUTF8Names
	// NameLengthLimit is refusing a name too long for the backend to hold.
	NameLengthLimit
	// Creating is making a session with Create.
	Creating
	// AttachEnvironment is an attach command that carries the environment it
	// runs in.
	AttachEnvironment
	// HangingUpSessions is holding a session in the HangingUp state.
	HangingUpSessions
	// HistorylessSessions is holding a session in the Historyless state.
	HistorylessSessions
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
	t.Run("a session with no screen to give", func(t *testing.T) { historylessContract(t, newSubject) })
	t.Run("a session that is gone", func(t *testing.T) { goneContract(t, newSubject) })
	t.Run("a name held only under another spelling", func(t *testing.T) { spellingContract(t, newSubject) })
	t.Run("the name rules", func(t *testing.T) { nameContract(t, newSubject) })
	t.Run("a session made by Create", func(t *testing.T) { creatingContract(t, newSubject) })
	t.Run("a caller context that has ended", func(t *testing.T) { endedContract(t, newSubject) })
	t.Run("an attach from inside a session", func(t *testing.T) { switchContract(t, newSubject) })
}

func answeringContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("it is listed with its details and reached by every call", func(t *testing.T) {
		s := newSubject(t)
		want := s.Hold(t, "work", Answering)
		b, ctx := s.Backend(), t.Context()

		assertListing(t, listed(t, b, "work"), want)
		if exists, err := b.Exists(ctx, "work"); err != nil || !exists {
			t.Errorf("Exists = %v, %v; want true", exists, err)
		}
		if cmd, err := b.AttachCommand(ctx, "work"); err != nil || len(cmd.Argv) == 0 || cmd.Switches {
			t.Errorf("AttachCommand outside any session = %+v, %v; want an attach", cmd, err)
		}
		for _, rows := range []int{0, 10} {
			if vt, err := b.History(ctx, "work", rows); err != nil || len(vt) == 0 {
				t.Errorf("History(%d rows) = %q, %v; want the screen", rows, vt, err)
			}
		}
		if err := b.Detach(ctx, "work"); err != nil {
			t.Errorf("Detach = %v", err)
		}
	})

	t.Run("it is refused to Create, which names it", func(t *testing.T) {
		s := newSubject(t)
		s.Hold(t, "work", Answering)

		_, err := s.Backend().Create(t.Context(), "work", t.TempDir(), nil)
		assertExists(t, err, "work")
	})

	t.Run("it ends on Kill, freeing its name", func(t *testing.T) {
		s := newSubject(t)
		s.Hold(t, "work", Answering)
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
		name  string
		state State
	}{
		{"silent", Silent},
		{"refusing", Refusing},
		{"hanging up", HangingUp},
	} {
		t.Run(state.name+": it is listed by name only, fails every call with ErrTimeout, and keeps its name from Create", func(t *testing.T) {
			s := newSubject(t)
			if why := s.Lacks(HangingUpSessions); state.state == HangingUp && why != "" {
				t.Skip(why)
			}
			s.Hold(t, "wedged", state.state)
			b, ctx := s.Backend(), t.Context()

			if got := listed(t, b, "wedged"); got != (backend.Session{Name: "wedged"}) {
				t.Errorf("wedged listed as %+v, want only its name", got)
			}
			if _, err := b.Exists(ctx, "wedged"); !errors.Is(err, backend.ErrTimeout) {
				t.Errorf("Exists = %v, want ErrTimeout", err)
			}
			for _, op := range perSession {
				// Detach cannot tell a hangup on DetachAll from one that
				// ignored it.
				if op.name == "Detach" && state.state == HangingUp {
					continue
				}
				if err := op.call(ctx, b, "wedged"); !errors.Is(err, backend.ErrTimeout) || errors.Is(err, backend.ErrNoSuchSession) {
					t.Errorf("%s = %v, want ErrTimeout and not ErrNoSuchSession", op.name, err)
				}
			}
			_, err := b.Create(ctx, "wedged", t.TempDir(), nil)
			assertExists(t, err, "wedged")
			if got := listed(t, b, "wedged"); got.Answered {
				t.Errorf("wedged after the calls = %+v, want it still held, not answering", got)
			}
		})
	}
}

func historylessContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("it answers every call but History, which is ErrHistoryUnavailable, never empty", func(t *testing.T) {
		s := newSubject(t)
		if why := s.Lacks(HistorylessSessions); why != "" {
			t.Skip(why)
		}
		want := s.Hold(t, "blank", Historyless)
		b, ctx := s.Backend(), t.Context()

		assertListing(t, listed(t, b, "blank"), want)
		if exists, err := b.Exists(ctx, "blank"); err != nil || !exists {
			t.Errorf("Exists = %v, %v; want true", exists, err)
		}
		for _, rows := range []int{0, 10} {
			if vt, err := b.History(ctx, "blank", rows); !errors.Is(err, backend.ErrHistoryUnavailable) {
				t.Errorf("History(%d rows) = %q, %v; want ErrHistoryUnavailable", rows, vt, err)
			}
		}
	})
}

func goneContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("it is not listed, does not exist, and every call on it is ErrNoSuchSession", func(t *testing.T) {
		s := newSubject(t)
		s.Hold(t, "other", Answering)
		b, ctx := s.Backend(), t.Context()

		if got := names(t, b); !slices.Equal(got, []string{"other"}) {
			t.Errorf("List = %v, want only other", got)
		}
		assertGone(t, ctx, b, "gone")
	})
}

func spellingContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	for _, spelling := range spellings {
		t.Run(spelling.name+": no call reaches it under the other spelling, and Create is refused naming it", func(t *testing.T) {
			s := newSubject(t)
			if why := s.Lacks(spelling.behaviour); why != "" {
				t.Skip(why)
			}
			s.Hold(t, spelling.held, Answering)
			b, ctx := s.Backend(), t.Context()

			assertGone(t, ctx, b, spelling.asked)
			assertHeldAs(t, ctx, b, spelling.asked, spelling.held)
			_, err := b.Create(ctx, spelling.asked, t.TempDir(), nil)
			assertExists(t, err, spelling.held)
			if exists, err := b.Exists(ctx, spelling.held); err != nil || !exists {
				t.Errorf("Exists(%q) after the calls = %v, %v; want it still answering", spelling.held, exists, err)
			}
		})
	}
}

// spellings are the ways a filesystem can hold one name under another.
var spellings = []struct {
	name        string
	behaviour   Behaviour
	held, asked string
}{
	{"case", FoldsCase, "work", "WORK"},
	{"normalisation", FoldsNormalisation, "café", "café"},
}

func nameContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("lookup: every call refuses a name no session can have", func(t *testing.T) {
		assertInvalid(t, t.Context(), newSubject(t).Backend(), "", ".", "..", "a/b", "a\x00b")
	})

	t.Run("length: every call refuses a name too long to hold", func(t *testing.T) {
		s := newSubject(t)
		if why := s.Lacks(NameLengthLimit); why != "" {
			t.Skip(why)
		}
		b := s.Backend()
		long := strings.Repeat("n", 300)

		assertInvalid(t, t.Context(), b, long)
		if err := b.ValidateName(long); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("ValidateName of %d bytes = %v, want ErrInvalidName", len(long), err)
		}
	})

	t.Run("attach: a session named as zmx attach's own flag is reached by every call but AttachCommand", func(t *testing.T) {
		s := newSubject(t)
		s.Hold(t, "--labels", Answering)
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
		for _, name := range []string{"-x", "--vt", "x*", "a\nb"} {
			s := newSubject(t)
			s.Hold(t, name, Answering)

			assertReached(t, t.Context(), s.Backend(), name)
		}
	})

	t.Run("lookup: a session under a name that is not UTF-8 is reached by every call", func(t *testing.T) {
		s := newSubject(t)
		if why := s.Lacks(NonUTF8Names); why != "" {
			t.Skip(why)
		}
		s.Hold(t, "bad\xffname", Answering)

		assertReached(t, t.Context(), s.Backend(), "bad\xffname")
	})

	t.Run("creation: Create and ValidateName refuse what zmx misreads or cannot list", func(t *testing.T) {
		b := newSubject(t).Backend()
		for _, name := range []string{"logs", "-x", "x*", "a\nb", "bad\xffname"} {
			if err := b.ValidateName(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
			}
			if _, err := b.Create(t.Context(), name, t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
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

func creatingContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	t.Run("it makes a session that answers, running the command in the directory", func(t *testing.T) {
		s := newSubject(t)
		if why := s.Lacks(Creating); why != "" {
			t.Skip(why)
		}
		b, ctx, dir := s.Backend(), t.Context(), t.TempDir()

		made, err := b.Create(ctx, "made", dir, []string{"sleep", "1000"})
		if err != nil {
			t.Fatalf("Create = %v", err)
		}

		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		got := listed(t, b, "made")
		assertListing(t, got, backend.Session{Name: "made", Answered: true, Dir: real, Command: "sleep 1000"})
		if made.Clients, got.Clients = 0, 0; made != got {
			t.Errorf("Create returned %+v, want the session as List reports it, %+v", made, got)
		}
		if exists, err := b.Exists(ctx, "made"); err != nil || !exists {
			t.Errorf("Exists = %v, %v; want true", exists, err)
		}
	})
}

func endedContract(t *testing.T, newSubject func(t *testing.T) Subject) {
	ended := func(t *testing.T) (context.Context, error) {
		cause := errors.New("caller gave up")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)
		return ctx, cause
	}

	t.Run("every call that must reach the session fails with the caller's cause, changing nothing", func(t *testing.T) {
		s := newSubject(t)
		s.Hold(t, "work", Answering)
		b := s.Backend()
		ctx, cause := ended(t)

		if _, err := b.List(ctx); !errors.Is(err, cause) {
			t.Errorf("List = %v, want the cause", err)
		}
		if _, err := b.Create(ctx, "new", t.TempDir(), nil); !errors.Is(err, cause) {
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

	t.Run("checks that need no session come first, so their answers stand", func(t *testing.T) {
		s := newSubject(t)
		s.Hold(t, "work", Answering)
		b := s.Backend()
		ctx, _ := ended(t)

		assertGone(t, ctx, b, "gone")
		assertInvalid(t, ctx, b, "a/b")
		if _, err := b.AttachCommand(ctx, "--labels"); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("AttachCommand(--labels) = %v, want ErrInvalidName", err)
		}
		if _, err := b.Create(ctx, "-x", t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Create(-x) = %v, want ErrInvalidName", err)
		}
		if why := s.Lacks(FoldsCase); why == "" {
			assertGone(t, ctx, b, "WORK")
		}
	})

	t.Run("List with no session to ask lists none", func(t *testing.T) {
		ctx, _ := ended(t)

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
		{"busy", false},
		{"gone", false},
		{"", false},
	} {
		t.Run("inside "+quoted(tt.current), func(t *testing.T) {
			s := newSubject(t)
			s.Hold(t, "work", Answering)
			s.Hold(t, "home", Answering)
			s.Hold(t, "wedged", Silent)
			s.Hold(t, "busy", Refusing)
			s.SetCurrent(tt.current)

			cmd, err := s.Backend().AttachCommand(t.Context(), "work")
			if err != nil || cmd.Switches != tt.switches {
				t.Fatalf("AttachCommand(work) = %+v, %v; want Switches %v", cmd, err, tt.switches)
			}

			t.Run("its environment pins the directory and keeps the caller's session only to switch it", func(t *testing.T) {
				if why := s.Lacks(AttachEnvironment); why != "" {
					t.Skip(why)
				}
				dirs, sessions := values(cmd.Env, "ZMX_DIR"), values(cmd.Env, "ZMX_SESSION")
				if len(dirs) != 1 || !filepath.IsAbs(dirs[0]) {
					t.Errorf("Env ZMX_DIR = %q, want one absolute directory", dirs)
				}
				var want []string
				if tt.switches {
					want = []string{tt.current}
				}
				if !slices.Equal(sessions, want) {
					t.Errorf("Env ZMX_SESSION = %q, want %q", sessions, want)
				}
			})
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

// assertListing compares a listed session with what Hold said to expect.
func assertListing(t *testing.T, got, want backend.Session) {
	t.Helper()
	pid := got.PID == want.PID || want.PID == 0 && got.PID > 0
	created := got.Created.Equal(want.Created) || want.Created.IsZero() && !got.Created.IsZero() && !got.Created.After(time.Now())
	rest, wantRest := got, want
	rest.PID, rest.Created, wantRest.PID, wantRest.Created = 0, time.Time{}, 0, time.Time{}
	if !pid || !created || rest != wantRest {
		t.Errorf("listed %+v, want %+v (a zero PID or Created standing for one the backend chose)", got, want)
	}
}

// assertReached asserts that every call reaches the session held under name.
func assertReached(t *testing.T, ctx context.Context, b backend.Backend, name string) {
	t.Helper()
	if exists, err := b.Exists(ctx, name); err != nil || !exists {
		t.Errorf("Exists(%q) = %v, %v; want true", name, exists, err)
	}
	for _, op := range perSession {
		if err := op.call(ctx, b, name); err != nil {
			t.Errorf("%s(%q) = %v", op.name, name, err)
		}
	}
}

// assertGone asserts that no session is held under name.
func assertGone(t *testing.T, ctx context.Context, b backend.Backend, name string) {
	t.Helper()
	if exists, err := b.Exists(ctx, name); err != nil || exists {
		t.Errorf("Exists(%q) = %v, %v; want false", name, exists, err)
	}
	for _, op := range perSession {
		if err := op.call(ctx, b, name); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("%s(%q) = %v, want ErrNoSuchSession", op.name, name, err)
		}
	}
}

// assertInvalid asserts that every call refuses each name.
func assertInvalid(t *testing.T, ctx context.Context, b backend.Backend, names ...string) {
	t.Helper()
	for _, name := range names {
		if exists, err := b.Exists(ctx, name); err != nil || exists {
			t.Errorf("Exists(%q) = %v, %v; want false", name, exists, err)
		}
		for _, op := range perSession {
			if err := op.call(ctx, b, name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("%s(%q) = %v, want ErrInvalidName", op.name, name, err)
			}
		}
		if _, err := b.Create(ctx, name, "/", nil); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Create(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

// assertHeldAs asserts that every call naming asked refuses it as held only
// under held.
func assertHeldAs(t *testing.T, ctx context.Context, b backend.Backend, asked, held string) {
	t.Helper()
	for _, op := range perSession {
		err := op.call(ctx, b, asked)
		if got, ok := errors.AsType[*backend.HeldAsError](err); !ok || got.Held != held {
			t.Errorf("%s(%q) = %v, want a HeldAsError held as %q", op.name, asked, err, held)
		}
	}
}

func assertExists(t *testing.T, err error, held string) {
	t.Helper()
	if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != held {
		t.Errorf("Create = %v, want a SessionExistsError held as %q", err, held)
	}
}

// values are key's values in env, in order.
func values(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			out = append(out, v)
		}
	}
	return out
}

func quoted(name string) string {
	if name == "" {
		return "no session"
	}
	return `"` + name + `"`
}
