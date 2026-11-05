package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// productionGate stages a state directory and a backend for the production
// gate, with this boot's latch and the zmx dir in use recorded, as a finished
// bootstrap leaves them, when latched, and the saver session up
// when saverUp. Nothing past the gate is staged: the bootstrap's exe seam
// panics under cmd's TestMain.
func productionGate(t *testing.T, latched, saverUp bool) (*backendtest.Fake, string) {
	t.Helper()
	resetBootstrapOnce(t)
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if latched {
		latch, err := runningLatch()
		if err != nil {
			t.Fatalf("read this boot's id: %v", err)
		}
		if err := state.WriteLatch(dir, latch); err != nil {
			t.Fatalf("write the latch: %v", err)
		}
		if err := state.RecordZmxDir(dir, zmx.CanonicalDir(zmx.DirFor(os.Environ()))); err != nil {
			t.Fatalf("record the zmx dir: %v", err)
		}
	}
	fake := backendtest.New()
	fake.SetReachEnv([]string{"ZMX_DIR=" + filepath.Join(t.TempDir(), "zmx")})
	if saverUp {
		fake.Add(backend.Session{Name: saver.SessionName})
	}
	withBackendDeps(t, BackendDeps{Backend: fake})
	return fake, dir
}

// everyOp is every call the Fake records.
var everyOp = []backendtest.Op{
	backendtest.OpList, backendtest.OpSession, backendtest.OpExists, backendtest.OpCreate, backendtest.OpAttachCommand,
	backendtest.OpKill, backendtest.OpDetach, backendtest.OpHistory, backendtest.OpLeaderEnv, backendtest.OpAttachClients,
}

func TestProductionGate_WarmPathAsksTheBackendOnlyWhetherTheSaverIsThere(t *testing.T) {
	fake, dir := productionGate(t, true, true)
	for _, op := range everyOp {
		if op != backendtest.OpExists {
			fake.Fail(op, fmt.Errorf("%s called on the warm path", op))
		}
	}

	runner, err := owedBootstrap(t.Context())

	if runner != nil || err != nil {
		t.Fatalf("owedBootstrap = %v, %v; want no bootstrap owed", runner, err)
	}
	if calls := fake.Calls(); len(calls) != 1 || calls[0].Op != backendtest.OpExists || calls[0].Name != saver.SessionName {
		t.Errorf("backend calls = %+v, want the one presence check of %s", calls, saver.SessionName)
	}
	if _, err := os.Stat(state.BootstrapLock(dir)); !os.IsNotExist(err) {
		t.Errorf("stat bootstrap.lock = %v; want it never taken on the warm path", err)
	}
}

// Decision 67: the restoring flag covers restore's creates alone. The latch,
// removed before a bootstrap's first change, records one unfinished.
func TestProductionGate_RaisedRestoringFlagOwesNoBootstrap(t *testing.T) {
	_, dir := productionGate(t, true, true)
	if err := state.Restoring.Set(dir); err != nil {
		t.Fatal(err)
	}

	if runner, err := owedBootstrap(t.Context()); runner != nil || err != nil {
		t.Errorf("owedBootstrap = %v, %v; want no bootstrap owed", runner, err)
	}
}

func TestProductionGate_WarmCommandRunsNoBootstrap(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"open"}} {
		t.Run("portal "+strings.Join(args, " "), func(t *testing.T) {
			fake, dir := productionGate(t, true, true)
			withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string) error {
				if deferredBootstrapFromContext(cmd) != nil {
					t.Error("the picker was handed a bootstrap to wait for; want it straight onto its list")
				}
				return nil
			})

			if _, _, err := runRootCmd(t, args...); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if exists := fake.CallsTo(backendtest.OpExists); len(exists) != 1 || exists[0].Name != saver.SessionName {
				t.Errorf("Exists calls = %+v, want the one presence check of %s", exists, saver.SessionName)
			}
			if _, err := os.Stat(state.BootstrapLock(dir)); !os.IsNotExist(err) {
				t.Errorf("stat bootstrap.lock = %v; want it never taken on the warm path", err)
			}
		})
	}
}

// A cold gate reaches the bootstrap's exe seam, which cmd's TestMain makes
// panic, so no cmd test can run the real bootstrap by leaving its seams
// unstaged.
func TestProductionGate_ColdCommandReachesTheBootstrap(t *testing.T) {
	cases := []struct {
		name             string
		latched, saverUp bool
	}{
		{name: "the latch owed", latched: false, saverUp: true},
		{name: "the saver missing", latched: true, saverUp: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			productionGate(t, tt.latched, tt.saverUp)

			if got := panicOf(func() { _, _, _ = runRootCmd(t, "list") }); !strings.Contains(got, "bootstrapExe reached") {
				t.Errorf("portal list panicked with %q; want the bootstrap's exe seam reached", got)
			}
		})
	}
}

func TestBootstrapExe_PanicsInCmdTests(t *testing.T) {
	if got := panicOf(func() { _, _ = bootstrapExe() }); !strings.Contains(got, "bootstrapExe reached without withBootstrapDeps") {
		t.Errorf("bootstrapExe panicked with %q; want TestMain's poison", got)
	}
}

// panicOf is what f panics with, empty when it returns.
func panicOf(f func()) (got string) {
	defer func() {
		if r := recover(); r != nil {
			got = fmt.Sprint(r)
		}
	}()
	f()
	return ""
}
