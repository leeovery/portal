package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

// installMigrateCapture swaps the shared logtest.Sink into the process-wide log
// indirection for the duration of the test and returns it. The migrateConfigFile
// tests assert on the owning component and the per-call attr values via the
// sink's shared accessors.
func installMigrateCapture(t *testing.T) *logtest.Sink {
	t.Helper()
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)
	return sink
}

// seedOldFile creates the old macOS-path file with the given filename under
// tmpDir and returns oldPath, newPath (the latter not yet created).
func seedOldFile(t *testing.T, tmpDir, filename, content string) (oldPath, newPath string) {
	t.Helper()
	oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("failed to create old dir: %v", err)
	}
	oldPath = filepath.Join(oldDir, filename)
	if err := os.WriteFile(oldPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write old file: %v", err)
	}
	newPath = filepath.Join(tmpDir, ".config", "portal", filename)
	return oldPath, newPath
}

func TestMigrateConfigFileLogging(t *testing.T) {
	t.Run("emits one INFO migrate via=migrate path=new under component hooks for hooks.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, newPath := seedOldFile(t, tmpDir, "hooks.json", `{}`)
		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "hooks")

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelInfo {
			t.Errorf("level = %v, want INFO", rec.Level)
		}
		if rec.Msg != "migrate" {
			t.Errorf("msg = %q, want %q", rec.Msg, "migrate")
		}
		if got := rec.AttrString(t, "op"); got != "migrate" {
			t.Errorf("op = %q, want %q", got, "migrate")
		}
		if got := rec.AttrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if got := rec.AttrString(t, "via"); got != "migrate" {
			t.Errorf("via = %q, want %q", got, "migrate")
		}
		if got := rec.AttrString(t, "path"); got != newPath {
			t.Errorf("path = %q, want %q", got, newPath)
		}
		// Whole-file move has no entry key (decision a): no hook_key attr.
		if rec.HasAttr("hook_key") {
			t.Errorf("migrate line must not carry a hook_key attr: %+v", rec.Attrs)
		}
	})

	t.Run("emits under the owning component for each in-scope file", func(t *testing.T) {
		cases := []struct {
			filename  string
			component string
		}{
			{"aliases", "aliases"},
			{"projects.json", "projects"},
		}
		for _, tc := range cases {
			t.Run(tc.filename, func(t *testing.T) {
				tmpDir := t.TempDir()
				oldPath, newPath := seedOldFile(t, tmpDir, tc.filename, "data")
				sink := installMigrateCapture(t)

				migrateConfigFile(oldPath, newPath, tc.component)

				rec := sink.OnlyRecord(t)
				if rec.Level != slog.LevelInfo {
					t.Errorf("level = %v, want INFO", rec.Level)
				}
				if rec.Msg != "migrate" {
					t.Errorf("msg = %q, want %q", rec.Msg, "migrate")
				}
				if got := rec.AttrString(t, "op"); got != "migrate" {
					t.Errorf("op = %q, want %q", got, "migrate")
				}
				if got := rec.AttrString(t, "component"); got != tc.component {
					t.Errorf("component = %q, want %q", got, tc.component)
				}
				if got := rec.AttrString(t, "via"); got != "migrate" {
					t.Errorf("via = %q, want %q", got, "migrate")
				}
				if got := rec.AttrString(t, "path"); got != newPath {
					t.Errorf("path = %q, want %q", got, newPath)
				}
			})
		}
	})

	t.Run("emits nothing when the old path does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath := filepath.Join(tmpDir, "nonexistent", "portal", "projects.json")
		newPath := filepath.Join(tmpDir, ".config", "portal", "projects.json")
		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "projects")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records for absent-old, got %d: %+v", len(recs), recs)
		}
	})

	t.Run("emits nothing when the new path already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, newPath := seedOldFile(t, tmpDir, "projects.json", "old")
		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
			t.Fatalf("failed to write new file: %v", err)
		}
		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "projects")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records when new path occupied, got %d: %+v", len(recs), recs)
		}
	})

	t.Run("emits nothing on the stat-error branch of the new path", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, _ := seedOldFile(t, tmpDir, "projects.json", "old")

		// Make the parent of newPath unreadable so os.Stat(newPath) returns a
		// permission error (not "not exist") — the stat-error early return.
		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		if err := os.Chmod(newDir, 0o000); err != nil {
			t.Fatalf("failed to chmod new dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(newDir, 0o755) })
		newPath := filepath.Join(newDir, "projects.json")

		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "projects")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records on stat-error branch, got %d: %+v", len(recs), recs)
		}
	})

	t.Run("emits one WARN with error_class=write-failed-rename when os.Rename fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, _ := seedOldFile(t, tmpDir, "projects.json", "data")

		// Create the target directory read+execute-only so MkdirAll succeeds
		// (dir already exists) but os.Rename into it fails.
		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		if err := os.Chmod(newDir, 0o555); err != nil {
			t.Fatalf("failed to chmod new dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(newDir, 0o755) })
		newPath := filepath.Join(newDir, "projects.json")

		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "projects")

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if rec.Msg != "migrate" {
			t.Errorf("msg = %q, want %q", rec.Msg, "migrate")
		}
		if got := rec.AttrString(t, "op"); got != "migrate" {
			t.Errorf("op = %q, want %q", got, "migrate")
		}
		if got := rec.AttrString(t, "component"); got != "projects" {
			t.Errorf("component = %q, want %q", got, "projects")
		}
		if got := rec.AttrString(t, "via"); got != "migrate" {
			t.Errorf("via = %q, want %q", got, "migrate")
		}
		if got := rec.AttrString(t, "path"); got != newPath {
			t.Errorf("path = %q, want %q", got, newPath)
		}
		if got := rec.AttrString(t, "error_class"); got != "write-failed-rename" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-rename")
		}
		if !rec.HasAttr("error") {
			t.Errorf("WARN record missing error attr: %+v", rec.Attrs)
		}

		// Old file should still exist after the failed rename.
		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist after failed rename: %v", err)
		}
	})

	t.Run("emits one WARN with error_class=write-failed-temp-create when MkdirAll fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, _ := seedOldFile(t, tmpDir, "projects.json", "data")

		// Place newPath's parent under a read-only directory so MkdirAll of the
		// (not-yet-existing) parent fails with permission denied.
		roDir := filepath.Join(tmpDir, "ro")
		if err := os.Mkdir(roDir, 0o555); err != nil {
			t.Fatalf("failed to create read-only dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
		newPath := filepath.Join(roDir, "portal", "projects.json")

		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "projects")

		rec := sink.OnlyRecord(t)
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if rec.Msg != "migrate" {
			t.Errorf("msg = %q, want %q", rec.Msg, "migrate")
		}
		if got := rec.AttrString(t, "op"); got != "migrate" {
			t.Errorf("op = %q, want %q", got, "migrate")
		}
		if got := rec.AttrString(t, "via"); got != "migrate" {
			t.Errorf("via = %q, want %q", got, "migrate")
		}
		if got := rec.AttrString(t, "path"); got != filepath.Dir(newPath) {
			t.Errorf("path = %q, want %q", got, filepath.Dir(newPath))
		}
		if got := rec.AttrString(t, "error_class"); got != "write-failed-temp-create" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-temp-create")
		}
		if !rec.HasAttr("error") {
			t.Errorf("WARN record missing error attr: %+v", rec.Attrs)
		}
	})

	t.Run("emits nothing and does not panic when component is empty (unmapped)", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, newPath := seedOldFile(t, tmpDir, "projects.json", "data")
		sink := installMigrateCapture(t)

		migrateConfigFile(oldPath, newPath, "")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records for empty component, got %d: %+v", len(recs), recs)
		}
		// The move itself must still have happened (best-effort migration runs
		// regardless of whether it can be logged).
		if _, err := os.Stat(newPath); err != nil {
			t.Errorf("file should still migrate when component is empty: %v", err)
		}
	})
}

func TestConfigFilePathThreadsComponent(t *testing.T) {
	t.Run("threads the hooks component through the filename->component mapping", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		xdgDir := filepath.Join(tmpDir, "custom-xdg")
		t.Setenv("XDG_CONFIG_HOME", xdgDir)

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "hooks.json")
		if err := os.WriteFile(oldPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		sink := installMigrateCapture(t)

		if _, err := configFilePath("TEST_THREADS_UNSET", "hooks.json"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.OnlyRecord(t)
		if got := rec.AttrString(t, "component"); got != "hooks" {
			t.Errorf("component = %q, want %q", got, "hooks")
		}
		if rec.Msg != "migrate" {
			t.Errorf("msg = %q, want %q", rec.Msg, "migrate")
		}
		if got := rec.AttrString(t, "op"); got != "migrate" {
			t.Errorf("op = %q, want %q", got, "migrate")
		}
	})
}
