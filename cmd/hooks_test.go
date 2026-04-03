// Tests in this file mutate package-level state (hooksDeps) and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"encoding/json"
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

	t.Run("hooks bypasses tmux bootstrap", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// No bootstrapDeps set — if tmux check runs, it will fail
		// because tmux may not be available in CI.
		// The fact that this succeeds proves skipTmuxCheck works.

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error (tmux check should be skipped): %v", err)
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

// mockOptionSetter records calls to SetServerOption for test assertions.
type mockOptionSetter struct {
	calls []serverOptionCall
	err   error
}

type serverOptionCall struct {
	name  string
	value string
}

func (m *mockOptionSetter) SetServerOption(name, value string) error {
	m.calls = append(m.calls, serverOptionCall{name: name, value: value})
	return m.err
}

func TestHooksSetCommand(t *testing.T) {
	t.Run("sets hook and volatile marker for current pane", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify hook was written to file
		data := readHooksJSON(t, hooksFile)
		if data["%3"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("hook command = %q, want %q", data["%3"]["on-resume"], "claude --resume abc123")
		}

		// Verify volatile marker was set
		if len(mock.calls) != 1 {
			t.Fatalf("expected 1 SetServerOption call, got %d", len(mock.calls))
		}
		if mock.calls[0].name != "@portal-active-%3" {
			t.Errorf("option name = %q, want %q", mock.calls[0].name, "@portal-active-%3")
		}
		if mock.calls[0].value != "1" {
			t.Errorf("option value = %q, want %q", mock.calls[0].value, "1")
		}
	})

	t.Run("reads pane ID from TMUX_PANE environment variable", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%99")

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the pane ID from env was used in the store
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["%99"]; !ok {
			t.Error("expected hook entry for pane %99, not found")
		}

		// Verify the pane ID from env was used in the volatile marker
		if len(mock.calls) != 1 {
			t.Fatalf("expected 1 SetServerOption call, got %d", len(mock.calls))
		}
		if mock.calls[0].name != "@portal-active-%99" {
			t.Errorf("option name = %q, want %q", mock.calls[0].name, "@portal-active-%99")
		}
	})

	t.Run("returns error when TMUX_PANE is not set", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "")

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
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

		// Verify no side effects: no file written, no SetServerOption calls
		if _, statErr := os.Stat(hooksFile); statErr == nil {
			t.Error("hooks file should not have been created")
		}
		if len(mock.calls) != 0 {
			t.Errorf("expected 0 SetServerOption calls, got %d", len(mock.calls))
		}
	})

	t.Run("returns error when on-resume flag is not provided", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
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

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
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
		if data["%3"]["on-resume"] != "new-cmd" {
			t.Errorf("hook command = %q, want %q", data["%3"]["on-resume"], "new-cmd")
		}

		// Verify marker was set both times
		if len(mock.calls) != 2 {
			t.Fatalf("expected 2 SetServerOption calls, got %d", len(mock.calls))
		}
	})

	t.Run("writes correct JSON structure to hooks file", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "claude --resume abc123"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Read the raw JSON and verify the structure
		data := readHooksJSON(t, hooksFile)

		// Should have exactly one pane entry
		if len(data) != 1 {
			t.Fatalf("expected 1 pane entry, got %d", len(data))
		}

		// The pane entry should have exactly one event
		events, ok := data["%3"]
		if !ok {
			t.Fatal("expected entry for pane %3")
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event for pane %%3, got %d", len(events))
		}
		if events["on-resume"] != "claude --resume abc123" {
			t.Errorf("on-resume = %q, want %q", events["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("sets volatile marker with correct option name", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%7")

		mock := &mockOptionSetter{}
		hooksDeps = &HooksDeps{OptionSetter: mock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "set", "--on-resume", "some-cmd"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.calls) != 1 {
			t.Fatalf("expected 1 SetServerOption call, got %d", len(mock.calls))
		}
		wantName := "@portal-active-%7"
		if mock.calls[0].name != wantName {
			t.Errorf("option name = %q, want %q", mock.calls[0].name, wantName)
		}
		wantValue := "1"
		if mock.calls[0].value != wantValue {
			t.Errorf("option value = %q, want %q", mock.calls[0].value, wantValue)
		}
	})
}

