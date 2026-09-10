package hooks_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

// readFileBytes returns the file's exact bytes, failing when it is absent, so a
// before-read cannot silently record the emptiness of a file the fixture never
// wrote.
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return data
}

// modTime is the file's mtime, so a test can prove a no-op skipped Save even
// when a rewrite would have produced byte-identical content.
func modTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	return info.ModTime()
}

func TestLoad(t *testing.T) {
	t.Run("returns empty map when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, filepath.Join(dir, "nonexistent")))

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h) != 0 {
			t.Errorf("got %d entries, want 0", len(h))
		}
	})

	t.Run("returns empty map when file contains malformed JSON", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: "{invalid json!!!"})

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(h) != 0 {
			t.Errorf("got %d entries, want 0", len(h))
		}
	})

	t.Run("returns hooks from valid JSON file", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: `{"my-session:0.0":{"on-resume":"claude --resume abc123"},"my-session:0.1":{"on-resume":"claude --resume def456"}}`,
		})

		h, err := store.Load(hooks.ViaInternal)
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

func TestPersistence(t *testing.T) {
	t.Run("creates parent directory if missing", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "portal", "sub")
		filePath := hookstest.HooksPath(t, nested)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
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
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		loaded, err := store.Load(hooks.ViaInternal)
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

	t.Run("uses atomic write (no temp file survives beside hooks.json and its lock)", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("failed to read dir: %v", err)
		}

		for _, entry := range entries {
			if entry.Name() != "hooks.json" && entry.Name() != "hooks.json.lock" {
				t.Errorf("unexpected file in directory: %s", entry.Name())
			}
		}

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
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		h, err := store.Load(hooks.ViaInternal)
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
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-start", "echo hello", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		h, err := store.Load(hooks.ViaInternal)
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

	t.Run("it refuses to write over a malformed hooks.json", func(t *testing.T) {
		store, filePath := hookstest.StageStore(t, hookstest.Staging{Seed: "not json"})
		before := hookstest.HooksFileBytes(t, filePath)

		err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI)
		if !errors.Is(err, hooks.ErrMalformed) {
			t.Errorf("err = %v, want errors.Is ErrMalformed — a map loaded from nothing and written back is every other entry gone", err)
		}

		hookstest.AssertHooksFileUnchanged(t, filePath, before, "rewritten while malformed")
	})

	t.Run("overwrites existing entry for same key and event", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume xyz789", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		h, err := store.Load(hooks.ViaInternal)
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
	t.Run("it reports a removal when the named event was deleted", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true for a seeded entry")
		}

		h, err := store.Load(hooks.ViaInternal)
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

	t.Run("it drops the outer key when its last event goes", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true for a seeded entry")
		}

		h, err := store.Load(hooks.ViaInternal)
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

	t.Run("it keeps the other events of a key and reports a removal", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-start", "echo hello", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true for a seeded entry")
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		events, ok := h["my-session:0.0"]
		if !ok {
			t.Fatalf("outer key my-session:0.0 should be retained, got %+v", h)
		}
		if _, ok := events["on-resume"]; ok {
			t.Error("on-resume should have been removed")
		}
		if events["on-start"] != "echo hello" {
			t.Errorf("on-start = %q, want %q", events["on-start"], "echo hello")
		}
	})

	t.Run("it reports no removal when the key is absent", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		before := readFileBytes(t, filePath)
		beforeMod := modTime(t, filePath)

		removed, err := store.Remove("nonexistent:9.9", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if removed {
			t.Error("removed = true, want false for an absent key")
		}

		hookstest.AssertHooksFileUnchanged(t, filePath, before, "rewritten on a no-op removal")
		if after := modTime(t, filePath); !after.Equal(beforeMod) {
			t.Error("file was written on a no-op removal (Save should be skipped)")
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1 (original should remain)", len(h))
		}
	})

	t.Run("it reports no removal when the key is present but the event is not", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		before := readFileBytes(t, filePath)
		beforeMod := modTime(t, filePath)

		removed, err := store.Remove("my-session:0.0", "on-start", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if removed {
			t.Error("removed = true, want false when the named event is absent")
		}

		hookstest.AssertHooksFileUnchanged(t, filePath, before, "rewritten on a no-op removal")
		if after := modTime(t, filePath); !after.Equal(beforeMod) {
			t.Error("file was written on a no-op removal (Save should be skipped)")
		}

		h, err := store.Load(hooks.ViaInternal)
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

	t.Run("it leaves an absent hooks.json absent when it removes nothing", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if removed {
			t.Error("removed = true, want false with no hooks file at all")
		}

		if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("hooks.json should still not exist, stat err = %v", err)
		}
	})

	t.Run("it refuses to judge a malformed hooks.json and leaves it byte-identical", func(t *testing.T) {
		store, filePath := hookstest.StageStore(t, hookstest.Staging{Seed: "not json"})
		before := hookstest.HooksFileBytes(t, filePath)

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if !errors.Is(err, hooks.ErrMalformed) {
			t.Errorf("err = %v, want errors.Is ErrMalformed — an answer about a file that did not parse is a fiction", err)
		}
		if removed {
			t.Error("removed = true, want false for a malformed file")
		}

		hookstest.AssertHooksFileUnchanged(t, filePath, before, "rewritten while malformed")
	})

	t.Run("it leaves a key mapped to an empty event map in place", func(t *testing.T) {
		store, filePath := hookstest.StageStore(t, hookstest.Staging{Seed: `{"abc123": {}}`})
		before := hookstest.HooksFileBytes(t, filePath)

		removed, err := store.Remove("abc123", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if removed {
			t.Error("removed = true, want false for a key with no events")
		}

		hookstest.AssertHooksFileUnchanged(t, filePath, before, "rewritten on a no-op removal")
	})

	t.Run("it reports no removal when the save fails", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"my-session:0.0":{"on-resume":"claude --resume abc123"}}`, WritesDenied: true})

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err == nil {
			t.Fatal("expected error from Remove on read-only dir, got nil")
		}
		if removed {
			t.Error("removed = true, want false when the save failed")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}
	})
}

func TestList(t *testing.T) {
	t.Run("returns empty slice when no hooks", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))

		list, err := store.List(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(list) != 0 {
			t.Errorf("got %d hooks, want 0", len(list))
		}
	})

	t.Run("returns hooks sorted by key then event", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: `{"my-session:0.1":{"on-resume":"cmd1"},"my-session:0.0":{"on-start":"cmd0s","on-resume":"cmd0r"}}`,
		})

		list, err := store.List(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(list) != 3 {
			t.Fatalf("got %d hooks, want 3", len(list))
		}

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

func TestCleanStale(t *testing.T) {
	t.Run("removes entries for keys not in live set", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedA, "on-resume", "claude --resume def456", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale(enumerating("my-session:0.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 1 {
			t.Fatalf("got %d removed, want 1", len(removed))
		}
		if removed[0] != hookstest.ReapableSeedA {
			t.Errorf("removed[0] = %q, want %q", removed[0], hookstest.ReapableSeedA)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if _, ok := h["my-session:0.0"]; !ok {
			t.Error("key my-session:0.0 should have been kept")
		}
		if _, ok := h[hookstest.ReapableSeedA]; ok {
			t.Errorf("key %s should have been removed", hookstest.ReapableSeedA)
		}
	})

	t.Run("returns empty slice when store is empty", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))

		removed, err := store.CleanStale(enumerating("my-session:0.0", "my-session:0.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("got %d removed, want 0", len(removed))
		}
	})

	t.Run("returns empty slice when all keys are live", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale(enumerating("my-session:0.0", "my-session:0.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("got %d removed, want 0", len(removed))
		}
	})

	t.Run("removes all entries when live set is empty", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set(hookstest.ReapableSeedA, "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedB, "on-resume", "claude --resume def456", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale(enumerating())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 0 {
			t.Errorf("got %d keys, want 0", len(h))
		}
	})

	t.Run("only saves file when entries were removed", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		removed, err := store.CleanStale(enumerating("my-session:0.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("got %d removed, want 0", len(removed))
		}

		infoAfter, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
			t.Error("file was modified when no entries were removed")
		}
	})

	t.Run("cleans stale entries seeded straight into the file", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":"claude --resume old1"},%q:{"on-resume":"claude --resume old2"},"my-session:0.0":{"on-resume":"claude --resume new1"}}`, hookstest.ReapableSeedA, hookstest.ReapableSeedB),
		})

		removed, err := store.CleanStale(enumerating("my-session:0.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		sort.Strings(removed)
		if removed[0] != hookstest.ReapableSeedA {
			t.Errorf("removed[0] = %q, want %q", removed[0], hookstest.ReapableSeedA)
		}
		if removed[1] != hookstest.ReapableSeedB {
			t.Errorf("removed[1] = %q, want %q", removed[1], hookstest.ReapableSeedB)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(h) != 1 {
			t.Fatalf("got %d keys, want 1", len(h))
		}
		if _, ok := h["my-session:0.0"]; !ok {
			t.Error("key my-session:0.0 should have been kept")
		}
		if _, ok := h[hookstest.ReapableSeedA]; ok {
			t.Errorf("key %s should have been removed", hookstest.ReapableSeedA)
		}
		if _, ok := h[hookstest.ReapableSeedB]; ok {
			t.Errorf("key %s should have been removed", hookstest.ReapableSeedB)
		}
	})

	t.Run("handles mix of live and stale keys", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedB, "on-resume", "cmd-other0", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedA, "on-resume", "cmd1", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("other-session:0.1", "on-resume", "cmd-other1", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		removed, err := store.CleanStale(enumerating("my-session:0.0", "other-session:0.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		sort.Strings(removed)
		if removed[0] != hookstest.ReapableSeedA {
			t.Errorf("removed[0] = %q, want %q", removed[0], hookstest.ReapableSeedA)
		}
		if removed[1] != hookstest.ReapableSeedB {
			t.Errorf("removed[1] = %q, want %q", removed[1], hookstest.ReapableSeedB)
		}

		h, err := store.Load(hooks.ViaInternal)
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

func TestStaleKeys(t *testing.T) {
	t.Run("returns persisted keys absent from the live set", func(t *testing.T) {
		persisted := map[string]map[string]string{
			"live-a:0.0":            {"on-resume": "x"},
			hookstest.ReapableSeedB: {"on-resume": "y"},
			"live-c:0.0":            {"on-resume": "z"},
			hookstest.ReapableSeedD: {"on-resume": "w"},
		}
		live := []string{"live-a:0.0", "live-c:0.0", "extra-e:0.0"}

		got := hooks.StaleKeys(persisted, live)
		sort.Strings(got)
		want := []string{hookstest.ReapableSeedB, hookstest.ReapableSeedD}
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("StaleKeys = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("StaleKeys[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("returns empty when every persisted key is live", func(t *testing.T) {
		persisted := map[string]map[string]string{
			"a:0.0": {"on-resume": "x"},
			"b:0.0": {"on-resume": "y"},
		}
		got := hooks.StaleKeys(persisted, []string{"a:0.0", "b:0.0", "c:0.0"})
		if len(got) != 0 {
			t.Errorf("StaleKeys = %v, want empty", got)
		}
	})

	t.Run("returns every judgeable persisted key when the live set is empty", func(t *testing.T) {
		persisted := map[string]map[string]string{
			hookstest.ReapableSeedA: {"on-resume": "x"},
			hookstest.ReapableSeedB: {"on-resume": "y"},
		}
		got := hooks.StaleKeys(persisted, []string{})
		if len(got) != 2 {
			t.Fatalf("StaleKeys = %v, want both keys", got)
		}
	})

	t.Run("returns empty for an empty persisted map", func(t *testing.T) {
		got := hooks.StaleKeys(map[string]map[string]string{}, []string{"a:0.0"})
		if len(got) != 0 {
			t.Errorf("StaleKeys = %v, want empty", got)
		}
	})
}

func TestCleanStaleRemovesExactlyStaleKeys(t *testing.T) {
	dir := t.TempDir()
	store := hooks.NewStore(hookstest.HooksPath(t, dir))
	for _, k := range []string{hookstest.ReapableSeedA, hookstest.ReapableSeedB, hookstest.ReapableSeedC, hookstest.ReapableSeedD} {
		if err := store.Set(k, "on-resume", "cmd", hooks.ViaCLI); err != nil {
			t.Fatalf("seed set %q: %v", k, err)
		}
	}

	live := []string{hookstest.ReapableSeedA, hookstest.ReapableSeedC}

	persisted, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	predicted := hooks.StaleKeys(persisted, live)
	sort.Strings(predicted)
	if len(predicted) == 0 {
		t.Fatal("StaleKeys predicted nothing — an equality between two empty sets proves nothing here")
	}

	removed, err := store.CleanStale(enumerating(live...))
	if err != nil {
		t.Fatalf("CleanStale: %v", err)
	}
	sort.Strings(removed)

	if len(removed) != len(predicted) {
		t.Fatalf("CleanStale removed %v, StaleKeys predicted %v", removed, predicted)
	}
	for i := range predicted {
		if removed[i] != predicted[i] {
			t.Errorf("removed[%d] = %q, StaleKeys[%d] = %q", i, removed[i], i, predicted[i])
		}
	}
}

// partitionCleanStaleRecords splits the captured clean-stale records into the
// per-key lines and the batch summaries, so an assertion compares sets rather
// than the map-iteration order the per-key lines are emitted in.
func partitionCleanStaleRecords(t *testing.T, recs logtest.Records) (perKey, summaries logtest.Records) {
	t.Helper()
	for _, r := range recs {
		// The clean's own pre-read says so when it degrades, which it does
		// wherever no sidecar has been staged. That breadcrumb is the read's,
		// not one of the deletion's lines.
		if r.Msg == "load-unlocked" {
			continue
		}
		if r.Msg != "clean-stale" {
			t.Errorf("unexpected msg %q in %+v", r.Msg, r)
			continue
		}
		if got := r.AttrString(t, "op"); got != "clean-stale" {
			t.Errorf("op = %q, want %q", got, "clean-stale")
		}
		if got := r.AttrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		switch {
		case r.HasAttr("hook_key"):
			perKey = append(perKey, r)
		case r.HasAttr("entries"):
			summaries = append(summaries, r)
		default:
			t.Errorf("clean-stale record is neither per-key nor summary: %+v", r)
		}
	}
	return perKey, summaries
}

func TestCleanStaleLogging(t *testing.T) {
	t.Run("it logs one INFO per removed key carrying the key and the removed command", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedA, "on-resume", "cmd1", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedB, "on-resume", "cmd2", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := logtest.Install(t)
		removed, err := store.CleanStale(enumerating("my-session:0.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		perKey, summaries := partitionCleanStaleRecords(t, sink.Records())

		if len(perKey) != 2 {
			t.Fatalf("got %d per-key records, want 2: %+v", len(perKey), perKey)
		}
		want := map[string]string{hookstest.ReapableSeedA: "cmd1", hookstest.ReapableSeedB: "cmd2"}
		got := map[string]string{}
		for _, r := range perKey {
			if r.Level != slog.LevelInfo {
				t.Errorf("per-key level = %v, want INFO: %+v", r.Level, r)
			}
			if via := r.AttrString(t, "via"); via != "internal" {
				t.Errorf("per-key via = %q, want %q", via, "internal")
			}
			got[r.AttrString(t, "hook_key")] = r.AttrString(t, "value")
		}
		if !maps.Equal(got, want) {
			t.Errorf("per-key records = %v, want %v", got, want)
		}

		summary := summaries.Only(t, "INFO clean-stale summary record")
		if summary.Level != slog.LevelInfo {
			t.Errorf("summary level = %v, want INFO", summary.Level)
		}
		if got := summary.AttrString(t, "entries"); got != "2" {
			t.Errorf("summary entries = %q, want %q", got, "2")
		}
		if got := summary.AttrString(t, "via"); got != "internal" {
			t.Errorf("summary via = %q, want %q", got, "internal")
		}
		summary.RequireDuration(t, "took")
	})

	t.Run("it logs the removed command in the value attr when a stale on-resume entry is reaped", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":"cmd1"}}`, hookstest.ReapableSeedA),
		})

		sink := logtest.Install(t)
		if _, err := store.CleanStale(enumerating("my-session:0.0")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		perKey, _ := partitionCleanStaleRecords(t, sink.Records())
		removed := perKey.Only(t, "per-key clean-stale record")
		if got := removed.AttrString(t, "hook_key"); got != hookstest.ReapableSeedA {
			t.Errorf("hook_key = %q, want %q", got, hookstest.ReapableSeedA)
		}
		if got := removed.AttrString(t, "value"); got != "cmd1" {
			t.Errorf("value = %q, want %q", got, "cmd1")
		}
	})

	t.Run("it logs a non-empty value when the reaped entry is filed under another event", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-exit":"x"}}`, hookstest.ReapableSeedA),
		})

		sink := logtest.Install(t)
		if _, err := store.CleanStale(enumerating("my-session:0.0")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		perKey, _ := partitionCleanStaleRecords(t, sink.Records())
		removed := perKey.Only(t, "per-key clean-stale record")
		if got := removed.AttrString(t, "hook_key"); got != hookstest.ReapableSeedA {
			t.Errorf("hook_key = %q, want %q", got, hookstest.ReapableSeedA)
		}
		if got := removed.AttrString(t, "value"); got != "x" {
			t.Errorf("value = %q, want %q", got, "x")
		}
	})

	t.Run("it reports the events a reaped key actually held rather than one assumed name", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: fmt.Sprintf(`{%q:{"on-resume":"cmd1","on-exit":"x"}}`, hookstest.ReapableSeedA),
		})

		sink := logtest.Install(t)
		if _, err := store.CleanStale(enumerating("my-session:0.0")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		perKey, _ := partitionCleanStaleRecords(t, sink.Records())
		want := "on-exit=x; on-resume=cmd1"
		if got := perKey.Only(t, "per-key clean-stale record").AttrString(t, "value"); got != want {
			t.Errorf("value = %q, want %q", got, want)
		}
	})

	t.Run("it emits no per-key lines and warns in the summary when the save fails", func(t *testing.T) {
		body := fmt.Sprintf(`{%q:{"on-resume":"cmdA"}}`, hookstest.ReapableSeedA)
		store, seeded := hookstest.StageStore(t, hookstest.Staging{Seed: body, WritesDenied: true})
		sink := logtest.Install(t)

		if _, err := store.CleanStale(enumerating("my-session:0.0")); err == nil {
			t.Fatal("expected error from CleanStale on read-only dir, got nil")
		}

		perKey, summaries := partitionCleanStaleRecords(t, sink.Records())
		if len(perKey) != 0 {
			t.Errorf("got %d per-key records, want 0 — no key was removed from the file: %+v", len(perKey), perKey)
		}

		summary := summaries.Only(t, "clean-stale summary record")
		logtest.AssertRecord(t, summary, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "clean-stale",
			Component: "hooks",
			Op:        "clean-stale",
			Via:       "internal",
		})
		logtest.AssertWriteFailure(t, summary, "write-failed-temp-create", fileutil.ErrWriteTempCreate)

		hookstest.AssertHooksFileUnchanged(t, seeded, []byte(body), "changed on a failed save")
	})

	t.Run("omits entries_failed from the summary when no per-entry failures occur", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set(hookstest.ReapableSeedA, "on-resume", "cmd1", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := logtest.Install(t)
		if _, err := store.CleanStale(enumerating("my-session:0.0")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var summary logtest.Record
		var found bool
		for _, r := range sink.Records() {
			if r.Level == slog.LevelInfo && r.Msg == "clean-stale" {
				summary = r
				found = true
			}
		}
		if !found {
			t.Fatalf("no INFO clean-stale summary captured: %+v", sink.Records())
		}
		if summary.HasAttr("entries_failed") {
			t.Errorf("summary must omit entries_failed when no failures: %+v", summary.Attrs)
		}
	})

	t.Run("emits WARN with write-failed-* error_class (not unexpected) when the batched Save fails", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"x"},%q:{"on-resume":"y"}}`, hookstest.ReapableSeedA, hookstest.ReapableSeedB), WritesDenied: true})
		sink := logtest.Install(t)

		_, err := store.CleanStale(enumerating())
		if err == nil {
			t.Fatal("expected error from CleanStale on read-only dir, got nil")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		warn := sink.Records().WithMessage("clean-stale").AtExactLevel(slog.LevelWarn).Only(t, "WARN clean-stale record")
		logtest.AssertRecord(t, warn, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "clean-stale",
			Component: "hooks",
			Op:        "clean-stale",
			Via:       "internal",
		})
		if got := warn.AttrString(t, "entries"); got != "2" {
			t.Errorf("entries = %q, want %q", got, "2")
		}
		logtest.AssertWriteFailure(t, warn, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
		warn.RequireDuration(t, "took")
	})

	t.Run("emits no summary and skips Save when zero entries are removed", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		sink := logtest.Install(t)
		removed, err := store.CleanStale(enumerating("my-session:0.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed) != 0 {
			t.Fatalf("got %d removed, want 0", len(removed))
		}

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("zero-removal CleanStale emitted %d records, want 0: %+v", len(recs), recs)
		}

		infoAfter, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
			t.Error("file was modified on a zero-removal CleanStale (Save should be skipped)")
		}
	})
}

func TestSetLogging(t *testing.T) {
	t.Run("emits INFO op=set with value and via=cli for a new hook key", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))
		sink := logtest.Install(t)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelInfo,
			Msg:       "set",
			Component: "hooks",
			Op:        "set",
			Via:       "cli",
		})
		if got := rec.AttrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if got := rec.AttrString(t, "value"); got != "claude --resume abc123" {
			t.Errorf("value = %q, want %q", got, "claude --resume abc123")
		}
	})

	t.Run("emits INFO op=modify when the key exists with a different value", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}

		sink := logtest.Install(t)
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume xyz789", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelInfo,
			Msg:       "modify",
			Component: "hooks",
			Op:        "modify",
			Via:       "cli",
		})
		if got := rec.AttrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if got := rec.AttrString(t, "value"); got != "claude --resume xyz789" {
			t.Errorf("value = %q, want %q", got, "claude --resume xyz789")
		}
	})

	t.Run("emits DEBUG op=set-noop and skips Save when key+value already match", func(t *testing.T) {
		dir := t.TempDir()
		filePath := hookstest.HooksPath(t, dir)
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		sink := logtest.Install(t)
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on noop set: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelDebug,
			Msg:       "set-noop",
			Component: "hooks",
			Op:        "set-noop",
			Via:       "cli",
		})
		if got := rec.AttrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if rec.HasAttr("value") {
			t.Errorf("set-noop record should not carry a value attr: %+v", rec.Attrs)
		}

		infoAfter, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
			t.Error("file was modified on a set-noop (Save should be skipped)")
		}
	})

	t.Run("emits WARN with error_class=write-failed-temp-create when AtomicWrite fails on Set", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{WritesDenied: true})
		sink := logtest.Install(t)

		err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI)
		if err == nil {
			t.Fatal("expected error from Set on read-only dir, got nil")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "set",
			Component: "hooks",
			Op:        "set",
			Via:       "cli",
		})
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
	})
}

func TestSetEmitsOpAsJSONField(t *testing.T) {
	// Not logtest.Install: this asserts the JSON *rendering* of the emission,
	// which a logtest.Sink (it captures structurally and renders as text) does
	// not produce.
	var buf bytes.Buffer
	log.SetTestHandler(t, slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := t.TempDir()
	store := hooks.NewStore(hookstest.HooksPath(t, dir))
	if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("failed to parse JSON log line %q: %v", buf.String(), err)
	}
	if got := rec["op"]; got != "set" {
		t.Errorf(`JSON "op" field = %v, want "set" (line: %s)`, got, buf.String())
	}
	if got := rec["component"]; got != "hooks" {
		t.Errorf(`JSON "component" field = %v, want "hooks"`, got)
	}
}

func TestRemoveLogging(t *testing.T) {
	t.Run("it still emits INFO op=rm for a real removal", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(hookstest.HooksPath(t, dir))

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", hooks.ViaCLI); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := logtest.Install(t)
		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}
		if !removed {
			t.Error("removed = false, want true for a seeded entry")
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelInfo,
			Msg:       "rm",
			Component: "hooks",
			Op:        "rm",
			Via:       "cli",
		})
		if got := rec.AttrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if rec.HasAttr("value") {
			t.Errorf("rm record should not carry a value attr: %+v", rec.Attrs)
		}
	})

	t.Run("it emits no record at all when it removes nothing", func(t *testing.T) {
		cases := []struct {
			name string
			seed map[string]map[string]string
			key  string
			ev   hooks.Event
		}{
			{
				name: "absent key",
				seed: map[string]map[string]string{"my-session:0.0": {"on-resume": "claude --resume abc123"}},
				key:  "nonexistent:9.9",
				ev:   "on-resume",
			},
			{
				name: "absent event",
				seed: map[string]map[string]string{"my-session:0.0": {"on-resume": "claude --resume abc123"}},
				key:  "my-session:0.0",
				ev:   "on-start",
			},
			{
				name: "absent file",
				seed: nil,
				key:  "my-session:0.0",
				ev:   "on-resume",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				store := hooks.NewStore(hookstest.HooksPath(t, dir))
				for key, events := range tc.seed {
					for event, command := range events {
						if err := store.Set(key, hooks.Event(event), command, hooks.ViaCLI); err != nil {
							t.Fatalf("unexpected error on set: %v", err)
						}
					}
				}

				sink := logtest.Install(t)
				removed, err := store.Remove(tc.key, tc.ev, hooks.ViaCLI)
				if err != nil {
					t.Fatalf("unexpected error on remove: %v", err)
				}
				if removed {
					t.Error("removed = true, want false")
				}

				if recs := sink.Records(); len(recs) != 0 {
					t.Errorf("a removal that removed nothing emitted %d log records, want 0: %+v", len(recs), recs)
				}
			})
		}
	})

	t.Run("emits WARN with error_class=write-failed-temp-create when AtomicWrite fails on Remove", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: `{"my-session:0.0":{"on-resume":"claude --resume abc123"}}`, WritesDenied: true})
		sink := logtest.Install(t)

		removed, err := store.Remove("my-session:0.0", "on-resume", hooks.ViaCLI)
		if err == nil {
			t.Fatal("expected error from Remove on read-only dir, got nil")
		}
		if removed {
			t.Error("removed = true, want false when the save failed")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "rm",
			Component: "hooks",
			Op:        "rm",
			Via:       "cli",
		})
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
	})
}
