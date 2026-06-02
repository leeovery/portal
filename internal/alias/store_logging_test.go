package alias_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/alias"
	"github.com/leeovery/portal/internal/log"
)

// captureSink is a slog.Handler that records every emitted record together with
// the attrs bound via WithAttrs (notably the component attr that log.For binds
// at the logger, not at each call site) so the alias store tests can assert on
// component=aliases and the per-call attrs faithfully.
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

func (r captureRecord) hasAttr(key string) bool {
	_, ok := r.attrs[key]
	return ok
}

// readOnlyDirAliasPath returns a path inside a 0500 (read-only) directory whose
// parent already exists, so that os.WriteFile (not os.MkdirAll) is the failing
// phase — the write-failed-write classification. The directory is created under
// a t.TempDir so cleanup can remove it.
func readOnlyDirAliasPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	if err := os.Mkdir(roDir, 0o500); err != nil {
		t.Fatalf("failed to create read-only dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })
	return filepath.Join(roDir, "aliases")
}

func TestSetAndSave(t *testing.T) {
	t.Run("emits INFO op=set with value and via=cli after persisting a new alias", func(t *testing.T) {
		dir := t.TempDir()
		store := alias.NewStore(filepath.Join(dir, "aliases"))
		sink := installCapture(t)

		if err := store.SetAndSave("p", "/code/portal", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "set" {
			t.Errorf("msg = %q, want %q", rec.msg, "set")
		}
		if got := rec.attrString(t, "op"); got != "set" {
			t.Errorf("op = %q, want %q", got, "set")
		}
		if got := rec.attrString(t, "component"); got != "aliases" {
			t.Errorf("component = %q, want %q", got, "aliases")
		}
		if got := rec.attrString(t, "alias"); got != "p" {
			t.Errorf("alias = %q, want %q", got, "p")
		}
		if got := rec.attrString(t, "value"); got != "/code/portal" {
			t.Errorf("value = %q, want %q", got, "/code/portal")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}

		// Side effect: the alias actually persisted.
		loaded, err := alias.NewStore(filepath.Join(dir, "aliases")).Load()
		if err != nil {
			t.Fatalf("reload failed: %v", err)
		}
		if loaded["p"] != "/code/portal" {
			t.Errorf("persisted value = %q, want %q", loaded["p"], "/code/portal")
		}
	})

	t.Run("emits INFO op=modify when the alias exists with a different path", func(t *testing.T) {
		dir := t.TempDir()
		store := alias.NewStore(filepath.Join(dir, "aliases"))
		store.Set("p", "/code/old")
		if err := store.Save(); err != nil {
			t.Fatalf("seed save failed: %v", err)
		}
		sink := installCapture(t)

		if err := store.SetAndSave("p", "/code/new", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.msg, "modify")
		}
		if got := rec.attrString(t, "op"); got != "modify" {
			t.Errorf("op = %q, want %q", got, "modify")
		}
		if got := rec.attrString(t, "alias"); got != "p" {
			t.Errorf("alias = %q, want %q", got, "p")
		}
		if got := rec.attrString(t, "value"); got != "/code/new" {
			t.Errorf("value = %q, want %q", got, "/code/new")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})

	t.Run("emits DEBUG op=set-noop and skips the persist when the alias path is unchanged", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "aliases")
		store := alias.NewStore(path)
		store.Set("p", "/code/portal")
		if err := store.Save(); err != nil {
			t.Fatalf("seed save failed: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat after seed failed: %v", err)
		}
		seedModTime := info.ModTime()

		sink := installCapture(t)

		if err := store.SetAndSave("p", "/code/portal", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelDebug {
			t.Errorf("level = %v, want DEBUG", rec.level)
		}
		if rec.msg != "set-noop" {
			t.Errorf("msg = %q, want %q", rec.msg, "set-noop")
		}
		if got := rec.attrString(t, "op"); got != "set-noop" {
			t.Errorf("op = %q, want %q", got, "set-noop")
		}
		if got := rec.attrString(t, "alias"); got != "p" {
			t.Errorf("alias = %q, want %q", got, "p")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}

		// The file must NOT have been rewritten.
		after, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat after set-noop failed: %v", err)
		}
		if !after.ModTime().Equal(seedModTime) {
			t.Errorf("file was rewritten on set-noop: modtime changed %v -> %v", seedModTime, after.ModTime())
		}
	})

	t.Run("emits WARN with write-failed-write error_class when persist fails", func(t *testing.T) {
		path := readOnlyDirAliasPath(t)
		store := alias.NewStore(path)
		sink := installCapture(t)

		err := store.SetAndSave("p", "/code/portal", "cli")
		if err == nil {
			t.Fatal("expected error from persist into read-only dir, got nil")
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.level)
		}
		if rec.msg != "set" {
			t.Errorf("msg = %q, want %q", rec.msg, "set")
		}
		if got := rec.attrString(t, "op"); got != "set" {
			t.Errorf("op = %q, want %q", got, "set")
		}
		if got := rec.attrString(t, "error_class"); got != "write-failed-write" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-write")
		}
		if !rec.hasAttr("error") {
			t.Errorf("WARN record missing error attr: %+v", rec.attrs)
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})
}

