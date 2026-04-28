// Tests in this file mutate package-level Cobra state and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// runStateStatus executes "portal state status" with PORTAL_STATE_DIR pointing
// at dir, returning stdout, stderr, and the rootCmd.Execute error.
func runStateStatus(t *testing.T, dir string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	t.Setenv("PORTAL_STATE_DIR", dir)
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"state", "status"})
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

// writeIndexAt writes a sessions.json file with the supplied saved_at timestamp
// and a single session containing a single window with a single pane.
func writeIndexAt(t *testing.T, dir string, savedAt time.Time) {
	t.Helper()
	idx := state.Index{
		Version: state.SchemaVersion,
		SavedAt: savedAt,
		Sessions: []state.Session{
			{
				Name: "work",
				Windows: []state.Window{
					{
						Index: 0,
						Name:  "main",
						Panes: []state.Pane{
							{Index: 0, CWD: "/tmp"},
						},
					},
				},
			},
		},
	}
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

// writePIDAlive writes daemon.pid pointing at the current process so the
// liveness probe always succeeds in-test.
func writePIDAlive(t *testing.T, dir string) {
	t.Helper()
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(state.DaemonPID(dir), []byte(pid+"\n"), 0o600); err != nil {
		t.Fatalf("write daemon.pid: %v", err)
	}
}

// writeVersion writes daemon.version with the supplied marker.
func writeVersion(t *testing.T, dir string, version string) {
	t.Helper()
	if err := os.WriteFile(state.DaemonVersion(dir), []byte(version+"\n"), 0o600); err != nil {
		t.Fatalf("write daemon.version: %v", err)
	}
}

// touchEmptyLog creates an empty portal.log so warnings scanning is exercised
// without producing any matches.
func touchEmptyLog(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(state.PortalLog(dir), nil, 0o600); err != nil {
		t.Fatalf("write portal.log: %v", err)
	}
}

func TestStateStatusHealthyOutput(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)

	outBuf, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Fatalf("Execute returned %v; want nil for healthy state", err)
	}

	out := outBuf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("expected 7 output lines (header + 6 fields); got %d:\n%s", len(lines), out)
	}
	if lines[0] != "Portal state:" {
		t.Errorf("line 0 = %q; want %q", lines[0], "Portal state:")
	}
	pid := os.Getpid()
	wantDaemon := "  Save daemon: running (pid " + strconv.Itoa(pid) + ", version v0.4.2)"
	if lines[1] != wantDaemon {
		t.Errorf("line 1 = %q; want %q", lines[1], wantDaemon)
	}
	if !strings.HasPrefix(lines[2], "  Last save: ") || !strings.HasSuffix(lines[2], " ago") {
		t.Errorf("line 2 = %q; want \"Last save: <duration> ago\"", lines[2])
	}
	if lines[3] != "  Sessions captured: 1" {
		t.Errorf("line 3 = %q; want \"  Sessions captured: 1\"", lines[3])
	}
	if lines[4] != "  Panes captured: 1" {
		t.Errorf("line 4 = %q; want \"  Panes captured: 1\"", lines[4])
	}
	if !strings.HasPrefix(lines[5], "  State size: ") || !strings.HasSuffix(lines[5], " on disk") {
		t.Errorf("line 5 = %q; want \"  State size: <size> on disk\"", lines[5])
	}
	if lines[6] != "  Recent warnings: 0 (last: none)" {
		t.Errorf("line 6 = %q; want \"  Recent warnings: 0 (last: none)\"", lines[6])
	}
}

func TestStateStatusDaemonNotRunning(t *testing.T) {
	dir := t.TempDir()
	// no daemon.pid -> not running
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)

	outBuf, _, err := runStateStatus(t, dir)
	if err == nil {
		t.Fatal("Execute returned nil; want non-nil ErrStatusUnhealthy when daemon absent")
	}
	if err != ErrStatusUnhealthy {
		t.Errorf("Execute err = %v; want ErrStatusUnhealthy", err)
	}
	if !strings.Contains(outBuf.String(), "  Save daemon: not running\n") {
		t.Errorf("output missing \"Save daemon: not running\":\n%s", outBuf.String())
	}
}

