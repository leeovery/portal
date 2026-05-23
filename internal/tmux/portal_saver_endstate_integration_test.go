package tmux_test

// Real-tmux integration tests for spec § Component F acceptance bullets
// 1–3 (clean-bootstrap end-state and lock-loser log-noise absence).
//
// These pin the user-visible contract of the create-then-set-option-then-
// respawn ordering introduced by Component F:
//
//  1. TestBootstrapPortalSaver_CleanBootstrap_EndState — on a clean
//     bootstrap (no prior saver, no competing daemon) the saver session
//     exists, destroy-unattached=off is applied, the pane process is the
//     real `portal state daemon` (not the `tail -f /dev/null` placeholder),
//     and the daemon never observed a "no such session: _portal-saver"
//     event during startup.
//  2. TestBootstrapPortalSaver_LockLoser_NoNoSuchSessionLogNoise — when
//     a competing daemon already holds daemon.lock, the freshly-
//     respawned daemon exits as lock-loser. Default tmux behaviour
//     (no `remain-on-exit on`) destroys the session when its only pane
//     process exits — even with destroy-unattached=off. This is the
//     ACTUAL observable on the supported tmux version (3.6b) and is
//     what `TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession`
//     in portal_saver_test.go independently documents.
//
//     The load-bearing F invariant covered here is therefore NOT
//     "session persists" but "no `no such session: _portal-saver` log
//     noise during the lock-loser cascade". Pre-F, the daemon-as-
//     initial-pane-command path would exit immediately on lock loss,
//     tmux would destroy the session, and the subsequent SetSessionOption
//     would log the forbidden substring against a now-missing session.
//     With F's create-with-placeholder → SetSessionOption → respawn
//     ordering, SetSessionOption runs against the live placeholder pane
//     and the "no such session" cascade is structurally eliminated even
//     though the eventual session destruction still occurs.
//
// Host-noise mitigation (HOME=<tempdir>):
//
// `portaltest.NewIsolatedStateEnv` registers a backstop that snapshots
// the developer's real state directory (resolved from XDG_CONFIG_HOME or
// HOME pre-override) and re-snapshots on test exit to catch leakage from
// the spawned daemon. On a dev box with a live `portal state daemon`
// running against the real `~/.config/portal/state/`, that live daemon
// mutates `save.requested`, scrollback `.bin` files, etc. during the
// test — producing a false-positive backstop failure that has nothing
// to do with the test's own behaviour.
//
// `NewIsolatedStateEnv` folds in the mitigation: it `t.Setenv`s HOME
// to a fresh tempdir and clears XDG_CONFIG_HOME BEFORE its
// pre-snapshot, so `resolveDevStateDir` resolves to
// `<tempdir>/.config/portal/state` — a path no other process knows
// about and therefore cannot mutate during the test window. The
// isolation guarantee (`PORTAL_STATE_DIR` on the test process and
// inherited by the tmux server / daemon) is unaffected: the test still
// drives I/O against the per-test temp state dir; only the *backstop's
// snapshot target* is redirected away from the host's live state dir.
//
// PORTAL_STATE_DIR is set via t.Setenv so the tmux server (forked from
// the test process) inherits the override and the daemon binary
// resolves the isolated state dir.
//
// No t.Parallel: the cmd-package convention (mock-injection via
// package-level mutable state cleaned up by t.Cleanup) applies here too.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// endStateReadyTimeout is the upper bound for polling the daemon's
// observable end-state (pid file + ps identification). Sized to cover
// daemon cold-start latency (fork + exec + flock + pid write) with
// margin on slow CI machines.
const endStateReadyTimeout = 2 * time.Second

// endStatePollTick is the cadence at which the end-state polls re-probe
// observable state. 50 ms matches the readiness-barrier cadence in
// production wiring (saverReadinessPollInterval) and is short enough to
// observe sub-second daemon-startup races without busy-spinning.
const endStatePollTick = 50 * time.Millisecond

