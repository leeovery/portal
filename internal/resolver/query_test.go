package resolver_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

// mockAliasLookup implements resolver.AliasLookup for testing.
type mockAliasLookup struct {
	aliases map[string]string
}

func (m *mockAliasLookup) Get(name string) (string, bool) {
	path, ok := m.aliases[name]
	return path, ok
}

func (m *mockAliasLookup) Keys() []string {
	keys := make([]string, 0, len(m.aliases))
	for name := range m.aliases {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	return keys
}

// mockZoxideQuerier implements resolver.ZoxideQuerier for testing.
type mockZoxideQuerier struct {
	result string
	err    error
}

func (m *mockZoxideQuerier) Query(terms string) (string, error) {
	return m.result, m.err
}

// mockDirValidator implements resolver.DirValidator for testing.
type mockDirValidator struct {
	existing map[string]bool
}

func (m *mockDirValidator) Exists(path string) bool {
	return m.existing[path]
}

// mockSessionLister implements resolver.SessionLister for testing. names is the
// user-visible (leading-underscore-filtered) session set — the same view the
// real tmux client returns from ListSessionNames.
type mockSessionLister struct {
	names []string
	err   error
}

func (m *mockSessionLister) ListSessionNames() ([]string, error) {
	return m.names, m.err
}

func TestQueryResolver_Resolve(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		aliases      map[string]string
		zoxideResult string
		zoxideErr    error
		existingDirs map[string]bool
		wantPath     string
		wantDomain   resolver.Domain
		wantMiss     bool
		wantTarget   string
		wantErr      string
	}{
		{
			name:         "non-path argument resolved via alias",
			query:        "myapp",
			aliases:      map[string]string{"myapp": "/Users/lee/Code/myapp"},
			zoxideResult: "/some/other/path",
			zoxideErr:    nil,
			existingDirs: map[string]bool{"/Users/lee/Code/myapp": true},
			wantPath:     "/Users/lee/Code/myapp",
			wantDomain:   "alias",
		},
		{
			name:         "alias miss falls through to zoxide",
			query:        "proj",
			aliases:      map[string]string{},
			zoxideResult: "/Users/lee/Code/proj",
			zoxideErr:    nil,
			existingDirs: map[string]bool{"/Users/lee/Code/proj": true},
			wantPath:     "/Users/lee/Code/proj",
			wantDomain:   "zoxide",
		},
		{
			name:         "zoxide miss falls through to miss",
			query:        "unknown",
			aliases:      map[string]string{},
			zoxideResult: "",
			zoxideErr:    resolver.ErrNoMatch,
			existingDirs: map[string]bool{},
			wantMiss:     true,
			wantTarget:   "unknown",
		},
		{
			name:         "miss result carries raw target string",
			query:        "searchterm",
			aliases:      map[string]string{},
			zoxideResult: "",
			zoxideErr:    resolver.ErrNoMatch,
			existingDirs: map[string]bool{},
			wantMiss:     true,
			wantTarget:   "searchterm",
		},
		{
			name:         "resolved alias directory validated for existence",
			query:        "stale",
			aliases:      map[string]string{"stale": "/nonexistent/path"},
			zoxideResult: "",
			zoxideErr:    resolver.ErrNoMatch,
			existingDirs: map[string]bool{},
			wantErr:      "Directory not found: /nonexistent/path",
		},
		{
			name:         "resolved zoxide directory validated for existence",
			query:        "proj",
			aliases:      map[string]string{},
			zoxideResult: "/gone/directory",
			zoxideErr:    nil,
			existingDirs: map[string]bool{},
			wantErr:      "Directory not found: /gone/directory",
		},
		{
			name:         "zoxide not installed skipped silently",
			query:        "myquery",
			aliases:      map[string]string{},
			zoxideResult: "",
			zoxideErr:    resolver.ErrZoxideNotInstalled,
			existingDirs: map[string]bool{},
			wantMiss:     true,
			wantTarget:   "myquery",
		},
		{
			name:         "query matches alias; alias path used not zoxide",
			query:        "myapp",
			aliases:      map[string]string{"myapp": "/alias/path"},
			zoxideResult: "/zoxide/path",
			zoxideErr:    nil,
			existingDirs: map[string]bool{"/alias/path": true, "/zoxide/path": true},
			wantPath:     "/alias/path",
			wantDomain:   "alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliasLookup := &mockAliasLookup{aliases: tt.aliases}
			zoxide := &mockZoxideQuerier{result: tt.zoxideResult, err: tt.zoxideErr}
			dirValidator := &mockDirValidator{existing: tt.existingDirs}

			qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
			result, err := qr.Resolve(tt.query)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantMiss {
				mr, ok := result.(*resolver.MissResult)
				if !ok {
					t.Fatalf("expected MissResult, got %T", result)
				}
				if mr.Target != tt.wantTarget {
					t.Errorf("MissResult.Target = %q, want %q", mr.Target, tt.wantTarget)
				}
				return
			}

			pr, ok := result.(*resolver.PathResult)
			if !ok {
				t.Fatalf("expected PathResult, got %T", result)
			}
			if pr.Path != tt.wantPath {
				t.Errorf("PathResult.Path = %q, want %q", pr.Path, tt.wantPath)
			}
			if tt.wantDomain != "" && pr.Domain != tt.wantDomain {
				t.Errorf("PathResult.Domain = %q, want %q", pr.Domain, tt.wantDomain)
			}
		})
	}
}

