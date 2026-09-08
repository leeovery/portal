package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/xdg"
)

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

// denyRenameFixture stages a seeded old file whose destination directory is
// read+execute-only, so MkdirAll succeeds on the existing dir but the rename
// into it fails.
func denyRenameFixture(t *testing.T) (oldPath, newPath, component string) {
	t.Helper()
	tmpDir := t.TempDir()
	oldPath, _ = seedOldFile(t, tmpDir, "projects.json", "data")

	newDir := filepath.Join(tmpDir, ".config", "portal")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("failed to create new dir: %v", err)
	}
	if err := os.Chmod(newDir, 0o555); err != nil {
		t.Fatalf("failed to chmod new dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(newDir, 0o755) })

	return oldPath, filepath.Join(newDir, "projects.json"), "projects"
}

func TestMigrateConfigFileLogging(t *testing.T) {
	t.Run("emits one INFO migrate via=migrate path=new under component hooks for hooks.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, newPath := seedOldFile(t, tmpDir, "hooks.json", `{}`)
		sink := logtest.Install(t)

		migrateConfigFile(oldPath, newPath, "hooks")

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelInfo,
			Msg:       "migrate",
			Component: "hooks",
			Op:        "migrate",
			Via:       "migrate",
		})
		if got := rec.AttrString(t, "path"); got != newPath {
			t.Errorf("path = %q, want %q", got, newPath)
		}
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
				sink := logtest.Install(t)

				migrateConfigFile(oldPath, newPath, tc.component)

				rec := sink.Records().Only(t, "log record")
				logtest.AssertRecord(t, rec, logtest.RecordWant{
					Level:     slog.LevelInfo,
					Msg:       "migrate",
					Component: tc.component,
					Op:        "migrate",
					Via:       "migrate",
				})
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
		sink := logtest.Install(t)

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
		sink := logtest.Install(t)

		migrateConfigFile(oldPath, newPath, "projects")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records when new path occupied, got %d: %+v", len(recs), recs)
		}
	})

	t.Run("emits nothing on the stat-error branch of the new path", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, _ := seedOldFile(t, tmpDir, "projects.json", "old")

		// An unreadable parent makes os.Stat(newPath) return a permission error
		// rather than "not exist", hitting the stat-error early return.
		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		if err := os.Chmod(newDir, 0o000); err != nil {
			t.Fatalf("failed to chmod new dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(newDir, 0o755) })
		newPath := filepath.Join(newDir, "projects.json")

		sink := logtest.Install(t)

		migrateConfigFile(oldPath, newPath, "projects")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records on stat-error branch, got %d: %+v", len(recs), recs)
		}
	})

	t.Run("it wraps the migrate rename failure in the rename write-phase sentinel", func(t *testing.T) {
		oldPath, newPath, component := denyRenameFixture(t)

		sink := logtest.Install(t)

		migrateConfigFile(oldPath, newPath, component)

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "migrate",
			Component: "projects",
			Op:        "migrate",
			Via:       "migrate",
		})
		if got := rec.AttrString(t, "path"); got != newPath {
			t.Errorf("path = %q, want %q", got, newPath)
		}
		logtest.AssertWriteFailure(t, rec, "write-failed-rename", fileutil.ErrWriteRename)

		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist after failed rename: %v", err)
		}
	})

	t.Run("it wraps the migrate temp-create failure in the temp-create write-phase sentinel", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, _ := seedOldFile(t, tmpDir, "projects.json", "data")

		// A read-only grandparent makes MkdirAll of the missing parent fail with
		// permission denied.
		roDir := filepath.Join(tmpDir, "ro")
		if err := os.Mkdir(roDir, 0o555); err != nil {
			t.Fatalf("failed to create read-only dir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
		newPath := filepath.Join(roDir, "portal", "projects.json")

		sink := logtest.Install(t)

		migrateConfigFile(oldPath, newPath, "projects")

		rec := sink.Records().Only(t, "log record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{
			Level:     slog.LevelWarn,
			Msg:       "migrate",
			Component: "projects",
			Op:        "migrate",
			Via:       "migrate",
		})
		if got := rec.AttrString(t, "path"); got != filepath.Dir(newPath) {
			t.Errorf("path = %q, want %q", got, filepath.Dir(newPath))
		}
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
	})

	t.Run("it fails when the carried error does not wrap the classified phase sentinel", func(t *testing.T) {
		sink := logtest.Install(t)

		migrateConfigFile(denyRenameFixture(t))

		rec := sink.Records().Only(t, "log record")
		spy := &harnesstest.Recorder{}
		logtest.AssertWriteFailure(spy, rec, "write-failed-rename", fileutil.ErrWriteWrite)

		if len(spy.Errors) != 1 {
			t.Errorf("AssertWriteFailure reported %d failures against a mismatched sentinel, want 1: %v", len(spy.Errors), spy.Errors)
		}
	})

	t.Run("it renders the same error_class token as before", func(t *testing.T) {
		sink := logtest.Install(t)

		migrateConfigFile(denyRenameFixture(t))

		line := sink.Records().Only(t, "log record")
		if got := line.AttrString(t, "error_class"); got != "write-failed-rename" {
			t.Errorf("error_class = %q, want %q", got, "write-failed-rename")
		}
		if !strings.Contains(sink.Body(), " error_class=write-failed-rename") {
			t.Errorf("rendered line does not carry the unchanged error_class token: %q", sink.Body())
		}
	})

	t.Run("emits nothing and does not panic when component is empty (unmapped)", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldPath, newPath := seedOldFile(t, tmpDir, "projects.json", "data")
		sink := logtest.Install(t)

		migrateConfigFile(oldPath, newPath, "")

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records for empty component, got %d: %+v", len(recs), recs)
		}
		// The best-effort migration still runs even when it cannot be logged.
		if _, err := os.Stat(newPath); err != nil {
			t.Errorf("file should still migrate when component is empty: %v", err)
		}
	})
}

func TestConfigFilePathThreadsComponent(t *testing.T) {
	t.Run("threads the hooks component through the file identity", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		t.Setenv("PORTAL_HOOKS_FILE", "")
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

		sink := logtest.Install(t)

		if _, err := configFilePath(xdg.HooksFile); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		rec := sink.Records().Only(t, "log record")
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
