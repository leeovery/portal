package hooks_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
)

// captureSink is a slog.Handler that records every emitted record together with
// the attrs bound via WithAttrs (notably the component attr that log.For binds
// at the logger, not at each call site) so the hooks store tests can assert on
// component=hooks and the per-call attrs faithfully.
type captureSink struct {
	mu      sync.Mutex
	records []captureRecord
	// shared points at the records-owning sink so handlers derived via
	// WithAttrs/WithGroup record into the same buffer; nil on the root sink.
	shared *captureSink
	// bound holds attrs accumulated via WithAttrs (e.g. component).
	bound []slog.Attr
}

type captureRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

func (s *captureSink) owner() *captureSink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *captureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *captureSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &captureSink{shared: s.owner(), bound: next}
}

func (s *captureSink) WithGroup(_ string) slog.Handler {
	return &captureSink{shared: s.owner(), bound: s.bound}
}

func (s *captureSink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	rec := captureRecord{level: r.Level, msg: r.Message, attrs: attrs}
	owner := s.owner()
	owner.mu.Lock()
	owner.records = append(owner.records, rec)
	owner.mu.Unlock()
	return nil
}

func (s *captureSink) all() []captureRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]captureRecord, len(s.records))
	copy(out, s.records)
	return out
}

// installCapture swaps a capturing handler into the process-wide log
// indirection for the duration of the test and returns the sink.
func installCapture(t *testing.T) *captureSink {
	t.Helper()
	sink := &captureSink{}
	log.SetTestHandler(t, sink)
	return sink
}

// onlyRecord returns the single captured record, failing if there is not
// exactly one.
func (s *captureSink) onlyRecord(t *testing.T) captureRecord {
	t.Helper()
	recs := s.all()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 log record, got %d: %+v", len(recs), recs)
	}
	return recs[0]
}

func (r captureRecord) attrString(t *testing.T, key string) string {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.attrs)
	}
	return v.String()
}