func TestQueryResolver_Resolve_PathLikeArguments(t *testing.T) {
	t.Run("path containing / resolved directly", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)
		subDir := filepath.Join(dir, "mydir")
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		// Query contains "/" which triggers path-like detection
		query := subDir + "/."

		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve(query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected PathResult, got %T", result)
		}
		if pr.Path != subDir {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, subDir)
		}
		if pr.Domain != "path" {
			t.Errorf("PathResult.Domain = %q, want %q", pr.Domain, "path")
		}
	})

	t.Run("path starting with . resolved directly", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)
		subDir := filepath.Join(dir, "mydir")
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		// Query starts with "." which triggers path-like detection
		query := "./mydir"

		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		// Change working directory so ./mydir resolves to our temp subdir
		t.Chdir(dir)

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve(query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected PathResult, got %T", result)
		}
		if pr.Path != subDir {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, subDir)
		}
	})

	t.Run("path starting with ~ resolved directly", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		// Query starts with "~" which triggers path-like detection
		// ~ itself is a valid directory (user home)
		query := "~"

		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve(query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected PathResult, got %T", result)
		}
		if pr.Path != home {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, home)
		}
	})
}

func TestQueryResolver_Resolve_PathLikeNotSentToAliasOrZoxide(t *testing.T) {
	t.Run("path containing / not sent through alias/zoxide chain", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)

		aliasLookup := &mockAliasLookup{aliases: map[string]string{dir: "/some/alias/path"}}
		zoxideCalled := false
		zoxide := &trackingZoxideQuerier{
			result:  "/zoxide/path",
			onQuery: func() { zoxideCalled = true },
		}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		_, err := qr.Resolve(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if zoxideCalled {
			t.Error("zoxide should not be called for path-like arguments")
		}
	})

	t.Run("path starting with . not sent through alias/zoxide chain", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)
		subDir := filepath.Join(dir, "mydir")
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		t.Chdir(dir)

		aliasLookup := &mockAliasLookup{aliases: map[string]string{"./mydir": "/some/alias/path"}}
		zoxideCalled := false
		zoxide := &trackingZoxideQuerier{
			result:  "/zoxide/path",
			onQuery: func() { zoxideCalled = true },
		}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		_, err := qr.Resolve("./mydir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if zoxideCalled {
			t.Error("zoxide should not be called for path-like arguments starting with .")
		}
	})

	t.Run("path starting with ~ not sent through alias/zoxide chain", func(t *testing.T) {
		aliasLookup := &mockAliasLookup{aliases: map[string]string{"~": "/some/alias/path"}}
		zoxideCalled := false
		zoxide := &trackingZoxideQuerier{
			result:  "/zoxide/path",
			onQuery: func() { zoxideCalled = true },
		}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		_, err := qr.Resolve("~")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if zoxideCalled {
			t.Error("zoxide should not be called for path-like arguments starting with ~")
		}
	})
}

// trackingZoxideQuerier tracks whether Query was called.
type trackingZoxideQuerier struct {
	result  string
	err     error
	onQuery func()
}

func (t *trackingZoxideQuerier) Query(terms string) (string, error) {
	if t.onQuery != nil {
		t.onQuery()
	}
	return t.result, t.err
}

