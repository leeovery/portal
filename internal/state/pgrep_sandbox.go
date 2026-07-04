//go:build integration

package state

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Daemon-pgrep test sandbox — INTEGRATION-BUILD ONLY (compile-time absent from
// the shipped binary; the identity stubs in pgrep_sandbox_prod.go take its place
// under //go:build !integration).
//
// PURPOSE — enforce the absolute test-isolation invariant: an integration test
// must NEVER enumerate (and therefore the orphan sweep must never SIGKILL) a
// `portal state daemon` the test did not spawn — above all, the developer's live
// daemon. Every test runs on the developer's machine alongside that live daemon,
// so an unscoped `pgrep -fx 'portal state daemon'` in a sweep test used to hand
// the real daemon's PID straight to SIGKILL.
//
// MECHANISM — single chokepoint. Both the production sweep and the test
// count-assertions funnel through state.PgrepPortalDaemons, and the sweep only
// SIGKILLs PIDs its Pgrep seam returns. Filtering the enumeration HERE therefore
// (a) structurally prevents the sweep from ever targeting the real daemon and
// (b) fixes the flaky count assertions — in one place, covering direct-sweep and
// full-bootstrap tests alike.
//
// OWNERSHIP MODEL — DEFAULT-DENY, keyed on the STATE DIRECTORY (not the PID).
// Once enabled, PgrepPortalDaemons surfaces a PID only if it is the CURRENT
// daemon.pid of a registered isolated state dir (or an explicitly registered
// PID). Keying on the state dir is load-bearing: the saver daemon respawns
// during bootstrap (placeholder → respawn, version-gate), so its PID is not
// stable — but each incarnation rewrites <stateDir>/daemon.pid, so reading that
// file every enumeration tracks the live PID with no re-registration. A PID
// registry alone went stale on the first respawn (scoped pgrep returned []).
// The developer's live daemon uses a different (unregistered) state dir, so it
// is never owned — including after its own ~10s churn respawns, since its fresh
// PID is likewise never in a registered dir's daemon.pid.
//
// Registration sources (all automatic): portaltest.IsolateStateForTest registers
// the test's primary state dir; portaltest.SpawnIsolatedDaemon registers each
// orphan's state dir. RegisterSandboxDaemon remains for the rare direct-PID case
// (e.g. multiple daemons sharing one state dir, where daemon.pid holds only the
// last writer). Enable + reset are driven by IsolateStateForTest.
//
// Concurrency: the daemon test suites use no t.Parallel (package convention), so
// within a test binary these run sequentially; the mutex guards only the
// theoretical case of a background goroutine calling PgrepPortalDaemons during a
// register. Separate test packages are separate processes with their own state.

var (
	sandboxMu        sync.Mutex
	sandboxEnabled   bool
	sandboxOwnedPID  map[int]bool
	sandboxOwnedDirs map[string]bool
	sandboxSources   []func() (int, bool)
)

// EnableDaemonSandbox turns on default-deny pgrep filtering for the current test
// process. Idempotent. Called by portaltest.IsolateStateForTest.
func EnableDaemonSandbox() {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	sandboxEnabled = true
	if sandboxOwnedPID == nil {
		sandboxOwnedPID = make(map[int]bool)
	}
	if sandboxOwnedDirs == nil {
		sandboxOwnedDirs = make(map[string]bool)
	}
}

// RegisterSandboxStateDir marks dir as a test-owned state directory. Its current
// daemon.pid is treated as test-owned on every enumeration — respawn-immune,
// because a fresh daemon rewrites daemon.pid without needing re-registration.
func RegisterSandboxStateDir(dir string) {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	if sandboxOwnedDirs == nil {
		sandboxOwnedDirs = make(map[string]bool)
	}
	sandboxOwnedDirs[dir] = true
}

// RegisterSandboxDaemon records an explicit test-owned PID. Belt-and-suspenders
// alongside state-dir ownership; needed only for daemons that never own a
// registered state dir's daemon.pid (e.g. the loser of a shared-state-dir race).
func RegisterSandboxDaemon(pid int) {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	if sandboxOwnedPID == nil {
		sandboxOwnedPID = make(map[int]bool)
	}
	sandboxOwnedPID[pid] = true
}

// RegisterSandboxDaemonSource registers a callback that returns a currently-live
// test-owned PID (and false when none). It is the most robust ownership signal:
// a source reading the live _portal-saver pane_pid tracks the saver across
// respawns AND is immune to tests that deliberately overwrite the legitimate
// daemon.pid with an orphan's PID (the PreFixDysfunction reproduction). The
// closure lives in the test package (which may read tmux), so state stores only
// the func value — no import cycle.
func RegisterSandboxDaemonSource(fn func() (int, bool)) {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	sandboxSources = append(sandboxSources, fn)
}

// ResetDaemonSandbox disables filtering and clears the registry. Registered as
// IsolateStateForTest's t.Cleanup so sandbox state cannot bleed across tests.
func ResetDaemonSandbox() {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	sandboxEnabled = false
	sandboxOwnedPID = nil
	sandboxOwnedDirs = nil
	sandboxSources = nil
}

// sandboxFilterPgrep drops every PID not owned by the current test while the
// sandbox is enabled; identity otherwise. A PID is owned iff it is an explicitly
// registered PID OR the current daemon.pid of a registered state dir. Safety
// property: an unregistered PID (the real daemon, or anything the test did not
// spawn) can never appear in a sweep candidate set, so it can never be SIGKILLed.
func sandboxFilterPgrep(pids []int) []int {
	sandboxMu.Lock()
	defer sandboxMu.Unlock()
	if !sandboxEnabled {
		return pids
	}
	owned := make(map[int]bool, len(sandboxOwnedPID)+len(sandboxOwnedDirs))
	for p := range sandboxOwnedPID {
		owned[p] = true
	}
	for dir := range sandboxOwnedDirs {
		if p, ok := readDaemonPIDFile(dir); ok {
			owned[p] = true
		}
	}
	for _, src := range sandboxSources {
		if p, ok := src(); ok {
			owned[p] = true
		}
	}
	out := make([]int, 0, len(pids))
	for _, p := range pids {
		if owned[p] {
			out = append(out, p)
		}
	}
	return out
}

// readDaemonPIDFile reads <stateDir>/daemon.pid and returns the parsed PID.
// Returns (0, false) on any error or non-positive value — a missing/half-written
// daemon.pid simply yields no ownership this enumeration (callers poll).
func readDaemonPIDFile(stateDir string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(stateDir, "daemon.pid"))
	if err != nil {
		return 0, false
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || p <= 0 {
		return 0, false
	}
	return p, true
}
