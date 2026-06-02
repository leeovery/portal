package project_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
)

// installCapture swaps the shared logtest.Sink into the process-wide log
// indirection for the duration of the test and returns it. The project store
// tests assert on component=projects and the per-call attr values via the
// sink's shared accessors.
func installCapture(t *testing.T) *logtest.Sink {
	t.Helper()
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)
	return sink
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.Level)
		}
		if rec.Msg != "set" {
			t.Errorf("msg = %q, want %q", rec.Msg, "set")
		}
		if got := rec.AttrString(t, "op"); got != "set" {
			t.Errorf("op = %q, want %q", got, "set")
		}
		if got := rec.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		// project attr = the project NAME (per the closed-vocabulary definition).
		if got := rec.AttrString(t, "project"); got != "portal" {
			t.Errorf("project = %q, want %q", got, "portal")
		}
		// path attr = the filesystem path.
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		// value attr carries the verbatim new value (the name being set).
		if got := rec.AttrString(t, "value"); got != "portal" {
			t.Errorf("value = %q, want %q", got, "portal")
		}
		if got := rec.AttrString(t, "via"); got != "internal" {
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.Level)
		}
		if rec.Msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.Msg, "modify")
		}
		if got := rec.AttrString(t, "op"); got != "modify" {
			t.Errorf("op = %q, want %q", got, "modify")
		}
		if got := rec.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		// project attr = the (new) project NAME; path attr = filesystem path.
		if got := rec.AttrString(t, "project"); got != "portal-renamed" {
			t.Errorf("project = %q, want %q", got, "portal-renamed")
		}
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		if got := rec.AttrString(t, "value"); got != "portal-renamed" {
			t.Errorf("value = %q, want %q", got, "portal-renamed")
		}
		if got := rec.AttrString(t, "via"); got != "cli" {
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if rec.Msg != "set" {
			t.Errorf("msg = %q, want %q", rec.Msg, "set")
		}
		if got := rec.AttrString(t, "op"); got != "set" {
			t.Errorf("op = %q, want %q", got, "set")
		}
		if got := rec.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := rec.AttrString(t, "project"); got != "portal" {
			t.Errorf("project = %q, want %q", got, "portal")
		}
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		if got := rec.AttrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		errVal, ok := rec.Attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", rec.Attrs)
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.Level)
		}
		if rec.Msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.Msg, "modify")
		}
		if got := rec.AttrString(t, "op"); got != "modify" {
			t.Errorf("op = %q, want %q", got, "modify")
		}
		if got := rec.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		// project attr = the (new) project NAME; path attr = filesystem path.
		if got := rec.AttrString(t, "project"); got != "portal-new" {
			t.Errorf("project = %q, want %q", got, "portal-new")
		}
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		if got := rec.AttrString(t, "value"); got != "portal-new" {
			t.Errorf("value = %q, want %q", got, "portal-new")
		}
		if got := rec.AttrString(t, "via"); got != "cli" {
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

		if recs := sink.Records(); len(recs) != 0 {
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if rec.Msg != "modify" {
			t.Errorf("msg = %q, want %q", rec.Msg, "modify")
		}
		if got := rec.AttrString(t, "op"); got != "modify" {
			t.Errorf("op = %q, want %q", got, "modify")
		}
		if got := rec.AttrString(t, "project"); got != "portal-new" {
			t.Errorf("project = %q, want %q", got, "portal-new")
		}
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		if got := rec.AttrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		errVal, ok := rec.Attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", rec.Attrs)
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.Level)
		}
		if rec.Msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.Msg, "rm")
		}
		if got := rec.AttrString(t, "op"); got != "rm" {
			t.Errorf("op = %q, want %q", got, "rm")
		}
		if got := rec.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		// project attr = the project NAME of the removed entry; path = filesystem path.
		if got := rec.AttrString(t, "project"); got != "portal" {
			t.Errorf("project = %q, want %q", got, "portal")
		}
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		if got := rec.AttrString(t, "via"); got != "cli" {
			t.Errorf("via = %q, want %q", got, "cli")
		}
		if _, ok := rec.Attrs["value"]; ok {
			t.Errorf("rm record should not carry a value attr: %+v", rec.Attrs)
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.Level)
		}
		if rec.Msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.Msg, "rm")
		}
		if got := rec.AttrString(t, "op"); got != "rm" {
			t.Errorf("op = %q, want %q", got, "rm")
		}
		// Absent path: the filesystem path is still logged under path; there is no
		// matching entry, so the project NAME is empty.
		if got := rec.AttrString(t, "path"); got != "/code/absent" {
			t.Errorf("path = %q, want %q", got, "/code/absent")
		}
		if got := rec.AttrString(t, "project"); got != "" {
			t.Errorf("project = %q, want empty (no matching entry)", got)
		}
		if got := rec.AttrString(t, "via"); got != "cli" {
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

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if rec.Msg != "rm" {
			t.Errorf("msg = %q, want %q", rec.Msg, "rm")
		}
		if got := rec.AttrString(t, "op"); got != "rm" {
			t.Errorf("op = %q, want %q", got, "rm")
		}
		if got := rec.AttrString(t, "path"); got != "/code/portal" {
			t.Errorf("path = %q, want %q", got, "/code/portal")
		}
		if got := rec.AttrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		errVal, ok := rec.Attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", rec.Attrs)
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

		var debugs []logtest.Record
		var infos []logtest.Record
		for _, r := range sink.Records() {
			if r.Msg != "clean-stale" {
				t.Errorf("unexpected msg %q in %+v", r.Msg, r)
				continue
			}
			if got := r.AttrString(t, "op"); got != "clean-stale" {
				t.Errorf("op = %q, want %q", got, "clean-stale")
			}
			if got := r.AttrString(t, "component"); got != "projects" {
				t.Errorf("component = %q, want %q", got, "projects")
			}
			switch r.Level {
			case slog.LevelDebug:
				debugs = append(debugs, r)
			case slog.LevelInfo:
				infos = append(infos, r)
			default:
				t.Errorf("unexpected level %v in %+v", r.Level, r)
			}
		}

		if len(debugs) != 2 {
			t.Fatalf("got %d DEBUG clean-stale records, want 2: %+v", len(debugs), debugs)
		}
		// project attr = the project NAME; path attr = the filesystem path.
		debugNames := make(map[string]bool, len(debugs))
		debugPaths := make(map[string]bool, len(debugs))
		for _, r := range debugs {
			if got := r.AttrString(t, "via"); got != "internal" {
				t.Errorf("DEBUG via = %q, want %q", got, "internal")
			}
			debugNames[r.AttrString(t, "project")] = true
			debugPaths[r.AttrString(t, "path")] = true
		}
		for _, want := range []string{"gone1", "gone2"} {
			if !debugNames[want] {
				t.Errorf("missing DEBUG clean-stale for project name %q: %+v", want, debugs)
			}
		}
		for _, want := range []string{stale1, stale2} {
			if !debugPaths[want] {
				t.Errorf("missing DEBUG clean-stale for path %q: %+v", want, debugs)
			}
		}

		if len(infos) != 1 {
			t.Fatalf("got %d INFO summary records, want 1: %+v", len(infos), infos)
		}
		summary := infos[0]
		if got := summary.AttrString(t, "op"); got != "clean-stale" {
			t.Errorf("summary op = %q, want %q", got, "clean-stale")
		}
		if got := summary.AttrString(t, "entries"); got != "2" {
			t.Errorf("summary entries = %q, want %q", got, "2")
		}
		if got := summary.AttrString(t, "via"); got != "internal" {
			t.Errorf("summary via = %q, want %q", got, "internal")
		}
		if _, ok := summary.Attrs["took"]; !ok {
			t.Errorf("summary missing took attr: %+v", summary.Attrs)
		}
		if _, ok := summary.Attrs["entries_failed"]; ok {
			t.Errorf("summary must omit entries_failed when no failures: %+v", summary.Attrs)
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

		var warn logtest.Record
		var found bool
		for _, r := range sink.Records() {
			if r.Level == slog.LevelWarn && r.Msg == "clean-stale" {
				warn = r
				found = true
			}
		}
		if !found {
			t.Fatalf("no WARN clean-stale record captured: %+v", sink.Records())
		}
		if got := warn.AttrString(t, "op"); got != "clean-stale" {
			t.Errorf("op = %q, want %q", got, "clean-stale")
		}
		if got := warn.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := warn.AttrString(t, "via"); got != "internal" {
			t.Errorf("via = %q, want %q", got, "internal")
		}
		if got := warn.AttrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q (must be write-failed-*, not unexpected)", got, "write-failed-temp-create")
		}
		if _, ok := warn.Attrs["took"]; !ok {
			t.Errorf("WARN missing took attr: %+v", warn.Attrs)
		}
		errVal, ok := warn.Attrs["error"]
		if !ok {
			t.Fatalf("WARN record missing error attr: %+v", warn.Attrs)
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

	if recs := sink.Records(); len(recs) != 0 {
		t.Errorf("Save emitted %d log records, want 0: %+v", len(recs), recs)
	}
}
