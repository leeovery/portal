package project_test

import (
	"errors"
	"log/slog"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
)

func TestMutationLandsThoughTheDirectorySyncFails(t *testing.T) {
	store := project.NewStore(filepath.Join(t.TempDir(), "projects.json"))
	fileutil.FailDirSyncForTest(t, syscall.EIO)
	sink := logtest.Install(t)

	if err := store.Upsert("/code/portal", "portal", "internal"); err != nil {
		t.Fatalf("Upsert = %v, want the landed write to succeed", err)
	}

	projects, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "portal" {
		t.Errorf("projects = %+v, want portal", projects)
	}
	sink.Records().Matching("projects", "set").AtExactLevel(slog.LevelInfo).Only(t, "the mutation's own record")
	rec := sink.Records().Matching("projects", "unsynced").Only(t, "the unsynced warning")
	if rec.Level != slog.LevelWarn {
		t.Errorf("level = %v, want WARN", rec.Level)
	}
	if got := rec.AttrString(t, "error_class"); got != "write-failed-fsync" {
		t.Errorf("error_class = %q, want write-failed-fsync", got)
	}
	if err := rec.ErrorAttr(t, "error"); !errors.Is(err, syscall.EIO) {
		t.Errorf("error = %v, want the sync's EIO", err)
	}
}
