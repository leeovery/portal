package state_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

var running = state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}

func TestLatch(t *testing.T) {
	t.Run("a latch whose directory sync fails has landed, and warns under the bootstrap's component", func(t *testing.T) {
		dir := t.TempDir()
		fileutil.FailDirSyncForTest(t, syscall.EIO)
		sink := logtest.Install(t)

		if err := state.WriteLatch(dir, running); err != nil {
			t.Fatalf("WriteLatch = %v, want the landed write to succeed", err)
		}

		if !state.LatchCurrent(dir, running) {
			t.Error("LatchCurrent = false, want the latch landed")
		}
		rec := sink.Records().Matching("bootstrap", "latch unsynced").Only(t, "the unsynced warning")
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		logtest.AssertWriteFailure(t, rec, "write-failed-fsync", fileutil.ErrWriteFsync)
	})

	t.Run("a latch written by this boot and version is current", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WriteLatch(dir, running); err != nil {
			t.Fatalf("WriteLatch: %v", err)
		}

		if !state.LatchCurrent(dir, running) {
			t.Error("LatchCurrent = false, want true")
		}
		if got, err := state.ReadLatch(dir); err != nil || got != running {
			t.Errorf("ReadLatch = %+v, %v; want %+v", got, err, running)
		}
	})

	for name, now := range map[string]state.Latch{
		"another boot id":                   {BootID: "F58F3A1E-8799-43F8-9C20-A52B7239690D", Version: running.Version},
		"another version":                   {BootID: running.BootID, Version: "0.13.1"},
		"a dev build where a release wrote": {BootID: running.BootID, Version: "dev"},
	} {
		t.Run("a latch from "+name+" is not current", func(t *testing.T) {
			dir := t.TempDir()
			if err := state.WriteLatch(dir, running); err != nil {
				t.Fatal(err)
			}

			if state.LatchCurrent(dir, now) {
				t.Error("LatchCurrent = true, want false")
			}
		})
	}

	t.Run("a dev build's latch is current to that build and to no rebuild of its tree", func(t *testing.T) {
		const built = "dev-754e31b498e0-modified-20261103T231632.195584591Z"
		dir := t.TempDir()
		if err := state.WriteLatch(dir, state.Latch{BootID: running.BootID, Version: built}); err != nil {
			t.Fatal(err)
		}

		if !state.LatchCurrent(dir, state.Latch{BootID: running.BootID, Version: built}) {
			t.Error("LatchCurrent = false for the build that wrote it, want true")
		}
		for name, rebuilt := range map[string]string{
			"the same modified tree, written later": "dev-754e31b498e0-modified-20261103T231632.195584592Z",
			"its revision committed":                "dev-754e31b498e0-20261103T231632.195584591Z",
			"another revision":                      "dev-0a1b2c3d4e5f-modified-20261103T231632.195584591Z",
		} {
			if state.LatchCurrent(dir, state.Latch{BootID: running.BootID, Version: rebuilt}) {
				t.Errorf("LatchCurrent = true for a rebuild from %s (%s), want false", name, rebuilt)
			}
		}
	})

	t.Run("a latch an unidentified build wrote is not current to the same build", func(t *testing.T) {
		dir := t.TempDir()
		unidentified := state.Latch{BootID: running.BootID, Version: devversion.Unidentified}
		if err := state.WriteLatch(dir, unidentified); err != nil {
			t.Fatal(err)
		}

		if state.LatchCurrent(dir, unidentified) {
			t.Error("LatchCurrent = true, want false: the version names no one build")
		}
	})

	t.Run("no latch is not current", func(t *testing.T) {
		dir := t.TempDir()

		if state.LatchCurrent(dir, running) {
			t.Error("LatchCurrent = true, want false")
		}
		if _, err := state.ReadLatch(dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadLatch = %v, want ErrNotExist", err)
		}
	})

	for name, body := range map[string]string{
		"corrupt":                      `{"boot_id":`,
		"not an object":                `"bootstrapped"`,
		"missing its version":          `{"boot_id":"` + running.BootID + `"}`,
		"empty, matching an empty now": `{}`,
	} {
		t.Run("a latch "+name+" is not current", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(state.Bootstrapped(dir), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			if state.LatchCurrent(dir, running) || state.LatchCurrent(dir, state.Latch{}) {
				t.Error("LatchCurrent = true, want false")
			}
		})
	}

	t.Run("an unreadable latch is not current", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(state.Bootstrapped(dir), 0o700); err != nil {
			t.Fatal(err)
		}

		if state.LatchCurrent(dir, running) {
			t.Error("LatchCurrent = true, want false")
		}
	})

	t.Run("it holds the boot id and version as JSON, 0600, in a directory made 0700", func(t *testing.T) {
		withUmask(t, 0)
		dir := filepath.Join(t.TempDir(), "state")

		if err := state.WriteLatch(dir, running); err != nil {
			t.Fatalf("WriteLatch: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "bootstrapped"))
		if err != nil {
			t.Fatalf("read bootstrapped: %v", err)
		}
		var raw map[string]string
		if err := json.Unmarshal(data, &raw); err != nil || raw["boot_id"] != running.BootID || raw["version"] != running.Version || len(raw) != 2 {
			t.Errorf("bootstrapped = %s, %v; want boot_id and version alone", data, err)
		}
		assertMode(t, dir, 0o700)
		assertMode(t, state.Bootstrapped(dir), 0o600)
	})

	t.Run("a rewrite replaces the file whole, renamed over", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WriteLatch(dir, state.Latch{BootID: "old", Version: "0.12.9"}); err != nil {
			t.Fatal(err)
		}
		before := inode(t, state.Bootstrapped(dir))

		if err := state.WriteLatch(dir, running); err != nil {
			t.Fatal(err)
		}

		if inode(t, state.Bootstrapped(dir)) == before {
			t.Error("bootstrapped kept its inode: written in place, not renamed over")
		}
		if !state.LatchCurrent(dir, running) {
			t.Error("LatchCurrent = false after the rewrite, want true")
		}
	})

	for name, latch := range map[string]state.Latch{
		"no boot id": {Version: running.Version},
		"no version": {BootID: running.BootID},
	} {
		t.Run("a latch with "+name+" is refused, writing nothing", func(t *testing.T) {
			dir := t.TempDir()

			if err := state.WriteLatch(dir, latch); !errors.Is(err, state.ErrLatchIncomplete) {
				t.Errorf("WriteLatch = %v, want ErrLatchIncomplete", err)
			}
			assertAbsent(t, state.Bootstrapped(dir))
		})
	}
}

func inode(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Sys().(*syscall.Stat_t).Ino
}
