package resolver_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestIsPathArgument(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{
			name: "true for paths with /",
			arg:  "foo/bar",
			want: true,
		},
		{
			name: "true for absolute path with leading /",
			arg:  "/usr/local",
			want: true,
		},
		{
			name: "true for paths starting with .",
			arg:  ".",
			want: true,
		},
		{
			name: "true for relative path starting with ./",
			arg:  "./subdir",
			want: true,
		},
		{
			name: "true for parent directory ..",
			arg:  "..",
			want: true,
		},
		{
			name: "true for paths starting with ~",
			arg:  "~/Code",
			want: true,
		},
		{
			name: "true for tilde alone",
			arg:  "~",
			want: true,
		},
		{
			name: "false for plain words",
			arg:  "myproject",
			want: false,
		},
		{
			name: "false for empty string",
			arg:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.IsPathArgument(tt.arg)
			if got != tt.want {
				t.Errorf("IsPathArgument(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	t.Run("resolves relative path to absolute", func(t *testing.T) {
		dir := t.TempDir()
		// Resolve symlinks so macOS /var -> /private/var matches filepath.Abs output
		dir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("failed to eval symlinks: %v", err)
		}
		subDir := filepath.Join(dir, "sub")
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		// Change to dir so "sub" is a relative path
		t.Chdir(dir)

		got, err := resolver.ResolvePath("sub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !filepath.IsAbs(got) {
			t.Errorf("expected absolute path, got %q", got)
		}
		if got != subDir {
			t.Errorf("ResolvePath(\"sub\") = %q, want %q", got, subDir)
		}
	})

	t.Run("expands tilde to home directory", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		got, err := resolver.ResolvePath("~")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != home {
			t.Errorf("ResolvePath(\"~\") = %q, want %q", got, home)
		}
	})

	t.Run("expands tilde with subpath", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		// Use a directory that should exist under home
		got, err := resolver.ResolvePath("~/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != home {
			t.Errorf("ResolvePath(\"~/\") = %q, want %q", got, home)
		}
	})

	t.Run("returns error for non-existent path", func(t *testing.T) {
		_, err := resolver.ResolvePath("/nonexistent/path/that/does/not/exist")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		want := "Directory not found: /nonexistent/path/that/does/not/exist"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("returns error when path is a file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		_, err := resolver.ResolvePath(filePath)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		want := "not a directory: " + filePath
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("resolves dot to current working directory", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}

		got, err := resolver.ResolvePath(".")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != cwd {
			t.Errorf("ResolvePath(\".\") = %q, want %q", got, cwd)
		}
	})

	t.Run("resolves absolute path as-is", func(t *testing.T) {
		dir := t.TempDir()

		got, err := resolver.ResolvePath(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != dir {
			t.Errorf("ResolvePath(%q) = %q, want %q", dir, got, dir)
		}
	})
}

func TestNormalisePath(t *testing.T) {
	t.Run("returns absolute path unchanged", func(t *testing.T) {
		got := resolver.NormalisePath("/Users/lee/Code/mac2/api")
		if got != "/Users/lee/Code/mac2/api" {
			t.Errorf("NormalisePath(%q) = %q, want %q", "/Users/lee/Code/mac2/api", got, "/Users/lee/Code/mac2/api")
		}
	})

	t.Run("expands tilde to home directory", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		got := resolver.NormalisePath("~/Code/mac2/api")
		want := filepath.Join(home, "Code/mac2/api")
		if got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", "~/Code/mac2/api", got, want)
		}
	})

	t.Run("expands bare tilde to home directory", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		got := resolver.NormalisePath("~")
		if got != home {
			t.Errorf("NormalisePath(%q) = %q, want %q", "~", got, home)
		}
	})

	t.Run("resolves relative path to absolute", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}

		got := resolver.NormalisePath("subdir/project")
		want := filepath.Join(cwd, "subdir/project")
		if got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", "subdir/project", got, want)
		}
	})

	t.Run("resolves dot-relative path to absolute", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}

		got := resolver.NormalisePath("./myproject")
		want := filepath.Join(cwd, "myproject")
		if got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", "./myproject", got, want)
		}
	})

	t.Run("always returns absolute path", func(t *testing.T) {
		got := resolver.NormalisePath("relative")
		if !filepath.IsAbs(got) {
			t.Errorf("NormalisePath(%q) = %q, expected absolute path", "relative", got)
		}
	})
}
