package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/spf13/cobra"
)

// stageCompletion stages a backend holding sessions named names, with current
// as the session this process runs in, "" for none.
func stageCompletion(t *testing.T, current string, names ...string) *backendtest.Fake {
	t.Helper()
	fake := backendtest.New()
	for _, name := range names {
		fake.Add(backend.Session{Name: name})
	}
	fake.SetCurrent(current)
	withBackendDeps(t, BackendDeps{Backend: fake})
	return fake
}

// stageFailedCompletionListing stages a backend whose listing fails.
func stageFailedCompletionListing(t *testing.T) {
	t.Helper()
	fake := stageCompletion(t, "", "api-1")
	fake.Fail(backendtest.OpList, errors.New("read zmx dir: permission denied"))
}

func TestCompleteSessionNames(t *testing.T) {
	t.Run("returns all names plus NoFileComp for empty prefix", func(t *testing.T) {
		stageCompletion(t, "", "api-1", "web-2")

		names, directive := completeSessionNames("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "web-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("prefix-filters by toComplete", func(t *testing.T) {
		stageCompletion(t, "", "api-1", "api-2", "web-3")

		names, directive := completeSessionNames("ap")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "api-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it still offers the attached session on the plain session-name completer", func(t *testing.T) {
		stageCompletion(t, "portal-a1b2", "portal-a1b2", "web-9")

		names, _ := completeSessionNames("")

		if want := []string{"portal-a1b2", "web-9"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v (-s and the bare positional are unchanged)", names, want)
		}
	})

	t.Run("it offers a labelled session by its name, never its label", func(t *testing.T) {
		stageCompletion(t, "", "api-x7Kd9a", "web-Qm3pLz")
		setLabel(t, isolateStateDir(t), "web-Qm3pLz", "web checkout")

		if names, _ := completeSessionNames("w"); !slices.Equal(names, []string{"web-Qm3pLz"}) {
			t.Errorf("names = %v, want [web-Qm3pLz]", names)
		}
		if names, _ := completeSearchTerm("/web"); !slices.Equal(names, []string{"/web-Qm3pLz"}) {
			t.Errorf("search names = %v, want [/web-Qm3pLz]", names)
		}
	})

	t.Run("offers no names when the listing fails", func(t *testing.T) {
		stageFailedCompletionListing(t)

		names, directive := completeSessionNames("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none", names)
		}
	})

	t.Run("offers no names when there are no sessions", func(t *testing.T) {
		stageCompletion(t, "")

		names, directive := completeSessionNames("")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want empty slice", names)
		}
	})
}

func withCompletionAliasKeys(t *testing.T, fn func() []string) {
	t.Helper()
	withFuncSeam(t, &completionAliasKeys, fn)
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
		stageCompletion(t, "", "api-1", "web-2")

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
		stageCompletion(t, "", "api-1")

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
		stageCompletion(t, "", "api-1", "web-2")

		names, directive := killCmd.ValidArgsFunction(killCmd, nil, "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"api-1", "web-2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("kill offers nothing once one positional present", func(t *testing.T) {
		fake := stageCompletion(t, "", "api-1")

		names, directive := killCmd.ValidArgsFunction(killCmd, []string{"x"}, "")
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none once a positional is present", calls)
		}

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if names != nil {
			t.Errorf("names = %v, want nil (ExactArgs(1) — only the first positional completes)", names)
		}
	})
}

func TestCompletionExcludesInternalSessions(t *testing.T) {
	stageCompletion(t, "", "_portal-saver", "my-work")

	names, directive := completeSessionNames("")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if want := []string{"my-work"}; !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v (internal _-prefixed sessions must be filtered)", names, want)
	}

	searchNames, directive := completeSearchTerm("/")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if want := []string{"/my-work"}; !slices.Equal(searchNames, want) {
		t.Errorf("search names = %v, want %v (internal _-prefixed sessions must be filtered)", searchNames, want)
	}
}

// Goes through the real root command, so cobra's own filtering applies to the
// returned candidates.
func completionCandidates(t *testing.T, args ...string) []string {
	t.Helper()
	cands, _ := completionResult(t, args...)
	return cands
}

// completionResult returns both halves of a __complete answer: the candidates
// and the directive cobra ends the output with.
func completionResult(t *testing.T, args ...string) ([]string, cobra.ShellCompDirective) {
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
	directive := cobra.ShellCompDirectiveDefault
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if line == "" {
			continue
		}
		if raw, found := strings.CutPrefix(line, ":"); found {
			n, err := strconv.Atoi(raw)
			if err != nil {
				t.Fatalf("__complete %v: undecodable directive line %q: %v", args, line, err)
			}
			directive = cobra.ShellCompDirective(n)
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		cands = append(cands, name)
	}
	return cands, directive
}

func TestCompletionHidesInternalSurface(t *testing.T) {
	t.Run("open flag completion excludes the hidden --ack flag", func(t *testing.T) {
		// toComplete "-" makes cobra emit flag-name candidates.
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

func TestCompleteSearchTerm(t *testing.T) {
	t.Run("it completes the term after the slash and keeps the slash on the candidate", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		names, directive := completeSearchTerm("/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"/portal-a1b2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it offers every live session name for a bare slash", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		names, directive := completeSearchTerm("/")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"/portal-a1b2", "/web-9"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it offers nothing for a term that prefixes no name", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		names, directive := completeSearchTerm("/ort")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none (the offer is prefix-shaped)", names)
		}
	})

	t.Run("it does not offer the session the caller is attached to", func(t *testing.T) {
		stageCompletion(t, "portal-a1b2", "portal-a1b2", "port-agent")

		names, directive := completeSearchTerm("/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"/port-agent"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v (the attached session is unreachable through the sigil)", names, want)
		}
	})

	t.Run("it holds back the attached session for a bare slash too", func(t *testing.T) {
		stageCompletion(t, "portal-a1b2", "portal-a1b2", "web-9")

		names, _ := completeSearchTerm("/")

		if want := []string{"/web-9"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it offers every live name outside a session", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		names, _ := completeSearchTerm("/")

		if want := []string{"/portal-a1b2", "/web-9"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v (no current session drops nothing)", names, want)
		}
	})

	t.Run("it offers no candidates when the session read fails", func(t *testing.T) {
		stageFailedCompletionListing(t)

		names, directive := completeSearchTerm("/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none", names)
		}
	})
}

func TestCompleteOpenPositional(t *testing.T) {
	t.Run("it routes a search word through the search branch", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		names, directive := completeOpenPositional(&cobra.Command{}, nil, "/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"/portal-a1b2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it leaves a non-search word on today's completer", func(t *testing.T) {
		// A word carrying a second slash is a path shape, not the search form: it
		// must never be answered with session names stitched behind a slash.
		tests := []struct {
			name       string
			toComplete string
			want       []string
		}{
			{name: "absolute path with a second segment", toComplete: "/Users/lee"},
			{name: "single segment with a trailing slash", toComplete: "/tmp/"},
			{name: "tilde path", toComplete: "~/Code/pro"},
			{name: "dot-relative path", toComplete: "."},
			{name: "bare word", toComplete: "we", want: []string{"web-9"}},
			{name: "empty word", toComplete: "", want: []string{"portal-a1b2", "web-9"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stageCompletion(t, "", "portal-a1b2", "web-9")

				names, directive := completeOpenPositional(&cobra.Command{}, nil, tt.toComplete)

				if directive != cobra.ShellCompDirectiveNoFileComp {
					t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
				}
				plain, _ := completeSessionNames(tt.toComplete)
				if !slices.Equal(names, plain) {
					t.Errorf("names = %v, want %v (identical to completeSessionNames)", names, plain)
				}
				if !slices.Equal(names, tt.want) {
					t.Errorf("names = %v, want %v", names, tt.want)
				}
			})
		}
	})
}

func TestSearchCompletionWiring(t *testing.T) {
	t.Run("it routes open's positional completer through the search branch", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		names, directive := openCmd.ValidArgsFunction(openCmd, nil, "/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"/portal-a1b2"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it answers a search word end to end through __complete", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		cands := completionCandidates(t, "__complete", "open", "/po")

		if want := []string{"/portal-a1b2"}; !slices.Equal(cands, want) {
			t.Errorf("candidates = %v, want %v", cands, want)
		}
	})

	t.Run("it leaves the --session flag completer on plain session names", func(t *testing.T) {
		fn, ok := openCmd.GetFlagCompletionFunc("session")
		if !ok {
			t.Fatal("--session flag completion not registered")
		}
		stageCompletion(t, "", "portal-a1b2")

		names, directive := fn(openCmd, nil, "/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none (a flag value keeps the plain name completer)", names)
		}
	})

	t.Run("it leaves kill's positional completer unchanged", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2")

		names, directive := killCmd.ValidArgsFunction(killCmd, nil, "/po")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none", names)
		}
	})

	t.Run("it leaves a second positional on today's behaviour", func(t *testing.T) {
		stageCompletion(t, "", "portal-a1b2", "web-9")

		// Routed through __complete rather than called directly: the completer
		// reads openCmd's dash index, which pflag carries over from whatever a
		// sibling test last parsed, and only a real completion run parses the
		// line being asserted about.
		names, directive := completionResult(t, "__complete", "open", "/term", "")

		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
		}
		if want := []string{"portal-a1b2", "web-9"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})
}