func TestQueryResolver_Resolve_SessionDomain(t *testing.T) {
	t.Run("exact user-visible session-name hit returns SessionResult", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{"api-x7Kd9a", "web-abc123"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", result)
		}
		if sr.Name != "api-x7Kd9a" {
			t.Errorf("SessionResult.Name = %q, want %q", sr.Name, "api-x7Kd9a")
		}
		if sr.Domain != "session" {
			t.Errorf("SessionResult.Domain = %q, want %q", sr.Domain, "session")
		}
	})

	t.Run("session-domain checked before path/alias/zoxide", func(t *testing.T) {
		// The session hit must win even when the same name would also resolve
		// via alias/zoxide — session-domain is first in the precedence chain.
		sessions := &mockSessionLister{names: []string{"myapp"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{"myapp": "/Users/lee/Code/myapp"}}
		zoxide := &mockZoxideQuerier{result: "/Users/lee/Code/myapp"}
		dirValidator := &mockDirValidator{existing: map[string]bool{"/Users/lee/Code/myapp": true}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult (session wins over alias), got %T", result)
		}
		if sr.Name != "myapp" {
			t.Errorf("SessionResult.Name = %q, want %q", sr.Name, "myapp")
		}
	})

	t.Run("underscore-prefixed name never matches (filtered lister)", func(t *testing.T) {
		// The lister returns the leading-underscore-filtered view, so internal
		// _-prefixed sessions are absent — open _portal-saver falls through as
		// if the session did not exist.
		sessions := &mockSessionLister{names: []string{"api-x7Kd9a"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("_portal-saver")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := result.(*resolver.SessionResult); ok {
			t.Fatal("expected fall-through for filtered internal session, got *SessionResult")
		}
		mr, ok := result.(*resolver.MissResult)
		if !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
		if mr.Target != "_portal-saver" {
			t.Errorf("MissResult.Target = %q, want %q", mr.Target, "_portal-saver")
		}
	})

	t.Run("no session match falls through to directory chain", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{"api-x7Kd9a"}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{"myapp": "/Users/lee/Code/myapp"}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{"/Users/lee/Code/myapp": true}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected *PathResult, got %T", result)
		}
		if pr.Path != "/Users/lee/Code/myapp" {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, "/Users/lee/Code/myapp")
		}
	})

	t.Run("empty session set treated as no match", func(t *testing.T) {
		sessions := &mockSessionLister{names: []string{}}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("anything")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := result.(*resolver.SessionResult); ok {
			t.Fatal("expected fall-through for empty session set, got *SessionResult")
		}
	})

	t.Run("lister error collapses to no match, not a resolve error", func(t *testing.T) {
		sessions := &mockSessionLister{err: errors.New("tmux unreachable")}
		aliasLookup := &mockAliasLookup{aliases: map[string]string{}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(sessions, aliasLookup, zoxide, dirValidator)
		result, err := qr.Resolve("anything")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := result.(*resolver.SessionResult); ok {
			t.Fatal("expected fall-through on lister error, got *SessionResult")
		}
		if _, ok := result.(*resolver.MissResult); !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
	})
}

// failingAliasLookup fails the test if Get is ever called. ResolveSessionPin is
// session-domain only, so it must never consult the alias lookup.
type failingAliasLookup struct{ t *testing.T }

func (f *failingAliasLookup) Get(name string) (string, bool) {
	f.t.Helper()
	f.t.Fatalf("ResolveSessionPin must not consult aliases (Get called with %q)", name)
	return "", false
}

func (f *failingAliasLookup) Keys() []string {
	f.t.Helper()
	f.t.Fatalf("session/path pin must not enumerate alias keys (Keys called)")
	return nil
}

// failingZoxideQuerier fails the test if Query is ever called. ResolveSessionPin
// is session-domain only, so it must never consult zoxide.
type failingZoxideQuerier struct{ t *testing.T }

func (f *failingZoxideQuerier) Query(terms string) (string, error) {
	f.t.Helper()
	f.t.Fatalf("ResolveSessionPin must not consult zoxide (Query called with %q)", terms)
	return "", nil
}

// failingSessionLister fails the test if ListSessionNames is ever called.
// ResolvePathPin is path-domain only, so it must never consult the session set.
type failingSessionLister struct{ t *testing.T }

func (f *failingSessionLister) ListSessionNames() ([]string, error) {
	f.t.Helper()
	f.t.Fatalf("ResolvePathPin must not consult the session set (ListSessionNames called)")
	return nil, nil
}

