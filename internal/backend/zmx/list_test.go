package zmx_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

func listing(lines ...string) map[string]reply {
	return map[string]reply{"list": {out: strings.Join(lines, "")}}
}

func TestList(t *testing.T) {
	t.Run("it reads every field zmx v0.8.1 prints", func(t *testing.T) {
		r := newRunner(t, listing(
			"  name=dotfiles-PpaTER\tpid=4242\tclients=2\tcreated=1793650796\tcwd=file://host.local/Users/lee/Code/my%20dots\tcmd=sleep 1000\n",
		))

		got, err := newBackend(r).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		want := []backend.Session{{
			Name:    "dotfiles-PpaTER",
			Dir:     "/Users/lee/Code/my dots",
			Clients: 2,
			PID:     4242,
			Created: time.Unix(1793650796, 0),
			Command: "sleep 1000",
		}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("List = %+v, want %+v", got, want)
		}
	})

	t.Run("it reads the list verbatim, so a name's leading spaces survive", func(t *testing.T) {
		r := newRunner(t, listing("  name= spaced\tpid=1\tclients=0\tcreated=1\n"))

		got, err := newBackend(r).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 || got[0].Name != " spaced" {
			t.Errorf("List = %+v, want the one session %q", got, " spaced")
		}
		if calls := r.callsTo("list"); len(calls) != 1 || !calls[0].raw {
			t.Errorf("list ran as %+v, want one verbatim run", calls)
		}
	})

	t.Run("it reads odd names whole", func(t *testing.T) {
		for _, name := range []string{"a=b", "has space", "→ arrow", "ünï-çødé", "_portal-saver", "a.b:c"} {
			r := newRunner(t, listing("  name="+name+"\tpid=1\tclients=0\tcreated=1\n"))

			got, err := newBackend(r).List()
			if err != nil {
				t.Fatalf("List for %q: %v", name, err)
			}
			if len(got) != 1 || got[0].Name != name {
				t.Errorf("List = %+v, want the one session %q", got, name)
			}
		}
	})

	t.Run("it reads the line marking the caller's own session", func(t *testing.T) {
		r := newRunner(t, listing("→ name=here\tpid=1\tclients=1\tcreated=1\n"))

		got, err := newBackend(r).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 || got[0].Name != "here" {
			t.Errorf("List = %+v, want the one session %q", got, "here")
		}
	})

	t.Run("it returns an empty list, not nil, when zmx holds no session", func(t *testing.T) {
		got, err := newBackend(newRunner(t, listing())).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("List = %#v, want an empty non-nil list", got)
		}
	})

	t.Run("it ignores labels, ended tasks and the fields after cmd", func(t *testing.T) {
		r := newRunner(t, listing(
			"  name=task\tpid=7\tclients=0\tcreated=5\tcwd=/srv\tcmd=make\tended=9\texit_code=0\tcwd=label\towner=lee\n",
		))

		got, err := newBackend(r).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 || got[0].Dir != "/srv" || got[0].Command != "make" {
			t.Errorf("List = %+v, want dir /srv and command make", got)
		}
	})

	t.Run("it leaves Dir, Command and Created empty when zmx reports none", func(t *testing.T) {
		got, err := newBackend(newRunner(t, listing("  name=bare\tpid=1\tclients=0\tcreated=0\n"))).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 || got[0].Dir != "" || got[0].Command != "" || !got[0].Created.IsZero() {
			t.Errorf("List = %+v, want empty Dir, Command and Created", got)
		}
	})

	t.Run("it reads the directory out of each cwd form", func(t *testing.T) {
		for cwd, want := range map[string]string{
			"file://host.local/a/b":             "/a/b",
			"file:///a/b":                       "/a/b",
			"file://host/a%25b%20c":             "/a%b c",
			"kitty-shell-cwd://host.local/a b":  "/a b",
			"/plain/path":                       "/plain/path",
			"file://host/bad%zz":                "",
			"https://host/a":                    "",
			"kitty-shell-cwd://host.local":      "",
			"file://host.local/truncated%2":     "",
			"file://other-host.example/srv/app": "/srv/app",
		} {
			got, err := newBackend(newRunner(t, listing("  name=s\tpid=1\tclients=0\tcreated=1\tcwd="+cwd+"\n"))).List()
			if err != nil {
				t.Fatalf("List with cwd %q: %v", cwd, err)
			}
			if got[0].Dir != want {
				t.Errorf("Dir from cwd %q = %q, want %q", cwd, got[0].Dir, want)
			}
		}
	})

	t.Run("it keeps a session that did not answer, marked unreachable", func(t *testing.T) {
		r := newRunner(t, listing(
			"  name=busy\terr=Timeout\tstatus=unreachable\n",
			"  name=ok\tpid=1\tclients=0\tcreated=1\n",
		))

		got, err := newBackend(r).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		want := []backend.Session{
			{Name: "busy", Unreachable: true},
			{Name: "ok", PID: 1, Created: time.Unix(1, 0)},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("List = %+v, want %+v", got, want)
		}
	})

	t.Run("it drops a session whose daemon is gone", func(t *testing.T) {
		r := newRunner(t, listing("  name=dead\terr=ConnectionRefused\tstatus=cleaning up\n"))

		got, err := newBackend(r).List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("List = %+v, want the gone session left out", got)
		}
	})

	t.Run("it fails on a line it cannot read rather than skipping it", func(t *testing.T) {
		for _, line := range []string{
			"garbage",
			"  pid=1\tclients=0\tcreated=1",
			"  name=x\tpid=one\tclients=0\tcreated=1",
			"  name=x\tpid=1\tclients=0",
			"  name=x\tclients=0\tpid=1\tcreated=1",
			"  name=x\tpid=1\tclients=0\tcreated=soon",
			"  name=x\terr=Timeout",
		} {
			r := newRunner(t, listing("  name=fine\tpid=1\tclients=0\tcreated=1\n", line+"\n"))

			got, err := newBackend(r).List()
			if err == nil {
				t.Errorf("List of %q = %+v, want an error", line, got)
			}
			if got != nil {
				t.Errorf("List of %q returned %+v alongside its error, want nil", line, got)
			}
		}
	})

	t.Run("it returns a failed read as an error, never an empty list", func(t *testing.T) {
		boom := zmxFailure("error: ReadOnlyFileSystem\n")
		got, err := newBackend(newRunner(t, map[string]reply{"list": {err: boom}})).List()
		if !errors.Is(err, boom) {
			t.Errorf("List error = %v, want it to wrap %v", err, boom)
		}
		if got != nil {
			t.Errorf("List = %+v alongside its error, want nil", got)
		}
	})
}

func TestExists(t *testing.T) {
	list := listing("  name=alpha\tpid=1\tclients=0\tcreated=1\n", "  name=alpha-2\tpid=2\tclients=0\tcreated=1\n")

	t.Run("it finds a session by its exact name", func(t *testing.T) {
		for name, want := range map[string]bool{"alpha": true, "alpha-2": true, "alp": false, "beta": false} {
			got, err := newBackend(newRunner(t, list)).Exists(name)
			if err != nil {
				t.Fatalf("Exists(%q): %v", name, err)
			}
			if got != want {
				t.Errorf("Exists(%q) = %v, want %v", name, got, want)
			}
		}
	})

	t.Run("it returns a failed read as an error", func(t *testing.T) {
		boom := zmxFailure("error: boom\n")
		if _, err := newBackend(newRunner(t, map[string]reply{"list": {err: boom}})).Exists("alpha"); !errors.Is(err, boom) {
			t.Errorf("Exists error = %v, want it to wrap %v", err, boom)
		}
	})
}
