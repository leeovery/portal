package cmd

// Tests in this file mutate the package-level completionSessionNames seam and
// MUST NOT use t.Parallel.

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// withCompletionAliasKeys overrides the injectable alias-key seam for the
// duration of a test and restores it via t.Cleanup. The seam is the ONLY
// config-file touch-point of the alias completer; overriding it keeps the
// completion tests hermetic (no real aliases file read, no tmux client).
func withCompletionAliasKeys(t *testing.T, fn func() []string) {
	t.Helper()
	prev := completionAliasKeys
	completionAliasKeys = fn
	t.Cleanup(func() { completionAliasKeys = prev })
}

func TestCompleteAliasKeys(t *testing.T) {
	t.Run("returns all keys plus NoFileComp for empty prefix", func(t *testing.T) {
		withCompletionAliasKeys(t, func() []string { return []string{"blog", "work"} })

		keys, directive := completeAliasKeys("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"blog", "work"}; !slices.Equal(keys, want) {
			t.Errorf("keys = %v, want %v", keys, want)
		}
	})

	t.Run("prefix-filters by toComplete", func(t *testing.T) {
		withCompletionAliasKeys(t, func() []string { return []string{"work", "web"} })

		keys, directive := completeAliasKeys("wo")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"work"}; !slices.Equal(keys, want) {
			t.Errorf("keys = %v, want %v", keys, want)
		}
	})

	t.Run("empty and no panic when seam returns nil (missing aliases file)", func(t *testing.T) {
		withCompletionAliasKeys(t, func() []string { return nil })

		keys, directive := completeAliasKeys("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(keys) != 0 {
			t.Errorf("keys = %v, want empty slice", keys)
		}
	})
}

// TestCompletionAliasKeysProductionSeam exercises the REAL completionAliasKeys
// seam (loadAliasStore -> config-path aliases file via PORTAL_ALIASES_FILE),
// proving it needs no tmux client (pure config-file I/O) and that a seeded file
// yields its keys while a missing file yields nothing (no error, no panic).
func TestCompletionAliasKeysProductionSeam(t *testing.T) {
	t.Run("loads keys from the seeded aliases file", func(t *testing.T) {
		aliasFile := filepath.Join(t.TempDir(), "aliases")
		if err := os.WriteFile(aliasFile, []byte("work=/w\nblog=/b\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		keys, directive := completeAliasKeys("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"blog", "work"}; !slices.Equal(keys, want) {
			t.Errorf("keys = %v, want %v", keys, want)
		}
	})

	t.Run("missing aliases file yields no suggestions", func(t *testing.T) {
		t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(t.TempDir(), "does-not-exist"))

		keys, directive := completeAliasKeys("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(keys) != 0 {
			t.Errorf("keys = %v, want empty slice", keys)
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

	t.Run("open --alias flag completion registered and routes through completeAliasKeys", func(t *testing.T) {
		fn, ok := openCmd.GetFlagCompletionFunc("alias")
		if !ok {
			t.Fatal("--alias flag completion not registered")
		}
		withCompletionAliasKeys(t, func() []string { return []string{"work"} })

		keys, directive := fn(openCmd, nil, "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"work"}; !slices.Equal(keys, want) {
			t.Errorf("keys = %v, want %v", keys, want)
		}
	})

	t.Run("open --path has no Portal completion function (falls to shell)", func(t *testing.T) {
		if _, ok := openCmd.GetFlagCompletionFunc("path"); ok {
			t.Error("--path must have NO Portal completion func — cobra emits ShellCompDirectiveDefault so the shell provides path completion")
		}
	})

	t.Run("open --zoxide has no Portal completion function (falls to shell)", func(t *testing.T) {
		if _, ok := openCmd.GetFlagCompletionFunc("zoxide"); ok {
			t.Error("--zoxide must have NO Portal completion func — cobra emits ShellCompDirectiveDefault so the shell / zoxide provides completion")
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

// completionCandidates drives cobra's hidden __complete request verb through the
// real root command and returns the candidate tokens (the text before the first
// TAB on each line), dropping the trailing ":<directive>" line. It is the
// behavioural probe for "what the shell would be offered", so a hidden flag or a
// hidden command (filtered by cobra itself) is provably absent from the result.
// __complete is bootstrap-exempt (skipTmuxCheck), so no tmux client or injected
// deps are needed on the flag/subcommand paths exercised here.
func completionCandidates(t *testing.T, args ...string) []string {
	t.Helper()
	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("__complete %v: %v", args, err)
	}

	var cands []string
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		cands = append(cands, name)
	}
	return cands
}

// TestCompletionHidesInternalSurface proves cobra's generated completion never
// offers Portal's internal surface: the hidden --ack flag is absent from `open`'s
// flag completion, and the hidden `state` namespace is absent from top-level
// command completion. Each subtest also asserts a VISIBLE sibling IS offered, so
// the probe is non-vacuous (it fails if completion produced nothing at all).
func TestCompletionHidesInternalSurface(t *testing.T) {
	t.Run("open flag completion excludes the hidden --ack flag", func(t *testing.T) {
		// toComplete "-" makes cobra emit flag-name candidates for open.
		cands := completionCandidates(t, "__complete", "open", "-")
		if slices.Contains(cands, "--ack") {
			t.Errorf("open flag completion offered the hidden --ack flag; candidates=%v", cands)
		}
		if !slices.Contains(cands, "--session") {
			t.Errorf("open flag completion did not offer the visible --session flag; candidates=%v", cands)
		}
	})

	t.Run("top-level completion excludes the hidden state namespace", func(t *testing.T) {
		cands := completionCandidates(t, "__complete", "")
		if slices.Contains(cands, "state") {
			t.Errorf("top-level completion offered the hidden state namespace; candidates=%v", cands)
		}
		if !slices.Contains(cands, "open") {
			t.Errorf("top-level completion did not offer the visible open command; candidates=%v", cands)
		}
	})
}
