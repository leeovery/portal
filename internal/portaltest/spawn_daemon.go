package portaltest

// spawn_daemon.go — test-only helpers for spawning isolated
// `portal state daemon` subprocesses with a guaranteed reap on test
// exit.
//
// Consolidates the previously-duplicated `spawnOrphanDaemonIsolated`
// (cmd/bootstrap/composition_e2e_harness_integration_test.go) and
// `registerSubprocessCleanup` (cmd/bootstrap/orphan_sweep_integration_test.go)
// helpers, plus the line-for-line equivalent reap blocks formerly
// inlined in internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go
// and internal/tmux/portal_saver_endstate_integration_test.go. The
// `bootstrap_test` helpers could not be imported from the
// `internal/tmux` _test packages, forcing the verbatim copies the spec
// calls out — promoting to portaltest collapses the duplication.
//
// Test-only. The *testing.T first parameter enforces test-only usage
// structurally: production code cannot import this package without
// dragging in the testing stdlib package, which would fail
// `go build .` for the main binary.

import (
	"os/exec"
	"testing"
)

// SpawnIsolatedDaemon launches a `portal state daemon` subprocess with
// the supplied envSlice plus a per-call PORTAL_STATE_DIR pinned to a
// fresh t.TempDir(). Returns the started *exec.Cmd and the orphan's
// stateDir. Registers a guaranteed Kill+reap cleanup via
// RegisterSubprocessCleanup so the subprocess cannot leak across
// tests.
//
// The unqualified `"portal"` argv[0] is LOAD-BEARING. Darwin's ps
// reports argv[0] as `comm` (truncated to 15 chars), and
// state.IdentifyDaemon requires comm == "portal" exactly. Spawning
// via an absolute path would set comm to the path basename and the
// identity check would classify the daemon as IdentifyNotPortalDaemon.
// StagePortalBinary (in portalbintest) PATH-prepends the binary
// directory, so exec.Command resolves the bare name via the parent
// PATH which the caller inherits into envSlice.
//
// Each call gets its own PORTAL_STATE_DIR so multiple SpawnIsolatedDaemon
// calls within a single test can coexist without colliding on
// daemon.lock / daemon.pid — Component A scenarios that spawn N
// orphans alongside the legitimate saver-pane daemon rely on this.
// pgrep is system-wide and argv-anchored, so the orphans still appear
// in `pgrep -fx '^portal state daemon( |$)'` alongside any
// saver-pane daemon for the test's tmux server. Callers that want
// multiple daemons in a SHARED stateDir (the upgrade-path scenario,
// where v(N) writes the same daemon.pid the v(N+1) saver later
// overwrites) should NOT use this helper — spawn directly so the
// stateDir is controlled by the caller, then wrap the *exec.Cmd in
// RegisterSubprocessCleanup.
func SpawnIsolatedDaemon(t *testing.T, envSlice []string) (*exec.Cmd, string) {
	t.Helper()
	stateDir := t.TempDir()
	env := append([]string{}, envSlice...)
	env = append(env, "PORTAL_STATE_DIR="+stateDir)
	cmd := exec.Command("portal", "state", "daemon")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("portaltest: start isolated portal state daemon (stateDir=%s): %v", stateDir, err)
	}
	RegisterSubprocessCleanup(t, cmd)
	return cmd, stateDir
}

// RegisterSubprocessCleanup arranges a guaranteed SIGKILL + reap for
// the supplied command on test exit so spawned subprocesses cannot
// leak across tests. Returns a channel that closes when the reaper
// goroutine observes the process exit — callers that need to time
// process death (e.g. Component A's settle-window check) can select
// on it.
//
// The reaper goroutine and the t.Cleanup hook coordinate via the
// returned channel so calling Process.Wait directly from cleanup
// (which would be a second concurrent Wait on the same Process) is
// structurally avoided. Without the reaper an unreaped child stays
// kernel-resident as a zombie after SIGKILL — kill(pid, 0) returns 0
// (not ESRCH) until reaping completes, which would deadlock any ESRCH
// poll against the OS lifecycle rules.
//
// SIGKILL is belt-and-braces — the only signal guaranteed to terminate
// a daemon that may already be in its osExit path or otherwise
// unresponsive. Errors are swallowed because the typical case at
// cleanup time is "process already exited".
func RegisterSubprocessCleanup(t *testing.T, cmd *exec.Cmd) <-chan struct{} {
	t.Helper()
	reaped := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(reaped)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-reaped
	})
	return reaped
}
