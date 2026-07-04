//go:build integration

package state

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestPgrepSandbox_ExcludesUnregisteredPID is the load-bearing safety proof:
// while the sandbox is enabled, an UNREGISTERED pid (the developer's live
// daemon, or anything the test did not spawn) is dropped from the enumeration —
// so the orphan sweep, which SIGKILLs only what PgrepPortalDaemons returns, can
// never target it. Also verifies every ownership signal (explicit pid, state-dir
// daemon.pid, live source) surfaces its pid, and that a disabled sandbox is a
// pass-through (production parity).
func TestPgrepSandbox_ExcludesUnregisteredPID(t *testing.T) {
	t.Cleanup(ResetDaemonSandbox)

	const foreign = 999001 // stands in for the developer's real daemon
	const ownedPID = 999002
	const dirPID = 999003
	const srcPID = 999004

	// Disabled → identity pass-through (matches production behaviour exactly).
	ResetDaemonSandbox()
	if got := sandboxFilterPgrep([]int{foreign, ownedPID}); len(got) != 2 {
		t.Fatalf("disabled sandbox must be pass-through; got %v", got)
	}

	EnableDaemonSandbox()

	// Ownership signal 1: explicit pid.
	RegisterSandboxDaemon(ownedPID)

	// Ownership signal 2: current daemon.pid of a registered state dir.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte(strconv.Itoa(dirPID)+"\n"), 0o600); err != nil {
		t.Fatalf("seed daemon.pid: %v", err)
	}
	RegisterSandboxStateDir(dir)

	// Ownership signal 3: a live source callback (models the _portal-saver
	// pane_pid reader — respawn- and daemon.pid-manipulation-immune).
	RegisterSandboxDaemonSource(func() (int, bool) { return srcPID, true })

	got := sandboxFilterPgrep([]int{foreign, ownedPID, dirPID, srcPID})

	owned := map[int]bool{}
	for _, p := range got {
		owned[p] = true
	}
	if owned[foreign] {
		t.Fatalf("SANDBOX BREACH: unregistered pid %d survived the filter — the sweep could SIGKILL it. got=%v", foreign, got)
	}
	for _, p := range []int{ownedPID, dirPID, srcPID} {
		if !owned[p] {
			t.Errorf("owned pid %d was wrongly dropped (ownership signal broken); got=%v", p, got)
		}
	}

	// Respawn/manipulation immunity: overwrite daemon.pid with a DIFFERENT value
	// (as the PreFixDysfunction harness does) — the state-dir signal must track
	// the NEW value, and the source still owns srcPID regardless.
	if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte("999009\n"), 0o600); err != nil {
		t.Fatalf("rewrite daemon.pid: %v", err)
	}
	got2 := owns(sandboxFilterPgrep([]int{foreign, 999009, srcPID}))
	if got2[foreign] {
		t.Fatalf("SANDBOX BREACH after daemon.pid rewrite: foreign %d survived; got=%v", foreign, got2)
	}
	if !got2[999009] || !got2[srcPID] {
		t.Errorf("post-rewrite ownership lost: want 999009 and %d owned; got=%v", srcPID, got2)
	}
}

func owns(pids []int) map[int]bool {
	m := map[int]bool{}
	for _, p := range pids {
		m[p] = true
	}
	return m
}