// mockOptionDeleter records calls to DeleteServerOption for test assertions.
type mockOptionDeleter struct {
	calls []string
	err   error
}

func (m *mockOptionDeleter) DeleteServerOption(name string) error {
	m.calls = append(m.calls, name)
	return m.err
}

func TestHooksRmCommand(t *testing.T) {
	t.Run("removes hook and volatile marker for current pane", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		// Seed with an existing hook
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"%3": {"on-resume": "claude --resume abc123"},
		})

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify hook was removed from file
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["%3"]; ok {
			t.Error("expected pane %3 entry to be removed from hooks file")
		}

		// Verify volatile marker was deleted
		if len(delMock.calls) != 1 {
			t.Fatalf("expected 1 DeleteServerOption call, got %d", len(delMock.calls))
		}
		if delMock.calls[0] != "@portal-active-%3" {
			t.Errorf("delete option name = %q, want %q", delMock.calls[0], "@portal-active-%3")
		}
	})

	t.Run("reads pane ID from TMUX_PANE environment variable", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%42")

		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"%42": {"on-resume": "some-cmd"},
		})

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the pane ID from env was used in the store removal
		data := readHooksJSON(t, hooksFile)
		if _, ok := data["%42"]; ok {
			t.Error("expected pane %42 entry to be removed")
		}

		// Verify the pane ID from env was used in the volatile marker deletion
		if len(delMock.calls) != 1 {
			t.Fatalf("expected 1 DeleteServerOption call, got %d", len(delMock.calls))
		}
		if delMock.calls[0] != "@portal-active-%42" {
			t.Errorf("delete option name = %q, want %q", delMock.calls[0], "@portal-active-%42")
		}
	})

	t.Run("returns error when TMUX_PANE is not set", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "")

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
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

		// Verify no side effects
		if len(delMock.calls) != 0 {
			t.Errorf("expected 0 DeleteServerOption calls, got %d", len(delMock.calls))
		}
	})

	t.Run("returns error when on-resume flag is not provided", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%3")

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
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

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
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

		// Seed with multiple panes — only %3 should be removed
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"%3": {"on-resume": "claude --resume abc123"},
			"%7": {"on-resume": "npm start"},
		})

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)

		// %3 should be gone
		if _, ok := data["%3"]; ok {
			t.Error("expected pane %3 to be removed")
		}

		// %7 should remain
		if data["%7"]["on-resume"] != "npm start" {
			t.Errorf("pane %%7 on-resume = %q, want %q", data["%7"]["on-resume"], "npm start")
		}
	})

	t.Run("deletes volatile marker with correct option name", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%7")

		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"%7": {"on-resume": "some-cmd"},
		})

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(delMock.calls) != 1 {
			t.Fatalf("expected 1 DeleteServerOption call, got %d", len(delMock.calls))
		}
		wantName := "@portal-active-%7"
		if delMock.calls[0] != wantName {
			t.Errorf("option name = %q, want %q", delMock.calls[0], wantName)
		}
	})

	t.Run("cleans up pane key when last event removed", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		t.Setenv("TMUX_PANE", "%5")

		// Pane %5 has only one event — removing it should remove the pane key entirely
		writeHooksJSON(t, hooksFile, map[string]map[string]string{
			"%5": {"on-resume": "some-cmd"},
		})

		delMock := &mockOptionDeleter{}
		hooksDeps = &HooksDeps{OptionDeleter: delMock}
		t.Cleanup(func() { hooksDeps = nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "rm", "--on-resume"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data := readHooksJSON(t, hooksFile)
		if _, ok := data["%5"]; ok {
			t.Error("expected pane %5 key to be removed when last event deleted")
		}
		if len(data) != 0 {
			t.Errorf("expected empty hooks file, got %d entries", len(data))
		}
	})
}

// readHooksJSON is a test helper that reads and parses the hooks JSON file.
func readHooksJSON(t *testing.T, path string) map[string]map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read hooks file: %v", err)
	}
	var data map[string]map[string]string
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("failed to unmarshal hooks JSON: %v", err)
	}
	return data
}

// writeHooksJSON is a test helper that writes a hooks JSON file.
func writeHooksJSON(t *testing.T, path string, data map[string]map[string]string) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal hooks JSON: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("failed to write hooks file: %v", err)
	}
}
