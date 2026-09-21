package hooks_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// inodeOf identifies the file behind a path, so a test can tell an in-place
// rewrite from the rename AtomicWrite performs.
func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat of %s is not a *syscall.Stat_t: %T", path, info.Sys())
	}
	return uint64(st.Ino)
}

func TestMutationLockSidecar(t *testing.T) {
	t.Run("it creates the sidecar beside hooks.json on the first mutation", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if _, err := os.Stat(path); err != nil {
			t.Fatalf("hooks.json not written: %v", err)
		}
		if _, err := os.Stat(path + ".lock"); err != nil {
			t.Fatalf("sidecar not created beside hooks.json: %v", err)
		}
	})

	t.Run("it keeps the sidecar beside an overridden hooks path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "elsewhere", "custom-hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if _, err := os.Stat(path + ".lock"); err != nil {
			t.Fatalf("sidecar not beside the overridden path: %v", err)
		}
	})

	t.Run("it locks the sidecar rather than hooks.json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}
		sidecarInode := inodeOf(t, path+".lock")
		targetInode := inodeOf(t, path)

		for i := 1; i <= 3; i++ {
			if err := store.Set(fmt.Sprintf("k%d", i), "on-resume", hooks.Registration{Command: "cmd"}, hooks.ViaCLI); err != nil {
				t.Fatalf("Set %d: %v", i, err)
			}
			next := inodeOf(t, path)
			if next == targetInode {
				t.Fatalf("hooks.json inode unchanged across mutation %d — the rename did not happen", i)
			}
			targetInode = next
			if got := inodeOf(t, path+".lock"); got != sidecarInode {
				t.Fatalf("sidecar inode changed on mutation %d: %d, want %d", i, got, sidecarInode)
			}
		}
	})

	t.Run("it never unlinks the sidecar", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if _, err := os.Stat(path + ".lock"); err != nil {
			t.Fatalf("sidecar gone after a mutation: %v", err)
		}

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("no-op Set: %v", err)
		}
		if _, err := os.Stat(path + ".lock"); err != nil {
			t.Fatalf("sidecar gone after a no-op mutation: %v", err)
		}

		failing, failingPath := hookstest.StageStore(t, hookstest.Staging{WritesDenied: true})
		if err := failing.Set("k1", "on-resume", hooks.Registration{Command: "cmd1"}, hooks.ViaCLI); err == nil {
			t.Fatal("expected the read-only fixture to fail the save")
		}
		if _, err := os.Stat(failingPath + ".lock"); err != nil {
			t.Fatalf("sidecar gone after a failed mutation: %v", err)
		}
	})

	t.Run("it creates the config directory before acquiring", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent", "nested", "hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set on a fresh config directory: %v", err)
		}

		if _, err := os.Stat(path); err != nil {
			t.Fatalf("hooks.json not written: %v", err)
		}
		if _, err := os.Stat(path + ".lock"); err != nil {
			t.Fatalf("sidecar not created: %v", err)
		}
	})
}

func TestMutationLockExclusion(t *testing.T) {
	t.Run("it loses no entry under concurrent writers", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")

		const writers = 20
		errs := make([]error, writers)
		var wg sync.WaitGroup
		for i := range writers {
			wg.Go(func() {
				errs[i] = hooks.NewStore(path).Set(fmt.Sprintf("k%02d", i), "on-resume", hooks.Registration{Command: "cmd"}, hooks.ViaCLI)
			})
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("writer %d: %v", i, err)
			}
		}

		loaded, err := hooks.NewStore(path).Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		for i := range writers {
			if _, ok := loaded[fmt.Sprintf("k%02d", i)]; !ok {
				t.Errorf("key k%02d lost", i)
			}
		}
		if len(loaded) != writers {
			t.Fatalf("got %d entries, want %d", len(loaded), writers)
		}
	})

	t.Run("it loads only after a held lock is released", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		release := hookstest.HoldHooksSidecar(t, path)

		done := make(chan error, 1)
		go func() {
			done <- hooks.NewStore(path).Set("k2", "on-resume", hooks.Registration{Command: "cmd2"}, hooks.ViaCLI)
		}()

		time.Sleep(100 * time.Millisecond)
		if err := os.WriteFile(path, []byte(`{"k1":{"on-resume":"cmd1"}}`), 0o600); err != nil {
			t.Fatalf("seed under the hold: %v", err)
		}
		release()

		if err := <-done; err != nil {
			t.Fatalf("Set blocked on the sidecar: %v", err)
		}

		loaded, err := hooks.NewStore(path).Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if loaded["k1"]["on-resume"].Command != "cmd1" {
			t.Errorf("k1 lost — the blocked mutation loaded before the release: %v", loaded)
		}
		if loaded["k2"]["on-resume"].Command != "cmd2" {
			t.Errorf("k2 missing: %v", loaded)
		}
	})
}

