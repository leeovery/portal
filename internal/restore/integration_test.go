package restore_test

// Phase 3 integration tests exercise the end-to-end skeleton-restore pipeline
// against a real tmux server. Each test runs an isolated tmux instance via
// `tmux -S <unique-socket-path>` rooted in a per-test scratch dir, so the
// user's tmux is never touched and concurrent test runs cannot collide.
//
// Tests are gated on `tmux` being present on PATH; if not, they skip cleanly
// rather than fail. Heavy end-to-end paths (the 3-second hydrate timeout, the
// scrollback-file-missing branch) are intentionally skipped here — they have
// dedicated unit tests at the handler level and offer no incremental coverage
// at this scope. See the task brief for the rationale.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// skipIfNoTmux skips the test if tmux is not available. Integration tests
// require a real tmux binary on PATH.
func skipIfNoTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available; skipping integration test")
	}
}

// tmuxSocket gives an isolated tmux server scoped to t. The server uses
// `tmux -S <abs-socket-path>` rooted in /tmp so the path stays inside the
// platform's UNIX-socket length cap (104 bytes on darwin, 108 on linux), and
// the user's tmux server is never touched.
//
// Why not -L + TMUX_TMPDIR=t.TempDir()? Go's t.TempDir on darwin lives under
// /private/var/folders/... and tmux composes a socket path of
// $TMUX_TMPDIR/tmux-$UID/<L>. That blows past 104 bytes for any non-trivial
// test name. A short absolute -S path is the simplest robust workaround.
type tmuxSocket struct {
	socketPath string
}

// newTmuxSocket constructs a tmuxSocket and registers a t.Cleanup that issues
// kill-server on the isolated socket. The cleanup runs even when a test
// panics so a stray tmux server is never left behind.
func newTmuxSocket(t *testing.T) *tmuxSocket {
	t.Helper()
	dir, err := os.MkdirTemp("", "ptl-")
	if err != nil {
		t.Fatalf("mkdir temp socket dir: %v", err)
	}
	socketPath := filepath.Join(dir, "s")
	ts := &tmuxSocket{socketPath: socketPath}
	t.Cleanup(func() {
		ts.killServer()
		_ = os.RemoveAll(dir)
	})
	return ts
}

// tmuxCmd builds a *exec.Cmd targetting the isolated socket via -S. The
// `-f /dev/null` flag suppresses the user's ~/.tmux.conf so tests run against
// vanilla tmux defaults (notably base-index 0 and pane-base-index 0). tmux
// only acts on -f when starting a new server, but specifying it on every
// invocation is harmless and keeps the helper symmetric.
func (ts *tmuxSocket) tmuxCmd(args ...string) *exec.Cmd {
	base := []string{"-S", ts.socketPath, "-f", "/dev/null"}
	base = append(base, args...)
	return exec.Command("tmux", base...)
}

