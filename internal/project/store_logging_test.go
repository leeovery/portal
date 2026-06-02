package project_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/project"
)

// captureSink records every emitted record together with the attrs bound via
// WithAttrs (notably the component attr that log.For binds at the logger, not at
// each call site) so the project store tests can assert on component=projects
// and the per-call attrs faithfully. It mirrors the hooks store test sink.
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
	return filepath.Join(roDir, "projects.json")
}

func TestUpsertLogging(t *testing.T) {
	t.Run("emits INFO op=set with value=name and via=internal for a new project", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "projects.json"))
		sink := installCapture(t)

		if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
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
		if got := rec.attrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		// project attr = the identifying PATH (matches Remove/Rename/Upsert key).
		if got := rec.attrString(t, "project"); got != "/code/portal" {
			t.Errorf("project = %q, want %q", got, "/code/portal")
		}
		// value attr carries the name.
		if got := rec.attrString(t, "value"); got != "portal" {
			t.Errorf("value = %q, want %q", got, "portal")
		}
		if got := rec.attrString(t, "via"); got != "internal" {
			t.Errorf("via = %q, want %q", got, "internal")
		}
	})

	t.Run("emits INFO op=modify when Upsert targets an existing path", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "projects.json"))

		if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
			t.Fatalf("unexpected error on first upsert: %v", err)
		}

		sink := installCapture(t)
		if err := store.Upsert("/code/portal", "portal-renamed", "cli"); err != nil {
			t.Fatalf("unexpected error on second upsert: %v", err)
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
		if got := rec.attrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := rec.attrString(t, "project"); got != "/code/portal" {
			t.Errorf("project = %q, want %q", got, "/code/portal")
		}
		if got := rec.attrString(t, "value"); got != "portal-renamed" {
			t.Errorf("value = %q, want %q", got, "portal-renamed")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})

	t.Run("emits WARN with write-failed-* error_class when AtomicWrite fails on Upsert", func(t *testing.T) {
		path := readOnlyDirPath(t)
		store := project.NewStore(path)
		sink := installCapture(t)

		err := store.Upsert("/code/portal", "portal", "internal")
		if err == nil {
			t.Fatal("expected error from Upsert on read-only dir, got nil")
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
		if got := rec.attrString(t, "op"); got != "set" {
			t.Errorf("op = %q, want %q", got, "set")
		}
		if got := rec.attrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
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

func TestRenameLogging(t *testing.T) {
	t.Run("emits INFO op=modify via=cli for a Rename of a found path", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "projects.json"))

		if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
			t.Fatalf("unexpected error on upsert: %v", err)
		}

		sink := installCapture(t)
		if err := store.Rename("/code/portal", "portal-new", "cli"); err != nil {
			t.Fatalf("unexpected error on rename: %v", err)
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
		if got := rec.attrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := rec.attrString(t, "project"); got != "/code/portal" {
			t.Errorf("project = %q, want %q", got, "/code/portal")
		}
		if got := rec.attrString(t, "value"); got != "portal-new" {
			t.Errorf("value = %q, want %q", got, "portal-new")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})

	t.Run("emits nothing and does not Save when Rename targets an absent path", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
			t.Fatalf("unexpected error on upsert: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		sink := installCapture(t)
		if err := store.Rename("/code/absent", "anything", "cli"); err != nil {
			t.Fatalf("unexpected error on rename of absent path: %v", err)
		}

		if recs := sink.all(); len(recs) != 0 {
			t.Errorf("absent-path Rename emitted %d records, want 0: %+v", len(recs), recs)
		}

		infoAfter, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
			t.Error("file was modified on an absent-path Rename (Save should be skipped)")
		}
	})

	t.Run("emits WARN with write-failed-* error_class when AtomicWrite fails on Rename", func(t *testing.T) {
		// Seed a project on a writable path, then lock the parent dir 0500 so the
		// subsequent Rename Save fails at AtomicWrite's temp-create phase.
		dir := t.TempDir()
		seeded := filepath.Join(dir, "projects.json")
		if err := os.WriteFile(seeded, []byte(`{"projects":[{"path":"/code/portal","name":"portal","last_used":"2026-01-01T00:00:00Z"}]}`), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatalf("chmod parent dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		store := project.NewStore(seeded)
		sink := installCapture(t)

		err := store.Rename("/code/portal", "portal-new", "cli")
		if err == nil {
			t.Fatal("expected error from Rename on read-only dir, got nil")
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
		if got := rec.attrString(t, "op"); got != "modify" {
			t.Errorf("op = %q, want %q", got, "modify")
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

func TestRemoveLogging(t *testing.T) {
	t.Run("emits INFO op=rm via=cli without a value attr for Remove", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "projects.json"))

		if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
			t.Fatalf("unexpected error on upsert: %v", err)
		}

		sink := installCapture(t)
		if err := store.Remove("/code/portal", "cli"); err != nil {
			t.Fatalf("unexpected error on remove: %v", err)
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
		if got := rec.attrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := rec.attrString(t, "project"); got != "/code/portal" {
			t.Errorf("project = %q, want %q", got, "/code/portal")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
		if _, ok := rec.attrs["value"]; ok {
			t.Errorf("rm record should not carry a value attr: %+v", rec.attrs)
		}
	})

	t.Run("still emits INFO op=rm when removing an absent path", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "projects.json"))

		if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
			t.Fatalf("unexpected error on upsert: %v", err)
		}

		sink := installCapture(t)
		if err := store.Remove("/code/absent", "cli"); err != nil {
			t.Fatalf("unexpected error on remove of absent path: %v", err)
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
		if got := rec.attrString(t, "project"); got != "/code/absent" {
			t.Errorf("project = %q, want %q", got, "/code/absent")
		}
		if got := rec.attrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
	})

	t.Run("emits WARN with write-failed-* error_class when AtomicWrite fails on Remove", func(t *testing.T) {
		path := readOnlyDirPath(t)
		store := project.NewStore(path)
		sink := installCapture(t)

		err := store.Remove("/code/portal", "cli")
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
		if got := rec.attrString(t, "op"); got != "rm" {
			t.Errorf("op = %q, want %q", got, "rm")
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

func TestCleanStaleLogging(t *testing.T) {
	t.Run("emits per-entry DEBUG and one INFO summary for CleanStale removing N projects", func(t *testing.T) {
		dir := t.TempDir()
		// Two stale paths (under removed temp dirs) + one live path.
		stale1 := filepath.Join(t.TempDir(), "gone1")
		stale2 := filepath.Join(t.TempDir(), "gone2")
		live := t.TempDir()

		store := project.NewStore(filepath.Join(dir, "projects.json"))
		for _, p := range []struct{ path, name string }{
			{stale1, "gone1"},
			{stale2, "gone2"},
			{live, "live"},
		} {
			if err := store.Upsert(p.path, p.name, "internal"); err != nil {
				t.Fatalf("unexpected error on upsert: %v", err)
			}
		}

		sink := installCapture(t)
		removed, err := store.CleanStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(removed) != 2 {
			t.Fatalf("got %d removed, want 2", len(removed))
		}

		var debugs []captureRecord
		var infos []captureRecord
		for _, r := range sink.all() {
			if r.msg != "clean-stale" {
				t.Errorf("unexpected msg %q in %+v", r.msg, r)
				continue
			}
			if got := r.attrString(t, "op"); got != "clean-stale" {
				t.Errorf("op = %q, want %q", got, "clean-stale")
			}
			if got := r.attrString(t, "component"); got != "projects" {
				t.Errorf("component = %q, want %q", got, "projects")
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
		debugPaths := make(map[string]bool, len(debugs))
		for _, r := range debugs {
			if got := r.attrString(t, "via"); got != "internal" {
				t.Errorf("DEBUG via = %q, want %q", got, "internal")
			}
			debugPaths[r.attrString(t, "project")] = true
		}
		for _, want := range []string{stale1, stale2} {
			if !debugPaths[want] {
				t.Errorf("missing DEBUG clean-stale for project %q: %+v", want, debugs)
			}
		}

		if len(infos) != 1 {
			t.Fatalf("got %d INFO summary records, want 1: %+v", len(infos), infos)
		}
		summary := infos[0]
		if got := summary.attrString(t, "op"); got != "clean-stale" {
			t.Errorf("summary op = %q, want %q", got, "clean-stale")
		}
		if got := summary.attrString(t, "entries"); got != "2" {
			t.Errorf("summary entries = %q, want %q", got, "2")
		}
		if got := summary.attrString(t, "via"); got != "internal" {
			t.Errorf("summary via = %q, want %q", got, "internal")
		}
		if _, ok := summary.attrs["took"]; !ok {
			t.Errorf("summary missing took attr: %+v", summary.attrs)
		}
		if _, ok := summary.attrs["entries_failed"]; ok {
			t.Errorf("summary must omit entries_failed when no failures: %+v", summary.attrs)
		}
	})

	t.Run("emits no summary and skips Save when CleanStale removes zero projects", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		live := t.TempDir()

		store := project.NewStore(filePath)
		if err := store.Upsert(live, "live", "internal"); err != nil {
			t.Fatalf("unexpected error on upsert: %v", err)
		}

		infoBefore, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		sink := installCapture(t)
		removed, err := store.CleanStale()
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

	t.Run("emits WARN with write-failed-* error_class when the batched Save fails", func(t *testing.T) {
		// Seed one stale project on a writable path, then lock the parent dir 0500
		// so the CleanStale Save fails at AtomicWrite's temp-create phase.
		dir := t.TempDir()
		stale := filepath.Join(t.TempDir(), "gone")
		seeded := filepath.Join(dir, "projects.json")
		store := project.NewStore(seeded)
		if err := store.Upsert(stale, "gone", "internal"); err != nil {
			t.Fatalf("unexpected error on upsert: %v", err)
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatalf("chmod parent dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		sink := installCapture(t)
		_, err := store.CleanStale()
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
		if got := warn.attrString(t, "op"); got != "clean-stale" {
			t.Errorf("op = %q, want %q", got, "clean-stale")
		}
		if got := warn.attrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := warn.attrString(t, "via"); got != "internal" {
			t.Errorf("via = %q, want %q", got, "internal")
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
}

// TestSaveDoesNotLog proves Save is not an emitter — only Upsert/Rename/Remove/
// CleanStale are.
func TestSaveDoesNotLog(t *testing.T) {
	dir := t.TempDir()
	store := project.NewStore(filepath.Join(dir, "projects.json"))

	sink := installCapture(t)
	if err := store.Save([]project.Project{{Path: "/code/portal", Name: "portal"}}); err != nil {
		t.Fatalf("unexpected error on save: %v", err)
	}

	if recs := sink.all(); len(recs) != 0 {
		t.Errorf("Save emitted %d log records, want 0: %+v", len(recs), recs)
	}
}