func TestStateStatusLastSaveNeverWhenIndexMissing(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	// no sessions.json
	touchEmptyLog(t, dir)

	outBuf, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Fatalf("Execute returned %v; want nil when daemon healthy and no save yet", err)
	}
	if !strings.Contains(outBuf.String(), "  Last save: never\n") {
		t.Errorf("output missing \"Last save: never\":\n%s", outBuf.String())
	}
}

func TestStateStatusRecentWarningsNoneSuffixWhenZero(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)

	outBuf, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Fatalf("Execute returned %v; want nil for healthy state", err)
	}
	if !strings.Contains(outBuf.String(), "  Recent warnings: 0 (last: none)\n") {
		t.Errorf("output missing zero-warnings line:\n%s", outBuf.String())
	}
}

func TestStateStatusRecentWarningsLastLineSuffixWhenNonZero(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))

	now := time.Now().UTC()
	logLine := now.Format(time.RFC3339) + " | WARN | daemon | flush failed: disk full"
	if err := os.WriteFile(state.PortalLog(dir), []byte(logLine+"\n"), 0o600); err != nil {
		t.Fatalf("write portal.log: %v", err)
	}

	outBuf, _, err := runStateStatus(t, dir)
	// Recent warnings > 0 makes status unhealthy, expect ErrStatusUnhealthy.
	if err != ErrStatusUnhealthy {
		t.Errorf("Execute err = %v; want ErrStatusUnhealthy when warnings > 0", err)
	}
	want := "  Recent warnings: 1 (last: " + logLine + ")\n"
	if !strings.Contains(outBuf.String(), want) {
		t.Errorf("output missing warnings-with-last line %q:\n%s", want, outBuf.String())
	}
}

func TestStateStatusExitZeroWhenHealthy(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-1*time.Minute))
	touchEmptyLog(t, dir)

	_, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Errorf("Execute err = %v; want nil for healthy state", err)
	}
}

func TestStateStatusExitNonZeroWhenDaemonNotRunning(t *testing.T) {
	dir := t.TempDir()
	// No PID file at all.
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)

	_, _, err := runStateStatus(t, dir)
	if err != ErrStatusUnhealthy {
		t.Errorf("Execute err = %v; want ErrStatusUnhealthy", err)
	}
}

func TestStateStatusExitNonZeroWhenLastSaveStale(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-10*time.Minute))
	touchEmptyLog(t, dir)

	_, _, err := runStateStatus(t, dir)
	if err != ErrStatusUnhealthy {
		t.Errorf("Execute err = %v; want ErrStatusUnhealthy when last save > 5 min", err)
	}
}

func TestStateStatusExitNonZeroWhenRecentWarningsPresent(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))

	now := time.Now().UTC()
	logLine := now.Format(time.RFC3339) + " | ERROR | daemon | crashed"
	if err := os.WriteFile(state.PortalLog(dir), []byte(logLine+"\n"), 0o600); err != nil {
		t.Fatalf("write portal.log: %v", err)
	}

	_, _, err := runStateStatus(t, dir)
	if err != ErrStatusUnhealthy {
		t.Errorf("Execute err = %v; want ErrStatusUnhealthy when warnings > 0", err)
	}
}

func TestStateStatusExitZeroWhenNoLastSaveYet(t *testing.T) {
	// Fresh install: daemon running, no sessions.json yet. HasLastSave is
	// false and the stale-save check must NOT trigger an unhealthy exit.
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	touchEmptyLog(t, dir)

	_, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Errorf("Execute err = %v; want nil when daemon up but no save yet", err)
	}
}

func TestStateStatusVersionUnknownWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	// daemon.version intentionally absent
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)

	outBuf, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Fatalf("Execute err = %v; want nil for healthy state", err)
	}
	pid := os.Getpid()
	want := "  Save daemon: running (pid " + strconv.Itoa(pid) + ", version unknown)\n"
	if !strings.Contains(outBuf.String(), want) {
		t.Errorf("output missing %q:\n%s", want, outBuf.String())
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"sub-second is just now", 500 * time.Millisecond, "just now"},
		{"one second is just now", 1 * time.Second, "just now"},
		{"two seconds renders as seconds", 2 * time.Second, "2 seconds ago"},
		{"under one minute renders as seconds", 45 * time.Second, "45 seconds ago"},
		{"one minute renders as minutes", 60 * time.Second, "1 minutes ago"},
		{"under one hour renders as minutes", 30 * time.Minute, "30 minutes ago"},
		{"one hour renders as hours", 60 * time.Minute, "1 hours ago"},
		{"under one day renders as hours", 5 * time.Hour, "5 hours ago"},
		{"one day renders as days", 24 * time.Hour, "1 days ago"},
		{"multi-day renders as days", 72 * time.Hour, "3 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q; want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"500 bytes", 500, "500 B"},
		{"1.5 KB", 1024 + 512, "1.5 KB"},
		{"18.2 MB", 19084083, "18.2 MB"},
		{"2.0 GB", 2 * 1024 * 1024 * 1024, "2.0 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.n)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q; want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestStateStatusSilenceFlags(t *testing.T) {
	if !stateStatusCmd.SilenceErrors {
		t.Error("stateStatusCmd.SilenceErrors = false; want true so cobra prints no extra error text")
	}
	if !stateStatusCmd.SilenceUsage {
		t.Error("stateStatusCmd.SilenceUsage = false; want true")
	}
}

// TestStateStatusUnhealthyDoesNotEmitErrorBanner verifies that the empty-string
// ErrStatusUnhealthy does not pollute stderr — the user has already seen the
// rendered status; cobra should not append extra lines.
func TestStateStatusUnhealthyDoesNotEmitErrorBanner(t *testing.T) {
	dir := t.TempDir()
	// Daemon absent → unhealthy.
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)

	_, errBuf, err := runStateStatus(t, dir)
	if err != ErrStatusUnhealthy {
		t.Fatalf("Execute err = %v; want ErrStatusUnhealthy", err)
	}
	// runStateStatus binds rootCmd's stderr to errBuf. With SilenceErrors=true
	// the empty-message error must not surface as a printed line.
	if errBuf.Len() != 0 {
		t.Errorf("expected silent stderr; got %q", errBuf.String())
	}
}

// TestStateStatusStateSizeReflectsDiskUsage is a smoke check that the size
// line reports a non-zero size when files exist under the state directory.
func TestStateStatusStateSizeReflectsDiskUsage(t *testing.T) {
	dir := t.TempDir()
	writePIDAlive(t, dir)
	writeVersion(t, dir, "v0.4.2")
	writeIndexAt(t, dir, time.Now().Add(-30*time.Second))
	touchEmptyLog(t, dir)
	// Drop a known-size file under scrollback/ to grow state size.
	scroll := filepath.Join(dir, "scrollback")
	if err := os.MkdirAll(scroll, 0o700); err != nil {
		t.Fatalf("mkdir scrollback: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scroll, "x.bin"), bytes.Repeat([]byte{0xAA}, 2048), 0o600); err != nil {
		t.Fatalf("write scrollback file: %v", err)
	}

	outBuf, _, err := runStateStatus(t, dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(outBuf.String(), "  State size: ") {
		t.Errorf("missing State size line:\n%s", outBuf.String())
	}
	// The size must NOT be 0 B — there is at least the 2 KB scrollback file.
	if strings.Contains(outBuf.String(), "  State size: 0 B on disk") {
		t.Errorf("State size reports 0 B despite known on-disk file:\n%s", outBuf.String())
	}
}
