package resolver_test

import (
	"errors"
	"os"
	"path/filepath"
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
		wantDomain   string
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

// failingZoxideQuerier fails the test if Query is ever called. ResolveSessionPin
// is session-domain only, so it must never consult zoxide.
type failingZoxideQuerier struct{ t *testing.T }

func (f *failingZoxideQuerier) Query(terms string) (string, error) {
	f.t.Helper()
	f.t.Fatalf("ResolveSessionPin must not consult zoxide (Query called with %q)", terms)
	return "", nil
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

	t.Run("glob expansion attaches the first match with domain glob", func(t *testing.T) {
		// Matching preserves lister order; the single-target arity attaches the
		// first match (per-match window fan-out is the Phase 3 burst).
		qr := newPinResolver(t, []string{"api-1", "api-2", "web-abc"}, nil)

		result, err := qr.ResolveSessionPin("api-*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sr, ok := result.(*resolver.SessionResult)
		if !ok {
			t.Fatalf("expected *SessionResult, got %T", result)
		}
		if sr.Name != "api-1" {
			t.Errorf("SessionResult.Name = %q, want %q (first match)", sr.Name, "api-1")
		}
		if sr.Domain != "glob" {
			t.Errorf("SessionResult.Domain = %q, want %q", sr.Domain, "glob")
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
