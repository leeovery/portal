//go:build integration

// Composite end-to-end test harness for spec § "Composite End-to-End
// Verification" — task 6-1.
//
// This file builds the reusable scaffold consumed by tasks 6-2..6-6 to
// assert ship-readiness across the full A+B+C+E+F composition. The
// harness itself is the deliverable; the only assertion test in this
// file is TestCompositeHarness_PreState, which proves the harness
// produces the documented preconditions before any bootstrap fires:
//
//   - 3 live `portal state daemon` processes (pgrep -fx == 3).
//   - daemon.pid in the LEGITIMATE stateDir references orphan 1's PID
//     (NOT the saver-pane daemon's PID, NOT orphan 2's PID) —
//     simulating the reporter's "orphan-with-daemon.pid" case.
//   - Both orphan parent processes differ from the saver pane process.
//   - 2 user sessions on the isolated tmux server with non-trivial
//     pane output that the daemon's capture loop can later read.
//
// The downstream assertion tests in 6-2..6-6 consume the harness via
// setupCompositeHarness(t) and run their own post-bootstrap assertions
// against the converged healthy end-state. This file deliberately does
// NOT invoke the bootstrap pipeline — that is each consumer's
// responsibility.
//
// Shared scaffolding reused from composition_abc / orphan_sweep /
// upgrade_path tests in this same _test package:
//   - skipIfNoPgrep, registerSubprocessCleanup
//   - waitForSaverPanePID, waitForDaemonPID, waitForPgrepCount
//   - portaltest.PgrepPortalDaemons, pidAlive
//
// New scaffolding introduced here (specific to the 6-x consumer shape):
//   - compositeHarness struct
//   - setupCompositeHarness function
//   - spawnOrphanDaemonIsolated (returns the orphan's stateDir too,
//     since 6-1 needs to poll the orphan's own daemon.pid before
//     overwriting the legitimate one)
//   - seedUserSession (creates a user session with a small recurring
//     output script in pane 0)
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// compositeUserSessionCount is the number of user sessions seeded onto
// the isolated tmux server before bootstrap. Spec § "Composite End-to-End
// Verification" calls out "_portal-saver plus some user sessions" —
// two is the minimum that exercises multi-session capture without
// inflating fixture cost.
const compositeUserSessionCount = 2

// compositeUserSessionPrefix is the name stem for seeded user sessions.
// Numbered uX so they sort deterministically and are distinguishable
// from any test-fixture leakage on the host.
const compositeUserSessionPrefix = "u"

// compositeUserSessionSeedScript is the recurring-output command run as
// the initial process of each seeded user-session pane. The 100 ms cadence
// produces enough scrollback for downstream consumers to assert non-empty
// captures without flooding the pane buffer. `sh -c 'while sleep 0.1; do
// echo "hello $RANDOM"; done'` matches the task brief's example verbatim.
const compositeUserSessionSeedScript = `while sleep 0.1; do echo "hello $RANDOM"; done`

// compositePreStatePGrepTimeout bounds the pre-assertion poll waiting
// for pgrep to observe N=3 daemons. 3 s mirrors the composition_abc
// barrier — comfortably above the observed daemon-cold-start latency
// (~hundreds of ms × 3 daemons).
const compositePreStatePGrepTimeout = 3 * time.Second

// compositeHarness exposes the state the downstream 6-2..6-6 consumer
// tests need: env slice for spawning further isolated subprocesses, the
// legitimate stateDir under test, the tmux socket + client, and the
// three daemon PIDs (legitimate saver-pane plus the two orphans).
//
// User-session names are exposed so consumers can target specific
// sessions for scrollback or kill assertions. The slice ordering
// matches creation order (u0, u1, ...).
type compositeHarness struct {
	// Env is the filtered env slice from portaltest.NewIsolatedStateEnv,
	// suitable for further isolated subprocess spawns (e.g. a
	// `portal open` invocation in 6-x).
	Env []string

	// StateDir is the LEGITIMATE state directory — the one the saver
	// pane's daemon writes to, and the one downstream assertions
	// inspect for sessions.json / scrollback / daemon.pid invariants.
	StateDir string

	// Sock is the isolated tmux socket fixture. Cleanup is owned by
	// tmuxtest.New's t.Cleanup; consumers must NOT call KillServer.
	Sock *tmuxtest.Socket

	// Client is a *tmux.Client wired to Sock for in-process tmux ops.
	Client *tmux.Client

	// LegitimateDaemonPID is the PID of the saver-pane daemon
	// (pane_pid of the `_portal-saver` session at harness setup time).
	// Re-read post-bootstrap by consumers if the saver may have been
	// respawned.
	LegitimateDaemonPID int

	// Orphan1PID is the PID of the orphan daemon whose value is written
	// into the LEGITIMATE stateDir's daemon.pid as part of harness
	// setup — simulating the reporter's "orphan with daemon.pid
	// reference" case.
	Orphan1PID int

	// Orphan2PID is the PID of the orphan daemon that does NOT have
	// its PID propagated into the legitimate stateDir — present to
	// reach N=3 daemons and verify Component B's pgrep sweep covers
	// both orphans.
	Orphan2PID int

	// UserSessionNames lists the names of the seeded user sessions on
	// the isolated tmux server, in creation order.
	UserSessionNames []string
}

