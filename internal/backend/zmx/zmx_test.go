package zmx_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

// inherited is an environment from inside a zmx session with a prefix set, so
// every variable the env policy governs is present to be removed or replaced.
var inherited = []string{
	"PATH=/usr/bin:/bin",
	"ZMX_DIR=/Users/lee/.local/state/zmx",
	"ZMX_SESSION=live-session",
	"ZMX_SESSION_PREFIX=pre-",
	"TERM=xterm-ghostty",
}

var oneSession = "  name=present\tpid=1\tclients=1\tcreated=1\n"

// allReplies scripts every subcommand to succeed, with one session listed.
func allReplies() map[string]reply {
	return map[string]reply{
		"list":    {out: oneSession},
		"attach":  {},
		"kill":    {out: "killed session present\n"},
		"detach":  {},
		"history": {out: "screen"},
	}
}

func TestEnvPolicy(t *testing.T) {
	cases := []struct {
		name string
		call func(backend.Backend)
		sub  string
		// session is the ZMX_SESSION the run must carry; nil for none.
		session []string
	}{
		{"list", func(b backend.Backend) { _, _ = b.List() }, "list", nil},
		{"create", func(b backend.Backend) { _ = b.Create("new", "/srv", nil) }, "attach", nil},
		{"kill", func(b backend.Backend) { _ = b.Kill("present") }, "kill", nil},
		{"history", func(b backend.Backend) { _, _ = b.History("present") }, "history", nil},
		{"detach", func(b backend.Backend) { _ = b.Detach("present") }, "detach", []string{"present"}},
	}
	for _, tc := range cases {
		t.Run("it pins "+tc.name+" to the bound directory with no inherited session or prefix", func(t *testing.T) {
			r := newRunner(t, allReplies())
			tc.call(newBackend(r, inherited...))

			for _, c := range r.calls {
				env := c.cmd.Env
				if got := envValues(env, "ZMX_DIR"); !reflect.DeepEqual(got, []string{testDir}) {
					t.Errorf("zmx %s ZMX_DIR = %v, want exactly %q", c.cmd.Argv[1], got, testDir)
				}
				if got := envValues(env, "ZMX_SESSION_PREFIX"); got != nil {
					t.Errorf("zmx %s kept ZMX_SESSION_PREFIX %v", c.cmd.Argv[1], got)
				}
				want := []string(nil)
				if c.cmd.Argv[1] == tc.sub {
					want = tc.session
				}
				if got := envValues(env, "ZMX_SESSION"); !reflect.DeepEqual(got, want) {
					t.Errorf("zmx %s ZMX_SESSION = %v, want %v", c.cmd.Argv[1], got, want)
				}
				for _, kv := range []string{"PATH=/usr/bin:/bin", "TERM=xterm-ghostty"} {
					if !slices.Contains(env, kv) {
						t.Errorf("zmx %s dropped %s", c.cmd.Argv[1], kv)
					}
				}
			}
			if len(r.callsTo(tc.sub)) != 1 {
				t.Errorf("zmx %s ran %d times, want once", tc.sub, len(r.callsTo(tc.sub)))
			}
		})
	}

	t.Run("it keeps the caller's session on the attach command, so attaching inside a session switches it", func(t *testing.T) {
		cmd := newBackend(newRunner(t, nil), inherited...).AttachCommand("other")

		if got := envValues(cmd.Env, "ZMX_SESSION"); !reflect.DeepEqual(got, []string{"live-session"}) {
			t.Errorf("attach ZMX_SESSION = %v, want the inherited live-session", got)
		}
		if got := envValues(cmd.Env, "ZMX_DIR"); !reflect.DeepEqual(got, []string{testDir}) {
			t.Errorf("attach ZMX_DIR = %v, want exactly %q", got, testDir)
		}
		if got := envValues(cmd.Env, "ZMX_SESSION_PREFIX"); got != nil {
			t.Errorf("attach kept ZMX_SESSION_PREFIX %v", got)
		}
	})

	t.Run("it reads the environment afresh on every run", func(t *testing.T) {
		env := []string{"A=1"}
		r := newRunner(t, allReplies())
		b := newBackendWith(r, func() []string { return env })

		_, _ = b.List()
		env = []string{"A=2"}
		_, _ = b.List()

		if got := envValues(r.calls[1].cmd.Env, "A"); !reflect.DeepEqual(got, []string{"2"}) {
			t.Errorf("second run A = %v, want [2]", got)
		}
	})
}

