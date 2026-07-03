//go:build integration

// Scaffolding for the transient-listpanes integration test covering the
// `portal clean` RunE hook-cleanup callsite
// (cmd/cleanstale_transient_listpanes_clean_integration_test.go). The
// companion bootstrap-step callsite was retired when hooks stale-cleanup
// left the orchestrator, so this file no longer straddles two consumers;
// its helpers (isolateCleanStaleTestEnv, runTransientCleanStaleModeSubtest,
// configDirFromEnvSlice, staleHookCleanupLogLines, containsLineMatching)
// are consumed by that single file.
//
// isolateCleanStaleTestEnv is the single source of truth for the four
// invariant scaffolding steps: portaltest.IsolateStateForTest →
// Setenv("PORTAL_STATE_DIR") → Setenv("PORTAL_LOG_LEVEL", "debug") →
// Setenv("XDG_CONFIG_HOME", ...). runTransientCleanStaleModeSubtest is the
// table-driven driver for the mode_a / mode_b subtest shape (byte-identity
// hooks.json invariant + `runHookStaleCleanup` log-fingerprint needles);
// the callsite passes a `transientModeSpec` declaring only the deltas
// (entry-point invoker closure + an optional extra-assert closure to verify
// no "Removed stale hook:" line surfaces on stdout).
//
// The "normal_path" and "persisted_empty_early_exit" subtests are left as
// bespoke subtest bodies in the consumer file — they diverge enough in
// shape that table-driving them would obscure rather than clarify.
//
// Package + build tag match the consumer file so the shared helpers are
// visible at compile time and only compile under the integration build.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/transienttest"
)

// isolateCleanStaleTestEnv performs the four invariant scaffolding
// steps shared by every transient-listpanes integration subtest:
//
//  1. portaltest.IsolateStateForTest — scrubs HOME / XDG on the test
//     process and registers the fingerprint-diff backstop. Returns
//     the env slice (carrying the isolated XDG_CONFIG_HOME for any
//     would-be subprocesses) and the isolated stateDir.
//  2. Setenv("PORTAL_STATE_DIR", stateDir) — so any in-process or
//     subprocess code resolving the state dir lands on the isolated
//     one (the log handler, ReadPortalLogSafe, and the saver
//     pane's `portal state daemon` all observe this).
//  3. Setenv("PORTAL_LOG_LEVEL", "debug") — the assertions in both
//     callsites' subtests grep for Debug breadcrumbs that the
//     default log level (LevelWarn) would suppress.
//  4. Setenv("XDG_CONFIG_HOME", configDir) — the load-bearing
//     XDG_CONFIG_HOME re-push.
//
// XDG_CONFIG_HOME re-push rationale (documented here once so the
// two file-level helpers can stay focused on their extras):
// portaltest.IsolateStateForTest deliberately sets XDG_CONFIG_HOME=""
// on the test process — its fingerprint-diff backstop relies on the
// scrubbed test-process env to detect leaks against the developer's
// real config dir — and only injects the isolated XDG_CONFIG_HOME
// into the returned env slice (which subprocesses pick up via
// `cmd.Env = env`). Both the bootstrap orchestrator and the
// `portal clean` RunE run IN the test process, so cmd-package config
// path resolution (`configFilePath` → `xdg.ConfigBase` →
// `$XDG_CONFIG_HOME`) would resolve to the test process's HOME-based
// fallback and miss the seeded hooks.json. Re-pushing
// XDG_CONFIG_HOME onto the test process here AFTER IsolateStateForTest
// has snapshotted the pre-test state is safe — the backstop captures
// its baseline before this call returns, so the post-snapshot Setenv
// does not perturb it.
func isolateCleanStaleTestEnv(t *testing.T) (env []string, stateDir string) {
	t.Helper()
	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	t.Setenv("XDG_CONFIG_HOME", configDirFromEnvSlice(t, env))
	return env, stateDir
}

