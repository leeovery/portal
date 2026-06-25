package state_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// writeRealLogLine appends a single real-writer log line to path, sourcing the
// rendered bytes from the production render seam (log.RenderLineForTest) so the
// fixture is byte-identical to what the writer emits. The seam output already
// ends with a trailing '\n', so it is appended verbatim — never re-formatted
// from an independent format string. ts is normalised to UTC (matching the
// writer's render path) so the window math is unchanged.
func writeRealLogLine(t *testing.T, path string, ts time.Time, level slog.Level, component, msg string) {
	t.Helper()
	line := log.RenderLineForTest(t, ts.UTC(), level, component, msg)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		t.Fatalf("write log line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

// writeIndex encodes idx and writes it to sessions.json under dir.
func writeIndex(t *testing.T, dir string, idx state.Index) {
	t.Helper()
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

func TestCollectStatus_DaemonRunningFalseWhenPIDFileMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.DaemonRunning {
		t.Errorf("DaemonRunning = true; want false when daemon.pid is missing")
	}
	if rep.DaemonPID != 0 {
		t.Errorf("DaemonPID = %d; want 0 when daemon.pid is missing", rep.DaemonPID)
	}
}

func TestCollectStatus_DaemonRunningTrueForLivePID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if err := state.WritePIDFile(dir, os.Getpid()); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if !rep.DaemonRunning {
		t.Errorf("DaemonRunning = false; want true for our own PID")
	}
	if rep.DaemonPID != os.Getpid() {
		t.Errorf("DaemonPID = %d; want %d", rep.DaemonPID, os.Getpid())
	}
}

func TestCollectStatus_DaemonRunningFalseForDeadPID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if err := state.WritePIDFile(dir, 999999); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.DaemonRunning {
		t.Errorf("DaemonRunning = true; want false for unused PID 999999")
	}
	if rep.DaemonPID != 999999 {
		t.Errorf("DaemonPID = %d; want 999999", rep.DaemonPID)
	}
}

func TestCollectStatus_DaemonVersionFromVersionFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if err := state.WriteVersionFile(dir, "v1.2.3", nil); err != nil {
		t.Fatalf("WriteVersionFile: %v", err)
	}

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.DaemonVersion != "v1.2.3" {
		t.Errorf("DaemonVersion = %q; want %q", rep.DaemonVersion, "v1.2.3")
	}
}

func TestCollectStatus_DaemonVersionEmptyWhenVersionFileMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.DaemonVersion != "" {
		t.Errorf("DaemonVersion = %q; want empty when daemon.version is missing", rep.DaemonVersion)
	}
}

func TestCollectStatus_HasLastSaveFalseWhenSessionsJSONMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.HasLastSave {
		t.Errorf("HasLastSave = true; want false when sessions.json is missing")
	}
	if rep.SessionsCount != 0 {
		t.Errorf("SessionsCount = %d; want 0", rep.SessionsCount)
	}
	if rep.PanesCount != 0 {
		t.Errorf("PanesCount = %d; want 0", rep.PanesCount)
	}
	if !rep.LastSaveAt.IsZero() {
		t.Errorf("LastSaveAt = %v; want zero", rep.LastSaveAt)
	}
}

func TestCollectStatus_PopulatesCountsFromSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	saved := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	idx := state.Index{
		Version: state.SchemaVersion,
		SavedAt: saved,
		Sessions: []state.Session{
			{
				Name: "work",
				Windows: []state.Window{
					{Index: 0, Panes: []state.Pane{{Index: 0}, {Index: 1}}},
					{Index: 1, Panes: []state.Pane{{Index: 0}}},
				},
			},
			{
				Name: "play",
				Windows: []state.Window{
					{Index: 0, Panes: []state.Pane{{Index: 0}, {Index: 1}, {Index: 2}}},
				},
			},
		},
	}
	writeIndex(t, dir, idx)

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if !rep.HasLastSave {
		t.Errorf("HasLastSave = false; want true")
	}
	if rep.SessionsCount != 2 {
		t.Errorf("SessionsCount = %d; want 2", rep.SessionsCount)
	}
	// 2 + 1 + 3 = 6 panes total.
	if rep.PanesCount != 6 {
		t.Errorf("PanesCount = %d; want 6", rep.PanesCount)
	}
}

