package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAliasSetCommand(t *testing.T) {
	t.Run("sets new alias with absolute path", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "myproject", "/Users/lee/Code/project"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(aliasFile)
		if err != nil {
			t.Fatalf("failed to read aliases file: %v", err)
		}

		got := string(data)
		want := "myproject=/Users/lee/Code/project\n"
		if got != want {
			t.Errorf("aliases file content = %q, want %q", got, want)
		}
	})

	t.Run("expands tilde in path", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "m2api", "~/Code/mac2/api"})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(aliasFile)
		if err != nil {
			t.Fatalf("failed to read aliases file: %v", err)
		}

		got := string(data)
		want := "m2api=" + filepath.Join(home, "Code/mac2/api") + "\n"
		if got != want {
			t.Errorf("aliases file content = %q, want %q", got, want)
		}
	})

	t.Run("resolves relative path to absolute", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "proj", "relative/path"})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(aliasFile)
		if err != nil {
			t.Fatalf("failed to read aliases file: %v", err)
		}

		got := string(data)
		want := "proj=" + filepath.Join(cwd, "relative/path") + "\n"
		if got != want {
			t.Errorf("aliases file content = %q, want %q", got, want)
		}
	})

	t.Run("overwrites existing alias silently", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		// Set initial alias
		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "proj", "/first/path"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}

		// Overwrite with new path
		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"alias", "set", "proj", "/second/path"})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error on overwrite: %v", err)
		}

		data, err := os.ReadFile(aliasFile)
		if err != nil {
			t.Fatalf("failed to read aliases file: %v", err)
		}

		got := string(data)
		want := "proj=/second/path\n"
		if got != want {
			t.Errorf("aliases file content = %q, want %q", got, want)
		}
	})

	t.Run("aliases file contains absolute path after set", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "work", "~/Code/work"})
		err = rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(aliasFile)
		if err != nil {
			t.Fatalf("failed to read aliases file: %v", err)
		}

		got := string(data)
		want := "work=" + filepath.Join(home, "Code/work") + "\n"
		if got != want {
			t.Errorf("aliases file content = %q, want %q", got, want)
		}
		if !filepath.IsAbs(filepath.Join(home, "Code/work")) {
			t.Errorf("resolved path is not absolute")
		}
	})

	t.Run("exits 0 on success", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "test", "/some/path"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected exit 0 (no error), got: %v", err)
		}
	})

	t.Run("requires exactly two arguments", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "set", "onlyname"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for missing path argument, got nil")
		}
	})
}

func TestAliasRmCommand(t *testing.T) {
	t.Run("removes existing alias", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		// Seed aliases file
		if err := os.WriteFile(aliasFile, []byte("proj=/Users/lee/Code/project\nwork=/Users/lee/Code/work\n"), 0o644); err != nil {
			t.Fatalf("failed to write seed file: %v", err)
		}

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "rm", "proj"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(aliasFile)
		if err != nil {
			t.Fatalf("failed to read aliases file: %v", err)
		}

		got := string(data)
		want := "work=/Users/lee/Code/work\n"
		if got != want {
			t.Errorf("aliases file content = %q, want %q", got, want)
		}
	})

	t.Run("returns error for non-existent alias", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		// Empty aliases file
		if err := os.WriteFile(aliasFile, []byte(""), 0o644); err != nil {
			t.Fatalf("failed to write seed file: %v", err)
		}

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "rm", "nonexistent"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for non-existent alias, got nil")
		}

		want := "alias not found: nonexistent"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("exits 0 on success", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		if err := os.WriteFile(aliasFile, []byte("proj=/some/path\n"), 0o644); err != nil {
			t.Fatalf("failed to write seed file: %v", err)
		}

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "rm", "proj"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected exit 0 (no error), got: %v", err)
		}
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		resetRootCmd()
		rootCmd.SetArgs([]string{"alias", "rm"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for missing argument, got nil")
		}
	})
}

func TestAliasListCommand(t *testing.T) {
	t.Run("outputs aliases sorted by name", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		content := "zebra=/z/path\napple=/a/path\nmango=/m/path\n"
		if err := os.WriteFile(aliasFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write seed file: %v", err)
		}

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"alias", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "apple=/a/path\nmango=/m/path\nzebra=/z/path\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("produces empty output when no aliases", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		// No aliases file exists

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"alias", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("exits 0 on success", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"alias", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected exit 0 (no error), got: %v", err)
		}
	})

	t.Run("accepts no arguments", func(t *testing.T) {
		dir := t.TempDir()
		aliasFile := filepath.Join(dir, "aliases")
		t.Setenv("PORTAL_ALIASES_FILE", aliasFile)

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"alias", "list", "extraarg"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})
}
