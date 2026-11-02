package backendtest_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

func names(t *testing.T, f *backendtest.Fake) []string {
	t.Helper()
	sessions, err := f.List()
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
		got, err := backendtest.New().List()
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("List = %#v, %v; want an empty slice", got, err)
		}
	})

	t.Run("it creates a detached session with its dir, command and creation time", func(t *testing.T) {
		at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
		f := backendtest.New()
		f.Now = func() time.Time { return at }

		if err := f.Create("work", "/srv", []string{"portal", "state", "hydrate"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		s, ok := f.Session("work")
		if !ok {
			t.Fatal("work not held after Create")
		}
		if s.Dir != "/srv" || s.Command != "portal state hydrate" || !s.Created.Equal(at) || s.Attached() || s.PID == 0 {
			t.Errorf("session = %+v", s)
		}
		if exists, err := f.Exists("work"); err != nil || !exists {
			t.Errorf("Exists(work) = %v, %v; want true", exists, err)
		}
	})

	t.Run("it refuses to create over a held session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "work", Command: "orig"})

		if err := f.Create("work", "/srv", nil); !errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("Create = %v, want ErrSessionExists", err)
		}
		if s, _ := f.Session("work"); s.Command != "orig" {
			t.Errorf("session replaced: %+v", s)
		}
	})

	t.Run("it kills a session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a"}, backend.Session{Name: "b"})
		f.SetHistory("a", []byte("old"))

		if err := f.Kill("a"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if got := names(t, f); !reflect.DeepEqual(got, []string{"b"}) {
			t.Errorf("after Kill, List names = %v, want [b]", got)
		}
		_ = f.Create("a", "/", nil)
		if got, _ := f.History("a"); len(got) != 0 {
			t.Errorf("a recreated session inherited history %q", got)
		}
	})

	t.Run("it detaches every client of a session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Clients: 2})

		if err := f.Detach("a"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		if s, _ := f.Session("a"); s.Clients != 0 {
			t.Errorf("Clients after Detach = %d, want 0", s.Clients)
		}
	})

	t.Run("it returns the history it was given, verbatim", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a"})
		vt := []byte("\x1b[31mred\x1b[0m  \n\n")
		f.SetHistory("a", vt)

		got, err := f.History("a")
		if err != nil || !reflect.DeepEqual(got, vt) {
			t.Errorf("History = %q, %v; want %q", got, err, vt)
		}
	})

	t.Run("it reports a missing session as ErrNoSuchSession", func(t *testing.T) {
		f := backendtest.New()
		if err := f.Kill("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill = %v, want ErrNoSuchSession", err)
		}
		if err := f.Detach("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Detach = %v, want ErrNoSuchSession", err)
		}
		if _, err := f.History("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History = %v, want ErrNoSuchSession", err)
		}
	})

	t.Run("it refuses an empty name by default, and a name its Validate refuses", func(t *testing.T) {
		f := backendtest.New()
		if err := f.ValidateName(""); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("ValidateName(\"\") = %v, want ErrInvalidName", err)
		}
		if err := f.ValidateName("anything"); err != nil {
			t.Errorf("ValidateName(anything) = %v, want nil", err)
		}

		refusal := errors.New("refused")
		f.Validate = func(string) error { return refusal }
		if err := f.Create("x", "/", nil); !errors.Is(err, refusal) {
			t.Errorf("Create = %v, want Validate's refusal", err)
		}
		if _, ok := f.Session("x"); ok {
			t.Error("Create made a session whose name was refused")
		}
	})

	t.Run("it returns an injected failure before changing anything, until cleared", func(t *testing.T) {
		boom := errors.New("boom")
		f := backendtest.New(backend.Session{Name: "a", Clients: 1})
		for _, op := range []backendtest.Op{backendtest.OpList, backendtest.OpExists, backendtest.OpCreate, backendtest.OpKill, backendtest.OpDetach, backendtest.OpHistory} {
			f.Fail(op, boom)
		}

		if got, err := f.List(); !errors.Is(err, boom) || got != nil {
			t.Errorf("List = %v, %v; want nil, boom", got, err)
		}
		if _, err := f.Exists("a"); !errors.Is(err, boom) {
			t.Errorf("Exists = %v, want boom", err)
		}
		if err := f.Create("b", "/", nil); !errors.Is(err, boom) {
			t.Errorf("Create = %v, want boom", err)
		}
		if err := f.Kill("a"); !errors.Is(err, boom) {
			t.Errorf("Kill = %v, want boom", err)
		}
		if err := f.Detach("a"); !errors.Is(err, boom) {
			t.Errorf("Detach = %v, want boom", err)
		}
		if _, err := f.History("a"); !errors.Is(err, boom) {
			t.Errorf("History = %v, want boom", err)
		}
		if s, ok := f.Session("a"); !ok || s.Clients != 1 {
			t.Errorf("a failed call changed the session: %+v, %v", s, ok)
		}
		if _, ok := f.Session("b"); ok {
			t.Error("a failed Create made a session")
		}

		f.Fail(backendtest.OpList, nil)
		if _, err := f.List(); err != nil {
			t.Errorf("List after clearing = %v, want nil", err)
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
		f := backendtest.New(backend.Session{Name: "a"})

		_, _ = f.List()
		_ = f.Create("b", "/srv", []string{"sh"})
		cmd := f.AttachCommand("b")
		_ = f.Kill("a")

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