// setupCompositeHarness reconstructs the reporter's three-daemon failure
// scenario and returns a compositeHarness for downstream assertion tests
// to consume. See the file-header comment for the harness contract and
// the call-site comments below for per-step rationale.
//
// Teardown is registered via t.Cleanup (LIFO):
//   - tmuxtest.New registers tmux kill-server (runs LAST per LIFO).
//   - portaltest.NewIsolatedStateEnv registers the fingerprint backstop.
//   - registerSubprocessCleanup arranges SIGKILL+Wait for each orphan
//     subprocess (runs BEFORE tmux teardown so orphans are reaped while
//     the test environment is still intact).
func setupCompositeHarness(t *testing.T) *compositeHarness {
	t.Helper()

	// Step 1: skip guards + binary staging. These must run before any
	// subprocess spawn so the test cleanly skips on minimal-container
	// CI rather than failing inside a spawn.
	//
	// Ordering note: portalbintest.StagePortalBinary runs `go build`,
	// which resolves the Go module cache from $HOME/go/pkg/mod when
	// GOMODCACHE is unset. portaltest.NewIsolatedStateEnv re-points
	// HOME at a fresh t.TempDir internally (as part of its folded-in
	// host-noise scrub), so staging BEFORE NewIsolatedStateEnv keeps
	// `go build` against the developer's real (writable, populated)
	// module cache. Staging AFTER would populate a read-only module
	// cache under the tempdir that t.TempDir cleanup cannot remove.
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	// Step 2: isolated env + stateDir. NewIsolatedStateEnv folds in
	// the HOME=<tempdir> / XDG_CONFIG_HOME="" host-noise scrub before
	// snapshotting the dev state dir. The portaltest backstop fires
	// post-test to catch any leakage from the spawned daemons.
	envSlice, stateDir := portaltest.NewIsolatedStateEnv(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// Step 3: isolated tmux server + client. The server inherits the
	// test process's env (including PORTAL_STATE_DIR + XDG_CONFIG_HOME)
	// via the standard exec.Command env inheritance.
	sock := tmuxtest.New(t, "ptl-comp-e2e-")
	client := sock.Client()

	// Step 4: seed user sessions with non-trivial recurring output.
	// These are the "user sessions" the spec references — the daemon's
	// capture loop will enumerate them and persist scrollback for the
	// preview-renders-non-empty assertion in 6-x consumers.
	userSessionNames := seedUserSessions(t, client, compositeUserSessionCount)

	// Step 5: bootstrap the legitimate _portal-saver session. After
	// this returns, the saver-pane process IS the legitimate daemon
	// and writes daemon.pid in the legitimate stateDir.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (legitimate saver): %v", err)
	}
	legitimateDaemonPID := waitForSaverPanePID(t, sock)
	waitForDaemonPID(t, stateDir, legitimateDaemonPID)

	// Step 6: spawn orphan 1 with its OWN PORTAL_STATE_DIR so it does
	// not immediately lose the daemon.lock against the legitimate
	// saver. This mirrors the 4-5 / orphan_sweep pattern: per-orphan
	// state dirs decouple lock acquisition from pgrep visibility so
	// all three daemons stay live long enough for pgrep to observe
	// N=3. pgrep's argv match is system-wide, so all three still
	// appear in `pgrep -fx '^portal state daemon( |$)'`.
	orphan1, orphan1StateDir := spawnOrphanDaemonIsolated(t, envSlice)
	// Wait until orphan 1 writes daemon.pid in its OWN stateDir. This
	// is the precondition for the next step: we cannot overwrite the
	// legitimate stateDir's daemon.pid with orphan 1's PID until we
	// have observed orphan 1 reach a point where it would itself have
	// written daemon.pid — which is the spec's "orphan-with-daemon.pid"
	// shape.
	waitForDaemonPID(t, orphan1StateDir, orphan1.Process.Pid)

	// Step 6 (continued): OVERWRITE the legitimate stateDir's
	// daemon.pid with orphan 1's PID. This is the load-bearing
	// reporter-simulation step: it produces the exact "daemon.pid
	// references a live orphan (not the saver-pane process)" shape
	// that triggers the kill-barrier-unreachable-orphan path in
	// Component A. WritePIDFile is the production primitive the
	// daemon itself uses, so this faithfully matches what the
	// reporter's broken install produced.
	if err := state.WritePIDFile(stateDir, orphan1.Process.Pid); err != nil {
		t.Fatalf("overwrite legitimate daemon.pid with orphan1 PID: %v", err)
	}

	// Step 7: spawn orphan 2 with its OWN PORTAL_STATE_DIR. Same
	// per-orphan-stateDir rationale as orphan 1. We do NOT touch the
	// legitimate daemon.pid for orphan 2 — only orphan 1 is the
	// "recorded" orphan in the reporter scenario. Orphan 2 is the
	// "second loose orphan" that Component B's pgrep sweep must
	// independently observe and kill.
	orphan2, _ := spawnOrphanDaemonIsolated(t, envSlice)

	// Step 8: pre-assertions. These prove the harness produces the
	// documented preconditions before any consumer touches it. A
	// failed pre-assertion here means the harness itself is broken
	// (not the production code) and the consumer test would surface
	// a misleading failure if we let it proceed.
	assertCompositePreState(t, stateDir, sock, legitimateDaemonPID,
		orphan1.Process.Pid, orphan2.Process.Pid)

	return &compositeHarness{
		Env:                 envSlice,
		StateDir:            stateDir,
		Sock:                sock,
		Client:              client,
		LegitimateDaemonPID: legitimateDaemonPID,
		Orphan1PID:          orphan1.Process.Pid,
		Orphan2PID:          orphan2.Process.Pid,
		UserSessionNames:    userSessionNames,
	}
}

