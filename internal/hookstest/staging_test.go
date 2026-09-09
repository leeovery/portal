package hookstest_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

func TestStageStore(t *testing.T) {
	t.Run("it stages a hooks.json with its sidecar by default", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: `{"tok01":{"on-resume":"npm start"}}`})

		if _, err := os.Stat(hookstest.SidecarPath(path)); err != nil {
			t.Fatalf("stat sidecar: %v", err)
		}

		sink := logtest.Install(t)
		h, err := store.Load(hooks.ViaDoctor)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if h["tok01"]["on-resume"] != "npm start" {
			t.Errorf("entries did not come back: %v", h)
		}
		if got := hookstest.UnlockedRecords(t, sink); len(got) != 0 {
			t.Errorf("read degraded despite a staged sidecar: %+v", got)
		}
	})

	t.Run("it stages no sidecar when the fixture asks for the absence", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed:          `{"tok01":{"on-resume":"npm start"}}`,
			SidecarAbsent: true,
		})

		if _, err := os.Stat(hookstest.SidecarPath(path)); !os.IsNotExist(err) {
			t.Fatalf("stat sidecar = %v, want it absent", err)
		}

		sink := logtest.Install(t)
		if _, err := store.Load(hooks.ViaDoctor); err != nil {
			t.Fatalf("Load: %v", err)
		}
		hookstest.AssertDegradedRead(t, sink, "doctor")
	})

	t.Run("it creates the sidecar before denying writes to the directory", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed:         `{"tok01":{"on-resume":"npm start"}}`,
			WritesDenied: true,
		})

		if _, err := os.Stat(hookstest.SidecarPath(path)); err != nil {
			t.Fatalf("stat sidecar: %v", err)
		}

		err := store.Set("tok02", "on-resume", "ls", hooks.ViaCLI)
		if err == nil {
			t.Fatal("Set succeeded under a directory that permits no file creation")
		}
		// The mutation took its lock and read cleanly, so it failed at the
		// write rather than earlier at the sidecar's own open.
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("Set failed with %v, want a temp-create write failure", err)
		}
	})

	t.Run("it stages a directory at the hooks.json path so every read fails", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat staged path: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("staged path %s is not a directory", path)
		}
		if _, err := store.Load(hooks.ViaCLI); err == nil {
			t.Error("Load succeeded against a directory — the fixture is readable")
		}
	})

	t.Run("it seeds the entries a caller names", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{
			"tok01": "claude --resume abc",
			"tok02": "npm start",
		}})

		h, err := store.Load(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(h) != 2 {
			t.Fatalf("got %d entries, want 2: %v", len(h), h)
		}
		if got := h["tok01"]["on-resume"]; got != "claude --resume abc" {
			t.Errorf("tok01 on-resume = %q, want %q", got, "claude --resume abc")
		}
		if got := h["tok02"]["on-resume"]; got != "npm start" {
			t.Errorf("tok02 on-resume = %q, want %q", got, "npm start")
		}
	})

	t.Run("it stages into a named directory that does not exist yet", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "named-config")
		_, path := hookstest.StageStore(t, hookstest.Staging{
			Dir:  dir,
			Seed: `{"tok01":{"on-resume":"npm start"}}`,
		})

		if want := hookstest.HooksPath(t, dir); path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat staged hooks.json: %v", err)
		}
	})
}

func TestStageStoreBody(t *testing.T) {
	t.Run("it stages a multi-event hooks body", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Body: map[string]map[string]string{
			"tok01": {"on-resume": "npm start", "on-close": "npm stop"},
		}})

		var staged map[string]map[string]string
		if err := json.Unmarshal(hookstest.HooksFileBytes(t, path), &staged); err != nil {
			t.Fatalf("unmarshal staged hooks.json: %v", err)
		}
		if got, want := staged["tok01"]["on-close"], "npm stop"; got != want {
			t.Errorf("on-close = %q, want %q — the body's second event was not staged", got, want)
		}

		h, err := store.Load(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got, want := h["tok01"]["on-resume"], "npm start"; got != want {
			t.Errorf("on-resume = %q, want %q", got, want)
		}
	})

	t.Run("it stages an empty body as an empty hooks.json rather than no file", func(t *testing.T) {
		_, path := hookstest.StageStore(t, hookstest.Staging{Body: map[string]map[string]string{}})

		if got := hookstest.HooksFileBytes(t, path); string(got) != "{}" {
			t.Errorf("staged bytes = %q, want %q", got, "{}")
		}
	})
}

func TestHooksPath(t *testing.T) {
	t.Run("it returns the hooks path without staging a file", func(t *testing.T) {
		dir := t.TempDir()

		path := hookstest.HooksPath(t, dir)

		if want := filepath.Join(dir, "hooks.json"); path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stat = %v, want the path unstaged", err)
		}
		if _, err := os.Stat(hookstest.SidecarPath(path)); !os.IsNotExist(err) {
			t.Errorf("stat sidecar = %v, want it unstaged", err)
		}
	})

	t.Run("it names the same path the stager stages", func(t *testing.T) {
		dir := t.TempDir()

		_, staged := hookstest.StageStore(t, hookstest.Staging{Dir: dir, Seed: `{}`})

		if got := hookstest.HooksPath(t, dir); got != staged {
			t.Errorf("HooksPath = %q, StageStore staged %q", got, staged)
		}
	})
}

func TestSidecarPath(t *testing.T) {
	t.Run("it names the lock file beside the hooks.json", func(t *testing.T) {
		path := hookstest.HooksPath(t, t.TempDir())

		if got, want := hookstest.SidecarPath(path), path+".lock"; got != want {
			t.Errorf("SidecarPath = %q, want %q", got, want)
		}
	})
}