func TestCreate(t *testing.T) {
	t.Run("it runs zmx attach with the command, in the directory", func(t *testing.T) {
		replies := allReplies()
		replies["list"] = reply{}
		r := newRunner(t, replies)

		if err := newBackend(r).Create("work", "/srv/app", []string{"portal", "state", "hydrate", "--file", "a b"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		calls := r.callsTo("attach")
		if len(calls) != 1 {
			t.Fatalf("attach ran %d times, want once", len(calls))
		}
		want := []string{"zmx", "attach", "work", "portal", "state", "hydrate", "--file", "a b"}
		if !reflect.DeepEqual(calls[0].cmd.Argv, want) {
			t.Errorf("argv = %q, want %q", calls[0].cmd.Argv, want)
		}
		if calls[0].cmd.Dir != "/srv/app" {
			t.Errorf("Dir = %q, want /srv/app", calls[0].cmd.Dir)
		}
	})

	t.Run("it runs zmx attach with no command for the user's shell", func(t *testing.T) {
		replies := allReplies()
		replies["list"] = reply{}
		r := newRunner(t, replies)

		if err := newBackend(r).Create("work", "/srv", nil); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got := r.callsTo("attach")[0].cmd.Argv; !reflect.DeepEqual(got, []string{"zmx", "attach", "work"}) {
			t.Errorf("argv = %q, want zmx attach work", got)
		}
	})

	t.Run("it refuses a live name rather than attaching to it", func(t *testing.T) {
		r := newRunner(t, allReplies())

		err := newBackend(r).Create("present", "/srv", []string{"true"})
		if !errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("Create error = %v, want ErrSessionExists", err)
		}
		if len(r.callsTo("attach")) != 0 {
			t.Error("Create ran zmx attach on a live name")
		}
	})

	t.Run("it refuses an invalid name without running zmx", func(t *testing.T) {
		r := newRunner(t, nil)

		if err := newBackend(r).Create("-h", "/srv", nil); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Create error = %v, want ErrInvalidName", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("Create ran %v", r.calls)
		}
	})

	t.Run("it returns a failed attach or list as an error", func(t *testing.T) {
		boom := zmxFailure("error: boom\n")
		for _, sub := range []string{"list", "attach"} {
			replies := allReplies()
			replies["list"] = reply{}
			replies[sub] = reply{err: boom}

			if err := newBackend(newRunner(t, replies)).Create("work", "/srv", nil); !errors.Is(err, boom) {
				t.Errorf("Create with zmx %s failing = %v, want it to wrap %v", sub, err, boom)
			}
		}
	})
}

func TestAttachCommand(t *testing.T) {
	t.Run("it is zmx attach of the session", func(t *testing.T) {
		cmd := newBackend(newRunner(t, nil)).AttachCommand("dotfiles-PpaTER")

		if want := []string{"zmx", "attach", "dotfiles-PpaTER"}; !reflect.DeepEqual(cmd.Argv, want) {
			t.Errorf("Argv = %q, want %q", cmd.Argv, want)
		}
		if cmd.Dir != "" {
			t.Errorf("Dir = %q, want the caller's", cmd.Dir)
		}
	})
}

func TestKill(t *testing.T) {
	t.Run("it runs zmx kill of the session", func(t *testing.T) {
		r := newRunner(t, allReplies())

		if err := newBackend(r).Kill("present"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if got := r.calls; len(got) != 1 || !reflect.DeepEqual(got[0].cmd.Argv, []string{"zmx", "kill", "present"}) {
			t.Errorf("runs = %+v, want only zmx kill present", got)
		}
	})

	t.Run("it reports a missing session as ErrNoSuchSession", func(t *testing.T) {
		r := newRunner(t, map[string]reply{"kill": {err: zmxFailure("error: failed to kill session=gone: SessionNotFound\n")}})

		if err := newBackend(r).Kill("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill error = %v, want ErrNoSuchSession", err)
		}
	})

	t.Run("it keeps any other failure distinct from a missing session", func(t *testing.T) {
		boom := zmxFailure("error: failed to kill session=busy: SessionUnresponsive\n")
		err := newBackend(newRunner(t, map[string]reply{"kill": {err: boom}})).Kill("busy")
		if !errors.Is(err, boom) || errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill error = %v, want %v and not ErrNoSuchSession", err, boom)
		}
	})

	t.Run("it does not read another session's not-found as its own", func(t *testing.T) {
		boom := zmxFailure("error: failed to kill session=gone-2: SessionNotFound\n")
		if err := newBackend(newRunner(t, map[string]reply{"kill": {err: boom}})).Kill("gone"); errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill error = %v, want it not read as ErrNoSuchSession", err)
		}
	})

	t.Run("it refuses a name zmx kill would read as a prefix, without running zmx", func(t *testing.T) {
		r := newRunner(t, nil)

		if err := newBackend(r).Kill("dotfiles*"); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Kill error = %v, want ErrInvalidName", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("Kill ran %v", r.calls)
		}
	})
}

