package resolver_test

import (
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestHasGlobMeta(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "asterisk", input: "api-*", expected: true},
		{name: "question mark", input: "api-?", expected: true},
		{name: "open bracket", input: "api-[12]", expected: true},
		{name: "bare bracket start", input: "foo[", expected: true},
		{name: "plain name", input: "api", expected: false},
		{name: "nanoid session name", input: "api-x7Kd9a", expected: false},
		{name: "path-like tilde", input: "~/Code/blog", expected: false},
		{name: "path-like dot", input: "./mydir", expected: false},
		{name: "closing bracket alone is not a starter", input: "foo]", expected: false},
		{name: "empty", input: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolver.HasGlobMeta(tt.input); got != tt.expected {
				t.Errorf("HasGlobMeta(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMatchSessions(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		names    []string
		expected []string
	}{
		{
			name:     "prefix glob matches subset in order",
			pattern:  "api-*",
			names:    []string{"api-1", "web-3", "api-2"},
			expected: []string{"api-1", "api-2"},
		},
		{
			name:     "no matches",
			pattern:  "api-*",
			names:    []string{"web-3"},
			expected: []string{},
		},
		{
			name:     "malformed pattern yields no matches",
			pattern:  "foo[",
			names:    []string{"foo1", "foo2"},
			expected: []string{},
		},
		{
			name:     "empty name set yields no matches",
			pattern:  "api-*",
			names:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.MatchSessions(tt.pattern, tt.names)
			if len(got) != len(tt.expected) {
				t.Fatalf("MatchSessions(%q, %v) = %v, want %v", tt.pattern, tt.names, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("MatchSessions(%q, %v)[%d] = %q, want %q", tt.pattern, tt.names, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// TestQueryResolver_Resolve_GlobFallsThroughToMiss pins the NON-glob-only contract
// of the single-target Resolve chain. Glob expansion is EXCLUSIVELY the burst's job
// (ResolveBareAll → expandSessionGlobAll, all-match — see query_all_test.go): the
// single glob-expansion primitive fans a pattern out to EVERY matching session.
// Resolve itself no longer pre-checks or expands globs. A glob value reaching
// Resolve is never a literal session name, never a path argument (session-style
// globs carry no '/', '.', '~'), never an alias key, and never a zoxide hit, so it
// falls through the whole session→path→alias→zoxide chain to a *MissResult — a LOUD
// hard-fail via the caller, NEVER a silent first-match. This is the safety net for
// the routing assumption: if a glob ever slips past the multi-target gate into
// Resolve, it fails loudly instead of degrading "burst every match" to "attach the
// first". (The all-match burst behaviour lives in TestQueryResolver_ResolveBareAll /
// TestQueryResolver_ResolveSessionPinAll and stays unchanged.)
func TestQueryResolver_Resolve_GlobFallsThroughToMiss(t *testing.T) {
	t.Run("multi-match session glob does NOT collapse to the first match", func(t *testing.T) {
		// The regression this task removes: the old glob pre-check returned
		// matches[0] (api-1) as a SessionResult{Domain:"glob"}. With the branch
		// gone the glob falls through to a miss — glob fan-out is the burst's job.
		sessions := &mockSessionLister{names: []string{"api-1", "api-2", "web-3"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if sr, ok := result.(*resolver.SessionResult); ok {
			t.Fatalf("glob must not collapse to a first-match SessionResult, got %+v", sr)
		}
		mr, ok := result.(*resolver.MissResult)
		if !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
		if mr.Target != "api-*" {
			t.Errorf("MissResult.Target = %q, want %q", mr.Target, "api-*")
		}
	})

	t.Run("session glob matching zero sessions falls through to miss", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{"web-3"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mr, ok := result.(*resolver.MissResult)
		if !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
		if mr.Target != "api-*" {
			t.Errorf("MissResult.Target = %q, want %q", mr.Target, "api-*")
		}
	})

	t.Run("session glob matching only internal sessions falls through to miss", func(t *testing.T) {
		// The lister returns the leading-underscore-filtered view, so a glob
		// that would match only internal _portal-* sessions sees an empty set.
		sessions := &mockSessionLister{names: []string{"api-1"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("_portal-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mr, ok := result.(*resolver.MissResult)
		if !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
		if mr.Target != "_portal-*" {
			t.Errorf("MissResult.Target = %q, want %q", mr.Target, "_portal-*")
		}
	})

	t.Run("malformed glob falls through to miss", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{"foo1", "foo2"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("foo[")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mr, ok := result.(*resolver.MissResult)
		if !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
		if mr.Target != "foo[" {
			t.Errorf("MissResult.Target = %q, want %q", mr.Target, "foo[")
		}
	})
}