func TestCompleteOpenPositionalSeparatorBound(t *testing.T) {
	twoSessions := func(t *testing.T) { stageCompletion(t, "", "portal-a1b2", "web-9") }

	t.Run("it offers no sigil completion for a word among a trailing command's arguments", func(t *testing.T) {
		twoSessions(t)

		cands := completionCandidates(t, "__complete", "open", "~/Code/api", "--", "ls", "/po")

		for _, cand := range cands {
			if strings.HasPrefix(cand, "/") {
				t.Errorf("candidates = %v, want no /-prefixed candidate for a word past the separator", cands)
			}
		}
	})

	t.Run("it offers no session names for a word among a trailing command's arguments", func(t *testing.T) {
		twoSessions(t)

		cands, directive := completionResult(t, "__complete", "open", "~/Code/api", "--", "ls", "/po")

		if directive != cobra.ShellCompDirectiveDefault {
			t.Errorf("directive = %v, want ShellCompDirectiveDefault", directive)
		}
		if len(cands) != 0 {
			t.Errorf("candidates = %v, want none past the separator", cands)
		}
	})

	t.Run("it leaves filename completion on for a path argument to the trailing command", func(t *testing.T) {
		twoSessions(t)

		cands, directive := completionResult(t, "__complete", "open", "~/Code/api", "--", "ls", "src/")

		if directive != cobra.ShellCompDirectiveDefault {
			t.Errorf("directive = %v, want ShellCompDirectiveDefault", directive)
		}
		if len(cands) != 0 {
			t.Errorf("candidates = %v, want none past the separator", cands)
		}
	})

	t.Run("it still offers sigil completions for a pre-dash word on a line with no separator", func(t *testing.T) {
		twoSessions(t)

		cands := completionCandidates(t, "__complete", "open", "/po")

		if want := []string{"/portal-a1b2"}; !slices.Equal(cands, want) {
			t.Errorf("candidates = %v, want %v", cands, want)
		}
	})

	t.Run("it still offers sigil completions for a pre-dash word beside another target", func(t *testing.T) {
		twoSessions(t)

		cands := completionCandidates(t, "__complete", "open", "api", "/po")

		if want := []string{"/portal-a1b2"}; !slices.Equal(cands, want) {
			t.Errorf("candidates = %v, want %v", cands, want)
		}
	})

	t.Run("it still offers sigil completions for a pre-dash word on a line carrying -f", func(t *testing.T) {
		twoSessions(t)

		cands := completionCandidates(t, "__complete", "open", "--filter", "api", "/po")

		if want := []string{"/portal-a1b2"}; !slices.Equal(cands, want) {
			t.Errorf("candidates = %v, want %v", cands, want)
		}
	})

	t.Run("it keeps the sigil arm for the word immediately after the separator", func(t *testing.T) {
		twoSessions(t)

		cands := completionCandidates(t, "__complete", "open", "--", "/po")

		if want := []string{"/portal-a1b2"}; !slices.Equal(cands, want) {
			t.Errorf("candidates = %v, want %v", cands, want)
		}
	})

	t.Run("it keeps the sigil arm for the boundary word beside a target", func(t *testing.T) {
		twoSessions(t)

		cands := completionCandidates(t, "__complete", "open", "api", "--", "/po")

		if want := []string{"/portal-a1b2"}; !slices.Equal(cands, want) {
			t.Errorf("candidates = %v, want %v", cands, want)
		}
	})

	t.Run("it reads a line with no separator as pre-dash despite cobra's probe parse", func(t *testing.T) {
		cmd := &cobra.Command{Use: "open"}
		// The two parses cobra runs before calling a completer: the probe with an
		// appended separator, then the line itself.
		if err := cmd.ParseFlags([]string{"api", "--"}); err != nil {
			t.Fatalf("probe parse: %v", err)
		}
		if err := cmd.ParseFlags([]string{"api"}); err != nil {
			t.Fatalf("parse: %v", err)
		}

		if dash := cmd.ArgsLenAtDash(); dash < 0 {
			t.Fatalf("ArgsLenAtDash() = %d, want a non-negative index left by the probe parse", dash)
		}
		if !completingPreDashPositional(cmd, []string{"api"}) {
			t.Error("completingPreDashPositional() = false, want true for a line carrying no separator")
		}
	})
}
