//go:build integration

// Real-tmux integration test for spec § Daemon-Owned Hooks Cleanup —
// "A stale hook entry cannot misfire" / § Test Strategy → Integration
// (real tmux) → Daemon cleanup. It stands up a live `portal state
// daemon` against a real isolated tmux server (isolated state + config +
// hooks.json) and proves the daemon reaps a stale hooks.json entry on
// its THROTTLED IDLE cadence while retaining a live-keyed entry.
//
// What the branch landed (dependencies 3-1/3-2/3-3):
//   - `const hookCleanupInterval = 10 * time.Second` (task 3-2).
//   - `daemonDeps.lastCleanup` initialised to daemon-START time (task 3-1),
//     so the FIRST cleanup fires ~10s after start, not on the first idle
//     tick (~1s).
//   - The throttled gate `maybeRunHookCleanup` placed on the tick's IDLE
//     branch (`!dirty && !gap`, after the `@portal-restoring` check),
//     replacing the bare idle return (task 3-3). Capture-pending ticks
//     (`dirty || gap`) skip cleanup; scrollback always wins.
//
// With MaxGap = 30s and TickerPeriod = 1s: `daemonDeps.LastSaveAt` is
// zero at start, so the FIRST tick is a gap-capture (idle branch not
// reached, cleanup skipped) which stamps LastSaveAt≈now; every tick from
// then until ~31s is idle (dirty=false, gap=false), so the ~10s cleanup
// fires inside that uninterrupted idle window — a ~15-25s observation
// window sees the first cleanup before any subsequent gap-capture tick.
//
// ─── ISOLATION MANDATE (load-bearing) ────────────────────────────────
// This test spawns a real `portal state daemon`, so it MUST run under
// portaltest.IsolateStateForTest (per CLAUDE.md daemon-test isolation
// rule). A leaked test daemon writing to the developer's real
// ~/.config/portal/state/ is the canonical corruption incident. The
// isolated tmuxtest `-S` socket keeps the developer's ~31 live sessions
// untouched.
//
// PORTAL_HOOKS_FILE hazard: the cmd-package TestMain
// (testmain_isolation_test.go) poisons PORTAL_HOOKS_FILE to
// /nonexistent/... so a test that forgets to isolate fails loudly.
// IsolateStateForTest scrubs XDG_CONFIG_HOME but NOT PORTAL_HOOKS_FILE.
// We therefore t.Setenv PORTAL_HOOKS_FILE to a writable isolated path
// BEFORE IsolateStateForTest so (a) the derived env slice carries the
// good path (single entry, poison displaced) and (b) the tmux server —
// and thus the saver-pane daemon it hosts — inherits the same path via
// the test-process env. SeedHooksJSON(t, env, …) and the daemon's
// loadHookStore() then resolve the IDENTICAL hooks.json.
//
// ─── WHY BootstrapPortalSaver, NOT a direct exec.Command spawn ────────
// Component D (saver-membership self-supervision, landed from the
// saver-kill-respawn-loop-leaks-daemons feature) makes the daemon's
// per-tick probe eject the process after selfSupervisionHysteresisTicks
// (=3) consecutive ticks where it is NOT the live `_portal-saver` pane
// process. A daemon spawned directly via exec.Command has no
// `_portal-saver` pane bound to its PID, so it self-ejects at ~3.6s
// (empirically measured, tmux 3.7, 2026-07-03) — long before the ~10s
// hook-cleanup interval, so a direct spawn could NEVER observe the
// throttled cleanup. The only way to keep a real daemon alive past 10s
// is to host it AS the `_portal-saver` pane via the production
// tmux.BootstrapPortalSaver cold-start path (placeholder → set
// destroy-unattached=off → respawn-pane to `portal state daemon` →
// readiness barrier), so the probe observes pane_pid == os.Getpid() on
// every tick. This mirrors TestSelfEject_LegitimateColdStartDoesNotFalse-
// Positive (state_daemon_self_supervision_integration_test.go) — the
// established pattern for a long-lived real-daemon integration test —
// and, because the daemon lives inside the tmux server, it is reaped
// when the `_portal-saver` session (and then the server) is torn down in
// t.Cleanup, BEFORE the isolated tempdir is removed. There is no
// standalone *exec.Cmd to wrap with RegisterSubprocessCleanup on this
// path (that helper is for exec.Command-spawned daemons); the reap
// contract is honoured by the saver-session + server teardown ordering.
//
// No t.Parallel: the cmd-package convention applies (package-level
// mutable-state mock injection elsewhere in the package).

