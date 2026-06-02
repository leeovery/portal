package portaltest

// log_level_resolved.go — test-only assertion that PORTAL_LOG_LEVEL
// propagated to a spawned portal process.
//
// The `process: log-level resolved` line (emitted by internal/log.Init
// immediately after `process: start`) is the positive marker that proves
// the resolved level took effect. It bypasses the level filter, so it is
// present even at PORTAL_LOG_LEVEL=warn/error. Integration tests that set
// PORTAL_LOG_LEVEL assert on this line via AssertLogLevelResolved so a
// silent propagation failure (tmux clearing the env on respawn-pane, or a
// harness forgetting to pass it) surfaces as a test failure rather than
// degraded-but-passing coverage.
//
// Test-only. AssertLogLevelResolved takes *testing.T first, keeping it in
// the package's *testing.T-first majority (the testing import would fail
// production builds). The pure parser findLogLevelResolved is unexported
// and unit-tested directly.

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// AssertLogLevelResolved scans the portal.log at logPath for the
// `process: log-level resolved` line matching the given pid and asserts the
// resolved level matches expected with source="env". Used by integration
// tests that set PORTAL_LOG_LEVEL.
//
// Callers typically pass state.PortalLog(stateDir) — the portal.log symlink,
// which os.ReadFile follows to today's day file automatically. Only text-mode
// (the production tail/grep default) is parsed; JSON mode is out of scope.
//
// It fails the test when:
//   - the file cannot be read (the marker can't be found if the log is gone);
//   - no log-level resolved line exists for pid (env did not propagate);
//   - the matched line's source is not "env" (default/fallback => the harness
//     did not set PORTAL_LOG_LEVEL or set it to an invalid value);
//   - the matched line's resolved level differs from expected.
func AssertLogLevelResolved(t *testing.T, logPath string, pid int, expected string) {
	t.Helper()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("AssertLogLevelResolved: cannot read log at %s: %v", logPath, err)
		return
	}
	content := string(data)

	resolved, source, found := findLogLevelResolved(content, pid)
	if !found {
		t.Errorf("PORTAL_LOG_LEVEL did not propagate: no process: log-level resolved line for pid=%d\n--- %s ---\n%s",
			pid, logPath, content)
		return
	}
	if source != "env" {
		t.Errorf("PORTAL_LOG_LEVEL was not the resolution source for pid=%d: source=%q (want env; default/fallback means the harness did not set it or set it invalidly)",
			pid, source)
	}
	if resolved != expected {
		t.Errorf("resolved log level for pid=%d = %q, want %q", pid, resolved, expected)
	}
}

// findLogLevelResolved scans content line-by-line for the process: log-level
// resolved line whose pid attr equals pid, returning its resolved and source
// attr values. It tolerates baseline-attr ordering (attrs are parsed into a map,
// not by position) and strips surrounding double quotes from quoted values.
//
// Multiple processes may have written to the same day file (reboot recovery), so
// the pid match is load-bearing: every candidate line is checked and only the one
// whose pid attr equals pid is selected. Returns ("", "", false) when no matching
// line exists.
func findLogLevelResolved(content string, pid int) (resolved, source string, found bool) {
	wantPID := strconv.Itoa(pid)
	for _, line := range strings.Split(content, "\n") {
		if !isLogLevelResolvedLine(line) {
			continue
		}
		attrs := parseLogAttrs(line)
		if attrs["pid"] != wantPID {
			continue
		}
		return attrs["resolved"], attrs["source"], true
	}
	return "", "", false
}

// isLogLevelResolvedLine reports whether line is a process-component
// log-level resolved record. The component renders as the literal "process:"
// prefix before the message, so both the prefix and the message must be present.
func isLogLevelResolvedLine(line string) bool {
	return strings.Contains(line, "process:") &&
		strings.Contains(line, "log-level resolved")
}

// parseLogAttrs extracts the trailing key=value attr pairs of a text-mode log
// line into a map. It splits on whitespace, keeps only tokens containing '=',
// and strips surrounding double quotes from values. Quoted values containing
// spaces are not reconstructed — the attrs this helper reads (pid/resolved/
// source) are all single-token, and raw is only ever a single token in practice.
func parseLogAttrs(line string) map[string]string {
	attrs := make(map[string]string)
	for _, tok := range strings.Fields(line) {
		k, v, ok := strings.Cut(tok, "=")
		if !ok {
			continue
		}
		attrs[k] = strings.Trim(v, `"`)
	}
	return attrs
}