// assertCompositePreState validates the documented harness preconditions:
//
//  1. pgrep -fx '^portal state daemon( |$)' converges to 3 within the
//     pre-state budget.
//  2. daemon.pid in the LEGITIMATE stateDir reads back to orphan1PID
//     (NOT legitimateDaemonPID, NOT orphan2PID).
//  3. orphan1PID and orphan2PID are both different from
//     legitimateDaemonPID (the saver-pane process).
//  4. Both orphan PIDs are alive at the assertion instant (no premature
//     subprocess exit).
//
// Surfaces a rich diagnostic on failure so the harness-vs-consumer
// failure-locus question is answerable from the test output alone.
func assertCompositePreState(t *testing.T, stateDir string, sock *tmuxtest.Socket,
	legitimateDaemonPID, orphan1PID, orphan2PID int,
) {
	t.Helper()

	// Pre-assertion 1: N=3 daemons observable via pgrep.
	if !waitForPgrepCount(t, 3, compositePreStatePGrepTimeout) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("harness pre-state: pgrep -fx did not reach 3 within %s\n"+
			"  legitimate saver PID: %d (alive=%v)\n"+
			"  orphan1 PID: %d (alive=%v)\n"+
			"  orphan2 PID: %d (alive=%v)\n"+
			"  pgrep snapshot: %v\n"+
			"  hint: a daemon may have exited before pre-state assertion — harness is broken",
			compositePreStatePGrepTimeout,
			legitimateDaemonPID, pidAlive(legitimateDaemonPID),
			orphan1PID, pidAlive(orphan1PID),
			orphan2PID, pidAlive(orphan2PID),
			pids)
	}

	// Pre-assertion 2: daemon.pid in the legitimate stateDir references
	// orphan 1's PID (the reporter-scenario shape). Reading is via the
	// production primitive so a parse-shape regression in WritePIDFile
	// would also surface here.
	recordedPID, err := state.ReadPIDFile(stateDir)
	if err != nil {
		t.Fatalf("harness pre-state: read legitimate daemon.pid: %v", err)
	}
	if recordedPID != orphan1PID {
		t.Fatalf("harness pre-state: legitimate daemon.pid = %d; want orphan1 PID = %d\n"+
			"  legitimate saver PID: %d\n"+
			"  orphan2 PID: %d\n"+
			"  the daemon.pid overwrite step did not produce the reporter-scenario shape",
			recordedPID, orphan1PID, legitimateDaemonPID, orphan2PID)
	}

	// Pre-assertion 3: orphan PIDs differ from the saver-pane process.
	// The whole point of the orphan scenario is that the orphans are
	// NOT the saver-pane process; if they collided (vanishingly
	// unlikely without test bugs) the downstream Component A/B
	// assertions would be meaningless.
	if orphan1PID == legitimateDaemonPID {
		t.Fatalf("harness pre-state: orphan1 PID == saver pane PID == %d\n"+
			"  orphans MUST differ from the saver-pane process for the scenario to fire",
			orphan1PID)
	}
	if orphan2PID == legitimateDaemonPID {
		t.Fatalf("harness pre-state: orphan2 PID == saver pane PID == %d\n"+
			"  orphans MUST differ from the saver-pane process for the scenario to fire",
			orphan2PID)
	}

	// Pre-assertion 4: both orphans alive at the assertion instant.
	// kill(pid, 0) is the canonical liveness probe (mirrors pidAlive).
	if !pidAlive(orphan1PID) {
		t.Fatalf("harness pre-state: orphan1 PID %d not alive at pre-state assertion\n"+
			"  hint: orphan subprocess exited during harness setup", orphan1PID)
	}
	if !pidAlive(orphan2PID) {
		t.Fatalf("harness pre-state: orphan2 PID %d not alive at pre-state assertion\n"+
			"  hint: orphan subprocess exited during harness setup", orphan2PID)
	}

	// Pre-assertion 5 (belt-and-braces): the saver-pane process is
	// still the legitimate daemon (no orphan briefly mis-detected as
	// the saver). Re-read pane_pid; it must equal legitimateDaemonPID.
	currentSaverPID := readSaverPanePID(t, sock)
	if currentSaverPID != legitimateDaemonPID {
		t.Fatalf("harness pre-state: saver pane PID changed during setup\n"+
			"  setup-time PID: %d\n"+
			"  current PID: %d\n"+
			"  hint: the saver pane may have been respawned — harness assumptions invalid",
			legitimateDaemonPID, currentSaverPID)
	}
}