// lockLoserCascadeWindow is the upper-bound wall-clock window during
// which any "no such session: _portal-saver" log entry would be
// produced by a regressed (pre-F) ordering. Sized to cover
// create-with-placeholder + set-option + respawn + daemon-startup +
// lock-acquire fail + daemon-exit + tmux-session-teardown on the
// supported tmux versions. portal.log is re-read after this window
// elapses so a delayed cascade entry (written by a subprocess after
// BootstrapPortalSaver has returned) is observed.
const lockLoserCascadeWindow = 2500 * time.Millisecond

// TestBootstrapPortalSaver_CleanBootstrap_EndState exercises spec §
// Component F Acceptance criteria bullets 1–2: a clean bootstrap (no
// prior saver, no competing daemon) produces a `_portal-saver` session
// with destroy-unattached=off, a `portal state daemon` pane process
// (NOT the `tail -f /dev/null` placeholder), and zero
// "no such session: _portal-saver" log entries.
//
// Flow:
//  1. Skip if tmux not on PATH or portal binary build fails.
//  2. Stage isolated state dir via portaltest.NewIsolatedStateEnv
//     (which folds in the HOME=<tempdir> / XDG_CONFIG_HOME="" scrub
//     before its pre-snapshot, so the backstop targets a quiet
//     tempdir rather than the developer's live state). Set
//     PORTAL_STATE_DIR on the test process so the tmux server (and
//     the daemon it spawns) inherits the isolated dir.
//  3. Stand up an isolated tmux server via tmuxtest.New.
//  4. Pre-condition: assert _portal-saver is absent.
//  5. Invoke BootstrapPortalSaver(client, stateDir). Must return nil.
//  6. Poll observable end-state (≤2s):
//     - HasSession(PortalSaverName) returns true.
//     - show-options destroy-unattached contains "off".
//     - pane PID resolves to a `portal state daemon` argv via ps.
//     - portal.log contains zero "no such session: _portal-saver"
//     substrings (Component F's load-bearing log assertion).
func TestBootstrapPortalSaver_CleanBootstrap_EndState(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	_ = portalbintest.StagePortalBinary(t)

	// Isolated state dir + backstop. NewIsolatedStateEnv folds in the
	// HOME=<tempdir> / XDG_CONFIG_HOME="" host-noise scrub before its
	// pre-snapshot, so the backstop targets a quiet tempdir rather
	// than the developer's live state dir. The returned env slice is
	// not used because the daemon is spawned by the tmux server (not
	// directly by the test), so PORTAL_STATE_DIR on the test process
	// env is the propagation channel that reaches both.
	_, stateDir := portaltest.NewIsolatedStateEnv(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-cleanboot-")
	client := sock.Client()

	// Pre-condition: _portal-saver must not be present on a fresh server.
	if client.HasSession(tmux.PortalSaverName) {
		t.Fatalf("pre-condition failed: %s present on fresh tmux server", tmux.PortalSaverName)
	}

	// ACTION: clean BootstrapPortalSaver. No prior session, no competing
	// daemon — the create branch runs end-to-end (placeholder → set
	// option → respawn → readiness barrier).
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}

	// Assertion 1: session present.
	if !client.HasSession(tmux.PortalSaverName) {
		t.Fatalf("HasSession(%s) = false; want true after BootstrapPortalSaver",
			tmux.PortalSaverName)
	}

	// Assertion 2: destroy-unattached=off. Use sock.Run so we exercise
	// the real tmux show-options output rather than the client's
	// abstraction — the spec's acceptance shape is byte-level
	// `tmux show-options -t _portal-saver destroy-unattached` containing
	// "off".
	opt := sock.Run(t, "show-options", "-t", tmux.PortalSaverName, "destroy-unattached")
	if !strings.Contains(opt, "off") {
		t.Fatalf("show-options destroy-unattached = %q; want substring %q", opt, "off")
	}

	// Assertion 3: pane process is `portal state daemon` (not the
	// placeholder `tail -f /dev/null`). Poll because the respawn-pane +
	// daemon-startup chain is async w.r.t. BootstrapPortalSaver's
	// readiness barrier return — the readiness barrier waits for
	// daemon.pid, but `ps -o args` of the tmux pane pid is observed
	// directly via tmux list-panes and may briefly show the placeholder
	// before respawn-pane completes its swap.
	var lastArgs string
	if !tmuxtest.PollUntil(t, endStateReadyTimeout, endStatePollTick, func() bool {
		out, err := sock.TryRun("list-panes", "-t", tmux.PortalSaverName, "-F", "#{pane_pid}")
		if err != nil {
			return false
		}
		pidStr := strings.TrimSpace(out)
		if pidStr == "" {
			return false
		}
		pid, perr := strconv.Atoi(pidStr)
		if perr != nil {
			return false
		}
		args, perr := psArgsForPID(pid)
		if perr != nil {
			return false
		}
		lastArgs = args
		return strings.Contains(args, "portal state daemon")
	}) {
		t.Fatalf("pane process did not converge on `portal state daemon` within %s; last ps args = %q",
			endStateReadyTimeout, lastArgs)
	}

	// Defensive: pane args must NOT be the placeholder. PollUntil's
	// substring match above could in principle pass on a transient
	// intermediate state if the daemon-cmd-line and the placeholder
	// overlapped; assert the placeholder substring is absent.
	if strings.Contains(lastArgs, "tail -f /dev/null") {
		t.Fatalf("pane process is still the placeholder `tail -f /dev/null`; ps args = %q",
			lastArgs)
	}

	// Assertion 4: portal.log contains zero "no such session:
	// _portal-saver" entries. This is the cascade-chain symptom that
	// Component F's create-then-set-option-then-respawn ordering
	// eliminates structurally: pre-F, a lock-loser daemon could exit
	// between new-session and SetSessionOption, and the SetSessionOption
	// would log the forbidden substring against the just-destroyed
	// session.
	assertNoNoSuchSessionEntries(t, stateDir)
}