// run executes a tmux command on the isolated socket, fatalling the test on
// failure. Returns combined stdout+stderr verbatim.
func (ts *tmuxSocket) run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := ts.tmuxCmd(args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// tryRun executes a tmux command on the isolated socket and returns the
// combined output along with any exec error, leaving error handling to the
// caller. Used to drive negative paths ("expect this to fail").
func (ts *tmuxSocket) tryRun(args ...string) (string, error) {
	out, err := ts.tmuxCmd(args...).CombinedOutput()
	return string(out), err
}

// killServer tears down the isolated tmux server. Errors are intentionally
// ignored: the typical case is "server already dead from a prior kill-server
// in the test body," which is healthy.
func (ts *tmuxSocket) killServer() {
	_, _ = ts.tmuxCmd("kill-server").CombinedOutput()
}

// socketCommander wraps each tmux invocation with `-S <socketPath>` so a
// *tmux.Client built from it talks only to the test's isolated server. It
// satisfies tmux.Commander and is the production code's natural seam for
// pointing at a non-default socket.
type socketCommander struct {
	socketPath string
}

// Run executes tmux on the isolated socket and trims surrounding whitespace,
// matching tmux.RealCommander.Run. -f /dev/null mirrors tmuxSocket.tmuxCmd:
// tests must not pick up the developer's ~/.tmux.conf (notably base-index).
func (sc *socketCommander) Run(args ...string) (string, error) {
	full := append([]string{"-S", sc.socketPath, "-f", "/dev/null"}, args...)
	cmd := exec.Command("tmux", full...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RunRaw executes tmux on the isolated socket and returns its output verbatim,
// matching tmux.RealCommander.RunRaw.
func (sc *socketCommander) RunRaw(args ...string) (string, error) {
	full := append([]string{"-S", sc.socketPath, "-f", "/dev/null"}, args...)
	cmd := exec.Command("tmux", full...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// client returns a *tmux.Client wired to the socketCommander for ts.
func (ts *tmuxSocket) client() *tmux.Client {
	return tmux.NewClient(&socketCommander{socketPath: ts.socketPath})
}

// restoreWithMarker drives the bootstrap-owned @portal-restoring lifecycle
// inline for these integration tests: set the marker, run Restore(), unset
// the marker. Bootstrap.Orchestrator (cmd/bootstrap) owns this discipline in
// production; the helper exists only so the Phase-3 integration tests can
// exercise the same set/clear contract without re-implementing the marker
// API inside internal/restore. The clear is registered via defer so it runs
// on every exit path — including when Restore returns an error.
func restoreWithMarker(t *testing.T, client *tmux.Client, o *restore.Orchestrator) error {
	t.Helper()
	if err := client.SetServerOption(state.RestoringMarkerName, "1"); err != nil {
		return err
	}
	defer func() {
		if err := client.UnsetServerOption(state.RestoringMarkerName); err != nil {
			t.Logf("UnsetServerOption(%s): %v", state.RestoringMarkerName, err)
		}
	}()
	return o.Restore()
}

// waitForSession polls list-sessions until the target name appears or the
// deadline elapses. tmux's new-session is synchronous from the caller's POV
// but on slow CI systems a brief settle window has been observed before the
// session is queryable; the poll is cheaper than a flat sleep and short-
// circuits as soon as the session is visible.
func (ts *tmuxSocket) waitForSession(t *testing.T, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := ts.tryRun("has-session", "-t", name)
		if err == nil {
			return
		}
		_ = out
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session %q did not appear within %s", name, timeout)
}

// TestPhase3Integration_SaveRestoreRoundTrip is the headline smoke test: it
// captures a single live session, kills the tmux server, restores from the
// persisted index against a fresh server, and asserts the saved session is
// recreated with its skeleton marker set. A second Restore() call must be a
// silent no-op (live-session skip) so the test guards against double-creates.
func TestPhase3Integration_SaveRestoreRoundTrip(t *testing.T) {
	skipIfNoTmux(t)

	ts := newTmuxSocket(t)
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Stand up a single saved session with one window and one pane.
	ts.run(t, "new-session", "-d", "-s", "alpha")
	ts.waitForSession(t, "alpha", 2*time.Second)

	client := ts.client()

	// CAPTURE.
	idx, err := state.CaptureStructure(client, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "alpha" {
		t.Fatalf("expected one captured session named alpha, got %+v", idx.Sessions)
	}

	// PERSIST.
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := writeFile(state.SessionsJSON(stateDir), data); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	// KILL the server so tmux loses the live session entirely.
	ts.killServer()
	if _, err := ts.tryRun("list-sessions"); err == nil {
		t.Fatalf("expected list-sessions to error after kill-server")
	}

	// EnsureServer mirrors what bootstrap's PersistentPreRunE does: a fresh
	// tmux server is brought up before any set-option call, which itself does
	// not auto-start a server. The orchestrator assumes a live server.
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// RESTORE against the freshly-started server.
	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	// VERIFY: alpha is alive again.
	out := ts.run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected alpha in list-sessions; got %q", out)
	}

	// VERIFY: skeleton marker was set for alpha's single pane.
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("alpha", 0, 0)
	markerOut := ts.run(t, "show-options", "-sv", wantMarker)
	if strings.TrimSpace(markerOut) == "" {
		t.Errorf("expected marker %q to be set; got empty value", wantMarker)
	}

	// VERIFY: @portal-restoring was cleared after the marker block exited.
	if out, err := ts.tryRun("show-options", "-sv", state.RestoringMarkerName); err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("%s should be unset after marker block; got %q", state.RestoringMarkerName, out)
	}

	// VERIFY: re-running Restore is a silent no-op (live-session skip).
	if err := o.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}
	out2 := ts.run(t, "list-sessions", "-F", "#{session_name}")
	// Count occurrences of "alpha" — must remain exactly one.
	if got := strings.Count(out2, "alpha"); got != 1 {
		t.Errorf("expected exactly one alpha session after second Restore; got %d in %q", got, out2)
	}
}

// TestPhase3Integration_SweepOrphanFIFOs verifies the FIFO sweep against a
// fresh state directory: an orphan FIFO is removed, a live one is preserved.
// Sweep does not strictly require tmux but the test gate is kept for
// consistency with the rest of the integration suite.
func TestPhase3Integration_SweepOrphanFIFOs(t *testing.T) {
	skipIfNoTmux(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	liveKey := state.SanitizePaneKey("alpha", 0, 0)
	orphanKey := state.SanitizePaneKey("ghost", 0, 0)

	live := state.FIFOPath(stateDir, liveKey)
	orphan := state.FIFOPath(stateDir, orphanKey)
	if err := state.CreateFIFO(live); err != nil {
		t.Fatalf("CreateFIFO live: %v", err)
	}
	if err := state.CreateFIFO(orphan); err != nil {
		t.Fatalf("CreateFIFO orphan: %v", err)
	}

	liveSet := map[string]struct{}{liveKey: {}}
	if err := state.SweepOrphanFIFOs(stateDir, liveSet, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	if !pathExists(live) {
		t.Errorf("live FIFO %s was unexpectedly removed", live)
	}
	if pathExists(orphan) {
		t.Errorf("orphan FIFO %s was not removed", orphan)
	}
}

// TestPhase3Integration_CorruptSessionsJSON wires real tmux up to an
// orchestrator pointed at a corrupt sessions.json. Restore() must return
// a wrapped state.ErrCorruptIndex, log a WARN, and not create any
// sessions on the live server. (The user-facing stderr warning emission
// moved to cmd/bootstrap_warnings.go in Phase 6 task 6-9.)
func TestPhase3Integration_CorruptSessionsJSON(t *testing.T) {
	skipIfNoTmux(t)

	ts := newTmuxSocket(t)
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Garbage sessions.json drives ReadIndex's skip-with-error path.
	if err := writeFile(state.SessionsJSON(stateDir), []byte("{not json")); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	client := ts.client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	rwmErr := restoreWithMarker(t, client, o)
	if rwmErr == nil {
		t.Fatal("restoreWithMarker returned nil; expected wrapped state.ErrCorruptIndex")
	}
	if !errors.Is(rwmErr, state.ErrCorruptIndex) {
		t.Errorf("restoreWithMarker err = %v; want errors.Is(err, state.ErrCorruptIndex) = true", rwmErr)
	}

	// No user-visible sessions should have been created. The orchestrator
	// auto-starts a tmux server when it sets @portal-restoring, but no
	// new-session call should follow the corrupt-index abort, so any session
	// present must be tmux's own bootstrap session ("0"), not "alpha" or
	// similar saved names.
	out, err := ts.tryRun("list-sessions", "-F", "#{session_name}")
	if err == nil {
		// If tmux did auto-start a server it will list at most a default "0"
		// bootstrap session. Anything else means restore created a session.
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "0" {
				continue
			}
			t.Errorf("unexpected session %q after corrupt-index restore; out=%q", line, out)
		}
	}
}

// TestPhase3Integration_BaseIndexDrift exercises PredictLiveIndices against a
// real tmux server that has been configured with non-default base-index and
// pane-base-index. The orchestrator's prediction path must read the live
// values rather than assume zero. Setting up a saved-state restoration with
// base-index=1 is structurally complex (it would require either authoring a
// hand-built sessions.json or capturing under a base-index=1 server); the
// pragmatic shape adopted here is to validate the prediction primitive
// directly — Restore/ApplyWindowGeometry/ApplySkeletonMarkers all consume its
// output, so a correct prediction is the load-bearing precondition.
func TestPhase3Integration_BaseIndexDrift(t *testing.T) {
	skipIfNoTmux(t)

	ts := newTmuxSocket(t)
	// A bootstrap session is required to keep the server alive long enough to
	// set the global options; tmux exits with no sessions present.
	ts.run(t, "new-session", "-d", "-s", "_bootstrap")
	ts.waitForSession(t, "_bootstrap", 2*time.Second)

	ts.run(t, "set-option", "-g", "base-index", "1")
	ts.run(t, "set-option", "-g", "pane-base-index", "1")
	// Server-scope copies — tmux's `show-option -sv` reads these.
	ts.run(t, "set-option", "-s", "base-index", "1")
	ts.run(t, "set-option", "-s", "pane-base-index", "1")

	client := ts.client()
	r := &restore.SessionRestorer{Client: client, StateDir: t.TempDir()}

	base, paneBase := r.PredictLiveIndices()
	if base != 1 {
		t.Errorf("base-index = %d, want 1", base)
	}
	if paneBase != 1 {
		t.Errorf("pane-base-index = %d, want 1", paneBase)
	}
}

// writeFile is a thin wrapper that pins the file mode for state-directory
// writes (sessions.json is mode 0600 on production paths) so the test stays
// faithful to the on-disk semantics.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

// pathExists reports whether path exists. Returns false on any stat error
// (including ENOENT and EACCES) — callers that need to distinguish the two
// should call os.Lstat directly.
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
