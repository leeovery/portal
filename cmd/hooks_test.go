// Tests in this file mutate package-level state (hooksDeps) and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHooksListCommand(t *testing.T) {
	t.Run("outputs hooks in tab-separated format", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		data := map[string]map[string]string{
			"my-project-abc123:0.0": {"on-resume": "claude --resume abc123"},
		}
		writeHooksJSON(t, hooksFile, data)

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "my-project-abc123:0.0\ton-resume\tclaude --resume abc123\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("produces empty output when no hooks registered", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		writeHooksJSON(t, hooksFile, map[string]map[string]string{})

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("produces empty output when hooks file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Do not create the file

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("outputs hooks sorted by key then event", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		data := map[string]map[string]string{
			"proj-abc:1.0":   {"on-resume": "claude --resume def456"},
			"proj-abc:0.0":   {"on-resume": "claude --resume abc123"},
			"other-proj:0.0": {"on-resume": "npm start"},
		}
		writeHooksJSON(t, hooksFile, data)

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "other-proj:0.0\ton-resume\tnpm start\nproj-abc:0.0\ton-resume\tclaude --resume abc123\nproj-abc:1.0\ton-resume\tclaude --resume def456\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("hooks invokes tmux bootstrap (Phase 4 spec)", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Phase 4 moved hook firing into the hydrate helper, so
		// `portal hooks ...` now goes through the full bootstrap path
		// to keep CleanStale and skeleton restoration in scope. Stub
		// bootstrapDeps so the test does not depend on a real tmux.
		runner := &recordingRunner{}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.calls != 1 {
			t.Errorf("orchestrator Run call count = %d, want 1 for portal hooks list (Phase 4)", runner.calls)
		}
	})

	t.Run("accepts no arguments", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "list", "extraarg"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})
}

// mockKeyResolver implements StructuralKeyResolver for testing.
type mockKeyResolver struct {
	key string
	err error
}

func (m *mockKeyResolver) ResolveStructuralKey(_ string) (string, error) {
	return m.key, m.err
}

func TestHooksSetCommand(t *testing.T) {
	t.Run("sets hook for current pane", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify hook was written under structural key
		data := readHooksJSON(t, hooksFile)
		if data["my-session:0.0"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("hook command = %q, want %q", data["my-session:0.0"]["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("reads pane ID from TMUX_PANE environment variable", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%99")

		resolver := &mockKeyResolver{key: "proj-xyz:1.2"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify structural key was used in the store (not raw pane ID)
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["proj-xyz:1.2"]; !ok {
			t.Error("expected hook entry for structural key proj-xyz:1.2, not found")
		}
		if _, ok := data["%99"]; ok {
			t.Error("raw pane ID %99 should not be used as key")
		}
	})

	t.Run("returns error when TMUX_PANE is not set", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "")

		resolver := &mockKeyResolver{key: "unused:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "must be run from inside a tmux pane") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "must be run from inside a tmux pane")
		}

		// Verify no side effects: no file written
		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks file should not have been created")
		}
	})

	t.Run("returns error when on-resume flag is not provided", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for missing --on-resume flag, got nil")
		}
		if !strings.Contains(err.Error(), "on-resume") {
			t.Errorf("error = %q, want it to mention %q", err.Error(), "on-resume")
		}
	})

	t.Run("overwrites existing hook for same pane idempotently", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		// First set
		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "old-cmd"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("first set: unexpected error: %v", err)
		}

		// Second set overwrites
		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "new-cmd"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("second set: unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if data["my-session:0.0"]["on-resume"] != "new-cmd" {
			t.Errorf("hook command = %q, want %q", data["my-session:0.0"]["on-resume"], "new-cmd")
		}
	})

	t.Run("writes correct JSON structure to hooks file", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Read the raw JSON and verify the structure
		data := readHooksJSON(t, hooksFile)

		// Should have exactly one entry under structural key
		if len(data) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(data))
		}

		// The entry should be keyed by structural key, not raw pane ID
		events, ok := data["my-session:0.0"]
		if !ok {
			t.Fatal("expected entry for structural key my-session:0.0")
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event for my-session:0.0, got %d", len(events))
		}
		if events["on-resume"] != "claude --resume abc123" {
			t.Errorf("on-resume = %q, want %q", events["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("ResolveStructuralKey failure returns user-facing error", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{err: fmt.Errorf("tmux not responding")}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resolve") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "resolve")
		}

		// Verify no side effects: no file written
		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks file should not have been created")
		}
	})
}

