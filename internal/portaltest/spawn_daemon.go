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
	"os"
	"os/exec"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// SpawnIsolatedDaemon launches a `portal state daemon` subprocess with
// the supplied envSlice plus a per-call PORTAL_STATE_DIR pinned to a
// fresh t.TempDir() and TMUX pinned to tmuxSocketPath (the caller's
// per-test tmuxtest socket — REQUIRED, fatal if empty). Returns the
// started *exec.Cmd and the orphan's stateDir. Registers a guaranteed
// Kill+reap cleanup via RegisterSubprocessCleanup so the subprocess
// cannot leak across tests.
//
// TMUX pinning is load-bearing isolation: the test process usually runs
// inside the developer's real tmux, and a daemon inheriting that TMUX
// (or falling back to the default socket) attaches to the REAL server —
// ticking against, and capture-paning, the developer's live sessions.
// Appending TMUX after envSlice wins (exec.Cmd env dedupe is last-wins),
// overriding IsolateStateForTest's poison entry.
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
func SpawnIsolatedDaemon(t *testing.T, envSlice []string, tmuxSocketPath string) (*exec.Cmd, string) {
	t.Helper()
	if tmuxSocketPath == "" {
		t.Fatalf("portaltest: SpawnIsolatedDaemon requires the test's tmux socket path — " +
			"an orphan without one would attach to the developer's real tmux server")
	}
	stateDir := t.TempDir()
	env := append([]string{}, envSlice...)
	env = append(env, "PORTAL_STATE_DIR="+stateDir)
	env = append(env, "TMUX="+tmuxSocketPath+",0,0")
	cmd := exec.Command("portal", "state", "daemon")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("portaltest: start isolated portal state daemon (stateDir=%s): %v", stateDir, err)
	}
	RegisterSubprocessCleanup(t, cmd)
	// Register with the daemon-pgrep sandbox so this test-spawned daemon is
	// visible to the test's state.PgrepPortalDaemons (and thus a legitimate
	// sweep target). Register both the state dir (respawn-immune, via its
	// daemon.pid) and the initial PID (belt). No-op in non-integration builds.
	state.RegisterSandboxStateDir(stateDir)
	state.RegisterSandboxDaemon(cmd.Process.Pid)
	// Cross-process: append the orphan's state dir to the sandbox registry
	// file (created by IsolateStateForTest) so subprocess enumerations —
	// e.g. a test-driven `portal list` bootstrap whose orphan sweep must
	// converge this orphan — can see it too. The file is re-read on every
	// enumeration, so appending after subprocess env construction is fine.
	appendSandboxRegistryDir(t, stateDir)
	return cmd, stateDir
}

// appendSandboxRegistryDir appends dir to the cross-process sandbox
// registry file named by state.SandboxRegistryEnv, when set (it is set by
// IsolateStateForTest via t.Setenv; the daemon-spawning convention requires
// that helper, so absence just means there is no registry to extend).
func appendSandboxRegistryDir(t *testing.T, dir string) {
	t.Helper()
	path := os.Getenv(state.SandboxRegistryEnv)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("portaltest: open sandbox registry for append: %v", err)
	}
	if _, err := f.WriteString(dir + "\n"); err != nil {
		_ = f.Close()
		t.Fatalf("portaltest: append sandbox registry: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("portaltest: close sandbox registry: %v", err)
	}
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
