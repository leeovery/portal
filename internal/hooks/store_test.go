package hooks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
)

func TestLoad(t *testing.T) {
	t.Run("returns empty map when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "nonexistent", "hooks.json"))

		h, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h) != 0 {
			t.Errorf("got %d entries, want 0", len(h))
		}
	})

	t.Run("returns empty map when file contains malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")

		if err := os.WriteFile(filePath, []byte("{invalid json!!!"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		h, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h) != 0 {
			t.Errorf("got %d entries, want 0", len(h))
		}
	})

	t.Run("returns hooks from valid JSON file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")

		content := `{"%3":{"on-resume":"claude --resume abc123"},"%7":{"on-resume":"claude --resume def456"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		h, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h) != 2 {
			t.Fatalf("got %d panes, want 2", len(h))
		}

		if h["%3"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("%%3 on-resume = %q, want %q", h["%3"]["on-resume"], "claude --resume abc123")
		}
		if h["%7"]["on-resume"] != "claude --resume def456" {
			t.Errorf("%%7 on-resume = %q, want %q", h["%7"]["on-resume"], "claude --resume def456")
		}
	})
}

func TestSave(t *testing.T) {
	t.Run("creates parent directory if missing", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "portal", "sub")
		filePath := filepath.Join(nested, "hooks.json")
		store := hooks.NewStore(filePath)

		h := map[string]map[string]string{
			"%3": {"on-resume": "claude --resume abc123"},
		}

		if err := store.Save(h); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(nested)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected directory, got file")
		}
	})

	t.Run("writes valid JSON that can be loaded back", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		h := map[string]map[string]string{
			"%3": {"on-resume": "claude --resume abc123"},
			"%7": {"on-resume": "claude --resume def456"},
		}

		if err := store.Save(h); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load saved file: %v", err)
		}

		if len(loaded) != 2 {
			t.Fatalf("got %d panes, want 2", len(loaded))
		}
		if loaded["%3"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("%%3 on-resume = %q, want %q", loaded["%3"]["on-resume"], "claude --resume abc123")
		}
		if loaded["%7"]["on-resume"] != "claude --resume def456" {
			t.Errorf("%%7 on-resume = %q, want %q", loaded["%7"]["on-resume"], "claude --resume def456")
		}
	})

	t.Run("uses atomic write (file exists after save even if interrupted)", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		h := map[string]map[string]string{
			"%3": {"on-resume": "claude --resume abc123"},
		}

		if err := store.Save(h); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify no temp files remain in directory
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("failed to read dir: %v", err)
		}

		for _, entry := range entries {
			if entry.Name() != "hooks.json" {
				t.Errorf("unexpected file in directory: %s", entry.Name())
			}
		}

		// Verify the file exists and is valid
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if len(data) == 0 {
			t.Error("file is empty")
		}
	})
}

func TestSet(t *testing.T) {
	t.Run("adds a new hook for a new pane", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d panes, want 1", len(h))
		}
		if h["%3"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("%%3 on-resume = %q, want %q", h["%3"]["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("adds a second event to an existing pane", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("%3", "on-start", "echo hello"); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d panes, want 1", len(h))
		}
		if len(h["%3"]) != 2 {
			t.Fatalf("got %d events for %%3, want 2", len(h["%3"]))
		}
		if h["%3"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("%%3 on-resume = %q, want %q", h["%3"]["on-resume"], "claude --resume abc123")
		}
		if h["%3"]["on-start"] != "echo hello" {
			t.Errorf("%%3 on-start = %q, want %q", h["%3"]["on-start"], "echo hello")
		}
	})

	t.Run("overwrites existing entry for same pane and event", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("%3", "on-resume", "claude --resume xyz789"); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d panes, want 1", len(h))
		}
		if len(h["%3"]) != 1 {
			t.Fatalf("got %d events for %%3, want 1", len(h["%3"]))
		}
		if h["%3"]["on-resume"] != "claude --resume xyz789" {
			t.Errorf("%%3 on-resume = %q, want %q", h["%3"]["on-resume"], "claude --resume xyz789")
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("deletes a hook entry", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("%7", "on-resume", "claude --resume def456"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("%3", "on-resume"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d panes, want 1", len(h))
		}
		if _, ok := h["%3"]; ok {
			t.Error("pane %3 should have been removed")
		}
		if h["%7"]["on-resume"] != "claude --resume def456" {
			t.Errorf("%%7 on-resume = %q, want %q", h["%7"]["on-resume"], "claude --resume def456")
		}
	})

	t.Run("cleans up outer key when inner map is empty", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("%3", "on-resume"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 0 {
			t.Fatalf("got %d panes, want 0 (outer key should be cleaned up)", len(h))
		}
		if _, ok := h["%3"]; ok {
			t.Error("pane %3 should have been removed from outer map")
		}
	})

	t.Run("is a no-op when pane does not exist", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("%99", "on-resume"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d panes, want 1 (original should remain)", len(h))
		}
	})

	t.Run("is a no-op when event does not exist for pane", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("%3", "on-start"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d panes, want 1", len(h))
		}
		if h["%3"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("%%3 on-resume = %q, want %q", h["%3"]["on-resume"], "claude --resume abc123")
		}
	})
}

func TestList(t *testing.T) {
	t.Run("returns empty slice when no hooks", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		list, err := store.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(list) != 0 {
			t.Errorf("got %d hooks, want 0", len(list))
		}
	})

	t.Run("returns hooks sorted by pane ID then event", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")

		// Write hooks in non-sorted order
		content := `{"%7":{"on-resume":"cmd7"},"%3":{"on-start":"cmd3s","on-resume":"cmd3r"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		list, err := store.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(list) != 3 {
			t.Fatalf("got %d hooks, want 3", len(list))
		}

		// Expected order: %3/on-resume, %3/on-start, %7/on-resume
		wantPanes := []string{"%3", "%3", "%7"}
		wantEvents := []string{"on-resume", "on-start", "on-resume"}
		wantCmds := []string{"cmd3r", "cmd3s", "cmd7"}

		for i, hook := range list {
			if hook.PaneID != wantPanes[i] {
				t.Errorf("list[%d].PaneID = %q, want %q", i, hook.PaneID, wantPanes[i])
			}
			if hook.Event != wantEvents[i] {
				t.Errorf("list[%d].Event = %q, want %q", i, hook.Event, wantEvents[i])
			}
			if hook.Command != wantCmds[i] {
				t.Errorf("list[%d].Command = %q, want %q", i, hook.Command, wantCmds[i])
			}
		}
	})
}

func TestGet(t *testing.T) {
	t.Run("returns event map for registered pane", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("%3", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		events, err := store.Get("%3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(events) != 1 {
			t.Fatalf("got %d events, want 1", len(events))
		}
		if events["on-resume"] != "claude --resume abc123" {
			t.Errorf("on-resume = %q, want %q", events["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("returns empty map for unregistered pane", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		events, err := store.Get("%99")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(events) != 0 {
			t.Errorf("got %d events, want 0", len(events))
		}
	})
}