func TestDetach(t *testing.T) {
	t.Run("it runs zmx detach with the session named in ZMX_SESSION", func(t *testing.T) {
		r := newRunner(t, allReplies())

		if err := newBackend(r).Detach("present"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		calls := r.callsTo("detach")
		if len(calls) != 1 || !reflect.DeepEqual(calls[0].cmd.Argv, []string{"zmx", "detach"}) {
			t.Errorf("detach runs = %+v, want one bare zmx detach", calls)
		}
	})

	t.Run("it reports a missing session as ErrNoSuchSession without running zmx detach", func(t *testing.T) {
		r := newRunner(t, allReplies())

		if err := newBackend(r).Detach("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Detach error = %v, want ErrNoSuchSession", err)
		}
		if len(r.callsTo("detach")) != 0 {
			t.Error("Detach ran zmx detach for a missing session")
		}
	})

	t.Run("it returns a failed list or detach as an error", func(t *testing.T) {
		boom := zmxFailure("error: boom\n")
		for _, sub := range []string{"list", "detach"} {
			replies := allReplies()
			replies[sub] = reply{err: boom}

			if err := newBackend(newRunner(t, replies)).Detach("present"); !errors.Is(err, boom) {
				t.Errorf("Detach with zmx %s failing = %v, want it to wrap %v", sub, err, boom)
			}
		}
	})

	t.Run("it refuses an invalid name without running zmx", func(t *testing.T) {
		r := newRunner(t, nil)

		if err := newBackend(r).Detach(""); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Detach error = %v, want ErrInvalidName", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("Detach ran %v", r.calls)
		}
	})
}

func TestHistory(t *testing.T) {
	t.Run("it returns zmx history --vt verbatim", func(t *testing.T) {
		vt := "\x1b[31mred\x1b[0m  \r\n\n  \x1b[2;1H\x1b]7;file://h/srv\x1b\\\n\n"
		r := newRunner(t, map[string]reply{"history": {out: vt}})

		got, err := newBackend(r).History("present")
		if err != nil {
			t.Fatalf("History: %v", err)
		}
		if string(got) != vt {
			t.Errorf("History = %q, want %q verbatim", got, vt)
		}
		calls := r.callsTo("history")
		if len(calls) != 1 || !calls[0].raw || !reflect.DeepEqual(calls[0].cmd.Argv, []string{"zmx", "history", "present", "--vt"}) {
			t.Errorf("history runs = %+v, want one verbatim zmx history present --vt", calls)
		}
	})

	t.Run("it reports a missing session as ErrNoSuchSession", func(t *testing.T) {
		r := newRunner(t, map[string]reply{"history": {err: zmxFailure("error: session \"gone\" does not exist\n")}})

		if _, err := newBackend(r).History("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History error = %v, want ErrNoSuchSession", err)
		}
	})

	t.Run("it keeps any other failure distinct from a missing session", func(t *testing.T) {
		boom := zmxFailure("error: session \"busy\" is unresponsive (Timeout)\n")
		got, err := newBackend(newRunner(t, map[string]reply{"history": {err: boom}})).History("busy")
		if !errors.Is(err, boom) || errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History error = %v, want %v and not ErrNoSuchSession", err, boom)
		}
		if got != nil {
			t.Errorf("History = %q alongside its error, want nil", got)
		}
	})

	t.Run("it refuses a name zmx history would read as a flag, without running zmx", func(t *testing.T) {
		r := newRunner(t, nil)

		if _, err := newBackend(r).History("--vt"); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("History error = %v, want ErrInvalidName", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("History ran %v", r.calls)
		}
	})
}

func TestCurrent(t *testing.T) {
	t.Run("it is the session named by ZMX_SESSION", func(t *testing.T) {
		if got := newBackend(newRunner(t, nil), inherited...).Current(); got != "live-session" {
			t.Errorf("Current = %q, want live-session", got)
		}
	})

	t.Run("it is empty outside a session", func(t *testing.T) {
		if got := newBackend(newRunner(t, nil), "PATH=/bin").Current(); got != "" {
			t.Errorf("Current = %q, want empty", got)
		}
	})
}