package cmd_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/leeovery/portal/internal/transienttest"
)

const (
	// hookCleanupIntervalMirror mirrors cmd.hookCleanupInterval (= 10s,
	// unexported — this _test file is package cmd_test). Duplicating the
	// value is acceptable: the production const is stable (task 3-2 tuning
	// detail) and any drift does NOT produce a false positive — the
	// observation budget below is the interval PLUS generous slack, so a
	// larger production interval only means more headroom inside the same
	// budget. Track production if the interval is ever revised.
	hookCleanupIntervalMirror = 10 * time.Second

	// daemonReadyBudget / daemonReadyPoll bound the post-BootstrapPortalSaver
	// poll for a live daemon.pid. BootstrapPortalSaver's own readiness
	// barrier (≤ saverReadinessTimeout = 2s) usually leaves daemon.pid
	// populated the moment it returns; the extra budget absorbs a slow-CI
	// WARN-and-return-on-timeout.
	daemonReadyBudget = 3 * time.Second
	daemonReadyPoll   = 50 * time.Millisecond

	// hookCleanupObservationBudget / hookCleanupPollTick bound the poll for
	// the stale key to disappear. The first cleanup fires ~10s after daemon
	// start; the interval + 15s slack comfortably absorbs the first
	// gap-capture tick, capture jitter, and CI scheduling on a busy host
	// while short-circuiting the instant the reap is observed.
	hookCleanupObservationBudget = hookCleanupIntervalMirror + 15*time.Second
	hookCleanupPollTick          = 250 * time.Millisecond

	// preIntervalSafetyCeiling: if this much wall time has already elapsed
	// since t0 when the "no reap before one interval" read runs, the host
	// is too slow to reliably establish that window (the cleanup fires at
	// daemon-start + ~10s). We then log and skip that single sub-assertion
	// rather than false-fail — the reap + retain assertions below still
	// pin the load-bearing behaviour.
	preIntervalSafetyCeiling = hookCleanupIntervalMirror - 2*time.Second

	// staleHookKey is a nanoid-bearing structural key
	// (#{session_name}:#{window_index}.#{pane_index}) with NO matching live
	// pane on the test server. GenerateSessionName guarantees such a name
	// never re-appears unless the exact saved session is restored, so a
	// genuinely-stale entry can only be reaped — never misfire. The daemon's
	// CleanStale must remove it.
	staleHookKey = "gone-XxXxXx:0.0"

	// liveWorkSession is the session whose sole pane supplies the LIVE hook
	// key. Its structural key is read back from tmux (not assumed) so the
	// seeded live entry matches exactly what ListAllPanes enumerates.
	liveWorkSession = "work"
)