func TestQueryResolver_ResolveSessionPin(t *testing.T) {
	// newPinResolver builds a QueryResolver whose alias and zoxide seams FAIL the
	// test if consulted — ResolveSessionPin is session-domain only, so every pin
	// case doubles as a "never consults the directory chain" guard.
	newPinResolver := func(t *testing.T, names []string, err error) *resolver.QueryResolver {
		return resolver.NewQueryResolver(
			&mockSessionLister{names: names, err: err},
			&failingAliasLookup{t: t},
			&failingZoxideQuerier{t: t},
			&mockDirValidator{existing: map[string]bool{}},
		)
	}

	t.Run("exact user-visible session-name hit returns SessionResult domain session", func(t *testing.T) {
		qr := newPinResolver(t, []string{"api-x7Kd9a", "web-abc123"}, nil)

		result, err := qr.ResolveSessionPin("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", result)
		}
		if sr.Name != "api-x7Kd9a" {
			t.Errorf("SessionResult.Name = %q, want %q", sr.Name, "api-x7Kd9a")
		}
		if sr.Domain != "session" {
			t.Errorf("SessionResult.Domain = %q, want %q", sr.Domain, "session")
		}
	})

	t.Run("multi-match glob does NOT collapse to the first match — loud miss", func(t *testing.T) {
		// Regression guard (report 13-1): the single-pin path must NEVER silently
		// fork a glob-bearing -s value to matches[0]. Glob fan-out is exclusively the
		// burst's job (ResolveSessionPinAll → expandSessionGlobAll, all-match). A glob
		// reaching ResolveSessionPin is not a literal session name, so it falls through
		// to the verbatim attach miss — the same loud hard-fail as a zero-match glob —
		// independent of the isMultiTarget/os.Args gate that normally diverts globs to
		// the burst.
		qr := newPinResolver(t, []string{"api-1", "api-2", "web-abc"}, nil)

		result, err := qr.ResolveSessionPin("api-*")
		if result != nil {
			t.Errorf("expected nil result — a multi-match glob must not collapse to a SessionResult, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail error for a multi-match glob, got nil")
		}
		if want := "No session found: api-*"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("zero-match glob hard-fails with the verbatim attach miss message", func(t *testing.T) {
		qr := newPinResolver(t, []string{"web-abc"}, nil)

		result, err := qr.ResolveSessionPin("api-*")
		if result != nil {
			t.Errorf("expected nil result on a miss, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail error for a zero-match glob, got nil")
		}
		if want := "No session found: api-*"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("exact miss hard-fails with the verbatim attach miss message", func(t *testing.T) {
		qr := newPinResolver(t, []string{"web-abc"}, nil)

		result, err := qr.ResolveSessionPin("api")
		if result != nil {
			t.Errorf("expected nil result on a miss, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail error for an exact miss, got nil")
		}
		if want := "No session found: api"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("internal underscore-prefixed session is a miss (filtered lister)", func(t *testing.T) {
		// The lister returns the leading-underscore-filtered view, so pinning
		// _portal-saver never matches — it is treated as if it did not exist.
		qr := newPinResolver(t, []string{"api-x7Kd9a"}, nil)

		result, err := qr.ResolveSessionPin("_portal-saver")
		if result != nil {
			t.Errorf("expected nil result for a filtered internal session, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a filtered internal session, got nil")
		}
		if want := "No session found: _portal-saver"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("empty session set is a miss", func(t *testing.T) {
		qr := newPinResolver(t, []string{}, nil)

		result, err := qr.ResolveSessionPin("anything")
		if result != nil {
			t.Errorf("expected nil result for an empty session set, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for an empty session set, got nil")
		}
		if want := "No session found: anything"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("lister error collapses to a miss, not a resolve error", func(t *testing.T) {
		qr := newPinResolver(t, nil, errors.New("tmux unreachable"))

		result, err := qr.ResolveSessionPin("anything")
		if result != nil {
			t.Errorf("expected nil result on a lister error, got %T", result)
		}
		if err == nil {
			t.Fatal("expected the miss hard-fail on a lister error, got nil")
		}
		if want := "No session found: anything"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("never consults aliases or zoxide on a miss", func(t *testing.T) {
		// A bare Resolve would fall through a session miss to alias/zoxide. The pin
		// must not: with a name that also exists as an alias, the failing seams
		// would fatal the test if consulted, and the pin hard-fails instead.
		qr := newPinResolver(t, []string{"web-abc"}, nil)

		_, err := qr.ResolveSessionPin("myapp")
		if err == nil {
			t.Fatal("expected hard-fail (session-domain only), got nil")
		}
		if want := "No session found: myapp"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})
}

// TestQueryResolver_ExactSessionMatch_UnifiedAcrossEntryPoints pins the shared
// exact-session-match rule and its single lister-error policy across the three
// entry points that consume it — Resolve, ResolveSessionPin, and
// ResolveSessionPinAll. The match rule (ListSessionNames + membership) and the
// error policy (a lister error collapses to "no match", never escalates) now live
// in one helper; these characterization tests lock in that all three sites route
// through it and stay in sync.
func TestQueryResolver_ExactSessionMatch_UnifiedAcrossEntryPoints(t *testing.T) {
	newResolver := func(names []string, err error) *resolver.QueryResolver {
		return resolver.NewQueryResolver(
			&mockSessionLister{names: names, err: err},
			&mockAliasLookup{aliases: map[string]string{}},
			&mockZoxideQuerier{err: resolver.ErrNoMatch},
			&mockDirValidator{existing: map[string]bool{}},
		)
	}

	t.Run("exact hit resolves via Resolve", func(t *testing.T) {
		qr := newResolver([]string{"api-x7Kd9a", "web-abc"}, nil)

		result, err := qr.Resolve("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", result)
		}
		if sr.Name != "api-x7Kd9a" || sr.Domain != resolver.DomainSession {
			t.Errorf("result = {%q, %q}, want {api-x7Kd9a, session}", sr.Name, sr.Domain)
		}
	})

	t.Run("exact hit resolves via ResolveSessionPin", func(t *testing.T) {
		qr := newResolver([]string{"api-x7Kd9a", "web-abc"}, nil)

		result, err := qr.ResolveSessionPin("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", result)
		}
		if sr.Name != "api-x7Kd9a" || sr.Domain != resolver.DomainSession {
			t.Errorf("result = {%q, %q}, want {api-x7Kd9a, session}", sr.Name, sr.Domain)
		}
	})

	t.Run("exact hit resolves via ResolveSessionPinAll", func(t *testing.T) {
		qr := newResolver([]string{"api-x7Kd9a", "web-abc"}, nil)

		results, err := qr.ResolveSessionPinAll("api-x7Kd9a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		sr, ok := results[0].(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", results[0])
		}
		if sr.Name != "api-x7Kd9a" || sr.Domain != resolver.DomainSession {
			t.Errorf("result = {%q, %q}, want {api-x7Kd9a, session}", sr.Name, sr.Domain)
		}
	})

	// The unified lister-error policy: a ListSessionNames error is treated as "no
	// match" at every entry point — never escalated to a resolve error. Each site
	// keeps its own downstream behaviour on the resulting non-match (Resolve falls
	// through to a miss, ResolveSessionPin hard-fails, ResolveSessionPinAll collects
	// a miss), but none surfaces the lister error itself.
	t.Run("lister error is no match with no escalation via Resolve", func(t *testing.T) {
		qr := newResolver(nil, errors.New("tmux unreachable"))

		result, err := qr.Resolve("anything")
		if err != nil {
			t.Fatalf("expected no error escalation, got: %v", err)
		}
		if _, ok := result.(*resolver.SessionResult); ok {
			t.Fatal("expected fall-through on a lister error, got *SessionResult")
		}
		if _, ok := result.(*resolver.MissResult); !ok {
			t.Fatalf("expected *MissResult, got %T", result)
		}
	})

	t.Run("lister error is no match with no escalation via ResolveSessionPin", func(t *testing.T) {
		qr := newResolver(nil, errors.New("tmux unreachable"))

		result, err := qr.ResolveSessionPin("anything")
		if result != nil {
			t.Errorf("expected nil result on a lister error, got %T", result)
		}
		if err == nil {
			t.Fatal("expected the miss hard-fail on a lister error, got nil")
		}
		if want := "No session found: anything"; err.Error() != want {
			t.Errorf("error = %q, want %q — the lister error must collapse to the normal miss, not escalate", err.Error(), want)
		}
	})

	t.Run("lister error is no match with no escalation via ResolveSessionPinAll", func(t *testing.T) {
		qr := newResolver(nil, errors.New("tmux unreachable"))

		results, err := qr.ResolveSessionPinAll("anything")
		if err != nil {
			t.Fatalf("expected a collected miss, not an error escalation, got: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("len(results) = %d, want 1", len(results))
		}
		miss, ok := results[0].(*resolver.MissResult)
		if !ok {
			t.Fatalf("expected *MissResult, got %T", results[0])
		}
		if miss.Target != "anything" {
			t.Errorf("miss.Target = %q, want %q", miss.Target, "anything")
		}
	})
}

func TestQueryResolver_ResolvePathPin(t *testing.T) {
	// newPathPinResolver builds a QueryResolver whose session, alias, and zoxide
	// seams FAIL the test if consulted — ResolvePathPin is path-domain only (it
	// reuses ResolvePath and touches none of the resolution seams), so every case
	// doubles as a "never consults session/alias/zoxide" guard.
	newPathPinResolver := func(t *testing.T) *resolver.QueryResolver {
		return resolver.NewQueryResolver(
			&failingSessionLister{t: t},
			&failingAliasLookup{t: t},
			&failingZoxideQuerier{t: t},
			&mockDirValidator{existing: map[string]bool{}},
		)
	}

	t.Run("existing directory returns PathResult domain path", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)

		qr := newPathPinResolver(t)
		result, err := qr.ResolvePathPin(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected *PathResult, got %T", result)
		}
		if pr.Path != dir {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, dir)
		}
		if pr.Domain != "path" {
			t.Errorf("PathResult.Domain = %q, want %q", pr.Domain, "path")
		}
	})

	t.Run("directory whose name contains glob metacharacters is reachable", func(t *testing.T) {
		// The reason -p exists: a glob-named dir (foo[1]) is UNREACHABLE as a bare
		// positional (the multi-target routing gate treats it as a session glob and
		// routes it to the burst, where it matches zero sessions and hard-fails). -p
		// reuses ResolvePath, which STATS the literal path — [1] is never expanded —
		// so the dir resolves and mints.
		tmp := t.TempDir()
		tmp, _ = filepath.EvalSymlinks(tmp)
		globDir := filepath.Join(tmp, "foo[1]")
		if err := os.Mkdir(globDir, 0o755); err != nil {
			t.Fatalf("failed to create glob-named dir: %v", err)
		}

		qr := newPathPinResolver(t)
		result, err := qr.ResolvePathPin(globDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected *PathResult for a glob-named dir, got %T", result)
		}
		if pr.Path != globDir {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, globDir)
		}
		if pr.Domain != "path" {
			t.Errorf("PathResult.Domain = %q, want %q", pr.Domain, "path")
		}
	})

	t.Run("non-existent directory hard-fails with Directory not found", func(t *testing.T) {
		qr := newPathPinResolver(t)
		result, err := qr.ResolvePathPin("/nonexistent/path/that/does/not/exist")
		if result != nil {
			t.Errorf("expected nil result on a miss, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a non-existent dir, got nil")
		}
		if want := "Directory not found: /nonexistent/path/that/does/not/exist"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("a file (not a directory) hard-fails with not a directory", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)
		filePath := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		qr := newPathPinResolver(t)
		result, err := qr.ResolvePathPin(filePath)
		if result != nil {
			t.Errorf("expected nil result for a file, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a non-directory file, got nil")
		}
		if want := "not a directory: " + filePath; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("tilde is expanded to the home directory", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		qr := newPathPinResolver(t)
		result, err := qr.ResolvePathPin("~")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected *PathResult, got %T", result)
		}
		if pr.Path != home {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, home)
		}
	})
}

func TestQueryResolver_ResolveAliasPin(t *testing.T) {
	// newAliasPinResolver builds a QueryResolver whose session and zoxide seams
	// FAIL the test if consulted — ResolveAliasPin is alias-domain only (it looks
	// the key up directly in the alias store, bypassing the session→path→alias→
	// zoxide precedence), so every case doubles as a "never consults session or
	// zoxide" guard. The failing session lister also proves a same-named shadowing
	// session is never checked.
	newAliasPinResolver := func(t *testing.T, aliases map[string]string, existing map[string]bool) *resolver.QueryResolver {
		return resolver.NewQueryResolver(
			&failingSessionLister{t: t},
			&mockAliasLookup{aliases: aliases},
			&failingZoxideQuerier{t: t},
			&mockDirValidator{existing: existing},
		)
	}

	t.Run("known key returns PathResult domain alias with the validated dir", func(t *testing.T) {
		// The failing session lister proves a same-named shadowing session is never
		// consulted: -a bypasses the session→path→alias precedence entirely.
		qr := newAliasPinResolver(t,
			map[string]string{"myapp": "/Users/lee/Code/myapp"},
			map[string]bool{"/Users/lee/Code/myapp": true},
		)

		result, err := qr.ResolveAliasPin("myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected *PathResult, got %T", result)
		}
		if pr.Path != "/Users/lee/Code/myapp" {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, "/Users/lee/Code/myapp")
		}
		if pr.Domain != "alias" {
			t.Errorf("PathResult.Domain = %q, want %q", pr.Domain, "alias")
		}
	})

	t.Run("single-match key glob no longer expands in the single-pin — loud miss", func(t *testing.T) {
		// Regression guard (report 13-1): the single-pin path no longer glob-expands at
		// ANY arity — glob fan-out is exclusively the burst's job (ResolveAliasPinAll).
		// A glob reaching ResolveAliasPin is not a literal alias key, so even a
		// single-match glob falls through to the "No alias found" hard-fail, independent
		// of the isMultiTarget/os.Args gate that normally diverts globs to the burst.
		qr := newAliasPinResolver(t,
			map[string]string{"workflow-a": "/code/wa", "blog": "/code/blog"},
			map[string]bool{"/code/wa": true},
		)

		result, err := qr.ResolveAliasPin("workflow-*")
		if result != nil {
			t.Errorf("expected nil result — a glob must not collapse to a PathResult, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail error for a single-match glob, got nil")
		}
		if want := "No alias found: workflow-*"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("multi-match key glob does NOT mint the first match — loud miss", func(t *testing.T) {
		// Regression guard (report 13-1): the single-pin path must NEVER silently fork a
		// glob-bearing -a value to the first matched key. A glob reaching ResolveAliasPin
		// is not a literal alias key, so it falls through to the "No alias found"
		// hard-fail — the same loud fail as a zero-match glob — independent of the
		// isMultiTarget/os.Args gate that normally diverts globs to the burst.
		qr := newAliasPinResolver(t,
			map[string]string{"workflow-b": "/code/wb", "workflow-a": "/code/wa"},
			map[string]bool{"/code/wa": true, "/code/wb": true},
		)

		result, err := qr.ResolveAliasPin("workflow-*")
		if result != nil {
			t.Errorf("expected nil result — a multi-match glob must not collapse to a PathResult, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail error for a multi-match glob, got nil")
		}
		if want := "No alias found: workflow-*"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("unknown key hard-fails with No alias found", func(t *testing.T) {
		qr := newAliasPinResolver(t,
			map[string]string{"api": "/code/api"},
			map[string]bool{"/code/api": true},
		)

		result, err := qr.ResolveAliasPin("nope")
		if result != nil {
			t.Errorf("expected nil result for an unknown key, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for an unknown key, got nil")
		}
		if want := "No alias found: nope"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("glob matching zero keys hard-fails with No alias found", func(t *testing.T) {
		qr := newAliasPinResolver(t,
			map[string]string{"api": "/code/api"},
			map[string]bool{"/code/api": true},
		)

		result, err := qr.ResolveAliasPin("web-*")
		if result != nil {
			t.Errorf("expected nil result for a zero-match glob, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a zero-match glob, got nil")
		}
		if want := "No alias found: web-*"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("resolved key whose dir is gone hard-fails with Directory not found", func(t *testing.T) {
		// Distinct from the unknown-key miss: the key resolves, but its directory
		// no longer exists — the shared disk validation (validatedPath) rejects it.
		qr := newAliasPinResolver(t,
			map[string]string{"stale": "/gone/dir"},
			map[string]bool{},
		)

		result, err := qr.ResolveAliasPin("stale")
		if result != nil {
			t.Errorf("expected nil result for a gone dir, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a gone dir, got nil")
		}
		if want := "Directory not found: /gone/dir"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		var dirErr *resolver.DirNotFoundError
		if !errors.As(err, &dirErr) {
			t.Errorf("expected *DirNotFoundError, got %T", err)
		}
	})
}

func TestQueryResolver_ResolveZoxidePin(t *testing.T) {
	// newZoxidePinResolver builds a QueryResolver whose session and alias seams
	// FAIL the test if consulted — ResolveZoxidePin is zoxide-domain only (it
	// queries zoxide and touches neither the session set nor the alias store), so
	// every case doubles as a "never consults session or alias" guard.
	newZoxidePinResolver := func(t *testing.T, zoxide resolver.ZoxideQuerier, existing map[string]bool) *resolver.QueryResolver {
		return resolver.NewQueryResolver(
			&failingSessionLister{t: t},
			&failingAliasLookup{t: t},
			zoxide,
			&mockDirValidator{existing: existing},
		)
	}

	t.Run("best-match dir returns PathResult domain zoxide", func(t *testing.T) {
		dir := t.TempDir()
		dir, _ = filepath.EvalSymlinks(dir)
		qr := newZoxidePinResolver(t, &mockZoxideQuerier{result: dir}, map[string]bool{dir: true})

		result, err := qr.ResolveZoxidePin("proj")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pr, ok := result.(*resolver.PathResult)
		if !ok {
			t.Fatalf("expected *PathResult, got %T", result)
		}
		if pr.Path != dir {
			t.Errorf("PathResult.Path = %q, want %q", pr.Path, dir)
		}
		if pr.Domain != "zoxide" {
			t.Errorf("PathResult.Domain = %q, want %q", pr.Domain, "zoxide")
		}
	})

	t.Run("zoxide not installed surfaces ErrZoxideNotInstalled explicitly", func(t *testing.T) {
		// The whole point of the pin: zoxide-absence is surfaced verbatim (a script
		// sees WHY), distinct from the bare chain's silent fall-through.
		qr := newZoxidePinResolver(t, &mockZoxideQuerier{err: resolver.ErrZoxideNotInstalled}, map[string]bool{})

		result, err := qr.ResolveZoxidePin("proj")
		if result != nil {
			t.Errorf("expected nil result when zoxide is not installed, got %T", result)
		}
		if !errors.Is(err, resolver.ErrZoxideNotInstalled) {
			t.Fatalf("expected ErrZoxideNotInstalled, got %v", err)
		}
	})

	t.Run("no match hard-fails with No zoxide match for", func(t *testing.T) {
		qr := newZoxidePinResolver(t, &mockZoxideQuerier{err: resolver.ErrNoMatch}, map[string]bool{})

		result, err := qr.ResolveZoxidePin("nope")
		if result != nil {
			t.Errorf("expected nil result on a no-match, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a no-match, got nil")
		}
		if want := "No zoxide match for: nope"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		// A no-match is a distinct outcome from not-installed: it must NOT surface as
		// ErrZoxideNotInstalled (the caller distinguishes them via errors.Is).
		if errors.Is(err, resolver.ErrZoxideNotInstalled) {
			t.Error("a no-match must not surface as ErrZoxideNotInstalled")
		}
	})

	t.Run("best-match dir gone hard-fails with Directory not found", func(t *testing.T) {
		// Distinct from a no-match: zoxide resolves, but its best-match directory no
		// longer exists — the shared disk validation (validatedPath) rejects it.
		qr := newZoxidePinResolver(t, &mockZoxideQuerier{result: "/gone/dir"}, map[string]bool{})

		result, err := qr.ResolveZoxidePin("proj")
		if result != nil {
			t.Errorf("expected nil result for a gone dir, got %T", result)
		}
		if err == nil {
			t.Fatal("expected hard-fail for a gone dir, got nil")
		}
		if want := "Directory not found: /gone/dir"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		var dirErr *resolver.DirNotFoundError
		if !errors.As(err, &dirErr) {
			t.Errorf("expected *DirNotFoundError, got %T", err)
		}
	})
}

func TestQueryResolver_Resolve_NonExistentResolvedDirectory(t *testing.T) {
	t.Run("non-existent resolved directory prints error and exits 1", func(t *testing.T) {
		aliasLookup := &mockAliasLookup{aliases: map[string]string{"myapp": "/does/not/exist"}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(&mockSessionLister{}, aliasLookup, zoxide, dirValidator)
		_, err := qr.Resolve("myapp")

		if err == nil {
			t.Fatal("expected error for non-existent directory, got nil")
		}

		want := "Directory not found: /does/not/exist"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		var dirErr *resolver.DirNotFoundError
		if !errors.As(err, &dirErr) {
			t.Errorf("expected DirNotFoundError, got %T", err)
		}
	})
}