func TestMutationLockRelease(t *testing.T) {
	t.Run("it releases the lock on the set-noop arm", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("no-op Set: %v", err)
		}
		hookstest.AssertSidecarFree(t, path)

		if err := store.Set("k1", "on-resume", hooks.Registration{Command: "cmd1"}, hooks.ViaCLI); err != nil {
			t.Fatalf("following mutation: %v", err)
		}
	})

	t.Run("it releases the lock when it removes nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}
		removed, err := store.Remove("absent", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if removed {
			t.Fatal("Remove reported a removal for an absent key")
		}
		hookstest.AssertSidecarFree(t, path)

		if err := store.Set("k1", "on-resume", hooks.Registration{Command: "cmd1"}, hooks.ViaCLI); err != nil {
			t.Fatalf("following mutation: %v", err)
		}
	})

	t.Run("it releases the lock when the save fails", func(t *testing.T) {
		failing, failingPath := hookstest.StageStore(t, hookstest.Staging{WritesDenied: true})

		if err := failing.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err == nil {
			t.Fatal("expected the read-only fixture to fail the save")
		}
		hookstest.AssertSidecarFree(t, failingPath)

		writable := filepath.Join(t.TempDir(), "hooks.json")
		if err := hooks.NewStore(writable).Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("mutation on a writable path after a failed one: %v", err)
		}
	})
}

func TestMutationLockBound(t *testing.T) {
	t.Run("it acquires exactly once per mutation", func(t *testing.T) {
		// A self-block costs the full bound, so the threshold below is what
		// separates the two outcomes. The bound is generous because the
		// budget it grants an uncontended write (open + flock + read +
		// marshal + rename) is shared with every other package in a
		// parallel unit lane; the failure mode is an order of magnitude
		// away from it regardless.
		bound := 500 * time.Millisecond
		hooks.SetLockTimeoutForTest(t, bound)

		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		start := time.Now()
		if err := store.Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if elapsed := time.Since(start); elapsed >= bound/2 {
			t.Fatalf("Set took %v against a %v bound — it blocked against its own hold", elapsed, bound)
		}
	})

	t.Run("it returns the sentinel and writes nothing when the lock will not yield", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 50*time.Millisecond)

		path := filepath.Join(t.TempDir(), "hooks.json")
		hookstest.HoldHooksSidecar(t, path)

		err := hooks.NewStore(path).Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI)
		if !errors.Is(err, hooks.ErrLockHeld) {
			t.Fatalf("Set error = %v, want ErrLockHeld", err)
		}
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hooks.json written despite the timeout: %v", statErr)
		}
	})

	t.Run("it fails through the sidecar acquire when the config directory cannot be created", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "ro")
		if err := os.Mkdir(parent, 0o500); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
		path := filepath.Join(parent, "portal", "hooks.json")

		err := hooks.NewStore(path).Set("k0", "on-resume", hooks.Registration{Command: "cmd0"}, hooks.ViaCLI)
		if err == nil {
			t.Fatal("Set succeeded under an uncreatable config directory")
		}
		if !strings.Contains(err.Error(), "hooks lock") {
			t.Errorf("error did not come through the sidecar acquire: %v", err)
		}
		if errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("error came through the write path, not the acquire: %v", err)
		}
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hooks.json written: %v", statErr)
		}
	})
}
