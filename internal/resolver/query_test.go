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

func TestQueryResolver_Resolve(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		aliases     map[string]string
		zoxideResult string
		zoxideErr   error
		existingDirs map[string]bool
		wantPath    string
		wantFallback bool
		wantQuery   string
		wantErr     string
	}{
		{
			name:         "non-path argument resolved via alias",
			query:        "myapp",
			aliases:      map[string]string{"myapp": "/Users/lee/Code/myapp"},
			zoxideResult: "/some/other/path",
			zoxideErr:    nil,
			existingDirs: map[string]bool{"/Users/lee/Code/myapp": true},
			wantPath:     "/Users/lee/Code/myapp",
			wantFallback: false,
		},
		{
			name:         "alias miss falls through to zoxide",
			query:        "proj",
			aliases:      map[string]string{},
			zoxideResult: "/Users/lee/Code/proj",
			zoxideErr:    nil,
			existingDirs: map[string]bool{"/Users/lee/Code/proj": true},
			wantPath:     "/Users/lee/Code/proj",
			wantFallback: false,
		},
		{
			name:         "zoxide miss falls through to TUI fallback",
			query:        "unknown",
			aliases:      map[string]string{},
			zoxideResult: "",
			zoxideErr:    resolver.ErrNoMatch,
			existingDirs: map[string]bool{},
			wantFallback: true,
			wantQuery:    "unknown",
		},
		{
			name:         "TUI fallback includes query string for filter pre-fill",
			query:        "searchterm",
			aliases:      map[string]string{},
			zoxideResult: "",
			zoxideErr:    resolver.ErrNoMatch,
			existingDirs: map[string]bool{},
			wantFallback: true,
			wantQuery:    "searchterm",
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
			wantFallback: true,
			wantQuery:    "myquery",
		},
		{
			name:         "query matches alias; alias path used not zoxide",
			query:        "myapp",
			aliases:      map[string]string{"myapp": "/alias/path"},
			zoxideResult: "/zoxide/path",
			zoxideErr:    nil,
			existingDirs: map[string]bool{"/alias/path": true, "/zoxide/path": true},
			wantPath:     "/alias/path",
			wantFallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliasLookup := &mockAliasLookup{aliases: tt.aliases}
			zoxide := &mockZoxideQuerier{result: tt.zoxideResult, err: tt.zoxideErr}
			dirValidator := &mockDirValidator{existing: tt.existingDirs}

			qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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

			if tt.wantFallback {
				fb, ok := result.(*resolver.FallbackResult)
				if !ok {
					t.Fatalf("expected FallbackResult, got %T", result)
				}
				if fb.Query != tt.wantQuery {
					t.Errorf("FallbackResult.Query = %q, want %q", fb.Query, tt.wantQuery)
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

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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
		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get working directory: %v", err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chdir(origDir) })

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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

		origDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get working directory: %v", err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chdir(origDir) })

		aliasLookup := &mockAliasLookup{aliases: map[string]string{"./mydir": "/some/alias/path"}}
		zoxideCalled := false
		zoxide := &trackingZoxideQuerier{
			result:  "/zoxide/path",
			onQuery: func() { zoxideCalled = true },
		}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
		_, err = qr.Resolve("./mydir")
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

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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

func TestQueryResolver_Resolve_NonExistentResolvedDirectory(t *testing.T) {
	t.Run("non-existent resolved directory prints error and exits 1", func(t *testing.T) {
		aliasLookup := &mockAliasLookup{aliases: map[string]string{"myapp": "/does/not/exist"}}
		zoxide := &mockZoxideQuerier{err: resolver.ErrNoMatch}
		dirValidator := &mockDirValidator{existing: map[string]bool{}}

		qr := resolver.NewQueryResolver(aliasLookup, zoxide, dirValidator)
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
