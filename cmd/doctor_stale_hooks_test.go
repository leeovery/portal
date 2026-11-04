package cmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
)

// errZmxDirUnreadable is the listing failure a stand-down fixture arms.
var errZmxDirUnreadable = errors.New("zmx directory unreadable")

// staleHooksCheck is the stale-hooks line of one diagnosis of install.
func staleHooksCheck(t *testing.T, install staleHooksInstall) checkResult {
	t.Helper()
	return findCheck(t, diagnose(t, install.deps), "stale hooks")
}

// The check counts the keys the daemon's sweep would remove: those naming
// neither a listed session nor an index entry.
func TestDoctorStaleHooksCheck(t *testing.T) {
	t.Run("it counts a key naming neither a listed session nor an index entry", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.ReapableSeedA))
		assertCheck(t, staleHooksCheck(t, install), checkFail, "1 stale hook entry")
	})

	t.Run("it counts the keys the tmux version wrote, in the plural", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.TmuxEraSeedA, hookstest.TmuxEraSeedB))
		assertCheck(t, staleHooksCheck(t, install), checkFail, "2 stale hook entries")
	})

	t.Run("it passes when a listed session or an index entry names every key", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB))
		assertCheck(t, staleHooksCheck(t, install), checkPass, "no stale hooks")
	})

	t.Run("it keeps a session that doesn't answer from counting", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA))
		install.backend.StopAnswering(hookstest.LiveSeedA)
		assertCheck(t, staleHooksCheck(t, install), checkPass, "no stale hooks")
	})

	t.Run("it passes with no hooks", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody())
		assertCheck(t, staleHooksCheck(t, install), checkPass, "no hooks")
	})

	t.Run("it is read-only: hooks.json, its directory and the state dir unchanged", func(t *testing.T) {
		install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		hooksDir, stateDir := filepath.Dir(install.hooksPath), install.deps.StateDir
		hooksBefore := snapshotDir(t, hooksDir)
		stateBefore := snapshotDir(t, stateDir)

		outBuf, _, err := runDoctorWith(t, install.deps)
		if err != ErrDoctorUnhealthy {
			t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy for the stale hook", err)
		}
		if !strings.Contains(outBuf.String(), "  ✗ stale hooks: 1 stale hook entry\n") {
			t.Errorf("report missing the stale count:\n%s", outBuf.String())
		}
		assertDirUnchanged(t, hooksDir, hooksBefore)
		assertDirUnchanged(t, stateDir, stateBefore)
		for _, call := range install.backend.Calls() {
			if call.Op != backendtest.OpList {
				t.Errorf("doctor called %s on the backend; the diagnosis only lists", call.Op)
			}
		}
	})
}

// staleHooksStandDown is an install whose only anomaly makes the sweep stand
// down under reason.
type staleHooksStandDown struct {
	reason hooksweep.Reason
	stage  func(t *testing.T, install staleHooksInstall)
}

func staleHooksStandDowns() []staleHooksStandDown {
	return []staleHooksStandDown{
		{hooksweep.ReasonRestoring, func(t *testing.T, install staleHooksInstall) {
			if err := state.Restoring.Set(install.deps.StateDir); err != nil {
				t.Fatalf("raise the restoring flag: %v", err)
			}
		}},
		{hooksweep.ReasonRestoringReadFailed, func(t *testing.T, install staleHooksInstall) {
			install.deps.StateDir = unreadableDir(t)
		}},
		{hooksweep.ReasonStoreReadFailed, func(t *testing.T, install staleHooksInstall) {
			_, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
			install.deps.HookStore = hooks.NewStore(path)
		}},
		{hooksweep.ReasonListFailed, func(t *testing.T, install staleHooksInstall) {
			install.backend.Fail(backendtest.OpList, errZmxDirUnreadable)
		}},
		{hooksweep.ReasonIndexUnusable, func(t *testing.T, install staleHooksInstall) {
			writeSessionsJSON(t, install.deps.StateDir, []byte(`{"version": 1, "sessions": []}`))
		}},
	}
}

// A stand-down is the count unknowable, never a stale hook: the line is not
// evaluable and never moves the exit code.
func TestDoctorStaleHooksCheckStandsDown(t *testing.T) {
	for _, tc := range staleHooksStandDowns() {
		t.Run("it is not evaluable on "+string(tc.reason), func(t *testing.T) {
			install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
			tc.stage(t, install)

			got := staleHooksCheck(t, install)
			assertCheck(t, got, checkNotEvaluable, notEvaluableDetails[tc.reason])
			if doctorUnhealthy([]checkResult{got}) {
				t.Error("a stand-down drove the exit code")
			}
		})
	}

	t.Run("it words a store it could not open as the file it could not read", func(t *testing.T) {
		install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		got := checkStaleHooks(t.Context(), nil, hookSweepSources(install.deps, install.deps.StateDir))
		assertCheck(t, got, checkNotEvaluable, storeReadStandDownPhrase)
	})

	t.Run("it renders a stand-down with the not-evaluable marker", func(t *testing.T) {
		install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		install.backend.Fail(backendtest.OpList, errZmxDirUnreadable)

		outBuf, _, _ := runDoctorWith(t, install.deps)
		if want := "  · stale hooks: " + listStandDownPhrase + "\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("report missing %q:\n%s", want, outBuf.String())
		}
	})
}