func TestCollectStatus_LastSaveAtMatchesSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	saved := time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC)
	writeIndex(t, dir, state.Index{
		Version: state.SchemaVersion,
		SavedAt: saved,
	})

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if !rep.LastSaveAt.Equal(saved) {
		t.Errorf("LastSaveAt = %v; want %v", rep.LastSaveAt, saved)
	}
}

func TestCollectStatus_StateSizeZeroWhenDirMissing(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does-not-exist")
	t.Setenv("PORTAL_STATE_DIR", missing)

	rep, err := state.CollectStatus(missing, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.StateSize != 0 {
		t.Errorf("StateSize = %d; want 0 when directory is missing", rep.StateSize)
	}
}

func TestCollectStatus_StateSizeSumsFileSizes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte("1234567890"), 0o600); err != nil {
		t.Fatalf("write a.bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte("ABC"), 0o600); err != nil {
		t.Fatalf("write b.bin: %v", err)
	}

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.StateSize != 13 {
		t.Errorf("StateSize = %d; want 13 (10 + 3)", rep.StateSize)
	}
}

func TestCollectStatus_StateSizeIncludesScrollbackSubdir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	scrollback := filepath.Join(dir, "scrollback")
	if err := os.MkdirAll(scrollback, 0o700); err != nil {
		t.Fatalf("mkdir scrollback: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.bin"), []byte("AAA"), 0o600); err != nil {
		t.Fatalf("write top.bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scrollback, "pane.bin"), []byte("BBBBB"), 0o600); err != nil {
		t.Fatalf("write pane.bin: %v", err)
	}

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.StateSize != 8 {
		t.Errorf("StateSize = %d; want 8 (3 + 5 across top + scrollback subdir)", rep.StateSize)
	}
}

func TestCollectStatus_RecentWarningsZeroWhenLogMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	rep, err := state.CollectStatus(dir, time.Now())
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 0 {
		t.Errorf("RecentWarnings = %d; want 0 when portal.log is missing", rep.RecentWarnings)
	}
	if rep.LastWarning != "" {
		t.Errorf("LastWarning = %q; want empty when portal.log is missing", rep.LastWarning)
	}
}

func TestCollectStatus_DoesNotScanPortalLogOld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)

	// Old log full of WARN entries within the cutoff window — must be ignored.
	oldPath := state.PortalLogOld(dir)
	for range 5 {
		writeRealLogLine(t, oldPath, recent, slog.LevelWarn, "daemon", "old warning")
	}

	// Current log holds a single WARN.
	logPath := state.PortalLog(dir)
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "current warning")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 1 {
		t.Errorf("RecentWarnings = %d; want 1 (portal.log.old must be ignored)", rep.RecentWarnings)
	}
}

func TestCollectStatus_CountsWarnAndErrorEntriesInWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-30 * time.Minute)

	logPath := state.PortalLog(dir)
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "warn-1")
	writeRealLogLine(t, logPath, recent, slog.LevelError, "daemon", "error-1")
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "restore", "warn-2")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 3 {
		t.Errorf("RecentWarnings = %d; want 3", rep.RecentWarnings)
	}
}

func TestCollectStatus_IgnoresInfoAndDebugEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-15 * time.Minute)

	logPath := state.PortalLog(dir)
	writeRealLogLine(t, logPath, recent, slog.LevelInfo, "daemon", "info-1")
	writeRealLogLine(t, logPath, recent, slog.LevelDebug, "daemon", "debug-1")
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "warn-1")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 1 {
		t.Errorf("RecentWarnings = %d; want 1 (INFO and DEBUG must be ignored)", rep.RecentWarnings)
	}
}

