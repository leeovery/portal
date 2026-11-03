package backendtest_test

import (
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

		if _, err := f.Create(t.Context(), "work", "/srv", []string{"sh", "-c", "sleep 1"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		s, ok := f.Held("work")
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

		if _, err := f.Create(t.Context(), "quoted", "/srv", command); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if s, _ := f.Held("quoted"); s.Command != zmx.DisplayCommand(command) {
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

		if _, err := f.Create(t.Context(), "linked", link, nil); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if s, _ := f.Held("linked"); s.Dir != want {
			t.Errorf("Dir = %q, want %q", s.Dir, want)
		}
	})

	t.Run("it kills a session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a"}, backend.Session{Name: "b"})
		f.SetHistory("a", []byte("old"))

		if err := f.Kill(t.Context(), "a"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if got := names(t, f); !reflect.DeepEqual(got, []string{"b"}) {
			t.Errorf("after Kill, List names = %v, want [b]", got)
		}
		_, _ = f.Create(t.Context(), "a", "/", nil)
		if got, err := f.History(t.Context(), "a", 0); !errors.Is(err, backend.ErrHistoryUnavailable) {
			t.Errorf("a recreated session's History = %q, %v; want ErrHistoryUnavailable", got, err)
		}
	})

	t.Run("it detaches every client of a session", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a", Clients: 2})

		if err := f.Detach(t.Context(), "a"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		if s, _ := f.Held("a"); s.Clients != 0 {
			t.Errorf("Clients after Detach = %d, want 0", s.Clients)
		}
	})

	t.Run("it returns the history it was given, verbatim, and records the rows asked for", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a"})
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
		f := backendtest.New(backend.Session{Name: "unset"}, backend.Session{Name: "blank"})
		f.SetHistory("blank", nil)

		for _, name := range []string{"unset", "blank"} {
			if got, err := f.History(t.Context(), name, 0); !errors.Is(err, backend.ErrHistoryUnavailable) {
				t.Errorf("History(%s) = %q, %v; want ErrHistoryUnavailable", name, got, err)
			}
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

	t.Run("it refuses to attach under a name zmx attach reads as its own flag", func(t *testing.T) {
		f := backendtest.New()
		for _, name := range []string{"-h", "--help", "--labels", "--labels=a=1"} {
			f.Add(backend.Session{Name: name})
			if _, err := f.AttachCommand(t.Context(), name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("AttachCommand(%q) = %v, want ErrInvalidName", name, err)
			}
		}
	})

	t.Run("it holds every session it is given as answering until told to stop", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "work", Answered: false, Dir: "/srv", PID: 7, Clients: 1})
		if exists, err := f.Exists(t.Context(), "work"); err != nil || !exists {
			t.Errorf("Exists of a session given with Answered false = %v, %v; want it answering", exists, err)
		}

		f.StopAnswering("work")
		if _, err := f.Exists(t.Context(), "work"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Exists after StopAnswering = %v, want ErrTimeout", err)
		}
		if got, err := f.List(t.Context()); err != nil || len(got) != 1 || got[0] != (backend.Session{Name: "work"}) {
			t.Errorf("List after StopAnswering = %+v, %v; want work with only its name", got, err)
		}

		f.Add(backend.Session{Name: "work"})
		if exists, err := f.Exists(t.Context(), "work"); err != nil || !exists {
			t.Errorf("Exists once added again = %v, %v; want it answering", exists, err)
		}
	})

	t.Run("it refuses to hold what the driver never could, by panicking", func(t *testing.T) {
		for call, do := range map[string]func(*backendtest.Fake){
			`Add("a/b")`:             func(f *backendtest.Fake) { f.Add(backend.Session{Name: "a/b"}) },
			`Add("")`:                func(f *backendtest.Fake) { f.Add(backend.Session{}) },
			`StopAnswering("ghost")`: func(f *backendtest.Fake) { f.StopAnswering("ghost") },
		} {
			t.Run(call, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Errorf("%s did not panic", call)
					}
				}()
				do(backendtest.New())
			})
		}
	})

	t.Run("it refuses a name its Validate refuses", func(t *testing.T) {
		refusal := errors.New("refused")
		f := backendtest.New()
		f.Validate = func(string) error { return refusal }

		if _, err := f.Create(t.Context(), "x", "/", nil); !errors.Is(err, refusal) {
			t.Errorf("Create = %v, want Validate's refusal", err)
		}
		if _, ok := f.Held("x"); ok {
			t.Error("Create made a session whose name was refused")
		}
	})

	t.Run("it returns an injected failure before changing anything, until cleared", func(t *testing.T) {
		boom := errors.New("boom")
		f := backendtest.New(backend.Session{Name: "a", Clients: 1})
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
		if _, err := f.Create(ctx, "b", "/", nil); !errors.Is(err, boom) {
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
		if s, ok := f.Held("a"); !ok || s.Clients != 1 {
			t.Errorf("a failed call changed the session: %+v, %v", s, ok)
		}
		if _, ok := f.Held("b"); ok {
			t.Error("a failed Create made a session")
		}

		f.Fail(backendtest.OpList, nil)
		if _, err := f.List(ctx); err != nil {
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

	t.Run("it reports the clients and the reach it was given, a session's clients going with it", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a"})
		clients := []backend.ClientProcess{{PID: 501, Env: []string{"TERM=xterm"}}}
		f.SetAttachClients("a", clients)
		f.SetReachEnv([]string{"ZMX_DIR=/tmp/zx-fake"})

		if got, err := f.AttachClients("a"); err != nil || !reflect.DeepEqual(got, clients) {
			t.Errorf("AttachClients(a) = %+v, %v; want %+v", got, err, clients)
		}
		if got := f.ReachEnv(); !reflect.DeepEqual(got, []string{"ZMX_DIR=/tmp/zx-fake"}) {
			t.Errorf("ReachEnv = %q, want what it was given", got)
		}
		_ = f.Kill(t.Context(), "a")
		if got, err := f.AttachClients("a"); err != nil || len(got) != 0 {
			t.Errorf("AttachClients(a) after its kill = %+v, %v; want none", got, err)
		}
		f.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		if _, err := f.AttachClients("a"); !errors.Is(err, backend.ErrUnreadableClient) {
			t.Errorf("AttachClients with a failure injected = %v, want it", err)
		}
	})

	t.Run("it records every call in order", func(t *testing.T) {
		f := backendtest.New(backend.Session{Name: "a"})
		ctx := t.Context()

		_, _ = f.List(ctx)
		_, _ = f.Create(ctx, "b", "/srv", []string{"sh"})
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
