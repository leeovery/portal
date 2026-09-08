package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxerr"
)

const colonSession = "a:b"

func TestRenameSessionRefusesColon(t *testing.T) {
	t.Run("it refuses a rename to a name containing a colon", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t))

		err := client.RenameSession("old-name", colonSession)

		if err == nil {
			t.Fatal("RenameSession to a colon-bearing name returned nil; want a refusal")
		}
		if !errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
			t.Errorf("error = %v; want it to wrap tmuxerr.ErrUnaddressableSessionName", err)
		}
	})

	t.Run("it names the offending character in the refusal message", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t))

		err := client.RenameSession("old-name", colonSession)

		if err == nil {
			t.Fatal("expected a refusal, got nil")
		}
		if !strings.Contains(err.Error(), `":"`) {
			t.Errorf("refusal message %q does not name the offending %q character", err.Error(), ":")
		}
	})

	t.Run("it issues no tmux command for a refused rename", func(t *testing.T) {
		mock := commandertest.New(t)
		client := tmux.NewClient(mock)

		_ = client.RenameSession("old-name", colonSession)

		if len(mock.Calls()) != 0 {
			t.Errorf("refused rename issued %d tmux calls (%v); want none", len(mock.Calls()), mock.Calls())
		}
	})

	t.Run("it renames a colon-free name unchanged", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "rename-session"))
		client := tmux.NewClient(mock)

		if err := client.RenameSession("old-name", "new-name"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls()) != 1 {
			t.Fatalf("expected 1 call, got %d: %v", len(mock.Calls()), mock.Calls())
		}
		const wantArgs = "rename-session -t =old-name: new-name"
		if got := strings.Join(mock.Calls()[0], " "); got != wantArgs {
			t.Errorf("called with %q, want %q", got, wantArgs)
		}
	})
}

// noSuchSessionCommander answers every command the way tmux answers a target it
// cannot resolve — the same stderr for a vanished session and for a live session
// whose name the exact-target form cannot express.
func noSuchSessionCommander(target string) *commandertest.Scripted {
	return commandertest.FromFunc(func(...string) (string, error) {
		return "", &tmux.CommandError{
			Stderr: "no such session: " + target,
			Err:    errors.New("exit status 1"),
		}
	})
}

func TestShowEnvironmentClassifiesUnaddressableName(t *testing.T) {
	t.Run("it classifies an unaddressable session name as anomalous rather than natural churn", func(t *testing.T) {
		client := tmux.NewClient(noSuchSessionCommander("=" + colonSession))

		_, err := client.ShowEnvironment(colonSession)

		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if errors.Is(err, tmuxerr.ErrNoSuchSession) {
			t.Errorf("error = %v; a colon-bearing name must NOT be classified as a vanished session", err)
		}
		if !errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
			t.Errorf("error = %v; want it to wrap tmuxerr.ErrUnaddressableSessionName", err)
		}
	})

	t.Run("it still treats a genuinely vanished session as natural churn", func(t *testing.T) {
		client := tmux.NewClient(noSuchSessionCommander("=gone"))

		_, err := client.ShowEnvironment("gone")

		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !errors.Is(err, tmuxerr.ErrNoSuchSession) {
			t.Errorf("error = %v; want it to wrap tmuxerr.ErrNoSuchSession", err)
		}
		if errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
			t.Errorf("error = %v; a colon-free name is addressable", err)
		}
	})
}

// perSessionOp is one operation addressing a single session that can report a
// failure back to its caller.
type perSessionOp struct {
	name   string
	invoke func(*tmux.Client, string) error
}

// perSessionOps are the operations that mint an absence sentinel: each must
// classify the name before it hands the failure back, or a live session tmux
// cannot address exactly reads as a vanished one.
var perSessionOps = []perSessionOp{
	{"HasSessionProbe", func(c *tmux.Client, s string) error { _, err := c.HasSessionProbe(s); return err }},
	{"KillSession", func(c *tmux.Client, s string) error { return c.KillSession(s) }},
	{"RenameSession", func(c *tmux.Client, s string) error { return c.RenameSession(s, "new-name") }},
	{"SwitchClient", func(c *tmux.Client, s string) error { return c.SwitchClient(s) }},
	{"SetSessionEnvironment", func(c *tmux.Client, s string) error { return c.SetSessionEnvironment(s, "K", "v") }},
	{"ShowEnvironment", func(c *tmux.Client, s string) error { _, err := c.ShowEnvironment(s); return err }},
	{"SaverPaneID", func(c *tmux.Client, s string) error { _, err := c.SaverPaneID(s); return err }},
}