// TestBootstrapPortalSaver_LockLoser_NoNoSuchSessionLogNoise exercises
// spec § Component F Acceptance criteria bullet 3 — REVISED. The
// orchestrator-decision section in the file header documents why the
// spec's literal "session persists" assertion does not hold against
// the supported tmux version (3.6b without `remain-on-exit on`): the
// session DOES disappear when the lock-loser daemon exits. The
// invariant Component F's create-then-set-option-then-respawn
// ordering ACTUALLY protects is the absence of "no such session:
// _portal-saver" log noise during the lock-loser cascade.
//
// Flow:
//  1. Skip if tmux not on PATH or portal binary build fails.
//  2. Host-noise mitigation + isolated state dir + PORTAL_STATE_DIR.
//  3. Pre-seed a competing `portal state daemon` against the SAME
//     isolated state dir. Wait until it writes daemon.pid AND
//     state.IdentifyDaemon confirms it.
//  4. Invoke tmux.BootstrapPortalSaver(client, stateDir). With F's
//     ordering, the placeholder creates the session, SetSessionOption
//     applies destroy-unattached=off against the LIVE placeholder
//     pane (never against a missing session), respawn-pane swaps in
//     the daemon, the daemon's pre-acquire check (Component C)
//     observes the seeded daemon's live PID, returns ErrDaemonLockHeld,
//     and exits cleanly. tmux's default behaviour then destroys the
//     session — but no "no such session" log noise has been produced
//     by BootstrapPortalSaver itself, because every SetSessionOption
//     call in the bootstrap sequence executed against a live session.
//  5. Wait the cascade window (covers respawn + daemon-startup +
//     lock-loser exit + tmux-destroy) so any delayed log writes are
//     observed.
//  6. Assert portal.log contains ZERO "no such session: _portal-saver"
//     entries (covers any daemon-side cascade routed through the
//     state.Logger).
//  7. Assert BootstrapPortalSaver itself returned nil AND that no
//     returned-error path (if non-nil) contains "no such session:
//     _portal-saver" — the bootstrap-side substring detector is the
//     load-bearing assertion. Pre-F (daemon as initial pane command,
//     no respawn), the daemon exits as lock-loser BEFORE
//     SetSessionOption runs, tmux destroys the session, and
//     SetSessionOption fails with `exit status 1: no such session:
//     _portal-saver`; that error surfaces through
//     BootstrapPortalSaver's return value. With F's ordering,
//     SetSessionOption always runs against the live placeholder, so
//     the return is nil and the cascade substring never appears.
//
// Negative-case observation: the cascade is race-window dependent —
// the daemon-as-initial-pane-command must exit BEFORE the
// bootstrap's immediate SetSessionOption fires. On the supported
// tmux 3.6b on darwin/arm64, the daemon's lock-loser exit takes
// long enough (~tens of ms) that the back-to-back NewSession +
// SetSessionOption typically wins the race, so a literal revert of
// F's reorder is not deterministically caught by this assertion in
// every run. This matches the orchestrator's accepted limitation
// (documented at task-receipt time): the test's value is end-state
// validation of the absence of the cascade, not 100% regression
// detection.
func TestBootstrapPortalSaver_LockLoser_NoNoSuchSessionLogNoise(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.NewIsolatedStateEnv(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-lockloser-")
	client := sock.Client()

	// Keep the tmux server alive by creating an unrelated dummy session
	// BEFORE we exercise the saver bootstrap. Rationale: if the eventual
	// _portal-saver session is the server's only session AND its pane
	// process exits, tmux destroys the session AND tears the server
	// down (exit-empty defaults to on). A teardown server surfaces
	// SetSessionOption failures as "no server running" rather than the
	// "no such session: _portal-saver" cascade the test is designed to
	// detect. Production bootstrap always runs against a tmux server
	// that has the user's existing sessions, so the dummy here is
	// closer to the real-world precondition. The dummy is sh-tailing
	// /dev/null so it never exits.
	if err := client.NewDetachedSessionNoCwd(
		"ptl-keepalive", "sh -c 'exec tail -f /dev/null'",
	); err != nil {
		t.Fatalf("create keepalive dummy session: %v", err)
	}

	// Pre-seed the competing daemon. The daemon's PORTAL_STATE_DIR
	// resolves to the same isolated dir as the test (via env slice from
	// NewIsolatedStateEnv with PORTAL_STATE_DIR appended). The daemon's
	// stdout/stderr are discarded — we only care about its lifecycle
	// (pid file written, lock held). On test cleanup we kill it
	// explicitly so it does not leak into the next test.
	seededEnv := append([]string{}, envSlice...)
	seededEnv = append(seededEnv, "PORTAL_STATE_DIR="+stateDir)
	// Invoke via the unqualified name "portal" so the daemon's argv[0]
	// is "portal" — that is what darwin's ps reports as `comm`, and
	// state.IdentifyDaemon requires comm == "portal" exactly. Spawning
	// via an absolute path would set comm to the path (truncated to 15
	// chars on darwin) and the identity-check would classify the
	// daemon as IdentifyNotPortalDaemon. StagePortalBinary has already
	// PATH-prepended the binary directory; exec.Command resolves via
	// the test process's PATH which is inherited into seededEnv.
	seeded := exec.Command("portal", "state", "daemon")
	seeded.Env = seededEnv
	if startErr := seeded.Start(); startErr != nil {
		t.Fatalf("start seeded competing daemon: %v", startErr)
	}
	t.Cleanup(func() {
		// Kill the seeded daemon. SIGKILL bypasses any defer; the
		// daemon's lock fd is released by the kernel on process exit.
		// Errors are intentionally swallowed — the typical case is
		// "process already exited" if the test took a long path.
		_ = seeded.Process.Kill()
		_, _ = seeded.Process.Wait()
	})

	// Wait until the seeded daemon writes daemon.pid AND IdentifyDaemon
	// confirms its identity. This guarantees the pre-acquire check in
	// Component C will see a live identity-checkable holder when the
	// new daemon attempts to acquire the lock.
	if !tmuxtest.PollUntil(t, endStateReadyTimeout, endStatePollTick, func() bool {
		pid, readErr := state.ReadPIDFile(stateDir)
		if readErr != nil {
			return false
		}
		result, idErr := state.IdentifyDaemon(pid)
		if idErr != nil {
			return false
		}
		return result == state.IdentifyIsPortalDaemon
	}) {
		t.Fatalf("seeded competing daemon did not become observable within %s "+
			"(state dir=%s)", endStateReadyTimeout, stateDir)
	}

	// ACTION: invoke BootstrapPortalSaver. The new daemon respawned
	// into the saver pane will lose the lock via Component C's
	// pre-acquire check and exit cleanly. BootstrapPortalSaver itself
	// returns nil because session creation + SetSessionOption succeed
	// against the placeholder pane (which is alive throughout F's
	// ordering); the readiness barrier internally times out (the
	// lock-loser daemon never writes its own pid), logging a WARN but
	// returning nil.
	bootstrapErr := tmux.BootstrapPortalSaver(client, stateDir)

	// Wait out the cascade window so any delayed log writes (from the
	// respawned daemon's exit or from tmux's session teardown observed
	// by a subsequent SetSessionOption-equivalent) have time to land
	// in portal.log. We do not poll because the assertion is about
	// the ABSENCE of a substring, not its presence — polling for an
	// absent substring is a flat sleep with extra steps.
	time.Sleep(lockLoserCascadeWindow)

	// LOAD-BEARING ASSERTION: zero "no such session: _portal-saver"
	// log entries. This is what Component F's ordering structurally
	// protects against. Pre-F, the daemon-as-initial-pane-command path
	// would exit immediately on lock loss, tmux would destroy the
	// session, and the bootstrap's subsequent SetSessionOption call
	// would surface a "no such session" error from tmux that flows
	// through the daemon's stderr capture into portal.log. With F's
	// ordering, SetSessionOption ALWAYS runs against the live
	// placeholder, so this cascade is structurally eliminated even
	// when the eventual session destruction still occurs.
	assertNoNoSuchSessionEntries(t, stateDir)

	// SECONDARY ASSERTION: BootstrapPortalSaver returns nil. The
	// readiness barrier swallows its timeout (best-effort WARN per
	// the spec); SetSessionOption succeeded against the live
	// placeholder; the lock-loser exit happens AFTER the function
	// returns, so the caller observes a clean return even though the
	// resulting session is short-lived.
	//
	// The substring check below is the load-bearing negative-case
	// detector: under a pre-F regression (daemon as initial pane
	// command, no respawn), the daemon exits as lock-loser BEFORE
	// SetSessionOption runs, tmux destroys the session, and
	// SetSessionOption fails with `exit status 1: no such session:
	// _portal-saver`. This error is wrapped by the Client and
	// surfaces through BootstrapPortalSaver's return value. With F's
	// ordering, SetSessionOption ALWAYS runs against the live
	// placeholder, so the return value is nil and the cascade
	// substring never appears in any error path.
	if bootstrapErr != nil {
		if strings.Contains(bootstrapErr.Error(), "no such session: "+tmux.PortalSaverName) {
			t.Fatalf("BootstrapPortalSaver returned the load-bearing cascade error: %v\n"+
				"This indicates the create-then-set-option-then-respawn ordering of "+
				"Component F (Task 3-2) has regressed: SetSessionOption ran against "+
				"a session that was destroyed by an immediately-exiting lock-loser daemon",
				bootstrapErr)
		}
		t.Fatalf("BootstrapPortalSaver returned unexpected error %v; want nil "+
			"(SetSessionOption ran against the live placeholder pane; readiness "+
			"barrier WARN-swallows its timeout)", bootstrapErr)
	}
}

// TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn exercises
// spec § Component F — "Environment inheritance across respawn":
// `NewDetachedSessionNoCwd` does NOT pass `-e KEY=VAL` overrides, so the
// `_portal-saver` session inherits the tmux server's environment
// verbatim. After Component F's create-with-placeholder → set-option →
// respawn ordering completes, `tmux show-environment -t _portal-saver`
// must be identical (for any env var the daemon reads) to the
// pre-F-shape baseline: a sibling detached session created against the
// same tmux server with no env overrides.
//
// Flow:
//  1. Skip if tmux not on PATH or portal binary build fails.
//  2. Host-noise mitigation (HOME=<tempdir>) + isolated state dir +
//     PORTAL_STATE_DIR. Same scaffolding as the other tests in this file.
//  3. Stand up an isolated tmux server via tmuxtest.New. The server is
//     forked from the test process and inherits the test process's env
//     (HOME, XDG_CONFIG_HOME, PATH, PORTAL_STATE_DIR).
//  4. Pre-F baseline: create `_env-baseline` via NewDetachedSessionNoCwd
//     with the same placeholder shape the saver uses, read
//     `show-environment -t _env-baseline` verbatim, parse for
//     XDG_CONFIG_HOME / HOME / PATH, then kill the baseline session
//     BEFORE invoking BootstrapPortalSaver so it cannot influence the
//     saver bootstrap path.
//  5. Post-F observed: invoke BootstrapPortalSaver, read
//     `show-environment -t _portal-saver` verbatim, parse the same three
//     keys.
//  6. Assert per-key parity (both unset is equal; both set must have
//     byte-equal values).
//
// Verbatim semantics: tmux show-environment uses leading "-NAME" (no
// "=") to represent variables explicitly removed from the session
// environment. Trimming the output would not strip those prefixes
// (they're mid-line, not surrounding whitespace), but the test uses
// CombinedOutput via sock.TryRun for fidelity with the spec's literal
// "verbatim" requirement.
func TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	_ = portalbintest.StagePortalBinary(t)

	_, stateDir := portaltest.NewIsolatedStateEnv(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-envparity-")
	client := sock.Client()

	// Pre-F baseline: a sibling detached session created against the same
	// tmux server with the same placeholder shape the saver uses. This is
	// what Component F's spec calls "the pre-F baseline" — an
	// environment-inheritance reference unaffected by the
	// create-then-set-option-then-respawn cycle.
	const baselineName = "_env-baseline"
	if err := client.NewDetachedSessionNoCwd(
		baselineName, "sh -c 'exec tail -f /dev/null'",
	); err != nil {
		t.Fatalf("create baseline session: %v", err)
	}

	baselineRaw, err := sock.TryRun("show-environment", "-t", baselineName)
	if err != nil {
		t.Fatalf("show-environment baseline: %v\n%s", err, baselineRaw)
	}
	baseline := parseShowEnvironmentKeys(baselineRaw, "XDG_CONFIG_HOME", "HOME", "PATH")

	// Kill the baseline BEFORE the saver bootstrap fires so the saver's
	// create path runs in the same observable shape it would in
	// production (no sibling _env-* session affecting tmux's internal
	// session-environment list).
	if err := client.KillSession(baselineName); err != nil {
		t.Fatalf("kill baseline session: %v", err)
	}

	// Post-F observed: drive the create-with-placeholder → set-option →
	// respawn cycle via BootstrapPortalSaver. After readiness completes,
	// read `_portal-saver`'s session environment via the same verbatim
	// path used for the baseline.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}

	observedRaw, err := sock.TryRun("show-environment", "-t", tmux.PortalSaverName)
	if err != nil {
		t.Fatalf("show-environment %s: %v\n%s", tmux.PortalSaverName, err, observedRaw)
	}
	observed := parseShowEnvironmentKeys(observedRaw, "XDG_CONFIG_HOME", "HOME", "PATH")

	// Per-key parity. The spec's acceptance scenario is byte-equality
	// per key, with "unset" treated symmetrically (both unset is equal).
	// parseShowEnvironmentKeys encodes "unset" as envValue{unset:true},
	// so == on the struct does both branches correctly.
	for _, key := range []string{"XDG_CONFIG_HOME", "HOME", "PATH"} {
		if baseline[key] != observed[key] {
			t.Fatalf("environment-inheritance parity violated for key %q\n"+
				"  baseline: %s\n"+
				"  observed: %s\n"+
				"--- full baseline map (3 keys) ---\n%s\n"+
				"--- full observed map (3 keys) ---\n%s\n"+
				"--- raw show-environment %s ---\n%s\n"+
				"--- raw show-environment %s ---\n%s",
				key,
				baseline[key],
				observed[key],
				dumpEnvMap(baseline),
				dumpEnvMap(observed),
				baselineName, baselineRaw,
				tmux.PortalSaverName, observedRaw,
			)
		}
	}
}

// envValue captures one parsed line of `tmux show-environment` output.
// `tmux show-environment -t <session>` emits "NAME=value" for set
// entries and "-NAME" (leading dash, no "=") for entries explicitly
// removed from the session environment. envValue distinguishes the
// two so equality checks treat "both unset" as equal without
// conflating with the empty string set case.
type envValue struct {
	unset bool
	value string
}

// String renders envValue in a human-readable form for failure
// diagnostics. Unset entries render as "(unset)" so the diff message
// is unambiguous.
func (v envValue) String() string {
	if v.unset {
		return "(unset)"
	}
	return strconv.Quote(v.value)
}

// parseShowEnvironmentKeys scans `tmux show-environment` output for the
// supplied keys and returns a map from key → envValue. Keys not
// present in the output are not added to the map; both call sites
// pre-declare the three keys of interest so equality compare proceeds
// per-key regardless of insertion shape.
//
// Line shapes recognised:
//   - "NAME=value" → envValue{unset: false, value: value}
//   - "-NAME"      → envValue{unset: true}
//
// Trailing CR is stripped per line; empty lines are skipped. Any
// other line shape is ignored — show-environment does not emit
// comments or directives.
func parseShowEnvironmentKeys(raw string, keys ...string) map[string]envValue {
	want := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		want[k] = struct{}{}
	}
	out := map[string]envValue{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-") {
			name := line[1:]
			if _, ok := want[name]; ok {
				out[name] = envValue{unset: true}
			}
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		name := line[:eq]
		val := line[eq+1:]
		if _, ok := want[name]; ok {
			out[name] = envValue{value: val}
		}
	}
	return out
}