func TestCollectStatus_ToleratesMalformedLogEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-5 * time.Minute)

	logPath := state.PortalLog(dir)

	// Append various lines that do NOT match the slog text layout: no colon,
	// fewer than two whitespace tokens, and a first token that is not an
	// RFC3339Nano timestamp. None should parse; the scan must continue.
	malformed := []string{
		"no colon at all here",
		"lonelytoken",
		"not-a-timestamp WARN daemon: bad ts",
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	for _, line := range malformed {
		if _, err := f.WriteString(line + "\n"); err != nil {
			_ = f.Close()
			t.Fatalf("write malformed: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "valid warn")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 1 {
		t.Errorf("RecentWarnings = %d; want 1 (only the valid WARN must count)", rep.RecentWarnings)
	}
}

func TestCollectStatus_UsesCallerSuppliedNowForWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	logPath := state.PortalLog(dir)

	// Both entries are 30 minutes apart and dated in 2020. With a now
	// anchored in 2026, both fall outside the 1-hour window. With a now
	// matching their era, both fall inside.
	ts1 := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2020, 1, 1, 12, 30, 0, 0, time.UTC)
	writeRealLogLine(t, logPath, ts1, slog.LevelWarn, "daemon", "old-1")
	writeRealLogLine(t, logPath, ts2, slog.LevelWarn, "daemon", "old-2")

	farFuture := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	rep, err := state.CollectStatus(dir, farFuture)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 0 {
		t.Errorf("RecentWarnings = %d; want 0 with now far after entries", rep.RecentWarnings)
	}

	contemporary := time.Date(2020, 1, 1, 13, 0, 0, 0, time.UTC)
	rep, err = state.CollectStatus(dir, contemporary)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 2 {
		t.Errorf("RecentWarnings = %d; want 2 with contemporary now", rep.RecentWarnings)
	}
}

func TestCollectStatus_LastWarningHoldsLastValidEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-20 * time.Minute)

	logPath := state.PortalLog(dir)
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "first warn")
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "second warn")
	writeRealLogLine(t, logPath, recent, slog.LevelWarn, "daemon", "last warn")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 3 {
		t.Errorf("RecentWarnings = %d; want 3", rep.RecentWarnings)
	}
	if want := "WARN daemon: last warn"; rep.LastWarning != want {
		t.Errorf("LastWarning = %q; want %q (last entry wins)", rep.LastWarning, want)
	}
}

func TestCollectStatus_SkipsEntriesOlderThanCutoff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	logPath := state.PortalLog(dir)
	// Two hours ago: outside the 1-hour window.
	writeRealLogLine(t, logPath, now.Add(-2*time.Hour), slog.LevelWarn, "daemon", "stale warn")
	// Twenty minutes ago: inside the window.
	writeRealLogLine(t, logPath, now.Add(-20*time.Minute), slog.LevelWarn, "daemon", "fresh warn")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings != 1 {
		t.Errorf("RecentWarnings = %d; want 1 (older-than-cutoff must be skipped)", rep.RecentWarnings)
	}
}

func TestCollectStatus_LastWarningRendersLevelComponentMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)

	writeRealLogLine(t, state.PortalLog(dir), recent, slog.LevelWarn, "daemon", "tick complete")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if want := "WARN daemon: tick complete"; rep.LastWarning != want {
		t.Errorf("LastWarning = %q; want %q", rep.LastWarning, want)
	}
}

func TestCollectStatus_LastWarningRendersWithoutComponent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)

	// Empty component → "WARN: <msg>" with no stray space before the colon.
	writeRealLogLine(t, state.PortalLog(dir), recent, slog.LevelWarn, "", "tick complete")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if want := "WARN: tick complete"; rep.LastWarning != want {
		t.Errorf("LastWarning = %q; want %q (no stray space before colon)", rep.LastWarning, want)
	}
}

func TestCollectStatus_LastWarningRendersWithoutTrailingSpaceWhenMessageEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)

	// Empty message → "WARN daemon:" with no trailing space.
	writeRealLogLine(t, state.PortalLog(dir), recent, slog.LevelWarn, "daemon", "")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if want := "WARN daemon:"; rep.LastWarning != want {
		t.Errorf("LastWarning = %q; want %q (no trailing space)", rep.LastWarning, want)
	}
}

// TestCollectStatus_CountsRealWriterWarnLineEndToEnd is the producer-coupled
// regression guard: the WARN fixture is sourced from the real internal/log
// writer via the render seam, so any future drift in the writer's line format
// breaks this test (the anti-false-green guarantee). It was the missing test
// that let the pipe-format reader rot silently.
func TestCollectStatus_CountsRealWriterWarnLineEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-10 * time.Minute)

	writeRealLogLine(t, state.PortalLog(dir), recent, slog.LevelWarn, "daemon", "flush failed")

	rep, err := state.CollectStatus(dir, now)
	if err != nil {
		t.Fatalf("CollectStatus: %v", err)
	}
	if rep.RecentWarnings < 1 {
		t.Errorf("RecentWarnings = %d; want >= 1 from a real-writer WARN line", rep.RecentWarnings)
	}
	if want := "WARN daemon: flush failed"; rep.LastWarning != want {
		t.Errorf("LastWarning = %q; want %q", rep.LastWarning, want)
	}
}
