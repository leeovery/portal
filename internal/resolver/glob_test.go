package resolver_test

import (
	"slices"
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

// trackingAliasLookup records whether Get was consulted, so a test can prove the
// glob pre-check returns before the directory chain runs.
type trackingAliasLookup struct {
	aliases map[string]string
	called  bool
}

func (t *trackingAliasLookup) Get(name string) (string, bool) {
	t.called = true
	path, ok := t.aliases[name]
	return path, ok
}

func (t *trackingAliasLookup) Keys() []string {
	keys := make([]string, 0, len(t.aliases))
	for name := range t.aliases {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}

func TestQueryResolver_Resolve_GlobPreCheck(t *testing.T) {
	t.Run("session glob expands to first user-visible match", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{"api-1", "api-2", "web-3"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", result)
		}
		if sr.Name != "api-1" {
			t.Errorf("SessionResult.Name = %q, want %q", sr.Name, "api-1")
		}
		if sr.Domain != "glob" {
			t.Errorf("SessionResult.Domain = %q, want %q", sr.Domain, "glob")
		}
	})

	t.Run("glob matching zero user-visible sessions hard-fails", func(t *testing.T) {
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

	t.Run("glob matching only internal sessions counts as zero", func(t *testing.T) {
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

	t.Run("glob skips directory chain even when same-named alias exists", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{"api-1"}}
		aliasLookup := &trackingAliasLookup{aliases: map[string]string{"api-*": "/some/alias/dir"}}
		zoxideCalled := false
		zoxide := &trackingZoxideQuerier{
			result:  "/zoxide/path",
			onQuery: func() { zoxideCalled = true },
		}
		dirValidator := &mockDirValidator{existing: map[string]bool{"/some/alias/dir": true}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := result.(*resolver.SessionResult); !ok {
			t.Fatalf("expected *SessionResult (glob wins over alias), got %T", result)
		}
		if aliasLookup.called {
			t.Error("alias lookup should not be consulted for a glob target")
		}
		if zoxideCalled {
			t.Error("zoxide should not be consulted for a glob target")
		}
	})

	t.Run("path with glob metacharacters is unreachable as a bare positional", func(t *testing.T) {
		// A directory path whose name contains glob metacharacters is captured
		// by the glob pre-check, matches zero sessions, and hard-fails — it is
		// never routed to ResolvePath (reachable only via -p in Phase 2).
		globPaths := []string{"foo[1]", "~/tmp/foo[1]"}
		for _, query := range globPaths {
			t.Run(query, func(t *testing.T) {
				sessions := &mockSessionLister{names: []string{"api-1"}}
				aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
				zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
				dirValidator := &mockDirValidator{existing: map[string]bool{}}

				qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
				result, err := qr.Resolve(query)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if _, ok := result.(*resolver.PathResult); ok {
					t.Fatalf("expected *MissResult, got *PathResult (glob pre-check bypassed)")
				}
				mr, ok := result.(*resolver.MissResult)
				if !ok {
					t.Fatalf("expected *MissResult, got %T", result)
				}
				if mr.Target != query {
					t.Errorf("MissResult.Target = %q, want %q", mr.Target, query)
				}
			})
		}
	})

	t.Run("malformed glob yields zero matches and hard-fails", func(t *testing.T) {
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