// transientModeSpec parameterises the table-driven driver below.
// Only the deltas between the mode_a / mode_b subtest bodies
// are declared — the seed map, the snapshot/byte-identity assertion,
// and the `runHookStaleCleanup` log-fingerprint needles are baked
// into the driver itself.
//
// Fields:
//   - name: the subtest name (also used in failure messages to
//     identify which mode failed).
//   - mode: which transienttest.FailureMode the Commander should
//     simulate. Drives both the install-commander step and the
//     mode-specific log-fingerprint needles the driver asserts.
//   - invoke: entry-point closure. Receives the env slice + stateDir
//     produced by the env-builder and is responsible for installing the
//     portal-clean callsite's commander seam (`cleanDeps.AllPaneLister`),
//     invoking the entry point, and returning any post-invocation
//     output the extraAssert may want to inspect. A non-nil err
//     return fails the subtest with a callsite-appropriate message
//     supplied by the closure.
//   - extraAssert: optional post-invocation assertions beyond the
//     shared byte-identity + log-fingerprint asserts. The portal-clean
//     callsite uses this to additionally verify no "Removed stale hook:"
//     line surfaces on stdout.
type transientModeSpec struct {
	name        string
	mode        transienttest.FailureMode
	invoke      func(t *testing.T, env []string, stateDir string) (output string, err error)
	extraAssert func(t *testing.T, output string, seededKeys []string)
}

// transientModeSeedEntries is the canonical seed map shared by every
// mode_a / mode_b subtest in both callsite files. Three entries are
// the minimum needed to make the `persisted=3` substring meaningful
// (any positive count would do; three is what the original subtests
// used and the log-fingerprint needles below pin it).
var transientModeSeedEntries = map[string]string{
	"alpha:0.0": "echo a",
	"beta:0.0":  "echo b",
	"gamma:0.0": "echo c",
}

