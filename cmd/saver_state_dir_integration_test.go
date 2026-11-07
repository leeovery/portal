//go:build integration

package cmd_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

// newOtherState is a second state directory for a test's zmx directory,
// guarded at teardown against the saver a Portal run against it leaves
// writing there. It precedes newBootWorld, so its guard runs after the zmx
// directory's cleanup ends that saver.
func newOtherState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	portaltest.RegisterStateDirTeardownGuard(t, dir)
	return dir
}

// withState is w with its portal run against stateDir instead: the same zmx
// directory, another Portal's state.
func (w *bootWorld) withState(stateDir string) *bootWorld {
	dir := w.dir
	dir.StateDir = stateDir
	return &bootWorld{binary: w.binary, dir: dir, env: append(slices.Clone(w.env), xdg.StateDir.EnvVar+"="+stateDir)}
}

// refused runs portal with args, which must fail on the saver of other, a
// state directory not w's, and returns what it printed.
func (w *bootWorld) refused(t *testing.T, other string, args ...string) string {
	t.Helper()
	out, err := w.command(args...).CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() == 0 {
		t.Fatalf("portal %s = %v; want it refused\n%s\n--- portal.log ---\n%s", strings.Join(args, " "), err, out, w.log())
	}
	if want := fmt.Sprintf(bootstrap.OtherStateDirMessage, other, w.dir.StateDir); !strings.Contains(string(out), want) {
		t.Errorf("portal %s printed %q, want it to contain %q", strings.Join(args, " "), out, want)
	}
	return string(out)
}

// stateFiles is stateDir's daemon.pid, latch and zmx-dir record, as read now.
func stateFiles(t *testing.T, stateDir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, path := range []string{state.DaemonPID(stateDir), state.Bootstrapped(stateDir), state.ZmxDirRecordPath(stateDir)} {
		data, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			files[path] = "(none)"
		case err != nil:
			t.Fatal(err)
		default:
			files[path] = string(data)
		}
	}
	return files
}

// assertSaving checks the saver session still runs pid, the daemon that
// stateDir's daemon.pid records.
func (w *bootWorld) assertSaving(t *testing.T, pid int, stateDir string) {
	t.Helper()
	if got := w.saverPID(t); got != pid {
		t.Errorf("the saver runs pid %d, want %d left running", got, pid)
	}
	if err := zmxtest.Signal(w.dir.Backend, pid, 0); err != nil {
		t.Errorf("the saver's daemon %d: %v, want it running", pid, err)
	}
	if recorded, err := state.ReadPIDFile(stateDir); recorded != pid || err != nil {
		t.Errorf("daemon.pid in %s = %d, %v; want %d", stateDir, recorded, err, pid)
	}
}

// The reproduction: a Portal with a state directory of its own and the zmx
// directory of another's running saver is refused with nothing sent and
// nothing changed, and its own ZMX_DIR, the way out the refusal names, then
// runs.
func TestAnotherStateDirsSaverIsRefusedAgainstZmx(t *testing.T) {
	scratch := newOtherState(t)
	w := newBootWorld(t)
	w.portal(t, "list", "--short")
	pid := w.saverPID(t)
	before := stateFiles(t, w.dir.StateDir)
	b := w.withState(scratch)

	b.refused(t, w.dir.StateDir, "list", "--short")

	w.assertSaving(t, pid, w.dir.StateDir)
	if after := stateFiles(t, w.dir.StateDir); !maps.Equal(after, before) {
		t.Errorf("state files = %v, want them as they were, %v", after, before)
	}
	for path, got := range stateFiles(t, scratch) {
		if got != "(none)" {
			t.Errorf("%s = %q, want nothing written: the refused Portal changes nothing", path, got)
		}
	}

	own := zmxtest.New(t)
	b.env = append(b.env, own.ReachEnv()...)
	b.portal(t, "list", "--short")
	if held, err := own.Exists(t.Context(), saver.SessionName); err != nil || !held {
		t.Errorf("Exists(%s) in its own zmx dir = %v, %v; want the scratch Portal's saver there\n--- portal.log ---\n%s", saver.SessionName, held, err, b.log())
	}
	w.assertSaving(t, pid, w.dir.StateDir)
}

// The reverse: another state directory's saver comes up first, so this one's
// first command is refused, and that saver goes on saving. Ended, as the
// refusal says to once that state is gone, it leaves this one to bring up
// its own.
func TestStateDirWhoseZmxDirAnotherSavesIsRefusedAgainstZmx(t *testing.T) {
	first := newOtherState(t)
	w := newBootWorld(t)
	b := w.withState(first)
	b.portal(t, "list", "--short")
	pid := b.saverPID(t)

	w.refused(t, first, "list", "--short")

	b.assertSaving(t, pid, first)
	for path, got := range stateFiles(t, w.dir.StateDir) {
		if got != "(none)" {
			t.Errorf("%s = %q, want nothing written: the refused Portal changes nothing", path, got)
		}
	}

	b.endSaver(t)
	w.portal(t, "list", "--short")
	if own := w.saverPID(t); own == pid {
		t.Errorf("the saver runs pid %d, the other state's ended daemon", own)
	}
}

// The warm check: a current latch counts a saver warm only when it runs the
// daemon this state's daemon.pid records, so a command after another state's
// saver replaced this one's is refused rather than run warm on it.
func TestWarmCommandOnAnotherStateDirsSaverIsRefusedAgainstZmx(t *testing.T) {
	other := newOtherState(t)
	w := newBootWorld(t)
	w.portal(t, "list", "--short")
	w.endSaver(t)
	b := w.withState(other)
	b.portal(t, "list", "--short")
	pid := b.saverPID(t)
	before := stateFiles(t, w.dir.StateDir)

	w.refused(t, other, "list", "--short")

	b.assertSaving(t, pid, other)
	if after := stateFiles(t, w.dir.StateDir); !maps.Equal(after, before) {
		t.Errorf("state files = %v, want them as they were, %v, the latch kept", after, before)
	}
}

// uninstall against another state directory's saver refuses in the
// bootstrap's terms and sends it nothing.
func TestUninstallRefusesAnotherStateDirsSaverAgainstZmx(t *testing.T) {
	scratch := newOtherState(t)
	w := newBootWorld(t)
	w.portal(t, "list", "--short")
	pid := w.saverPID(t)

	out := w.withState(scratch).refused(t, w.dir.StateDir, "uninstall")

	if want := fmt.Sprintf(bootstrap.OtherStateDirMessage, w.dir.StateDir, scratch) + "\n"; out != want {
		t.Errorf("output = %q, want the bootstrap's refusal alone, %q", out, want)
	}
	w.assertSaving(t, pid, w.dir.StateDir)
	if shutDownThroughSignal(w.log(), pid) {
		t.Errorf("portal.log:\n%s\nwant no shutdown of daemon %d", w.log(), pid)
	}
}