func TestPerSessionOpsClassifyUnaddressableName(t *testing.T) {
	t.Run("it classifies a colon-named session as unaddressable rather than absent", func(t *testing.T) {
		for _, op := range perSessionOps {
			t.Run(op.name, func(t *testing.T) {
				client := tmux.NewClient(noSuchSessionCommander("=" + colonSession))

				err := op.invoke(client, colonSession)

				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
					t.Errorf("error = %v; want it to wrap tmuxerr.ErrUnaddressableSessionName", err)
				}
				if errors.Is(err, tmuxerr.ErrNoSuchSession) {
					t.Errorf("error = %v; a live session tmux cannot address must not read as a vanished one", err)
				}
			})
		}
	})

	t.Run("it still reports a vanished session as no-such-session", func(t *testing.T) {
		for _, op := range perSessionOps {
			t.Run(op.name, func(t *testing.T) {
				client := tmux.NewClient(noSuchSessionCommander("=gone"))

				err := op.invoke(client, "gone")

				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !errors.Is(err, tmuxerr.ErrNoSuchSession) {
					t.Errorf("error = %v; want it to wrap tmuxerr.ErrNoSuchSession", err)
				}
				if errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
					t.Errorf("error = %v; a colon-free name is addressable", err)
				}
			})
		}
	})

	t.Run("it collapses a genuine absence but not an unaddressable name in SaverPanePIDOrAbsent", func(t *testing.T) {
		t.Run("gone session", func(t *testing.T) {
			client := tmux.NewClient(noSuchSessionCommander("=gone"))

			pid, present, err := tmux.SaverPanePIDOrAbsent(client, "gone")

			if pid != 0 || present || err != nil {
				t.Errorf("SaverPanePIDOrAbsent = (%d, %t, %v); want (0, false, nil)", pid, present, err)
			}
		})

		t.Run("colon-named session", func(t *testing.T) {
			client := tmux.NewClient(noSuchSessionCommander("=" + colonSession))

			pid, present, err := tmux.SaverPanePIDOrAbsent(client, colonSession)

			if err == nil {
				t.Fatalf("SaverPanePIDOrAbsent = (%d, %t, nil); want the unaddressable name surfaced as an error", pid, present)
			}
			if !errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
				t.Errorf("error = %v; want it to wrap tmuxerr.ErrUnaddressableSessionName", err)
			}
		})
	})
}

func TestValidateSessionName(t *testing.T) {
	t.Run("it refuses a session name beginning with $", func(t *testing.T) {
		err := tmux.ValidateSessionName("$foo")

		if err == nil {
			t.Fatal(`ValidateSessionName("$foo") = nil; want a refusal`)
		}
		if !errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
			t.Errorf("error = %v; want it to wrap tmuxerr.ErrUnaddressableSessionName", err)
		}
		if !strings.Contains(err.Error(), `"$"`) {
			t.Errorf("refusal message %q does not name the offending %q character", err.Error(), "$")
		}
	})

	t.Run("it accepts a $ that is not leading", func(t *testing.T) {
		if err := tmux.ValidateSessionName("a$b"); err != nil {
			t.Errorf(`ValidateSessionName("a$b") = %v; want nil`, err)
		}
	})

	t.Run("it accepts a name containing a period", func(t *testing.T) {
		if err := tmux.ValidateSessionName("a.b"); err != nil {
			t.Errorf(`ValidateSessionName("a.b") = %v; want nil`, err)
		}
	})
}

func TestValidateSessionNameFlagPrefix(t *testing.T) {
	t.Run("it refuses a session name beginning with a hyphen", func(t *testing.T) {
		err := tmux.ValidateSessionName("-bar")

		if err == nil {
			t.Fatal(`ValidateSessionName("-bar") = nil; want a refusal`)
		}
		if !errors.Is(err, tmuxerr.ErrUnaddressableSessionName) {
			t.Errorf("error = %v; want it to wrap tmuxerr.ErrUnaddressableSessionName", err)
		}
		if !errors.Is(err, tmux.ErrSessionNameFlagPrefix) {
			t.Errorf("error = %v; want it to wrap tmux.ErrSessionNameFlagPrefix", err)
		}
		if !strings.Contains(err.Error(), `"-"`) {
			t.Errorf("refusal message %q does not name the offending %q character", err.Error(), "-")
		}
	})

	t.Run("it accepts a hyphen that is not the first character", func(t *testing.T) {
		if err := tmux.ValidateSessionName("my-cool-app-abc123"); err != nil {
			t.Errorf(`ValidateSessionName("my-cool-app-abc123") = %v; want nil`, err)
		}
	})

	t.Run("it refuses the rename before composing the tmux argv", func(t *testing.T) {
		mock := commandertest.New(t)
		client := tmux.NewClient(mock)

		err := client.RenameSession("old-name", "-bar")

		if err == nil {
			t.Fatal("RenameSession to a hyphen-leading name returned nil; want a refusal")
		}
		if !errors.Is(err, tmux.ErrSessionNameFlagPrefix) {
			t.Errorf("error = %v; want it to wrap tmux.ErrSessionNameFlagPrefix", err)
		}
		if len(mock.Calls()) != 0 {
			t.Errorf("refused rename issued %d tmux calls (%v); want none", len(mock.Calls()), mock.Calls())
		}
	})
}