// seedUserSessions creates `count` user sessions on the isolated tmux
// server, each with a small recurring-output script in pane 0 so the
// daemon's capture loop has non-trivial scrollback to persist. Returns
// the session names in creation order.
//
// The shell command is wrapped in `sh -c '<script>'` because tmux's
// new-session shell-command argument is a single token consumed by
// exec; passing the bare `while ...; done` script directly would
// fail the new-session call.
func seedUserSessions(t *testing.T, client *tmux.Client, count int) []string {
	t.Helper()
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%s%d", compositeUserSessionPrefix, i)
		shellCmd := fmt.Sprintf("sh -c %q", compositeUserSessionSeedScript)
		if err := client.NewSessionWithCommand(name, "", shellCmd); err != nil {
			t.Fatalf("seed user session %q: %v", name, err)
		}
		names = append(names, name)
	}
	return names
}

// spawnOrphanDaemonIsolated launches an orphan `portal state daemon`
// subprocess with its OWN per-orphan PORTAL_STATE_DIR (a fresh
// t.TempDir) and returns both the *exec.Cmd and the orphan's stateDir.
//
// Used by Scenario A so multiple orphans can coexist with the
// saver-pane daemon without colliding on `daemon.lock` / `daemon.pid`.
// pgrep is system-wide and argv-anchored, so the orphans still appear
// in `pgrep -fx '^portal state daemon( |$)'` alongside the saver-pane
// daemon. Component B's identity check passes (real `portal state
// daemon` argv), the saver-pane PID legitimate-set check skips them
// (they are not the saver's pane process), and the sweep SIGKILLs
// them as designed.
//
// The composite harness uses the returned stateDir to poll the
// orphan's OWN daemon.pid before overwriting the legitimate stateDir's
// daemon.pid — that read-then-overwrite sequencing is the load-bearing
// reporter-scenario shape. Callers that don't need the stateDir
// discard it with `_`.
//
// Cleanup is registered via registerSubprocessCleanup — SIGKILL + Wait
// on test exit.
func spawnOrphanDaemonIsolated(t *testing.T, envSlice []string) (*exec.Cmd, string) {
	t.Helper()
	orphanStateDir := t.TempDir()
	env := append([]string{}, envSlice...)
	env = append(env, "PORTAL_STATE_DIR="+orphanStateDir)
	cmd := exec.Command("portal", "state", "daemon")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("start isolated orphan daemon (stateDir=%s): %v", orphanStateDir, err)
	}
	registerSubprocessCleanup(t, cmd)
	return cmd, orphanStateDir
}