// TestDaemon_ThrottledHookCleanup_ReapsStaleRetainsLiveOnIdleServer pins
// the real-tmux behaviour of the daemon-owned throttled hooks cleanup:
//
//	(a) NO reap before one interval — both the stale and live keys are
//	    still present shortly after the daemon becomes alive (well within
//	    the ~10s hookCleanupInterval measured from daemon start), on an
//	    idle server.
//	(b) The STALE key IS reaped after the interval elapses, on a server
//	    kept idle throughout (the idle branch is where the cleanup gate
//	    lives).
//	(c) The LIVE-keyed entry is RETAINED after the reap (CleanStale's
//	    live-pane-set filter keeps keys present in ListAllPanes).
//
// See the file header for the isolation mandate, the PORTAL_HOOKS_FILE
// hazard handling, and the (load-bearing) reason the daemon is hosted as
// the `_portal-saver` pane rather than spawned via exec.Command.
func TestDaemon_ThrottledHookCleanup_ReapsStaleRetainsLiveOnIdleServer(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// StagePortalBinary builds the binary into a t.TempDir and PATH-prepends
	// it (t.Setenv PATH) so the tmux-server-respawned `portal state daemon`
	// resolves the bare argv-0 from the freshly built binary.
	binDir := portalbintest.StagePortalBinary(t)
	if _, err := exec.LookPath("portal"); err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	// PORTAL_HOOKS_FILE override BEFORE IsolateStateForTest — see the
	// header's hazard note. The good path lives under a per-test tempdir;
	// t.Setenv replaces the TestMain poison in the process env, so the env
	// slice IsolateStateForTest derives from os.Environ() carries a single
	// good PORTAL_HOOKS_FILE entry (ResolveHooksFilePathFromEnv returns the
	// first match — there must be no poisoned duplicate ahead of it).
	hooksPath := filepath.Join(t.TempDir(), "portal", "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// IsolateStateForTest scrubs HOME / XDG_CONFIG_HOME on the test process
	// (fingerprint-diff backstop) and returns an env slice + isolated
	// stateDir. The returned env carries the good PORTAL_HOOKS_FILE.
	env, stateDir := portaltest.IsolateStateForTest(t)

	// Propagate the daemon-relevant env onto the TEST PROCESS so the tmux
	// server (auto-started by the first sock command below) — and therefore
	// the saver-pane daemon it hosts via respawn-pane — inherits them.
	// PORTAL_STATE_DIR pins the daemon's state writes to the isolated dir;
	// PORTAL_HOOKS_FILE (re-set for symmetry) pins loadHookStore() to the
	// same hooks.json the seed writes; PORTAL_LOG_LEVEL=INFO surfaces the
	// `hooks: clean-stale` audit breadcrumb into portal.log for diagnostics;
	// PATH keeps the built binary resolvable for respawn-pane.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)
	t.Setenv("PORTAL_LOG_LEVEL", "INFO")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Stand up the isolated tmux server. The first sock.Run below auto-starts
	// it, inheriting the env configured above.
	sock := tmuxtest.New(t, "ptl-daemon-hookclean-")
	client := sock.Client()

	// Create the live "work" session (one pane) and read its structural key
	// back from tmux with the canonical format so the seeded LIVE entry
	// matches exactly what the daemon's ListAllPanes enumerates.
	sock.Run(t, "new-session", "-d", "-s", liveWorkSession, "sh", "-c", "exec tail -f /dev/null")
	liveHookKey := strings.TrimSpace(sock.Run(t, "list-panes",
		"-t", liveWorkSession, "-F", tmux.StructuralKeyFormat))
	if liveHookKey == "" {
		t.Fatalf("could not read live pane structural key for session %q", liveWorkSession)
	}
	if liveHookKey == staleHookKey {
		t.Fatalf("test setup collision: live key %q equals the stale key constant", liveHookKey)
	}

	// Seed hooks.json with the stale + live entries via the production
	// hooks.Store (so the on-disk layout matches `portal hooks set`). Uses
	// the SAME env slice the daemon resolves from.
	transienttest.SeedHooksJSON(t, env, map[string]string{
		staleHookKey: "echo stale-should-be-reaped",
		liveHookKey:  "echo live-should-be-retained",
	})

	// Pre-spawn verification: BOTH keys present on disk before the daemon
	// runs (guards against a seed that silently landed on the wrong path).
	preKeys := readHookKeys(t, env)
	if _, ok := preKeys[staleHookKey]; !ok {
		t.Fatalf("pre-spawn: stale key %q absent after seed; keys=%v\n"+
			"  hooks.json path resolution mismatch — seed did not land where the daemon reads",
			staleHookKey, sortedKeys(preKeys))
	}
	if _, ok := preKeys[liveHookKey]; !ok {
		t.Fatalf("pre-spawn: live key %q absent after seed; keys=%v",
			liveHookKey, sortedKeys(preKeys))
	}

	// Teardown ordering: kill _portal-saver explicitly (LIFO — this cleanup
	// runs BEFORE tmuxtest.New's kill-server, which runs before the isolated
	// tempdir removal) so the daemon receives SIGHUP and exits before its
	// state dir vanishes. Tolerant of "already gone".
	t.Cleanup(func() {
		_, _ = sock.TryRun("kill-session", "-t", tmux.PortalSaverName)
	})

	// Host the daemon as the `_portal-saver` pane via the production
	// cold-start path so its saver-membership probe passes on every tick
	// (pane_pid == os.Getpid()) and it does NOT self-eject before the ~10s
	// cleanup. t0 is a conservative lower bound on daemon start (the daemon
	// starts DURING the respawn inside this call), used to anchor the
	// pre-interval window below.
	t0 := time.Now()
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v\n--- portal.log ---\n%s",
			err, portaltest.ReadPortalLogSafe(stateDir))
	}

	// Wait for the daemon to publish a live daemon.pid.
	if !tmuxtest.PollUntil(t, daemonReadyBudget, daemonReadyPoll, func() bool {
		return state.DaemonAlive(stateDir)
	}) {
		t.Fatalf("daemon did not become alive within %s of BootstrapPortalSaver return\n"+
			"--- portal.log ---\n%s", daemonReadyBudget, portaltest.ReadPortalLogSafe(stateDir))
	}

	pidData, err := os.ReadFile(state.DaemonPID(stateDir))
	if err != nil {
		t.Fatalf("read daemon.pid: %v\n--- portal.log ---\n%s",
			err, portaltest.ReadPortalLogSafe(stateDir))
	}
	daemonPID, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("parse daemon.pid contents %q: %v", string(pidData), err)
	}

	// Structural binding sanity: daemon.pid == _portal-saver pane pid. This
	// is the precondition for the saver-membership probe to pass on every
	// tick; a mismatch means the daemon would self-eject within ~3 ticks and
	// the reap assertion below would time out — fail fast with a clear cause.
	panePIDStr := strings.TrimSpace(sock.Run(t, "list-panes",
		"-t", tmux.PortalSaverName, "-F", "#{pane_pid}"))
	panePID, err := strconv.Atoi(panePIDStr)
	if err != nil {
		t.Fatalf("parse _portal-saver pane pid %q: %v", panePIDStr, err)
	}
	if daemonPID != panePID {
		t.Fatalf("structural-binding divergence: daemon.pid (%d) != _portal-saver pane pid (%d)\n"+
			"  the daemon must BE the saver pane process or Component D self-supervision "+
			"ejects it before the ~10s cleanup interval\n--- portal.log ---\n%s",
			daemonPID, panePID, portaltest.ReadPortalLogSafe(stateDir))
	}
	t.Logf("daemon alive as _portal-saver pane (pid=%d); live key=%q, stale key=%q",
		daemonPID, liveHookKey, staleHookKey)

	// ── Assertion (a): NO reap before one interval ──────────────────────
	// Read the hooks store shortly after the daemon is alive — well within
	// the ~10s interval measured from daemon start (t0 is a lower bound on
	// that start). Both keys must still be present. On a pathologically slow
	// host where too much wall time already elapsed, the pre-interval window
	// cannot be reliably established; log and skip this single sub-assertion.
	elapsedA := time.Since(t0)
	earlyKeys := readHookKeys(t, env)
	if elapsedA >= preIntervalSafetyCeiling {
		t.Logf("slow host: %s already elapsed since daemon-start lower bound (>= %s); "+
			"skipping the no-reap-before-interval sub-assertion (reap + retain below still pin behaviour)",
			elapsedA, preIntervalSafetyCeiling)
	} else {
		if _, ok := earlyKeys[staleHookKey]; !ok {
			t.Fatalf("stale key %q reaped only %s after daemon start (< interval %s); "+
				"lastCleanup must be anchored to daemon-START so the first cleanup fires "+
				"~one interval later, not immediately\n--- portal.log ---\n%s",
				staleHookKey, elapsedA, hookCleanupIntervalMirror, portaltest.ReadPortalLogSafe(stateDir))
		}
		if _, ok := earlyKeys[liveHookKey]; !ok {
			t.Fatalf("live key %q missing %s after daemon start (before any cleanup)\n"+
				"--- portal.log ---\n%s",
				liveHookKey, elapsedA, portaltest.ReadPortalLogSafe(stateDir))
		}
		t.Logf("no-reap-before-interval confirmed: both keys present at %s after daemon start", elapsedA)
	}

	// ── Assertion (b): STALE key reaped AFTER the interval, server idle ──
	// Keep the server idle throughout (no save.requested touch, nothing that
	// makes a tick dirty/gap before ~31s), so the daemon reaches its idle
	// branch and the ~10s cleanup fires. Poll until the stale key disappears.
	reaped := tmuxtest.PollUntil(t, hookCleanupObservationBudget, hookCleanupPollTick, func() bool {
		_, present := readHookKeys(t, env)[staleHookKey]
		return !present
	})
	if !reaped {
		finalKeys := readHookKeys(t, env)
		t.Fatalf("stale key %q was NOT reaped within %s of daemon start on an idle server\n"+
			"  spec § Daemon-Owned Hooks Cleanup: the daemon's throttled (~%s) idle-branch "+
			"cleanup MUST reap entries whose paneKey is not in the live pane set\n"+
			"  remaining hooks.json keys: %v\n"+
			"--- hooks.json (%s) ---\n%s\n--- portal.log ---\n%s",
			staleHookKey, hookCleanupObservationBudget, hookCleanupIntervalMirror,
			sortedKeys(finalKeys), hooksPath, string(transienttest.HooksJSONBytes(t, env)),
			portaltest.ReadPortalLogSafe(stateDir))
	}
	t.Logf("stale key %q reaped after the throttle interval on the idle server", staleHookKey)

	// ── Assertion (c): LIVE-keyed entry RETAINED after the reap ─────────
	postKeys := readHookKeys(t, env)
	if _, ok := postKeys[liveHookKey]; !ok {
		t.Fatalf("live key %q was removed by the cleanup; CleanStale must RETAIN entries "+
			"whose paneKey is present in the live pane set (ListAllPanes)\n"+
			"  remaining keys: %v\n--- hooks.json (%s) ---\n%s\n--- portal.log ---\n%s",
			liveHookKey, sortedKeys(postKeys), hooksPath,
			string(transienttest.HooksJSONBytes(t, env)), portaltest.ReadPortalLogSafe(stateDir))
	}
	// Belt-and-braces: the stale key must not have transiently reappeared.
	if _, ok := postKeys[staleHookKey]; ok {
		t.Fatalf("stale key %q reappeared after reap; keys=%v", staleHookKey, sortedKeys(postKeys))
	}
	t.Logf("live key %q retained after stale reap; final keys=%v", liveHookKey, sortedKeys(postKeys))
}

// readHookKeys reads the isolated hooks.json (resolved from env) and
// returns the set of structural keys. A missing file yields an empty set.
// AtomicWrite (temp + rename) guarantees the daemon's concurrent writes
// are observed whole, so json.Unmarshal never sees a partial file.
func readHookKeys(t *testing.T, env []string) map[string]struct{} {
	t.Helper()
	raw := transienttest.HooksJSONBytes(t, env)
	keys := make(map[string]struct{})
	if len(raw) == 0 {
		return keys
	}
	// hooks.json layout: map[structuralKey]map[event]command.
	var m map[string]map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal hooks.json: %v\n--- raw ---\n%s", err, string(raw))
	}
	for k := range m {
		keys[k] = struct{}{}
	}
	return keys
}

// sortedKeys returns the set's keys in deterministic order for diagnostics.
func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