// --fix prunes through the daemon's sweep, says how many it removed, then
// re-diagnoses.
func TestDoctorFixPrunesStaleHooks(t *testing.T) {
	t.Run("it prunes a stale hook, then re-diagnoses clean", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.ReapableSeedA))

		outBuf, _, err := runDoctorWith(t, install.deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil (healthy post-repair)", err)
		}

		out := outBuf.String()
		pruned := "Pruned 1 stale hook: " + hookstest.ReapableSeedA + "\n"
		first, second, ok := strings.Cut(out, pruned)
		if !ok {
			t.Fatalf("output missing %q:\n%s", pruned, out)
		}
		if !strings.Contains(first, "  ✗ stale hooks: 1 stale hook entry\n") {
			t.Errorf("the first report does not count the stale hook:\n%s", first)
		}
		if !strings.HasPrefix(second, "Portal doctor:\n") || !strings.Contains(second, "  ✓ stale hooks: no stale hooks\n") {
			t.Errorf("the re-diagnosis does not follow the prune clean:\n%s", second)
		}
		persisted, err := install.deps.HookStore.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load hooks.json: %v", err)
		}
		if _, ok := persisted[hookstest.ReapableSeedA]; ok || len(persisted) != 2 {
			t.Errorf("hooks.json holds %v after the prune; want only the listed and the indexed keys", keysOf(persisted))
		}
	})

	t.Run("it names how many it pruned, in the plural", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedA, hookstest.TmuxEraSeedB))

		outBuf, _, err := runDoctorWith(t, install.deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		if want := "Pruned 2 stale hooks: " + hookstest.TmuxEraSeedB + ", " + hookstest.ReapableSeedA + "\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, outBuf.String())
		}
	})

	t.Run("it prints no prune line when nothing is stale", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA))

		outBuf, _, err := runDoctorWith(t, install.deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		if strings.Contains(outBuf.String(), "stale hook prune") || strings.Contains(outBuf.String(), "Pruned") {
			t.Errorf("a prune that found nothing reported:\n%s", outBuf.String())
		}
	})

	for _, tc := range append(staleHooksStandDowns(), staleHooksStandDown{hooksweep.ReasonLockTimeout, func(t *testing.T, install staleHooksInstall) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		hookstest.HoldHooksSidecar(t, install.hooksPath)
	}}) {
		t.Run("it removes nothing and says why on "+string(tc.reason), func(t *testing.T) {
			install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
			tc.stage(t, install)
			before := readFileBytes(t, install.hooksPath)

			outBuf, _, _ := runDoctorWith(t, install.deps, "--fix")

			if want := "Skipped stale hook prune: " + skippedPrunePhrases[tc.reason] + "\n"; !strings.Contains(outBuf.String(), want) {
				t.Errorf("output missing %q:\n%s", want, outBuf.String())
			}
			assertHooksFileUnchanged(t, install.hooksPath, before, "touched by a prune that stood down")
		})
	}

	t.Run("it says so when the sweep fails", func(t *testing.T) {
		install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Dir:           filepath.Join(t.TempDir(), "lock-open-denied"),
			Seed:          hookstest.StaleHookSeed,
			SidecarAbsent: true,
			WritesDenied:  true,
		})
		install.deps.HookStore = store

		outBuf, _, _ := runDoctorWith(t, install.deps, "--fix")

		if want := "Skipped stale hook prune: " + sweepFailedStandDownPhrase + "\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, outBuf.String())
		}
	})
}

// Every reason the sweep can stand down under has its words on both
// surfaces, and no two reasons share them.
func TestStaleHooksStandDownVocabulary(t *testing.T) {
	for name, vocabulary := range map[string]map[hooksweep.Reason]string{
		"skipped prune": skippedPrunePhrases,
		"not evaluable": notEvaluableDetails,
	} {
		t.Run("it words every reason for the "+name+" line", func(t *testing.T) {
			if len(vocabulary) != len(hooksweep.Reasons) {
				t.Errorf("%d phrases for %d reasons; a phrase for no reason is dead copy", len(vocabulary), len(hooksweep.Reasons))
			}
			seen := map[string]hooksweep.Reason{}
			for _, reason := range hooksweep.Reasons {
				phrase := vocabulary[reason]
				if phrase == "" {
					t.Errorf("reason %q has no phrase", reason)
					continue
				}
				if other, dup := seen[phrase]; dup {
					t.Errorf("reasons %q and %q share the phrase %q", other, reason, phrase)
				}
				seen[phrase] = reason
			}
		})
	}
}

func snapshotDir(t *testing.T, dir string) map[string]portaltest.Fingerprint {
	t.Helper()
	snapshot, err := portaltest.SnapshotStateDir(dir)
	if err != nil {
		t.Fatalf("fingerprint %s: %v", dir, err)
	}
	return snapshot
}

func assertDirUnchanged(t *testing.T, dir string, before map[string]portaltest.Fingerprint) {
	t.Helper()
	for _, delta := range portaltest.DiffFingerprints(before, snapshotDir(t, dir)) {
		t.Errorf("doctor changed %s: %s", dir, portaltest.FormatDelta(delta))
	}
}