// TestCompositeHarness_PreState is the one consumer test in this file —
// included to prove the harness itself works. It runs setupCompositeHarness
// (which internally invokes assertCompositePreState) and then independently
// re-validates the documented preconditions on the returned struct.
//
// The downstream 6-2..6-6 assertion tests will consume the harness in
// the same way (call setupCompositeHarness, then run their own
// post-bootstrap assertions). This test exists to catch harness
// regressions BEFORE any downstream consumer is wired up — if it
// breaks, the downstream consumers will all break for harness-shaped
// reasons rather than production-code reasons.
func TestCompositeHarness_PreState(t *testing.T) {
	h := setupCompositeHarness(t)

	// Re-validate the documented preconditions from the returned
	// struct's field values (the harness's internal assertion already
	// passed, but the struct fields are the consumer's contract — a
	// regression where setupCompositeHarness asserts X but returns
	// !X in the struct would slip past the internal check).

	// PIDs are non-zero and distinct.
	if h.LegitimateDaemonPID <= 0 {
		t.Fatalf("h.LegitimateDaemonPID = %d; want > 0", h.LegitimateDaemonPID)
	}
	if h.Orphan1PID <= 0 {
		t.Fatalf("h.Orphan1PID = %d; want > 0", h.Orphan1PID)
	}
	if h.Orphan2PID <= 0 {
		t.Fatalf("h.Orphan2PID = %d; want > 0", h.Orphan2PID)
	}
	if h.Orphan1PID == h.LegitimateDaemonPID {
		t.Fatalf("h.Orphan1PID == h.LegitimateDaemonPID == %d; PIDs must be distinct", h.Orphan1PID)
	}
	if h.Orphan2PID == h.LegitimateDaemonPID {
		t.Fatalf("h.Orphan2PID == h.LegitimateDaemonPID == %d; PIDs must be distinct", h.Orphan2PID)
	}
	if h.Orphan1PID == h.Orphan2PID {
		t.Fatalf("h.Orphan1PID == h.Orphan2PID == %d; orphan PIDs must be distinct", h.Orphan1PID)
	}

	// daemon.pid in the legitimate stateDir references orphan 1.
	recordedPID, err := state.ReadPIDFile(h.StateDir)
	if err != nil {
		t.Fatalf("read legitimate daemon.pid: %v", err)
	}
	if recordedPID != h.Orphan1PID {
		t.Fatalf("legitimate daemon.pid = %d; want h.Orphan1PID = %d\n"+
			"  h.LegitimateDaemonPID = %d, h.Orphan2PID = %d",
			recordedPID, h.Orphan1PID, h.LegitimateDaemonPID, h.Orphan2PID)
	}

	// pgrep -fx still reports 3 daemons at consumer-observation time.
	// (The harness's internal pre-assertion already verified this;
	// re-checking from the consumer's perspective catches a regression
	// where the harness exits with N != 3 between the internal
	// assertion and the consumer's first observation.)
	pids, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("pgrep snapshot: %v", err)
	}
	if len(pids) != 3 {
		t.Fatalf("pgrep -fx returned %d daemons, want 3: %v\n"+
			"  h.LegitimateDaemonPID = %d (alive=%v)\n"+
			"  h.Orphan1PID = %d (alive=%v)\n"+
			"  h.Orphan2PID = %d (alive=%v)",
			len(pids), pids,
			h.LegitimateDaemonPID, pidAlive(h.LegitimateDaemonPID),
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID))
	}

	// Both orphans alive.
	if !pidAlive(h.Orphan1PID) {
		t.Fatalf("h.Orphan1PID %d not alive at consumer-observation time", h.Orphan1PID)
	}
	if !pidAlive(h.Orphan2PID) {
		t.Fatalf("h.Orphan2PID %d not alive at consumer-observation time", h.Orphan2PID)
	}

	// User sessions seeded and observable on the isolated tmux server.
	if len(h.UserSessionNames) != compositeUserSessionCount {
		t.Fatalf("len(h.UserSessionNames) = %d; want %d",
			len(h.UserSessionNames), compositeUserSessionCount)
	}
	out, err := h.Sock.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		t.Fatalf("list-sessions on isolated socket: %v\n%s", err, out)
	}
	listed := strings.Split(strings.TrimSpace(out), "\n")
	for _, want := range h.UserSessionNames {
		found := false
		for _, got := range listed {
			if strings.TrimSpace(got) == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("seeded user session %q not present in list-sessions output\n"+
				"  list-sessions output: %v", want, listed)
		}
	}
}

// Compile-time guard: ensure syscall is imported. The harness uses
// pidAlive (defined in orphan_sweep_integration_test.go) which calls
// syscall.Kill; if a future refactor moves pidAlive away from the
// _test package or changes its signature, this file's syscall reference
// surfaces the issue.
var _ = syscall.SIGKILL