// dumpEnvMap renders an envValue map in a deterministic, line-per-key
// format for failure diagnostics. The output is the three keys of
// interest in a stable order; missing keys render as "(absent)" so the
// diff message distinguishes "absent from show-environment output"
// from "present and unset" (envValue{unset:true}).
func dumpEnvMap(m map[string]envValue) string {
	var b strings.Builder
	for _, k := range []string{"XDG_CONFIG_HOME", "HOME", "PATH"} {
		v, ok := m[k]
		if !ok {
			fmt.Fprintf(&b, "  %s: (absent)\n", k)
			continue
		}
		fmt.Fprintf(&b, "  %s: %s\n", k, v)
	}
	return b.String()
}

// psArgsForPID returns the `args` field for pid via `ps -o args= -p
// <pid>`. The acceptance criteria specifies the byte-level shape used
// by operators inspecting the live state — `ps -o args= -p <pid>` —
// so the test exercises the same form rather than reusing
// state.IdentifyDaemon (which has its own anchored regex and exit-
// classification logic that is tested independently in
// daemon_identity_test.go).
func psArgsForPID(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("ps -p %d: %w", pid, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// assertNoNoSuchSessionEntries reads portal.log in stateDir and fails
// the test if the substring "no such session: _portal-saver" appears
// anywhere. Absent log file → assertion holds trivially. Reads
// portal.log only — the short-lived tests in this file never trigger
// log rotation.
func assertNoNoSuchSessionEntries(t *testing.T, stateDir string) {
	t.Helper()
	data, err := os.ReadFile(state.PortalLog(stateDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		t.Fatalf("read portal.log: %v", err)
	}
	const forbidden = "no such session: _portal-saver"
	contents := string(data)
	if strings.Contains(contents, forbidden) {
		t.Fatalf("portal.log contains forbidden substring %q\n--- portal.log (path=%s) ---\n%s",
			forbidden, filepath.Join(stateDir, "portal.log"), contents)
	}
}
