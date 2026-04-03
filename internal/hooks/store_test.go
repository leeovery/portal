package hooks_test

import (
	"os"
	"path/filepath"
	"sort"
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

		content := `{"my-session:0.0":{"on-resume":"claude --resume abc123"},"my-session:0.1":{"on-resume":"claude --resume def456"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)
		h, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h) != 2 {
			t.Fatalf("got %d keys, want 2", len(h))
		}

		if h["my-session:0.0"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("my-session:0.0 on-resume = %q, want %q", h["my-session:0.0"]["on-resume"], "claude --resume abc123")
		}
		if h["my-session:0.1"]["on-resume"] != "claude --resume def456" {
			t.Errorf("my-session:0.1 on-resume = %q, want %q", h["my-session:0.1"]["on-resume"], "claude --resume def456")
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
			"my-session:0.0": {"on-resume": "claude --resume abc123"},
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
			"my-session:0.0": {"on-resume": "claude --resume abc123"},
			"my-session:0.1": {"on-resume": "claude --resume def456"},
		}

		if err := store.Save(h); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load saved file: %v", err)
		}

		if len(loaded) != 2 {
			t.Fatalf("got %d keys, want 2", len(loaded))
		}
		if loaded["my-session:0.0"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("my-session:0.0 on-resume = %q, want %q", loaded["my-session:0.0"]["on-resume"], "claude --resume abc123")
		}
		if loaded["my-session:0.1"]["on-resume"] != "claude --resume def456" {
			t.Errorf("my-session:0.1 on-resume = %q, want %q", loaded["my-session:0.1"]["on-resume"], "claude --resume def456")
		}
	})

	t.Run("uses atomic write (file exists after save even if interrupted)", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		h := map[string]map[string]string{
			"my-session:0.0": {"on-resume": "claude --resume abc123"},
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
	t.Run("adds a new hook for a new key", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if h["my-session:0.0"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("my-session:0.0 on-resume = %q, want %q", h["my-session:0.0"]["on-resume"], "claude --resume abc123")
		}
	})

	t.Run("adds a second event to an existing key", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-start", "echo hello"); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if len(h["my-session:0.0"]) != 2 {
			t.Fatalf("got %d events for my-session:0.0, want 2", len(h["my-session:0.0"]))
		}
		if h["my-session:0.0"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("my-session:0.0 on-resume = %q, want %q", h["my-session:0.0"]["on-resume"], "claude --resume abc123")
		}
		if h["my-session:0.0"]["on-start"] != "echo hello" {
			t.Errorf("my-session:0.0 on-start = %q, want %q", h["my-session:0.0"]["on-start"], "echo hello")
		}
	})

	t.Run("overwrites existing entry for same key and event", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume xyz789"); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if len(h["my-session:0.0"]) != 1 {
			t.Fatalf("got %d events for my-session:0.0, want 1", len(h["my-session:0.0"]))
		}
		if h["my-session:0.0"]["on-resume"] != "claude --resume xyz789" {
			t.Errorf("my-session:0.0 on-resume = %q, want %q", h["my-session:0.0"]["on-resume"], "claude --resume xyz789")
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("deletes a hook entry", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("my-session:0.0", "on-resume"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if _, ok := h["my-session:0.0"]; ok {
			t.Error("key my-session:0.0 should have been removed")
		}
		if h["my-session:0.1"]["on-resume"] != "claude --resume def456" {
			t.Errorf("my-session:0.1 on-resume = %q, want %q", h["my-session:0.1"]["on-resume"], "claude --resume def456")
		}
	})

	t.Run("cleans up outer key when inner map is empty", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("my-session:0.0", "on-resume"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 0 {
			t.Fatalf("got %d keys, want 0 (outer key should be cleaned up)", len(h))
		}
		if _, ok := h["my-session:0.0"]; ok {
			t.Error("key my-session:0.0 should have been removed from outer map")
		}
	})

	t.Run("is a no-op when key does not exist", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("nonexistent:9.9", "on-resume"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1 (original should remain)", len(h))
		}
	})

	t.Run("is a no-op when event does not exist for key", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("my-session:0.0", "on-start"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if h["my-session:0.0"]["on-resume"] != "claude --resume abc123" {
			t.Errorf("my-session:0.0 on-resume = %q, want %q", h["my-session:0.0"]["on-resume"], "claude --resume abc123")
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

	t.Run("returns hooks sorted by key then event", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")

		// Write hooks in non-sorted order
		content := `{"my-session:0.1":{"on-resume":"cmd1"},"my-session:0.0":{"on-start":"cmd0s","on-resume":"cmd0r"}}`
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

		// Expected order: my-session:0.0/on-resume, my-session:0.0/on-start, my-session:0.1/on-resume
		wantKeys := []string{"my-session:0.0", "my-session:0.0", "my-session:0.1"}
		wantEvents := []string{"on-resume", "on-start", "on-resume"}
		wantCmds := []string{"cmd0r", "cmd0s", "cmd1"}

		for i, hook := range list {
			if hook.Key != wantKeys[i] {
				t.Errorf("list[%d].Key = %q, want %q", i, hook.Key, wantKeys[i])
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
	t.Run("returns event map for registered key", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		events, err := store.Get("my-session:0.0")
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

	t.Run("returns empty map for unregistered key", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		events, err := store.Get("nonexistent:9.9")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(events) != 0 {
			t.Errorf("got %d events, want 0", len(events))
		}
	})
}

func TestCleanStale(t *testing.T) {
	t.Run("removes entries for keys not in live set", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale([]string{"my-session:0.0"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 1 {
			t.Fatalf("got %d removed, want 1", len(removed))
		}
		if removed[0] != "my-session:0.1" {
			t.Errorf("removed[0] = %q, want %q", removed[0], "my-session:0.1")
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if _, ok := h["my-session:0.0"]; !ok {
			t.Error("key my-session:0.0 should have been kept")
		}
		if _, ok := h["my-session:0.1"]; ok {
			t.Error("key my-session:0.1 should have been removed")
		}
	})

	t.Run("returns empty slice when store is empty", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		removed, err := store.CleanStale([]string{"my-session:0.0", "my-session:0.1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("got %d removed, want 0", len(removed))
		}
	})

	t.Run("returns empty slice when all keys are live", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale([]string{"my-session:0.0", "my-session:0.1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("got %d removed, want 0", len(removed))
		}
	})

	t.Run("removes all entries when live set is empty", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 0 {
			t.Errorf("got %d keys, want 0", len(h))
		}
	})

	t.Run("only saves file when entries were removed", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		// Record mod time before CleanStale
		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		removed, err := store.CleanStale([]string{"my-session:0.0"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("got %d removed, want 0", len(removed))
		}

		// Mod time should be unchanged since no save occurred
		infoAfter, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
			t.Error("file was modified when no entries were removed")
		}
	})

	t.Run("handles mix of live and stale keys", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "cmd0"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("other-session:0.0", "on-resume", "cmd-other0"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "cmd1"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("other-session:0.1", "on-resume", "cmd-other1"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		// my-session:0.0 and other-session:0.1 are live; other-session:0.0 and my-session:0.1 are stale
		removed, err := store.CleanStale([]string{"my-session:0.0", "other-session:0.1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		// Sort removed for deterministic checking
		sort.Strings(removed)
		if removed[0] != "my-session:0.1" {
			t.Errorf("removed[0] = %q, want %q", removed[0], "my-session:0.1")
		}
		if removed[1] != "other-session:0.0" {
			t.Errorf("removed[1] = %q, want %q", removed[1], "other-session:0.0")
		}

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 2 {
			t.Fatalf("got %d keys, want 2", len(h))
		}
		if _, ok := h["my-session:0.0"]; !ok {
			t.Error("key my-session:0.0 should have been kept")
		}
		if _, ok := h["other-session:0.1"]; !ok {
			t.Error("key other-session:0.1 should have been kept")
		}
	})
}
