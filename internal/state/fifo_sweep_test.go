package state_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// readLogBody returns the contents of a portal.log written by a *state.Logger
// pointed at path. Helper centralises the open/read/fail flow used by sweep
// tests that assert on log output.
func readLogBody(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log %s: %v", path, err)
	}
	return string(data)
}

// openTestLogger opens a *state.Logger at dir/portal.log without rotation. The
// logger is closed via t.Cleanup so its file descriptor is flushed before
// tests read the log body.
func openTestLogger(t *testing.T, dir string) (*state.Logger, string) {
	t.Helper()
	logPath := filepath.Join(dir, "portal.log")
	lg, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() {
		_ = lg.Close()
	})
	return lg, logPath
}

func TestSweepOrphanFIFOs_RemovesOrphansAndPreservesLiveOnes(t *testing.T) {
	dir := t.TempDir()

	live := filepath.Join(dir, "hydrate-keep__0.0.fifo")
	orphan := filepath.Join(dir, "hydrate-gone__0.0.fifo")
	if err := state.CreateFIFO(live); err != nil {
		t.Fatalf("create live FIFO: %v", err)
	}
	if err := state.CreateFIFO(orphan); err != nil {
		t.Fatalf("create orphan FIFO: %v", err)
	}

	liveKeys := map[string]struct{}{"keep__0.0": {}}

	if err := state.SweepOrphanFIFOs(dir, liveKeys, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	if _, err := os.Lstat(live); err != nil {
		t.Errorf("live FIFO removed: %v", err)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan FIFO not removed: lstat err = %v", err)
	}
}

func TestSweepOrphanFIFOs_PreservesNonFIFORegularFile(t *testing.T) {
	dir := t.TempDir()

	regular := filepath.Join(dir, "hydrate-foo__0.0.fifo")
	if err := os.WriteFile(regular, []byte("not a fifo"), 0o600); err != nil {
		t.Fatalf("seed regular file: %v", err)
	}

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	info, err := os.Lstat(regular)
	if err != nil {
		t.Fatalf("regular file removed by sweep: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("file mode changed: got %v", info.Mode())
	}
}

func TestSweepOrphanFIFOs_ToleratesMissingStateDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	if err := state.SweepOrphanFIFOs(missing, map[string]struct{}{}, nil); err != nil {
		t.Errorf("expected nil for missing dir, got: %v", err)
	}
}

func TestSweepOrphanFIFOs_RemovesAllWhenLiveSetEmpty(t *testing.T) {
	dir := t.TempDir()

	paths := []string{
		filepath.Join(dir, "hydrate-a__0.0.fifo"),
		filepath.Join(dir, "hydrate-b__0.0.fifo"),
		filepath.Join(dir, "hydrate-c__0.0.fifo"),
	}
	for _, p := range paths {
		if err := state.CreateFIFO(p); err != nil {
			t.Fatalf("create FIFO %s: %v", p, err)
		}
	}

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	for _, p := range paths {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("FIFO %s not removed: lstat err = %v", p, err)
		}
	}
}

func TestSweepOrphanFIFOs_RoundTripsSanitizedPaneKeys(t *testing.T) {
	dir := t.TempDir()

	// Session "weird/name" sanitises with a collision suffix.
	sanitized := state.SanitizePaneKey("weird/name", 0, 0)
	path := filepath.Join(dir, "hydrate-"+sanitized+".fifo")
	if err := state.CreateFIFO(path); err != nil {
		t.Fatalf("create FIFO: %v", err)
	}

	live := map[string]struct{}{sanitized: {}}

	if err := state.SweepOrphanFIFOs(dir, live, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	if _, err := os.Lstat(path); err != nil {
		t.Errorf("sanitized-keyed FIFO incorrectly removed: %v", err)
	}
}

func TestSweepOrphanFIFOs_LogsAndContinuesOnPerFileFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based EACCES setup is unix-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0500 directory write protection")
	}

	dir := t.TempDir()

	a := filepath.Join(dir, "hydrate-a__0.0.fifo")
	b := filepath.Join(dir, "hydrate-b__0.0.fifo")
	if err := state.CreateFIFO(a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := state.CreateFIFO(b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	lg, logPath := openTestLogger(t, dir)

	// Strip write permission AFTER FIFOs are created so os.Remove fails for
	// both. Sweep should log warn for both and still return nil.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, lg); err != nil {
		t.Errorf("SweepOrphanFIFOs returned error: %v", err)
	}

	// Restore permissions so the log can be read.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}

	body := readLogBody(t, logPath)
	if !strings.Contains(body, a) {
		t.Errorf("log missing entry for %s; body = %q", a, body)
	}
	if !strings.Contains(body, b) {
		t.Errorf("log missing entry for %s; body = %q", b, body)
	}
	if !strings.Contains(body, "WARN") {
		t.Errorf("log missing WARN level; body = %q", body)
	}
}

func TestSweepOrphanFIFOs_LogsLinePerRemovedOrphan(t *testing.T) {
	dir := t.TempDir()

	orphan := filepath.Join(dir, "hydrate-gone__0.0.fifo")
	if err := state.CreateFIFO(orphan); err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	lg, logPath := openTestLogger(t, dir)

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, lg); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	body := readLogBody(t, logPath)
	if !strings.Contains(body, orphan) {
		t.Errorf("log missing path %s; body = %q", orphan, body)
	}
	if !strings.Contains(body, "INFO") {
		t.Errorf("log missing INFO level; body = %q", body)
	}
}

func TestSweepOrphanFIFOs_TreatsSymlinksAsNonFIFOs(t *testing.T) {
	dir := t.TempDir()

	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("payload"), 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	link := filepath.Join(dir, "hydrate-foo__0.0.fifo")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}

	if err := state.SweepOrphanFIFOs(dir, map[string]struct{}{}, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink removed by sweep: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link is no longer a symlink: mode = %v", info.Mode())
	}
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("symlink target removed: %v", err)
	}
}
