package cmd

// Tests in this file mutate the package-level completionSessionNames seam and
// MUST NOT use t.Parallel.

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/spf13/cobra"
)

// withCompletionSessionNames overrides the injectable session-name seam for the
// duration of a test and restores it via t.Cleanup. The seam is the ONLY tmux
// touch-point of the completer; overriding it keeps the completion tests
// hermetic (no real tmux client, no bootstrap).
func withCompletionSessionNames(t *testing.T, fn func() []string) {
	t.Helper()
	prev := completionSessionNames
	completionSessionNames = fn
	t.Cleanup(func() { completionSessionNames = prev })
}

func TestCompleteSessionNames(t *testing.T) {
	t.Run("returns all names plus NoFileComp for empty prefix", func(t *testing.T) {
		withCompletionSessionNames(t, func() []string { return []string{"api-1", "web-2"} })

		names, directive := completeSessionNames("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "web-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("prefix-filters by toComplete", func(t *testing.T) {
		withCompletionSessionNames(t, func() []string { return []string{"api-1", "api-2", "web-3"} })

		names, directive := completeSessionNames("ap")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "api-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("empty and no panic when seam returns nil (server down)", func(t *testing.T) {
		withCompletionSessionNames(t, func() []string { return nil })

		names, directive := completeSessionNames("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want empty slice", names)
		}
	})
}

func TestCompletionWiring(t *testing.T) {
	t.Run("open positional routes through completeSessionNames", func(t *testing.T) {
		if openCmd.ValidArgsFunction == nil {
			t.Fatal("openCmd.ValidArgsFunction is nil; expected session-name completer")
		}
		withCompletionSessionNames(t, func() []string { return []string{"api-1", "web-2"} })

		names, directive := openCmd.ValidArgsFunction(openCmd, nil, "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "web-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("open --session flag completion registered and routes through helper", func(t *testing.T) {
		fn, ok := openCmd.GetFlagCompletionFunc("session")
		if !ok {
			t.Fatal("--session flag completion not registered")
		}
		withCompletionSessionNames(t, func() []string { return []string{"api-1"} })

		names, directive := fn(openCmd, nil, "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("kill positional completes session names when no args", func(t *testing.T) {
		if killCmd.ValidArgsFunction == nil {
			t.Fatal("killCmd.ValidArgsFunction is nil; expected session-name completer")
		}
		withCompletionSessionNames(t, func() []string { return []string{"api-1", "web-2"} })

		names, directive := killCmd.ValidArgsFunction(killCmd, nil, "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "web-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("kill offers nothing once one positional present", func(t *testing.T) {
		withCompletionSessionNames(t, func() []string {
			t.Error("seam must not be called once a positional is present")
			return nil
		})

		names, directive := killCmd.ValidArgsFunction(killCmd, []string{"x"}, "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if names != nil {
			t.Errorf("names = %v, want nil (ExactArgs(1) — only the first positional completes)", names)
		}
	})
}

// TestCompletionExcludesInternalSessions proves the leading-underscore internal
// sessions (_portal-saver / _portal-bootstrap / etc.) are never suggested,
// inherited from ListSessionNames -> ListSessions' underscore-prefix drop. Uses
// a real per-test tmux -L socket (UNIT lane: fast real-tmux client, no daemon,
// no built binary, not integration-tagged).
func TestCompletionExcludesInternalSessions(t *testing.T) {
	socket := tmuxtest.New(t, "ptl-complete-")
	socket.Run(t, "new-session", "-d", "-s", "my-work")
	socket.Run(t, "new-session", "-d", "-s", "_portal-x")

	client := socket.Client()
	withCompletionSessionNames(t, func() []string {
		names, err := client.ListSessionNames()
		if err != nil {
			return nil
		}
		return names
	})

	names, directive := completeSessionNames("")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if want := []string{"my-work"}; !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v (internal _-prefixed sessions must be filtered)", names, want)
	}
}
