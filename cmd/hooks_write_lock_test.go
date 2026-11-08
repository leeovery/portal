package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// assertLockFailureReachesStderr pins the route a lock failure takes to the
// user: a plain error cobra neither silences nor dresses as a usage problem, so
// main's classify prints its message and exits 1.
func assertLockFailureReachesStderr(t *testing.T, out string, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a non-zero exit under a held lock, got nil")
	}
	if !errors.Is(err, hooks.ErrLockHeld) {
		t.Errorf("error = %v, want errors.Is ErrLockHeld", err)
	}
	if !strings.Contains(err.Error(), hooks.ErrLockHeld.Error()) {
		t.Errorf("error = %q, want it to carry the reason %q", err.Error(), hooks.ErrLockHeld.Error())
	}
	if _, ok := errors.AsType[*UsageError](err); ok {
		t.Errorf("error %v is a *UsageError; a lock timeout is not a usage error", err)
	}
	if IsSilentExitError(err) {
		t.Error("error is a silent-exit error; its reason would never reach stderr")
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("command output = %q, want no usage dump", out)
	}
}

// assertReturnsAtLockBound proves the mutation waits the bound out and returns,
// rather than blocking forever on a lock it can never take.
func assertReturnsAtLockBound(t *testing.T, verb string, run func() error) {
	t.Helper()
	start := time.Now()
	err := run()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a non-zero exit under a held lock, got nil")
	}
	if elapsed < lockBound {
		t.Errorf("hook %s returned after %v — it did not wait out the %v bound", verb, elapsed, lockBound)
	}
	// A generous multiple: the assertion separates bounded from unbounded, and
	// a unit lane under load must not read as a hang.
	if limit := 20 * lockBound; elapsed > limit {
		t.Errorf("hook %s took %v against a %v bound (limit %v) — it is not returning at the bound", verb, elapsed, lockBound, limit)
	}
}

func TestHookSetLockTimeout(t *testing.T) {
	t.Run("it exits non-zero from hook set on a lock timeout", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedB: {"on-resume": "npm start"},
		})
		before := readFileBytes(t, hooksFile)
		hookstest.HoldHooksSidecar(t, hooksFile)
		inSession(t, hookstest.SubjectSeedA)

		sink := logtest.Install(t)
		out, err := runHookSet(t, "claude --resume abc")
		assertLockFailureReachesStderr(t, out, err)
		hookstest.AssertLockWarn(t, sink, "set", hookstest.SubjectSeedA, "cli")
		assertHooksFileUnchanged(t, hooksFile, before)
	})

	t.Run("it leaves an absent hooks.json absent on a lock timeout", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, nil)
		hookstest.HoldHooksSidecar(t, hooksFile)
		inSession(t, hookstest.SubjectSeedA)

		if _, err := runHookSet(t, "claude --resume abc"); err == nil {
			t.Fatal("expected a non-zero exit under a held lock, got nil")
		}
		if _, statErr := os.Stat(hooksFile); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hooks.json created despite the timeout: %v", statErr)
		}
	})

	t.Run("it returns at the bound rather than hanging", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, nil)
		hookstest.HoldHooksSidecar(t, hooksFile)
		inSession(t, hookstest.SubjectSeedA)

		assertReturnsAtLockBound(t, "set", func() error {
			_, err := runHookSet(t, "claude --resume abc")
			return err
		})
	})
}

func TestHookRmLockTimeout(t *testing.T) {
	// The seed every held-lock row is driven against: one entry a successful
	// removal would take out, so a file left byte-identical says the mutation
	// never ran.
	oneEntry := map[string]map[string]string{
		hookstest.SubjectSeedA: {"on-resume": "claude --resume abc"},
	}

	t.Run("it exits non-zero from hook rm on a lock timeout", func(t *testing.T) {
		sink := logtest.Install(t)

		got := runRmCase(t, rmCase{
			current:  hookstest.SubjectSeedA,
			seeded:   oneEntry,
			holdLock: true,
			wantErr:  true,
		})

		assertLockFailureReachesStderr(t, got.out, got.err)
		hookstest.AssertLockWarn(t, sink, "rm", hookstest.SubjectSeedA, "cli")
		assertHooksFileUnchanged(t, got.hooksFile, got.before)
	})

	t.Run("it exits non-zero on the --key path", func(t *testing.T) {
		got := runRmCase(t, rmCase{
			seeded:   oneEntry,
			extra:    []string{"--key", hookstest.SubjectSeedA},
			holdLock: true,
			wantErr:  true,
		})

		assertLockFailureReachesStderr(t, got.out, got.err)
		assertHooksFileUnchanged(t, got.hooksFile, got.before)
	})

	t.Run("it returns at the bound rather than hanging", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, oneEntry)
		hookstest.HoldHooksSidecar(t, hooksFile)
		inSession(t, hookstest.SubjectSeedA)

		assertReturnsAtLockBound(t, "rm", func() error {
			_, err := runHookRm(t)
			return err
		})
	})
}