// readOnlyDirPath returns a path inside a 0500 (read-only) directory so that
// AtomicWrite fails at the temp-create phase. The directory is created under a
// t.TempDir so cleanup can remove it.
func readOnlyDirPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	if err := os.Mkdir(roDir, 0o500); err != nil {
		t.Fatalf("failed to create read-only dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })
	return filepath.Join(roDir, "hooks.json")
}

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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-start", "echo hello", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume xyz789", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("my-session:0.0", "on-resume", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("my-session:0.0", "on-resume", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("nonexistent:9.9", "on-resume", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		if err := store.Remove("my-session:0.0", "on-start", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "claude --resume def456", "cli"); err != nil {
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

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
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

	t.Run("old pane-ID entries cleaned on first run after upgrade", func(t *testing.T) {
		// After upgrading from pane-ID keys (%0, %3) to structural keys,
		// old entries won't match any live structural key and should be
		// removed by CleanStale. New structural-key entries are preserved.
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")

		// Write a hooks.json with a mix of old pane-ID and new structural keys
		content := `{"%0":{"on-resume":"claude --resume old1"},"%3":{"on-resume":"claude --resume old2"},"my-session:0.0":{"on-resume":"claude --resume new1"}}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := hooks.NewStore(filePath)

		// Live panes only contain structural keys
		removed, err := store.CleanStale([]string{"my-session:0.0"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		sort.Strings(removed)
		if removed[0] != "%0" {
			t.Errorf("removed[0] = %q, want %q", removed[0], "%0")
		}
		if removed[1] != "%3" {
			t.Errorf("removed[1] = %q, want %q", removed[1], "%3")
		}

		// Verify only the structural key entry remains
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
		if _, ok := h["%0"]; ok {
			t.Error("key %%0 should have been removed")
		}
		if _, ok := h["%3"]; ok {
			t.Error("key %%3 should have been removed")
		}
	})

	t.Run("handles mix of live and stale keys", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("other-session:0.0", "on-resume", "cmd-other0", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "cmd1", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("other-session:0.1", "on-resume", "cmd-other1", "cli"); err != nil {
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

func TestCleanStaleLogging(t *testing.T) {
	t.Run("emits per-entry DEBUG and one INFO summary with entries=N when removing N hooks", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "cmd1", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.2", "on-resume", "cmd2", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := installCapture(t)
		// Keep only 0.0 live -> 0.1 and 0.2 are stale (N=2).
		removed, err := store.CleanStale([]string{"my-session:0.0"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		recs := sink.all()

		var debugs []captureRecord
		var infos []captureRecord
		for _, r := range recs {
			if r.msg != "clean-stale" {
				t.Errorf("unexpected msg %q in %+v", r.msg, r)
				continue
			}
			if got := r.attrString(t, "component"); got != "hooks" {
				t.Errorf("component = %q, want %q", got, "hooks")
			}
			switch r.level {
			case slog.LevelDebug:
				debugs = append(debugs, r)
			case slog.LevelInfo:
				infos = append(infos, r)
			default:
				t.Errorf("unexpected level %v in %+v", r.level, r)
			}
		}

		if len(debugs) != 2 {
			t.Fatalf("got %d DEBUG clean-stale records, want 2: %+v", len(debugs), debugs)
		}
		debugKeys := make(map[string]bool, len(debugs))
		for _, r := range debugs {
			if got := r.attrString(t, "via"); got != "internal" {
				t.Errorf("DEBUG via = %q, want %q", got, "internal")
			}
			debugKeys[r.attrString(t, "hook_key")] = true
		}
		for _, want := range []string{"my-session:0.1", "my-session:0.2"} {
			if !debugKeys[want] {
				t.Errorf("missing DEBUG clean-stale for hook_key %q: %+v", want, debugs)
			}
		}

		if len(infos) != 1 {
			t.Fatalf("got %d INFO summary records, want 1: %+v", len(infos), infos)
		}
		summary := infos[0]
		if got := summary.attrString(t, "entries"); got != "2" {
			t.Errorf("summary entries = %q, want %q", got, "2")
		}
		if got := summary.attrString(t, "via"); got != "internal" {
			t.Errorf("summary via = %q, want %q", got, "internal")
		}
		if _, ok := summary.attrs["took"]; !ok {
			t.Errorf("summary missing took attr: %+v", summary.attrs)
		}
	})

	t.Run("omits entries_failed from the summary when no per-entry failures occur", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}
		if err := store.Set("my-session:0.1", "on-resume", "cmd1", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := installCapture(t)
		if _, err := store.CleanStale([]string{"my-session:0.0"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var summary captureRecord
		var found bool
		for _, r := range sink.all() {
			if r.level == slog.LevelInfo && r.msg == "clean-stale" {
				summary = r
				found = true
			}
		}
		if !found {
			t.Fatalf("no INFO clean-stale summary captured: %+v", sink.all())
		}
		if _, ok := summary.attrs["entries_failed"]; ok {
			t.Errorf("summary must omit entries_failed when no failures: %+v", summary.attrs)
		}
	})

	t.Run("emits WARN with write-failed-* error_class (not unexpected) when the batched Save fails", func(t *testing.T) {
		// Seed two entries on a writable path, then lock the parent dir 0500 so
		// the subsequent CleanStale Save fails at AtomicWrite's temp-create
		// phase. (readOnlyDirPath gives a path under an already-locked dir, which
		// would block the seed write — we need the seed to succeed first.)
		dir := t.TempDir()
		seeded := filepath.Join(dir, "hooks.json")
		if err := os.WriteFile(seeded, []byte(`{"a:0.0":{"on-resume":"x"},"b:0.0":{"on-resume":"y"}}`), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatalf("chmod parent dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		store := hooks.NewStore(seeded)
		sink := installCapture(t)

		// No live keys -> both entries stale -> Save attempted -> fails.
		_, err := store.CleanStale([]string{})
		if err == nil {
			t.Fatal("expected error from CleanStale on read-only dir, got nil")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		var warn captureRecord
		var found bool
		for _, r := range sink.all() {
			if r.level == slog.LevelWarn && r.msg == "clean-stale" {
				warn = r
				found = true
			}
		}
		if !found {
			t.Fatalf("no WARN clean-stale record captured: %+v", sink.all())
		}
		if got := warn.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := warn.attrString(t, "via"); got != "internal" {
			t.Errorf("via = %q, want %q", got, "internal")
		}
		if got := warn.attrString(t, "entries"); got != "2" {
			t.Errorf("entries = %q, want %q", got, "2")
		}
		if got := warn.attrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q (must be write-failed-*, not unexpected)", got, "write-failed-temp-create")
		}
		if _, ok := warn.attrs["took"]; !ok {
			t.Errorf("WARN missing took attr: %+v", warn.attrs)
		}
		errVal, ok := warn.attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", warn.attrs)
		}
		loggedErr, ok := errVal.Any().(error)
		if !ok {
			t.Fatalf("error attr is not an error value: %T", errVal.Any())
		}
		if !errors.Is(loggedErr, fileutil.ErrWriteTempCreate) {
			t.Errorf("logged error attr does not wrap the temp-create sentinel: %v", loggedErr)
		}
	})

	t.Run("emits no summary and skips Save when zero entries are removed", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "cmd0", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		sink := installCapture(t)
		removed, err := store.CleanStale([]string{"my-session:0.0"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed) != 0 {
			t.Fatalf("got %d removed, want 0", len(removed))
		}

		if recs := sink.all(); len(recs) != 0 {
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

func TestSaveAuditedLogging(t *testing.T) {
	t.Run("emits one INFO with op, entries=N and via on success", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		h["a:0.0"] = map[string]string{"on-resume": "x"}
		h["b:0.0"] = map[string]string{"on-resume": "y"}

		sink := installCapture(t)
		if err := store.SaveAudited(h, "modify", 2, "internal"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.msg, "modify")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "entries"); got != "2" {
			t.Errorf("entries = %q, want %q", got, "2")
		}
		if got := rec.attrString(t, "via"); got != "internal" {
			t.Errorf("via = %q, want %q", got, "internal")
		}

		// Side effect: the file was actually persisted.
		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("failed to reload: %v", err)
		}
		if len(loaded) != 2 {
			t.Errorf("got %d persisted keys, want 2", len(loaded))
		}
	})

	t.Run("emits one WARN with write-failed-* error_class on Save failure", func(t *testing.T) {
		path := readOnlyDirPath(t)
		store := hooks.NewStore(path)
		sink := installCapture(t)

		h := map[string]map[string]string{"a:0.0": {"on-resume": "x"}}
		err := store.SaveAudited(h, "modify", 1, "internal")
		if err == nil {
			t.Fatal("expected error from SaveAudited on read-only dir, got nil")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.level)
		}
		if rec.msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.msg, "modify")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "entries"); got != "1" {
			t.Errorf("entries = %q, want %q", got, "1")
		}
		if got := rec.attrString(t, "via"); got != "internal" {
			t.Errorf("via = %q, want %q", got, "internal")
		}
		if got := rec.attrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		errVal, ok := rec.attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", rec.attrs)
		}
		loggedErr, ok := errVal.Any().(error)
		if !ok {
			t.Fatalf("error attr is not an error value: %T", errVal.Any())
		}
		if !errors.Is(loggedErr, fileutil.ErrWriteTempCreate) {
			t.Errorf("logged error attr does not wrap the temp-create sentinel: %v", loggedErr)
		}
	})
}

func TestSetLogging(t *testing.T) {
	t.Run("emits INFO op=set with value and via=cli for a new hook key", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))
		sink := installCapture(t)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "set" {
			t.Errorf("msg = %q, want %q", rec.msg, "set")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if got := rec.attrString(t, "value"); got != "claude --resume abc123" {
			t.Errorf("value = %q, want %q", got, "claude --resume abc123")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})

	t.Run("emits INFO op=modify when the key exists with a different value", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}

		sink := installCapture(t)
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume xyz789", "cli"); err != nil {
			t.Fatalf("unexpected error on second set: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.msg, "modify")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if got := rec.attrString(t, "value"); got != "claude --resume xyz789" {
			t.Errorf("value = %q, want %q", got, "claude --resume xyz789")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})

	t.Run("emits DEBUG op=set-noop and skips Save when key+value already match", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "hooks.json")
		store := hooks.NewStore(filePath)

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		sink := installCapture(t)
		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on noop set: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelDebug {
			t.Errorf("level = %v, want DEBUG", rec.level)
		}
		if rec.msg != "set-noop" {
			t.Errorf("msg = %q, want %q", rec.msg, "set-noop")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
		if _, ok := rec.attrs["value"]; ok {
			t.Errorf("set-noop record should not carry a value attr: %+v", rec.attrs)
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
		path := readOnlyDirPath(t)
		store := hooks.NewStore(path)
		sink := installCapture(t)

		err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli")
		if err == nil {
			t.Fatal("expected error from Set on read-only dir, got nil")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.level)
		}
		if rec.msg != "set" {
			t.Errorf("msg = %q, want %q", rec.msg, "set")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		// The error attr must carry the wrapped error itself, so errors.Is on
		// the sentinel succeeds.
		errVal, ok := rec.attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", rec.attrs)
		}
		loggedErr, ok := errVal.Any().(error)
		if !ok {
			t.Fatalf("error attr is not an error value: %T", errVal.Any())
		}
		if !errors.Is(loggedErr, fileutil.ErrWriteTempCreate) {
			t.Errorf("logged error attr does not wrap the temp-create sentinel: %v", loggedErr)
		}
	})

	t.Run("does not log inside Save (set-noop proves Save is not the emitter)", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on first set: %v", err)
		}

		// Direct Save call must emit nothing — only Set/Remove are emitters.
		sink := installCapture(t)
		h, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if err := store.Save(h); err != nil {
			t.Fatalf("unexpected error on save: %v", err)
		}

		if recs := sink.all(); len(recs) != 0 {
			t.Errorf("Save emitted %d log records, want 0: %+v", len(recs), recs)
		}
	})
}

func TestRemoveLogging(t *testing.T) {
	t.Run("emits INFO op=rm without a value attr", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := installCapture(t)
		if err := store.Remove("my-session:0.0", "on-resume", "cli"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.msg, "rm")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "hook_key"); got != "my-session:0.0" {
			t.Errorf("hook_key = %q, want %q", got, "my-session:0.0")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
		if _, ok := rec.attrs["value"]; ok {
			t.Errorf("rm record should not carry a value attr: %+v", rec.attrs)
		}
	})

	t.Run("still emits INFO op=rm when removing an absent key", func(t *testing.T) {
		dir := t.TempDir()
		store := hooks.NewStore(filepath.Join(dir, "hooks.json"))

		if err := store.Set("my-session:0.0", "on-resume", "claude --resume abc123", "cli"); err != nil {
			t.Fatalf("unexpected error on set: %v", err)
		}

		sink := installCapture(t)
		if err := store.Remove("nonexistent:9.9", "on-resume", "cli"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.msg, "rm")
		}
		if got := rec.attrString(t, "hook_key"); got != "nonexistent:9.9" {
			t.Errorf("hook_key = %q, want %q", got, "nonexistent:9.9")
		}
	})

	t.Run("emits WARN with error_class=write-failed-temp-create when AtomicWrite fails on Remove", func(t *testing.T) {
		path := readOnlyDirPath(t)
		store := hooks.NewStore(path)
		sink := installCapture(t)

		err := store.Remove("my-session:0.0", "on-resume", "cli")
		if err == nil {
			t.Fatal("expected error from Remove on read-only dir, got nil")
		}
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("returned error not classified as temp-create: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.level)
		}
		if rec.msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.msg, "rm")
		}
		if got := rec.attrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.attrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		errVal, ok := rec.attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", rec.attrs)
		}
		loggedErr, ok := errVal.Any().(error)
		if !ok {
			t.Fatalf("error attr is not an error value: %T", errVal.Any())
		}
		if !errors.Is(loggedErr, fileutil.ErrWriteTempCreate) {
			t.Errorf("logged error attr does not wrap the temp-create sentinel: %v", loggedErr)
		}
	})
}