func TestHooksRmCommand(t *testing.T) {
	t.Run("removes hook for current pane", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		// Seed with an existing hook keyed by structural key
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "claude --resume abc123"},
		})

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify hook was removed from file using structural key
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.0"]; ok {
			t.Error("expected structural key my-session:0.0 entry to be removed from hooks file")
		}
	})

	t.Run("reads pane ID from TMUX_PANE and resolves structural key", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%42")

		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"proj-xyz:1.2": {"on-resume": "some-cmd"},
		})

		resolver := &mockKeyResolver{key: "proj-xyz:1.2"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify structural key was used in the store removal (not raw pane ID)
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["proj-xyz:1.2"]; ok {
			t.Error("expected structural key proj-xyz:1.2 entry to be removed")
		}
		if _, ok := data["%42"]; ok {
			t.Error("raw pane ID %42 should not be used as key")
		}
	})

	t.Run("returns error when TMUX_PANE is not set", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "")

		resolver := &mockKeyResolver{key: "unused:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "must be run from inside a tmux pane") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "must be run from inside a tmux pane")
		}
	})

	t.Run("returns error when on-resume flag is not provided", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for missing --on-resume flag, got nil")
		}
		if !strings.Contains(err.Error(), "on-resume") {
			t.Errorf("error = %q, want it to mention %q", err.Error(), "on-resume")
		}
	})

	t.Run("silent no-op when no hook exists for pane", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%99")

		// Empty hooks file
		writeHooksJSON(t, hooksFile, map[string]map[string]string{})

		resolver := &mockKeyResolver{key: "some-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("expected no error for non-existent hook, got: %v", err)
		}

		// Should produce no output
		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}
	})

	t.Run("removes correct JSON entry from hooks file", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		// Seed with multiple keys — only the resolved one should be removed
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "claude --resume abc123"},
			"other-proj:0.0": {"on-resume": "npm start"},
		})

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)

		// my-session:0.0 should be gone
		if _, ok := data["my-session:0.0"]; ok {
			t.Error("expected structural key my-session:0.0 to be removed")
		}

		// other-proj:0.0 should remain
		if data["other-proj:0.0"]["on-resume"] != "npm start" {
			t.Errorf("other-proj:0.0 on-resume = %q, want %q", data["other-proj:0.0"]["on-resume"], "npm start")
		}
	})

	t.Run("cleans up pane key when last event removed", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%5")

		// Structural key has only one event — removing it should remove the key entirely
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "some-cmd"},
		})

		resolver := &mockKeyResolver{key: "my-session:0.0"}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.0"]; ok {
			t.Error("expected structural key my-session:0.0 to be removed when last event deleted")
		}
		if len(data) != 0 {
			t.Errorf("expected empty hooks file, got %d entries", len(data))
		}
	})

	t.Run("ResolveStructuralKey failure returns user-facing error", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"my-session:0.0": {"on-resume": "some-cmd"},
		})

		resolver := &mockKeyResolver{err: fmt.Errorf("tmux not responding")}
		hooksDeps = &HooksDeps{KeyResolver: resolver}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resolve") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "resolve")
		}

		// Verify no side effects: hook still in file
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["my-session:0.0"]; !ok {
			t.Error("hook should not have been removed on resolver failure")
		}
	})
}
