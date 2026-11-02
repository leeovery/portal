package backendtest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
)

func names(t *testing.T, f *backendtest.Fake) []string {
	t.Helper()
	sessions, err := f.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := []string{}
	for _, s := range sessions {
		out = append(out, s.Name)
	}
	return out
}

func TestFake(t *testing.T) {
	t.Run("it lists the sessions it holds by name, Portal's own included", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "b"}, backend.Session{Name: "_portal-saver"}, backend.Session{Name: "a"})

		if got, want := names(t, f), []string{"_portal-saver", "a", "b"}; !reflect.DeepEqual(got, want) {
			t.Errorf("List names = %v, want %v", got, want)
		}
	})

	t.Run("it lists an empty, non-nil slice when it holds nothing", func(t *testing.T) {
		got, err := backendtest.New().List(t.Context())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("List = %#v, %v; want an empty slice", got, err)
		}
	})

	t.Run("it creates an answering, detached session with its command and creation time", func(t *testing.T) {
		at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
		f := backendtest.New()
		f.Now = func() time.Time { return at }

		if err := f.Create(t.Context(), "work", "/srv", []string{"sh", "-c", "sleep 1"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		s, ok := f.Session("work")
		if !ok {
			t.Fatal("work not held after Create")
		}
		if !s.Answered || s.Command != "sh -c 'sleep 1'" || !s.Created.Equal(at) || s.Attached() || s.PID == 0 {
			t.Errorf("session = %+v", s)
		}
		if exists, err := f.Exists(t.Context(), "work"); err != nil || !exists {
			t.Errorf("Exists(work) = %v, %v; want true", exists, err)
		}
	})

	t.Run("it reports the command as zmx renders it", func(t *testing.T) {
		f := backendtest.New()
		command := []string{"sh", "-c", "echo it's", ""}

		if err := f.Create(t.Context(), "quoted", "/srv", command); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if s, _ := f.Session("quoted"); s.Command != zmx.DisplayCommand(command) {
			t.Errorf("Command = %q, want %q", s.Command, zmx.DisplayCommand(command))
		}
	})

	t.Run("it reports the directory with its symlinks resolved, as zmx does", func(t *testing.T) {
		real := t.TempDir()
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		want, err := filepath.EvalSymlinks(real)
		if err != nil {
			t.Fatal(err)
		}
		f := backendtest.New()

		if err := f.Create(t.Context(), "linked", link, nil); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if s, _ := f.Session("linked"); s.Dir != want {
			t.Errorf("Dir = %q, want %q", s.Dir, want)
		}
	})

	t.Run("it refuses to create over a held session, under any case, naming it", func(t *testing.T) {
		for _, name := range []string{"work", "WORK", "Work"} {
			t.Run(name, func(t *testing.T) {
				f := backendtest.New(backend.Session{Name: "work", Command: "orig"})

				err := f.Create(t.Context(), name, "/srv", nil)

				exists, ok := errors.AsType[*backend.SessionExistsError](err)
				if !ok || !errors.Is(err, backend.ErrSessionExists) || exists.Held != "work" {
					t.Errorf("Create(%q) = %v, want a SessionExistsError held as work", name, err)
				}
				if got := names(t, f); !reflect.DeepEqual(got, []string{"work"}) {
					t.Errorf("after Create(%q), List names = %v, want [work]", name, got)
				}
			})
		}
	})

	t.Run("it acts on a session only under exactly its name", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "work", Answered: true})
		f.SetHistory("work", []byte("screen"))

		if err := f.Kill(t.Context(), "WORK"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill(WORK) = %v, want ErrNoSuchSession", err)
		}
		if _, err := f.History(t.Context(), "WORK", 0); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History(WORK) = %v, want ErrNoSuchSession", err)
		}
		if exists, err := f.Exists(t.Context(), "WORK"); err != nil || exists {
			t.Errorf("Exists(WORK) = %v, %v; want false", exists, err)
		}
	})

	t.Run("it kills a session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Answered: true}, backend.Session{Name: "b", Answered: true})
		f.SetHistory("a", []byte("old"))

		if err := f.Kill(t.Context(), "a"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if got := names(t, f); !reflect.DeepEqual(got, []string{"b"}) {
			t.Errorf("after Kill, List names = %v, want [b]", got)
		}
		_ = f.Create(t.Context(), "a", "/", nil)
		if got, err := f.History(t.Context(), "a", 0); !errors.Is(err, backend.ErrHistoryUnavailable) {
			t.Errorf("a recreated session's History = %q, %v; want ErrHistoryUnavailable", got, err)
		}
	})

	t.Run("it detaches every client of a session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Answered: true, Clients: 2})

		if err := f.Detach(t.Context(), "a"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		if s, _ := f.Session("a"); s.Clients != 0 {
			t.Errorf("Clients after Detach = %d, want 0", s.Clients)
		}
	})

	t.Run("it returns the history it was given, verbatim, and records the rows asked for", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Answered: true})
		vt := []byte("\x1b[31mred\x1b[0m  \n\n")
		f.SetHistory("a", vt)

		got, err := f.History(t.Context(), "a", 40)
		if err != nil || !reflect.DeepEqual(got, vt) {
			t.Errorf("History = %q, %v; want %q", got, err, vt)
		}
		if calls := f.CallsTo(backendtest.OpHistory); len(calls) != 1 || calls[0].Rows != 40 {
			t.Errorf("CallsTo(History) = %+v, want one call for 40 rows", calls)
		}
	})

	t.Run("it reports no history, never an empty one, for a session given none", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "unset", Answered: true}, backend.Session{Name: "blank", Answered: true})
		f.SetHistory("blank", nil)

		for _, name := range []string{"unset", "blank"} {
			if got, err := f.History(t.Context(), name, 0); !errors.Is(err, backend.ErrHistoryUnavailable) {
				t.Errorf("History(%s) = %q, %v; want ErrHistoryUnavailable", name, got, err)
			}
		}
	})

	t.Run("it reports a missing session as ErrNoSuchSession", func(t *testing.T) {
		f := backendtest.New()
		if err := f.Kill(t.Context(), "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill = %v, want ErrNoSuchSession", err)
		}
		if err := f.Detach(t.Context(), "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Detach = %v, want ErrNoSuchSession", err)
		}
		if _, err := f.History(t.Context(), "gone", 0); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History = %v, want ErrNoSuchSession", err)
		}
		if _, err := f.AttachCommand(t.Context(), "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("AttachCommand = %v, want ErrNoSuchSession", err)
		}
	})

	t.Run("it refuses the names zmx refuses, by default, at any length", func(t *testing.T) {
		f := backendtest.New()
		for _, name := range []string{"", "..", "logs", "a/b", "-x", "x*", "a\nb", "bad\xffname"} {
			if err := f.ValidateName(name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
			}
		}
		long := strings.Repeat("b", 200)
		if err := f.ValidateName(long); err != nil {
			t.Errorf("ValidateName of %d bytes = %v, want nil: the Fake has no directory to measure against", len(long), err)
		}
	})

	t.Run("it reaches a session under any name zmx holds, as List reports it", func(t *testing.T) {
		odd := []string{"-x", "x*", "logs", "a\nb", "bad\xffname"}
		f := backendtest.New()
		for _, name := range odd {
			f.Add(backend.Session{Name: name, Answered: true})
			f.SetHistory(name, []byte("screen"))
		}
		ctx := t.Context()

		for _, name := range odd {
			if exists, err := f.Exists(ctx, name); err != nil || !exists {
				t.Errorf("Exists(%q) = %v, %v; want true", name, exists, err)
			}
			if _, err := f.History(ctx, name, 0); err != nil {
				t.Errorf("History(%q) = %v", name, err)
			}
			if _, err := f.AttachCommand(ctx, name); err != nil {
				t.Errorf("AttachCommand(%q) = %v", name, err)
			}
			if err := f.Detach(ctx, name); err != nil {
				t.Errorf("Detach(%q) = %v", name, err)
			}
			if err := f.Kill(ctx, name); err != nil {
				t.Errorf("Kill(%q) = %v", name, err)
			}
		}
	})

	t.Run("it refuses to attach under a name zmx attach reads as its own flag", func(t *testing.T) {
		f := backendtest.New()
		for _, name := range []string{"-h", "--help", "--labels", "--labels=a=1"} {
			f.Add(backend.Session{Name: name, Answered: true})
			if _, err := f.AttachCommand(t.Context(), name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("AttachCommand(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("it fails every call on a session that does not answer with ErrTimeout, and refuses its name to Create", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "wedged"})
		f.SetHistory("wedged", []byte("screen"))
		ctx := t.Context()
		_, existsErr := f.Exists(ctx, "wedged")
		_, historyErr := f.History(ctx, "wedged", 0)
		_, attachErr := f.AttachCommand(ctx, "wedged")
		calls := map[string]error{
			"Exists":        existsErr,
			"Kill":          f.Kill(ctx, "wedged"),
			"Detach":        f.Detach(ctx, "wedged"),
			"History":       historyErr,
			"AttachCommand": attachErr,
		}

		for call, err := range calls {
			if !errors.Is(err, backend.ErrTimeout) || errors.Is(err, backend.ErrNoSuchSession) {
				t.Errorf("%s(wedged) = %v, want ErrTimeout", call, err)
			}
		}
		for _, name := range []string{"wedged", "WEDGED"} {
			err := f.Create(ctx, name, "/", nil)
			if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "wedged" {
				t.Errorf("Create(%s) = %v, want a SessionExistsError held as wedged", name, err)
			}
		}
		if s, ok := f.Session("wedged"); !ok || s.Answered {
			t.Errorf("wedged after the calls = %+v, %v; want it held, still not answering", s, ok)
		}
	})

	t.Run("it switches, as zmx attach does, only from inside a session it holds that answers", func(t *testing.T) {
		f := backendtest.New(
			backend.Session{Name: "target", Answered: true},
			backend.Session{Name: "home", Answered: true},
			backend.Session{Name: "wedged"},
		)
		for current, want := range map[string]bool{"home": true, "wedged": false, "gone": false, "": false} {
			f.SetCurrent(current)
			if cmd, err := f.AttachCommand(t.Context(), "target"); err != nil || cmd.Switches != want {
				t.Errorf("AttachCommand inside %q = %+v, %v; want Switches %v", current, cmd, err, want)
			}
		}
	})

	t.Run("it refuses a name its Validate refuses", func(t *testing.T) {
		refusal := errors.New("refused")
		f := backendtest.New()
		f.Validate = func(string) error { return refusal }

		if err := f.Create(t.Context(), "x", "/", nil); !errors.Is(err, refusal) {
			t.Errorf("Create = %v, want Validate's refusal", err)
		}
		if _, ok := f.Session("x"); ok {
			t.Error("Create made a session whose name was refused")
		}
	})

	t.Run("it returns an injected failure before changing anything, until cleared", func(t *testing.T) {
		boom := errors.New("boom")
		f := backendtest.New(backend.Session{Name: "a", Answered: true, Clients: 1})
		for _, op := range []backendtest.Op{backendtest.OpList, backendtest.OpExists, backendtest.OpCreate, backendtest.OpAttachCommand, backendtest.OpKill, backendtest.OpDetach, backendtest.OpHistory} {
			f.Fail(op, boom)
		}
		ctx := t.Context()

		if got, err := f.List(ctx); !errors.Is(err, boom) || got != nil {
			t.Errorf("List = %v, %v; want nil, boom", got, err)
		}
		if _, err := f.Exists(ctx, "a"); !errors.Is(err, boom) {
			t.Errorf("Exists = %v, want boom", err)
		}
		if err := f.Create(ctx, "b", "/", nil); !errors.Is(err, boom) {
			t.Errorf("Create = %v, want boom", err)
		}
		if _, err := f.AttachCommand(ctx, "a"); !errors.Is(err, boom) {
			t.Errorf("AttachCommand = %v, want boom", err)
		}
		if err := f.Kill(ctx, "a"); !errors.Is(err, boom) {
			t.Errorf("Kill = %v, want boom", err)
		}
		if err := f.Detach(ctx, "a"); !errors.Is(err, boom) {
			t.Errorf("Detach = %v, want boom", err)
		}
		if _, err := f.History(ctx, "a", 0); !errors.Is(err, boom) {
			t.Errorf("History = %v, want boom", err)
		}
		if s, ok := f.Session("a"); !ok || s.Clients != 1 {
			t.Errorf("a failed call changed the session: %+v, %v", s, ok)
		}
		if _, ok := f.Session("b"); ok {
			t.Error("a failed Create made a session")
		}

		f.Fail(backendtest.OpList, nil)
		if _, err := f.List(ctx); err != nil {
			t.Errorf("List after clearing = %v, want nil", err)
		}
	})

	t.Run("it fails a call whose context has ended, with the context's cause, changing nothing", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Answered: true})
		cause := errors.New("caller gave up")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)

		if err := f.Kill(ctx, "a"); !errors.Is(err, cause) {
			t.Errorf("Kill with an ended context = %v, want its cause", err)
		}
		if _, ok := f.Session("a"); !ok {
			t.Error("a Kill with an ended context removed the session")
		}
	})

	t.Run("it reports the current session it was given", func(t *testing.T) {
		f := backendtest.New()
		if got := f.Current(); got != "" {
			t.Errorf("Current = %q, want empty", got)
		}
		f.SetCurrent("here")
		if got := f.Current(); got != "here" {
			t.Errorf("Current = %q, want here", got)
		}
	})

	t.Run("it records every call in order", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Answered: true})
		ctx := t.Context()

		_, _ = f.List(ctx)
		_ = f.Create(ctx, "b", "/srv", []string{"sh"})
		cmd, err := f.AttachCommand(ctx, "b")
		if err != nil {
			t.Fatalf("AttachCommand: %v", err)
		}
		_ = f.Kill(ctx, "a")

		want := []backendtest.Call{
			{Op: backendtest.OpList},
			{Op: backendtest.OpCreate, Name: "b", Dir: "/srv", Command: []string{"sh"}},
			{Op: backendtest.OpAttachCommand, Name: "b"},
			{Op: backendtest.OpKill, Name: "a"},
		}
		if got := f.Calls(); !reflect.DeepEqual(got, want) {
			t.Errorf("Calls = %+v, want %+v", got, want)
		}
		if got := f.CallsTo(backendtest.OpKill); len(got) != 1 || got[0].Name != "a" {
			t.Errorf("CallsTo(Kill) = %+v", got)
		}
		if !reflect.DeepEqual(cmd.Argv, []string{backendtest.AttachProgram, "b"}) {
			t.Errorf("AttachCommand = %+v", cmd)
		}
	})
}
