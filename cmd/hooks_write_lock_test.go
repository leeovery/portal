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
	"github.com/leeovery/portal/internal/state"
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
		t.Setenv("TMUX_PANE", "%3")
		before := readFileBytes(t, hooksFile)
		hookstest.HoldHooksSidecar(t, hooksFile)

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		sink := logtest.Install(t)
		out, err := runHookSet(t, "claude --resume abc")
		assertLockFailureReachesStderr(t, out, err)
		hookstest.AssertLockWarn(t, sink, "set", hookstest.SubjectSeedA, "cli")
		assertHooksFileUnchanged(t, hooksFile, before)
	})

	t.Run("it leaves an absent hooks.json absent on a lock timeout", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")
		hookstest.HoldHooksSidecar(t, hooksFile)

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		if _, err := runHookSet(t, "claude --resume abc"); err == nil {
			t.Fatal("expected a non-zero exit under a held lock, got nil")
		}
		if _, statErr := os.Stat(hooksFile); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hooks.json created despite the timeout: %v", statErr)
		}
	})

	t.Run("it touches no save.requested on a timed-out registration", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, nil)
		stateDir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", stateDir)
		t.Setenv("TMUX_PANE", "%3")
		hookstest.HoldHooksSidecar(t, hooksFile)

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		if _, err := runHookSet(t, "claude --resume abc"); err == nil {
			t.Fatal("expected a non-zero exit under a held lock, got nil")
		}
		if saveRequestedExists(t, stateDir) {
			t.Error("a timed-out hook set touched save.requested")
		}
	})

	t.Run("it keeps the pane's token after a timed-out registration", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%7")
		release := hookstest.HoldHooksSidecar(t, hooksFile)

		pane := &stampedPane{}
		minted := 0
		withHooksDeps(t, HooksDeps{
			KeyResolver: pane,
			PaneStamper: pane,
			TokenMinter: func() (string, error) { minted++; return hookstest.SubjectSeedC, nil },
		})

		if _, err := runHookSet(t, "claude --resume abc"); err == nil {
			t.Fatal("expected a non-zero exit under a held lock, got nil")
		}

		if len(pane.stamps) != 1 {
			t.Fatalf("set-option call count = %d, want exactly 1 — the stamp stands, and nothing rolls it back: %+v",
				len(pane.stamps), pane.stamps)
		}
		stamp := pane.stamps[0]
		if stamp.target != "%7" || stamp.name != state.PortalPaneIDOption || stamp.value != hookstest.SubjectSeedC {
			t.Errorf("stamp = %+v, want %s=%s on %%7", stamp, state.PortalPaneIDOption, hookstest.SubjectSeedC)
		}
		if pane.token != hookstest.SubjectSeedC {
			t.Errorf("pane token = %q after the timeout, want it left in place", pane.token)
		}

		// The retry reads the token back and reuses it: no second mint, no second
		// stamp, and one entry under the token the failed attempt stamped.
		release()
		if _, err := runHookSet(t, "claude --resume abc"); err != nil {
			t.Fatalf("retry after the lock freed: %v", err)
		}

		if minted != 1 {
			t.Errorf("mint count = %d, want 1 — the retry reuses the stamped token", minted)
		}
		if len(pane.stamps) != 1 {
			t.Errorf("set-option call count = %d, want 1 — a pane already carrying a token is not re-stamped: %+v",
				len(pane.stamps), pane.stamps)
		}
		data := readHooksJSON(t, hooksFile)
		if len(data) != 1 {
			t.Fatalf("hooks.json = %v, want exactly one entry", data)
		}
		if data[hookstest.SubjectSeedC]["on-resume"].Command != "claude --resume abc" {
			t.Errorf("hooks.json = %v, want the entry under %s", data, hookstest.SubjectSeedC)
		}
	})

	t.Run("it returns at the bound rather than hanging", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, nil)
		t.Setenv("TMUX_PANE", "%3")
		hookstest.HoldHooksSidecar(t, hooksFile)

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

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
			name:     "resolved-token path under a held lock",
			paneID:   "%3",
			resolver: &mockKeyResolver{key: hookstest.SubjectSeedA},
			seeded:   oneEntry,
			holdLock: true,
			wantErr:  true,
		})

		assertLockFailureReachesStderr(t, got.out, got.err)
		hookstest.AssertLockWarn(t, sink, "rm", hookstest.SubjectSeedA, "cli")
		assertHooksFileUnchanged(t, got.hooksFile, got.before)
	})

	t.Run("it exits non-zero on the --pane-key path and still issues no tmux call", func(t *testing.T) {
		// runRmCase guards the poisoned pair for a --pane-key row itself, so the
		// no-tmux-call assertion rides along with the drive.
		got := runRmCase(t, rmCase{
			name:        "--pane-key path under a held lock",
			paneID:      "",
			paneKeyPath: true,
			seeded:      oneEntry,
			extra:       []string{"--pane-key", hookstest.SubjectSeedA},
			holdLock:    true,
			wantErr:     true,
		})

		assertLockFailureReachesStderr(t, got.out, got.err)
		assertHooksFileUnchanged(t, got.hooksFile, got.before)
	})

	t.Run("it returns at the bound rather than hanging", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, lockBound)
		_, hooksFile := hooksFileInTempDir(t, oneEntry)
		t.Setenv("TMUX_PANE", "%3")
		hookstest.HoldHooksSidecar(t, hooksFile)

		withHooksDeps(t, HooksDeps{KeyResolver: &mockKeyResolver{key: hookstest.SubjectSeedA}})

		assertReturnsAtLockBound(t, "rm", func() error {
			_, err := runHookRm(t)
			return err
		})
	})
}
