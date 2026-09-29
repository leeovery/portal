package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestDoctorStaleHooksDegradedRead(t *testing.T) {
	t.Run("it keeps the stale-hooks check green under a degraded read", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedA)}

		unlockedStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
		baseline, err := runDoctorDiagnosis(staleDeps(t.TempDir(), lister, unlockedStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		want := findCheck(t, baseline, "stale hooks")
		wantUnhealthy := doctorUnhealthy(baseline)

		heldStore, heldPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
		hookstest.HoldHooksSidecar(t, heldPath)

		sink := logtest.Install(t)
		degraded, err := runDoctorDiagnosis(staleDeps(t.TempDir(), lister, heldStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis under a held lock: %v", err)
		}
		got := findCheck(t, degraded, "stale hooks")

		if got.status != want.status {
			t.Errorf("status = %v under a degraded read, want %v", got.status, want.status)
		}
		if got.detail != want.detail {
			t.Errorf("detail = %q under a degraded read, want %q", got.detail, want.detail)
		}
		if doctorUnhealthy(degraded) != wantUnhealthy {
			t.Errorf("doctorUnhealthy = %v under a degraded read, want %v", doctorUnhealthy(degraded), wantUnhealthy)
		}
		hookstest.AssertDegradedRead(t, sink, "doctor")
	})

	t.Run("it leaves the config directory untouched across portal doctor", func(t *testing.T) {
		// A fresh install: the directory holds no hooks.json at all, so a read
		// that created either it or the sidecar would show up as a new entry.
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		configDir := t.TempDir()
		store := hooks.NewStore(filepath.Join(configDir, "hooks.json"))
		before := dirListing(t, configDir)

		if _, err := runDoctorDiagnosis(staleDeps(t.TempDir(), &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedA)}, store, nil)); err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}

		if after := dirListing(t, configDir); !reflect.DeepEqual(before, after) {
			t.Errorf("config directory changed across a read-only doctor: before=%v after=%v", before, after)
		}
	})
}

func TestHookListDegradedRead(t *testing.T) {
	t.Run("it lists hooks under a degraded read", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{
			hookstest.SubjectSeedA: {"on-resume": "npm start"},
		})
		withHooksDeps(t, HooksDeps{PaneLister: &recordingPaneHookLister{rows: []tmux.PaneHookRow{
			{Token: hookstest.SubjectSeedA, Location: "proj:0.0"},
		}}})

		sink := logtest.Install(t)
		want := runHookList(t)
		if degraded := hookstest.UnlockedRecords(t, sink); len(degraded) != 0 {
			t.Fatalf("the baseline read degraded: %+v", degraded)
		}

		hookstest.HoldHooksSidecar(t, hooksFile)
		got := runHookList(t)

		if got != want {
			t.Errorf("output under a degraded read = %q, want %q", got, want)
		}
		hookstest.AssertDegradedRead(t, sink, "cli")
	})

	t.Run("it takes no tmux read and creates nothing on a fresh install", func(t *testing.T) {
		// A fresh install: the directory holds neither hooks.json nor the
		// sidecar, so a read that created either would show up as a new entry.
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		_, hooksFile := hookstest.StageStore(t, hookstest.Staging{SidecarAbsent: true})
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)
		configDir := filepath.Dir(hooksFile)
		withHooksDeps(t, HooksDeps{PaneLister: &loudPaneHookLister{t: t}})

		before := dirListing(t, configDir)
		if got := runHookList(t); got != "" {
			t.Errorf("output = %q, want empty", got)
		}
		if after := dirListing(t, configDir); !reflect.DeepEqual(before, after) {
			t.Errorf("config directory changed across hook list: before=%v after=%v", before, after)
		}
	})
}

func TestHydrateLookupDegradedRead(t *testing.T) {
	t.Run("it names the hydrate helper on a degraded lookup", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{
			hookstest.SubjectSeedA: "claude --resume abc",
		}})
		hookstest.HoldHooksSidecar(t, path)

		exec := &stubExecShell{}
		sink := logtest.Install(t)
		execLookedUpRegistration(hydrateCfg(t, hydrateCfgOpts{
			HookKey:   hookstest.SubjectSeedA,
			OpenFIFO:  unexpectedOpenFIFO(t),
			HookStore: store,
			ExecShell: exec.fn(),
		}))

		if !exec.called {
			t.Fatal("the hydrate helper never reached its exec — a busy lock must not drop the pane")
		}
		hookstest.AssertDegradedRead(t, sink, "hydrate")
	})
}

func TestSweepPreReadBound(t *testing.T) {
	t.Run("it degrades the sweep pre-read at the short bound", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 5*time.Second)
		short := hooks.SnapshotLockBoundForTest(t)

		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: `{"` + hookstest.ReapableSeedA + `": {"on-resume": "cmd-a"}}`})
		hookstest.HoldHooksSidecar(t, path)

		// An empty live set stands the cycle down after the pre-read, so the
		// elapsed time measures that read alone rather than CleanStale's own
		// exclusive acquire behind it.
		lister := &stubStaleSweepReader{rows: tokenRows()}
		sink := logtest.Install(t)

		start := time.Now()
		err := sweepErr(lister, store)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("hooksweep.Run: %v", err)
		}
		if elapsed >= time.Second {
			t.Fatalf("the pre-read spent %v under a held lock — it waited at the mutation bound, not the derived pre-read bound (%v)", elapsed, short)
		}
		if elapsed < short {
			t.Errorf("the sweep returned after %v — the pre-read did not wait out the %v short bound", elapsed, short)
		}
		hookstest.AssertDegradedRead(t, sink, "internal")
	})
}

// dirListing is the directory's entry names, so a test can assert a read-only
// command added nothing to it.
func dirListing(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
