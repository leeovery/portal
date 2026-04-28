package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
)

func TestLookupOnResume(t *testing.T) {
	t.Run("returns no-hook when hooks.json is missing", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		cmd, ok, err := hooks.LookupOnResume(store, "session:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
		}
	})

	t.Run("returns no-hook when hooks.json is malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		if err := os.WriteFile(filePath, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "session:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
		}
	})

	t.Run("returns no-hook when the hook-key is absent", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		content := `{"other-session:0.0":{"on-resume":"echo hi"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "missing-session:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
		}
	})

	t.Run("returns no-hook when the key has no on-resume event", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		content := `{"session:0.0":{"on-attach":"echo attached"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "session:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
		}
	})

	t.Run("returns no-hook when the on-resume command is empty string", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		content := `{"session:0.0":{"on-resume":""}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "session:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
		}
	})

	t.Run("returns the command verbatim when on-resume is registered", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		const want = "echo hello world; ls -la"
		content := `{"session:0.0":{"on-resume":"echo hello world; ls -la"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "session:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Errorf("got ok=false, want true")
		}
		if cmd != want {
			t.Errorf("got cmd=%q, want %q", cmd, want)
		}
	})

	t.Run("round-trips hook keys containing colons in the session name", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		const want = "ls"
		content := `{"work:foo:0.0":{"on-resume":"ls"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "work:foo:0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Errorf("got ok=false, want true")
		}
		if cmd != want {
			t.Errorf("got cmd=%q, want %q", cmd, want)
		}
	})

	t.Run("surfaces a wrapped I/O error distinct from the no-hook case", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		// Create a directory at the path where Store expects a file.
		// os.ReadFile on a directory returns EISDIR (not ErrNotExist),
		// so Store.Load propagates it and LookupOnResume wraps it.
		if err := os.Mkdir(filePath, 0o700); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		store := hooks.NewStore(filePath)
		cmd, ok, err := hooks.LookupOnResume(store, "session:0.0")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if ok {
			t.Errorf("got ok=true, want false")
		}
		if cmd != "" {
			t.Errorf("got cmd=%q, want empty", cmd)
		}
		if !strings.Contains(err.Error(), "load hooks") {
			t.Errorf("error %q does not contain %q", err.Error(), "load hooks")
		}
	})
}