func TestDeleteAndSave(t *testing.T) {
	t.Run("emits INFO op=rm without a value attr for a successful delete", func(t *testing.T) {
		dir := t.TempDir()
		store := alias.NewStore(filepath.Join(dir, "aliases"))
		store.Set("p", "/code/portal")
		if err := store.Save(); err != nil {
			t.Fatalf("seed save failed: %v", err)
		}
		sink := installCapture(t)

		existed, err := store.DeleteAndSave("p", "cli")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !existed {
			t.Fatal("expected existed=true for a present alias")
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.level)
		}
		if rec.msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.msg, "rm")
		}
		if got := rec.attrString(t, "op"); got != "rm" {
			t.Errorf("op = %q, want %q", got, "rm")
		}
		if got := rec.attrString(t, "component"); got != "aliases" {
			t.Errorf("component = %q, want %q", got, "aliases")
		}
		if got := rec.attrString(t, "alias"); got != "p" {
			t.Errorf("alias = %q, want %q", got, "p")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
		if rec.hasAttr("value") {
			t.Errorf("rm record must not carry a value attr: %+v", rec.attrs)
		}
	})

	t.Run("emits nothing and returns existed=false when deleting an absent alias", func(t *testing.T) {
		dir := t.TempDir()
		store := alias.NewStore(filepath.Join(dir, "aliases"))
		sink := installCapture(t)

		existed, err := store.DeleteAndSave("missing", "cli")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if existed {
			t.Error("expected existed=false for an absent alias")
		}

		if recs := sink.all(); len(recs) != 0 {
			t.Errorf("expected no log records for absent-delete, got %d: %+v", len(recs), recs)
		}
	})

	t.Run("emits WARN with write-failed-write error_class when persist fails", func(t *testing.T) {
		path := readOnlyDirAliasPath(t)
		store := alias.NewStore(path)
		// Seed the in-memory map so Delete reports existed=true and Save runs.
		store.Set("p", "/code/portal")
		sink := installCapture(t)

		existed, err := store.DeleteAndSave("p", "cli")
		if err == nil {
			t.Fatal("expected error from persist into read-only dir, got nil")
		}
		if !existed {
			t.Error("expected existed=true (the entry was present in memory)")
		}

		rec := sink.onlyRecord(t)
		if rec.level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.level)
		}
		if rec.msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.msg, "rm")
		}
		if got := rec.attrString(t, "op"); got != "rm" {
			t.Errorf("op = %q, want %q", got, "rm")
		}
		if got := rec.attrString(t, "error_class"); got != "write-failed-write" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-write")
		}
		if !rec.hasAttr("error") {
			t.Errorf("WARN record missing error attr: %+v", rec.attrs)
		}
	})
}
