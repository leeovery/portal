package hooks_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

func TestMutationLockTimeoutWritesNothing(t *testing.T) {
	t.Run("it writes nothing when Set cannot take the lock", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)

		t.Run("an absent hooks.json stays absent", func(t *testing.T) {
			path := hookstest.HooksPath(t, t.TempDir())
			hookstest.HoldHooksSidecar(t, path)

			err := hooks.NewStore(path).Set("tok123", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI)
			if err == nil {
				t.Fatal("expected an error when the lock will not yield, got nil")
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("hooks.json created despite the timeout: %v", statErr)
			}
		})

		t.Run("an existing hooks.json is byte-identical", func(t *testing.T) {
			path := hookstest.HooksPath(t, t.TempDir())
			if err := os.WriteFile(path, []byte(`{"tok999":{"on-resume":"cmd-a"}}`), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}
			before := readFileBytes(t, path)
			hookstest.HoldHooksSidecar(t, path)

			if err := hooks.NewStore(path).Set("tok123", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err == nil {
				t.Fatal("expected an error when the lock will not yield, got nil")
			}

			hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a timed-out Set")
		})
	})

	t.Run("it writes nothing when Remove cannot take the lock", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)

		path := hookstest.HooksPath(t, t.TempDir())
		if err := os.WriteFile(path, []byte(`{"tok123":{"on-resume":"cmd-a"}}`), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		before := readFileBytes(t, path)
		hookstest.HoldHooksSidecar(t, path)

		removed, err := hooks.NewStore(path).Remove("tok123", "on-resume", hooks.ViaCLI)
		if err == nil {
			t.Fatal("expected an error when the lock will not yield, got nil")
		}
		if removed {
			t.Error("removed = true, want false when the lock could not be taken")
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a timed-out Remove")
	})

	t.Run("it matches the sentinel through the wrap", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)

		path := hookstest.HooksPath(t, t.TempDir())
		hookstest.HoldHooksSidecar(t, path)
		store := hooks.NewStore(path)

		setErr := store.Set("tok123", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI)
		if !errors.Is(setErr, hooks.ErrLockHeld) {
			t.Errorf("Set error = %v, want errors.Is ErrLockHeld", setErr)
		}

		_, rmErr := store.Remove("tok123", "on-resume", hooks.ViaCLI)
		if !errors.Is(rmErr, hooks.ErrLockHeld) {
			t.Errorf("Remove error = %v, want errors.Is ErrLockHeld", rmErr)
		}
	})
}

func TestMutationLockTimeoutLogging(t *testing.T) {
	t.Run("it emits one WARN under op=set for a timed-out registration", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		path := hookstest.HooksPath(t, t.TempDir())
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		if err := hooks.NewStore(path).Set("tok123", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err == nil {
			t.Fatal("expected an error when the lock will not yield, got nil")
		}

		hookstest.AssertLockWarn(t, sink, "set", "tok123", "cli")
	})

	t.Run("it files under op=set even where a completed call would have classified as modify", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		path := hookstest.HooksPath(t, t.TempDir())
		// The same key and event already carry a different command, so a call that
		// got as far as loading and classifying would file this under op=modify.
		// The op is decided before the lock is even attempted, so it stays set.
		if err := os.WriteFile(path, []byte(`{"tok123":{"on-resume":"cmd-a"}}`), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		if err := hooks.NewStore(path).Set("tok123", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err == nil {
			t.Fatal("expected an error when the lock will not yield, got nil")
		}

		hookstest.AssertLockWarn(t, sink, "set", "tok123", "cli")
	})

	t.Run("it emits one WARN under op=rm for a timed-out removal", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		path := hookstest.HooksPath(t, t.TempDir())
		if err := os.WriteFile(path, []byte(`{"tok123":{"on-resume":"cmd-a"}}`), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		removed, err := hooks.NewStore(path).Remove("tok123", "on-resume", hooks.ViaInternal)
		if err == nil {
			t.Fatal("expected an error when the lock will not yield, got nil")
		}
		if removed {
			t.Error("removed = true, want false when the lock could not be taken")
		}

		hookstest.AssertLockWarn(t, sink, "rm", "tok123", "internal")
	})

	t.Run("it still emits nothing when Remove removes nothing", func(t *testing.T) {
		path := hookstest.HooksPath(t, t.TempDir())
		store := hooks.NewStore(path)
		if err := store.Set("tok999", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err != nil {
			t.Fatalf("seed: %v", err)
		}
		before := readFileBytes(t, path)

		sink := logtest.Install(t)
		removed, err := store.Remove("tok123", "on-resume", hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if removed {
			t.Error("removed = true, want false for an absent key")
		}
		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("a removal that removed nothing emitted %d records, want 0: %+v", len(recs), recs)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed on a no-removal")
	})

	t.Run("it emits no store-side WARN when CleanStale cannot take the lock", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		path := hookstest.HooksPath(t, t.TempDir())
		if err := os.WriteFile(path, []byte(`{"tok123":{"on-resume":"cmd-a"}}`), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		removed, err := hooks.NewStore(path).CleanStale(enumerating())
		if !errors.Is(err, hooks.ErrLockHeld) {
			t.Fatalf("CleanStale error = %v, want errors.Is ErrLockHeld", err)
		}
		if len(removed) != 0 {
			t.Errorf("removed = %v, want none", removed)
		}
		// The clean's own snapshot read degrades against the same held sidecar
		// and says so; nothing else may be said.
		for _, r := range sink.Records() {
			if r.Msg != "load-unlocked" {
				t.Errorf("CleanStale emitted %q, want only the read degradation — the stood-down line belongs to its call site: %+v", r.Msg, r)
			}
		}
	})

	t.Run("it fails the write when the sidecar cannot be created", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "ro")
		if err := os.Mkdir(parent, 0o500); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
		path := hookstest.HooksPath(t, filepath.Join(parent, "portal"))

		sink := logtest.Install(t)
		if err := hooks.NewStore(path).Set("tok123", "on-resume", hooks.Registration{Command: "npm start"}, hooks.ViaCLI); err == nil {
			t.Fatal("Set succeeded under a directory that permits no file creation")
		}

		hookstest.AssertLockWarn(t, sink, "set", "tok123", "cli")
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hooks.json written: %v", statErr)
		}
	})
}
