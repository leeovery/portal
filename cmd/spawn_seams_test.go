package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
)

type fakeTerminalDetector struct {
	id spawn.Identity
}

func (f fakeTerminalDetector) Detect() spawn.Identity {
	return f.id
}

func isolateTerminalsFile(t *testing.T) {
	t.Helper()
	t.Setenv("PORTAL_TERMINALS_FILE", filepath.Join(t.TempDir(), "terminals.json"))
}

func TestBuildProductionSpawnSeams(t *testing.T) {
	isolateTerminalsFile(t)
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())
	sink := logtest.Install(t)

	fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
	seams := buildProductionSpawnSeams(context.Background(), fake)

	t.Run("Exists asks the backend, and reads a session it can't find as gone", func(t *testing.T) {
		if !seams.Exists("api-x7Kd9a") {
			t.Error("Exists(api-x7Kd9a) = false for a live session")
		}
		if seams.Exists("gone-c3d4") {
			t.Error("Exists(gone-c3d4) = true for a session the backend lacks")
		}
	})

	t.Run("Ack is the state directory's spawn-ack channel", func(t *testing.T) {
		if err := seams.Ack.(spawn.AckWriter).Write("b1", "t1"); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if got, err := seams.Ack.Collect("b1"); err != nil || len(got) != 1 {
			t.Errorf("Collect(b1) = %v, %v; want the one ack", got, err)
		}
		if _, err := os.Stat(filepath.Join(os.Getenv("PORTAL_STATE_DIR"), "spawn-acks", "b1-t1")); err != nil {
			t.Errorf("the ack is not in the state directory's spawn-acks: %v", err)
		}
	})

	t.Run("Logger is the spawn-component logger", func(t *testing.T) {
		seams.Logger.Info("seam probe")
		rec := sink.Records().Matching("spawn", "seam probe").Only(t, "log record")
		if got := rec.AttrString(t, "component"); got != "spawn" {
			t.Errorf("Logger component attr = %q, want spawn", got)
		}
	})

	t.Run("Detector, Resolve, Exe, Getenv are wired", func(t *testing.T) {
		if seams.Detector == nil || seams.Resolve == nil || seams.Exe == nil || seams.Getenv == nil {
			t.Fatal("a seam is nil")
		}
		if got, want := seams.Getenv("PATH"), os.Getenv("PATH"); got != want {
			t.Errorf("Getenv(PATH) = %q, want os.Getenv value %q", got, want)
		}
	})

	t.Run("a backend with no process-table search finds no raw client, so detection refuses", func(t *testing.T) {
		fake.SetCurrent("api-x7Kd9a")
		fake.SetLeaderEnv("api-x7Kd9a", backend.ClientEnv{{Name: "DISPLAY", Unset: true}})
		t.Cleanup(func() { fake.SetCurrent("") })

		if id := seams.Detector.Detect(); !id.IsNull() {
			t.Errorf("Detect = %+v, want NULL", id)
		}
		rec := sink.Records().Matching("spawn", "detection resolved no host-local terminal").Only(t, "detection record")
		want := `inside-session raw client walk: no one terminal leads the session: 0 zmx attach processes on session "api-x7Kd9a", want 1`
		if detail := rec.AttrString(t, "detail"); detail != want {
			t.Errorf("detail = %q, want no raw client found", detail)
		}
	})
}
