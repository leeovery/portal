package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedCleanLogsEnv isolates every config/state path a `portal clean` invocation
// touches onto temp dirs so the test never reads or writes the developer's real
// ~/.config/portal. It returns the isolated state dir (PORTAL_STATE_DIR) that the
// --logs sweep targets.
func seedCleanLogsEnv(t *testing.T) (stateDir string) {
	t.Helper()
	base := t.TempDir()
	stateDir = filepath.Join(base, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(base, "projects.json"))
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(base, "hooks.json"))

	// Inject a benign pane lister so the hook-cleanup tail never builds a real
	// tmux client against the test host. No hooks file exists, so the persisted=0
	// short-circuit fires before the lister is consulted anyway.
	cleanDeps = &CleanDeps{AllPaneLister: &mockCleanPaneLister{panes: []string{"x:0.0"}}}
	t.Cleanup(func() { cleanDeps = nil })
	return stateDir
}

// touchRotated writes an empty rotated-log file in dir and returns its path.
func touchRotated(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func runCleanCmd(t *testing.T, args ...string) {
	t.Helper()
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(append([]string{"clean"}, args...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("clean %v: unexpected error: %v", args, err)
	}
}

// TestCleanLogsFlagDefaultsFalse pins the flag registration + default.
func TestCleanLogsFlagDefaultsFalse(t *testing.T) {
	resetRootCmd()
	f := cleanCmd.Flags().Lookup("logs")
	if f == nil {
		t.Fatal("clean command has no --logs flag registered")
	}
	if f.DefValue != "false" {
		t.Errorf("--logs default = %q, want false", f.DefValue)
	}
}

// TestCleanWithoutLogsFlagPreservesRotatedLogs pins that `portal clean` (no
// --logs) does NOT trigger the sweep — every rotated file survives.
func TestCleanWithoutLogsFlagPreservesRotatedLogs(t *testing.T) {
	stateDir := seedCleanLogsEnv(t)

	priorDay := touchRotated(t, stateDir, "portal.log.2020-01-01")
	sentinel := touchRotated(t, stateDir, "portal.log.swept.2020-01-01")

	runCleanCmd(t)

	for _, p := range []string{priorDay, sentinel} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed by clean without --logs; rotated logs must be preserved", filepath.Base(p))
		}
	}
}

// TestCleanLogsFlagDeletesPriorDayKeepsToday pins the cutoff=today sweep at the
// cmd layer: every prior-day rotated file is deleted, today's base file (the
// current symlink target) survives.
func TestCleanLogsFlagDeletesPriorDayKeepsToday(t *testing.T) {
	stateDir := seedCleanLogsEnv(t)

	today := time.Now().Format("2006-01-02")
	priorDay := touchRotated(t, stateDir, "portal.log.2020-01-01")
	todayBase := touchRotated(t, stateDir, "portal.log."+today)

	runCleanCmd(t, "--logs")

	if _, err := os.Stat(priorDay); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("prior-day %s still present; --logs must delete every date < today", filepath.Base(priorDay))
	}
	if _, err := os.Stat(todayBase); err != nil {
		t.Errorf("today's %s removed; --logs (cutoff=today, strict <) must keep today's file", filepath.Base(todayBase))
	}
}

// TestCleanLogsFlagBypassesGateAndRemovesAllSentinels pins gate-bypass plus the
// all-sentinel prune: an existing swept.<today> sentinel does not block the
// sweep, and every swept.* sentinel (today's included) is removed.
func TestCleanLogsFlagBypassesGateAndRemovesAllSentinels(t *testing.T) {
	stateDir := seedCleanLogsEnv(t)

	today := time.Now().Format("2006-01-02")
	// today's sentinel present => a gated sweep would no-op; --logs must run.
	todaySentinel := touchRotated(t, stateDir, "portal.log.swept."+today)
	staleSentinel := touchRotated(t, stateDir, "portal.log.swept.2020-01-01")
	priorDay := touchRotated(t, stateDir, "portal.log.2020-01-01")

	runCleanCmd(t, "--logs")

	if _, err := os.Stat(priorDay); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("prior-day %s still present; --logs must bypass the swept.<today> gate", filepath.Base(priorDay))
	}
	for _, p := range []string{todaySentinel, staleSentinel} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still present; --logs must remove ALL swept.* sentinels", filepath.Base(p))
		}
	}
}
