package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/sessionindex"
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

	t.Run("it counts in the plural", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedA, hookstest.ReapableSeedB))
		assertCheck(t, staleHooksCheck(t, install), checkFail, "2 stale hook entries")
	})

	// The sweep won't reap the tmux version's keys, so the count waits on
	// them, and says so.
	t.Run("it is not evaluable while hooks.json holds keys the tmux version wrote, saying why", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedA, hookstest.TmuxEraSeedA, hookstest.TmuxEraSeedB))

		outBuf, _, err := runDoctorWith(t, install.deps)

		if err != nil {
			t.Errorf("Execute err = %v; want nil: a stand-down never moves the exit code", err)
		}
		if want := "  · stale hooks: " + tmuxKeysStandDownPhrase + " (not evaluable)\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("report missing %q:\n%s", want, outBuf.String())
		}
	})

	t.Run("once the tmux version's state is migrated, it counts a name in a token's shape as any other", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB, "fabric"))
		if err := os.Mkdir(state.TmuxStateDir(install.deps.StateDir), 0o700); err != nil {
			t.Fatal(err)
		}
		assertCheck(t, staleHooksCheck(t, install), checkFail, "1 stale hook entry")
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
		{hooksweep.ReasonStateDirUnresolved, func(t *testing.T, install staleHooksInstall) {
			install.deps.StateDir = ""
			t.Setenv("PORTAL_STATE_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("HOME", "")
			if dir, err := state.Dir(); err == nil {
				t.Fatalf("state.Dir() = %q with no env var, no XDG_CONFIG_HOME and no HOME; this case would be vacuous", dir)
			}
		}},
		{hooksweep.ReasonListFailed, func(t *testing.T, install staleHooksInstall) {
			install.backend.Fail(backendtest.OpList, errZmxDirUnreadable)
		}},
		{hooksweep.ReasonIndexUnusable, func(t *testing.T, install staleHooksInstall) {
			writeSessionsJSON(t, install.deps.StateDir, []byte(`{"version": 1, "sessions": []}`))
		}},
		{hooksweep.ReasonNoSessions, func(t *testing.T, install staleHooksInstall) {
			install.deps.ListSessions = backendtest.New().List
			if err := os.Remove(sessionindex.Path(install.deps.StateDir)); err != nil {
				t.Fatalf("remove sessions.json: %v", err)
			}
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

	t.Run("it renders a stand-down with the not-evaluable marker", func(t *testing.T) {
		install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		install.backend.Fail(backendtest.OpList, errZmxDirUnreadable)

		outBuf, _, _ := runDoctorWith(t, install.deps)
		if want := "  · stale hooks: " + listStandDownPhrase + "\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("report missing %q:\n%s", want, outBuf.String())
		}
	})
}

// A hooks.json that can't be read or doesn't parse is a defect no cadence
// repairs, so the check fails on it rather than waiting it out, and no other
// line reports it.
func TestDoctorStaleHooksCheckFailsOnAStoreItCannotRead(t *testing.T) {
	for _, tc := range storeDefects() {
		t.Run("it fails, once, on "+tc.name, func(t *testing.T) {
			install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
			tc.stage(t, install)

			assertCheck(t, staleHooksCheck(t, install), checkFail, tc.phrase)

			outBuf, _, err := runDoctorWith(t, install.deps)
			if err != ErrDoctorUnhealthy {
				t.Errorf("Execute err = %v; want ErrDoctorUnhealthy over a hooks.json doctor cannot judge", err)
			}
			var lines []string
			for line := range strings.Lines(outBuf.String()) {
				if strings.Contains(line, "hooks.json") {
					lines = append(lines, line)
				}
			}
			if want := "  ✗ stale hooks: " + tc.phrase + "\n"; len(lines) != 1 || lines[0] != want {
				t.Errorf("hooks.json lines = %q, want only %q", lines, want)
			}
		})
	}

	t.Run("it fails on a store it could not open, as the file it could not read", func(t *testing.T) {
		install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		got := checkStaleHooks(t.Context(), nil, hooksweep.Sources{List: install.deps.ListSessions, StateDir: install.deps.StateDir})
		assertCheck(t, got, checkFail, storeReadPhrase)
	})
}

// storeDefect is an install whose hooks.json doctor cannot judge, and the
// words both surfaces give it.
type storeDefect struct {
	name   string
	phrase string
	stage  func(t *testing.T, install staleHooksInstall)
}

func storeDefects() []storeDefect {
	return []storeDefect{
		{name: "an unreadable hooks.json", phrase: storeReadPhrase, stage: func(t *testing.T, install staleHooksInstall) {
			_, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
			install.deps.HookStore = hooks.NewStore(path)
		}},
		{name: "a hooks.json that doesn't parse", phrase: storeMalformedPhrase, stage: func(t *testing.T, install staleHooksInstall) {
			if err := os.WriteFile(install.hooksPath, []byte("{invalid json!!!"), 0o600); err != nil {
				t.Fatalf("write hooks.json: %v", err)
			}
		}},
	}
}

// The sessions are listed once a diagnosis, so a session that doesn't answer
// costs the sessions check and the stale-hooks check one bound between them.
func TestDoctorListsSessionsOnce(t *testing.T) {
	install := newStaleHooksInstall(t, hookstest.StaleHookSeed)

	results := diagnose(t, install.deps)

	assertCheck(t, findCheck(t, results, "stale hooks"), checkFail, "1 stale hook entry")
	if calls := install.backend.CallsTo(backendtest.OpList); len(calls) != 1 {
		t.Errorf("listed %d times in one diagnosis, want 1", len(calls))
	}
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
		pruned := "Pruned 1 stale hook:\n  " + hookstest.ReapableSeedA + "\n"
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

	t.Run("it names how many it pruned, then each key on its own line, sorted", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedB, hookstest.ReapableSeedA))

		outBuf, _, err := runDoctorWith(t, install.deps, "--fix")
		if err != nil {
			t.Fatalf("Execute err = %v; want nil", err)
		}
		if want := "Pruned 2 stale hooks:\n  " + hookstest.ReapableSeedA + "\n  " + hookstest.ReapableSeedB + "\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, outBuf.String())
		}
	})

	t.Run("it prunes nothing while hooks.json holds keys the tmux version wrote, saying why", func(t *testing.T) {
		install := newStaleHooksInstall(t, hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedA, hookstest.TmuxEraSeedB))
		before := readFileBytes(t, install.hooksPath)

		outBuf, _, _ := runDoctorWith(t, install.deps, "--fix")

		if want := "Skipped stale hook prune: " + tmuxKeysStandDownPhrase + "\n"; !strings.Contains(outBuf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, outBuf.String())
		}
		if after := readFileBytes(t, install.hooksPath); !bytes.Equal(after, before) {
			t.Errorf("hooks.json changed under a prune that stood down:\n%s", after)
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

			want := "Skipped stale hook prune: " + skippedPrunePhrases[tc.reason] + "\n"
			// The flag's read fails only in a state dir it can't read, where
			// the zmx dir record's read fails too, so --fix repairs nothing.
			if tc.reason == hooksweep.ReasonRestoringReadFailed {
				want = fixRefusedLine + "\n"
			}
			if !strings.Contains(outBuf.String(), want) {
				t.Errorf("output missing %q:\n%s", want, outBuf.String())
			}
			assertHooksFileUnchanged(t, install.hooksPath, before, "touched by a prune that stood down")
		})
	}

	failures := append(storeDefects(),
		storeDefect{name: "a sweep that fails", phrase: sweepFailedPhrase, stage: func(t *testing.T, install staleHooksInstall) {
			store, _ := hookstest.StageStore(t, hookstest.Staging{
				Dir:           filepath.Join(t.TempDir(), "lock-open-denied"),
				Seed:          hookstest.StaleHookSeed,
				SidecarAbsent: true,
				WritesDenied:  true,
			})
			install.deps.HookStore = store
		}},
		storeDefect{name: "a hook store it could not open", phrase: storeReadPhrase, stage: func(t *testing.T, install staleHooksInstall) {
			install.deps.HookStore = nil
			t.Setenv("PORTAL_HOOKS_FILE", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("HOME", "")
			if _, err := loadHookStore(); err == nil {
				t.Fatal("loadHookStore succeeded with no env var, no XDG_CONFIG_HOME and no HOME; this case would be vacuous")
			}
		}},
	)
	for _, tc := range failures {
		t.Run("it says so on "+tc.name, func(t *testing.T) {
			install := newStaleHooksInstall(t, hookstest.StaleHookSeed)
			tc.stage(t, install)

			outBuf, _, _ := runDoctorWith(t, install.deps, "--fix")

			if want := "Skipped stale hook prune: " + tc.phrase + "\n"; !strings.Contains(outBuf.String(), want) {
				t.Errorf("output missing %q:\n%s", want, outBuf.String())
			}
		})
	}
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
