// White-box tests for the log-level propagation assertion helper.
// findLogLevelResolved holds all the parsing logic and is exercised
// directly here; AssertLogLevelResolved is a thin file-reading wrapper
// whose only non-trivial seam (symlink-follow read) gets its own test.
package portaltest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// resolvedLine renders a process: log-level resolved line in the exact
// text-mode shape emitted by internal/log's handler, so the parser is
// tested against the real on-disk format.
func resolvedLine(resolved, source, raw string, pid int) string {
	return fmt.Sprintf(
		"2026-05-30T14:00:00Z INFO process: log-level resolved resolved=%s source=%s raw=%q pid=%d version=0.5.0 process_role=daemon",
		resolved, source, raw, pid,
	)
}

func TestFindLogLevelResolved_SelectsByPIDAmongMultipleProcesses(t *testing.T) {
	content := resolvedLine("info", "default", "", 111) + "\n" +
		resolvedLine("debug", "env", "DEBUG", 222) + "\n"

	resolved, source, found := findLogLevelResolved(content, 222)
	if !found {
		t.Fatalf("expected to find the line for pid=222")
	}
	if resolved != "debug" {
		t.Errorf("resolved = %q, want %q", resolved, "debug")
	}
	if source != "env" {
		t.Errorf("source = %q, want %q", source, "env")
	}

	// And the other process selects its own distinct line.
	resolved, source, found = findLogLevelResolved(content, 111)
	if !found {
		t.Fatalf("expected to find the line for pid=111")
	}
	if resolved != "info" || source != "default" {
		t.Errorf("pid=111 line = (%q,%q), want (info,default)", resolved, source)
	}
}

func TestFindLogLevelResolved_NotFoundWhenNoLineForPID(t *testing.T) {
	content := resolvedLine("debug", "env", "DEBUG", 222) + "\n"

	resolved, source, found := findLogLevelResolved(content, 999)
	if found {
		t.Errorf("expected found=false for absent pid, got (%q,%q,true)", resolved, source)
	}
	if resolved != "" || source != "" {
		t.Errorf("expected empty (resolved,source) when not found, got (%q,%q)", resolved, source)
	}
}

func TestFindLogLevelResolved_IgnoresNonResolvedLines(t *testing.T) {
	// A process: start line for the same pid must not be mistaken for the
	// resolved line.
	startLine := fmt.Sprintf(
		"2026-05-30T14:00:00Z INFO process: start cmd=portal args=open pid=%d version=0.5.0 process_role=daemon",
		222,
	)
	content := startLine + "\n"

	_, _, found := findLogLevelResolved(content, 222)
	if found {
		t.Errorf("expected found=false when only a process: start line exists for the pid")
	}
}

func TestFindLogLevelResolved_HappyPathReturnsResolvedAndSource(t *testing.T) {
	content := resolvedLine("warn", "env", "warn", 555) + "\n"

	resolved, source, found := findLogLevelResolved(content, 555)
	if !found {
		t.Fatalf("expected to find the line for pid=555")
	}
	if resolved != "warn" || source != "env" {
		t.Errorf("got (%q,%q), want (warn,env)", resolved, source)
	}
}

func TestFindLogLevelResolved_ToleratesBaselineAttrOrdering(t *testing.T) {
	// Non-standard ordering: pid before resolved/source, baselines interleaved.
	// The parser must key off attr names, not positions.
	line := "2026-05-30T14:00:00Z INFO process: log-level resolved pid=777 version=0.5.0 source=env process_role=daemon resolved=debug raw=\"DEBUG\""
	content := line + "\n"

	resolved, source, found := findLogLevelResolved(content, 777)
	if !found {
		t.Fatalf("expected to find the line for pid=777 with reordered attrs")
	}
	if resolved != "debug" || source != "env" {
		t.Errorf("got (%q,%q), want (debug,env)", resolved, source)
	}
}

func TestFindLogLevelResolved_StripsQuotesFromQuotedValues(t *testing.T) {
	// raw is rendered quoted by the handler; resolved/source are single tokens
	// but the parser must strip surrounding quotes uniformly if present.
	line := "2026-05-30T14:00:00Z INFO process: log-level resolved resolved=\"debug\" source=\"env\" raw=\"DEBUG\" pid=888 version=0.5.0 process_role=daemon"
	content := line + "\n"

	resolved, source, found := findLogLevelResolved(content, 888)
	if !found {
		t.Fatalf("expected to find the quoted-value line for pid=888")
	}
	if resolved != "debug" || source != "env" {
		t.Errorf("got (%q,%q), want (debug,env) after quote-strip", resolved, source)
	}
}

// TestAssertLogLevelResolved_FollowsSymlinkToCurrentDayFile writes a real day
// file plus a portal.log symlink pointing at it, points logPath at the symlink,
// and asserts the helper reads through it. Uses a fresh sub-T so a failure here
// would surface as a real test failure (the line is present and correct, so the
// helper must NOT fail).
func TestAssertLogLevelResolved_FollowsSymlinkToCurrentDayFile(t *testing.T) {
	dir := t.TempDir()
	dayFile := filepath.Join(dir, "portal-2026-05-30.bin")
	symlink := filepath.Join(dir, "portal.log")

	if err := os.WriteFile(dayFile, []byte(resolvedLine("debug", "env", "DEBUG", 1234)+"\n"), 0o644); err != nil {
		t.Fatalf("write day file: %v", err)
	}
	if err := os.Symlink(dayFile, symlink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// This must pass cleanly (no t.Errorf/Fatalf fired on the real T).
	AssertLogLevelResolved(t, symlink, 1234, "debug")
}