// runTransientCleanStaleModeSubtest executes the six-step shape
// shared by the mode_a / mode_b subtests in both callsite files:
//
//  1. isolateCleanStaleTestEnv — shared scaffolding.
//  2. SeedHooksJSON + snapshot `before` bytes.
//  3. spec.invoke — install callsite-specific commander seam and
//     fire the entry point.
//  4. snapshot `after` bytes and assert byte-identity (the wipe
//     invariant — the whole point of the workstream).
//  5. spec.extraAssert (if non-nil) — callsite-specific extras.
//  6. portal.log fingerprint needles for spec.mode.
//
// Failure messages reference spec.name so a regression in either mode
// subtest is unambiguous when they run in the same `go test` invocation.
func runTransientCleanStaleModeSubtest(t *testing.T, spec transientModeSpec) {
	t.Helper()

	env, stateDir := isolateCleanStaleTestEnv(t)

	transienttest.SeedHooksJSON(t, env, transientModeSeedEntries)
	before := transienttest.HooksJSONBytes(t, env)
	if len(before) == 0 {
		t.Fatalf("precondition: hooksJSONBytes returned empty slice after seed (subtest %s)", spec.name)
	}

	output, err := spec.invoke(t, env, stateDir)
	if err != nil {
		t.Fatalf("entry point returned error under %s; want nil (Warn-and-swallow contract): %v\n  output:\n%s",
			spec.name, err, output)
	}

	after := transienttest.HooksJSONBytes(t, env)
	if !bytes.Equal(before, after) {
		t.Fatalf("hooks.json mutated under %s — the wipe regression has returned\n"+
			"  before: %s\n"+
			"  after:  %s",
			spec.name, before, after)
	}

	seededKeys := make([]string, 0, len(transientModeSeedEntries))
	for k := range transientModeSeedEntries {
		seededKeys = append(seededKeys, k)
	}
	if spec.extraAssert != nil {
		spec.extraAssert(t, output, seededKeys)
	}

	lines := staleHookCleanupLogLines(portaltest.ReadPortalLogSafe(stateDir))
	if len(lines) == 0 {
		t.Fatalf("no `stale-hook cleanup:` lines found in portal.log under %s; want at least one\n"+
			"  full log:\n%s",
			spec.name, portaltest.ReadPortalLogSafe(stateDir))
	}

	switch spec.mode {
	case transienttest.FailExitNonZero:
		// mode (a): propagated-error Warn must be present; the
		// entry-point Debug (`live=...`) must be absent — the
		// err-from-ListAllPanes branch returns BEFORE the Debug
		// emission.
		if !containsLineMatching(lines, "stale-hook cleanup:", "list-panes failed", "simulated transient") {
			t.Fatalf("missing mode (a) propagated-error Warn line under %s; want a `stale-hook cleanup:` line containing `list-panes failed` and `simulated transient`\n"+
				"  matched stale-hook lines:\n%s",
				spec.name, strings.Join(lines, "\n"))
		}
		for _, line := range lines {
			if strings.Contains(line, "live=") {
				t.Fatalf("mode (a) emitted entry-point Debug (`live=...`) under %s; must be absent — the err-from-ListAllPanes branch returns before the Debug emission\n"+
					"  offending line: %s",
					spec.name, line)
			}
		}
	case transienttest.FailEmptyStdout:
		// mode (b): entry-point Debug (live=0, persisted=N) AND the
		// hazard-guard Warn must both be present.
		if !containsLineMatching(lines, "stale-hook cleanup:", "live=0", "persisted=3") {
			t.Fatalf("missing mode (b) entry-point Debug under %s; want a `stale-hook cleanup:` line containing `live=0` and `persisted=3`\n"+
				"  matched stale-hook lines:\n%s",
				spec.name, strings.Join(lines, "\n"))
		}
		if !containsLineMatching(lines, "stale-hook cleanup:", "zero live panes", "3 hook(s) present", "mass-deletion hazard") {
			t.Fatalf("missing mode (b) hazard-guard Warn under %s; want a `stale-hook cleanup:` line containing `zero live panes`, `3 hook(s) present`, and `mass-deletion hazard`\n"+
				"  matched stale-hook lines:\n%s",
				spec.name, strings.Join(lines, "\n"))
		}
	default:
		t.Fatalf("runTransientCleanStaleModeSubtest: unsupported FailureMode %v for subtest %s — driver supports only FailExitNonZero / FailEmptyStdout",
			spec.mode, spec.name)
	}
}

// configDirFromEnvSlice extracts the XDG_CONFIG_HOME value from the
// env slice produced by portaltest.IsolateStateForTest. The slice
// always contains exactly one such entry — its absence signals an
// isolation regression worth a fatal test failure.
func configDirFromEnvSlice(t *testing.T, env []string) string {
	t.Helper()
	const key = "XDG_CONFIG_HOME="
	for _, e := range env {
		if strings.HasPrefix(e, key) {
			return strings.TrimPrefix(e, key)
		}
	}
	t.Fatalf("configDirFromEnvSlice: XDG_CONFIG_HOME not present in env slice — IsolateStateForTest contract regression")
	return ""
}

// staleHookCleanupLogLines returns the subset of portal.log lines that
// carry the stale-hook cleanup prefix. Filtering on the prefix excludes
// bootstrap step 9's CleanStaleMarkers log lines (which have their own
// prefix) and any unrelated noise, narrowing the assertion surface to
// exactly the hook-cleanup path under test.
func staleHookCleanupLogLines(portalLog string) []string {
	const prefix = "stale-hook cleanup:"
	var matches []string
	for _, line := range strings.Split(portalLog, "\n") {
		if strings.Contains(line, prefix) {
			matches = append(matches, line)
		}
	}
	return matches
}

// containsLineMatching reports whether any line contains every
// substring in needles (AND semantics).
func containsLineMatching(lines []string, needles ...string) bool {
	for _, line := range lines {
		matched := true
		for _, n := range needles {
			if !strings.Contains(line, n) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
