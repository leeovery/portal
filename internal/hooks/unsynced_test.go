package hooks_test

import (
	"errors"
	"log/slog"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

func TestMutationLandsThoughTheDirectorySyncFails(t *testing.T) {
	store, _ := hookstest.StageStore(t, hookstest.Staging{})
	fileutil.FailDirSyncForTest(t, syscall.EIO)
	sink := logtest.Install(t)

	if err := store.Set(hookstest.LiveSeedA, hooks.EventOnResume, hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err != nil {
		t.Fatalf("Set = %v, want the landed write to succeed", err)
	}

	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatal(err)
	}
	if got := held[hookstest.LiveSeedA][hooks.EventOnResume.String()].Command; got != "npm start" {
		t.Errorf("hook = %q, want the one set", got)
	}
	sink.Records().Matching("hooks", "set").AtExactLevel(slog.LevelInfo).Only(t, "the mutation's own record")
	rec := sink.Records().Matching("hooks", "unsynced").Only(t, "the unsynced warning")
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
