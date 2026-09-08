//go:build integration

package restoretest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/xdg"
)

// BuildPortalBinaryDir compiles `portal` into a fresh t.TempDir. A restored pane
// respawns into `portal state hydrate`, so a test driving a real restore must
// reach that binary — via StagedHydrateExe (or the constructors over it), and
// via PrependPATH only where something other than the restore invokes `portal`
// by name.
func BuildPortalBinaryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := portalbintest.BuildPortalBinary(dir); err != nil {
		t.Fatalf("build portal binary: %v", err)
	}
	return dir
}

// BuildPortalBinaryStable deliberately registers no cleanup: under a
// sync.Once-cached build, t.TempDir would be removed when the triggering test
// exits and every later test would point at a deleted path. Removal is the
// caller's business.
func BuildPortalBinaryStable() (string, error) {
	dir, err := os.MkdirTemp("", "ptl-bin-")
	if err != nil {
		return "", fmt.Errorf("mkdir temp: %w", err)
	}
	if err := portalbintest.BuildPortalBinary(dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// StagedHydrateExe points a restore's pane-arming at the freshly-built binary in
// binDir, standing in for the os.Executable() a production restore resolves.
//
// Every test driving a real restore needs it, because Exe is opt-in on the
// restore types and an absent (or nil) one falls back to os.Executable(). Under
// `go test` that is the test binary, which stops flag parsing at the leading
// `state` positional: the armed pane re-runs the suite inside itself and exits
// 0, taking the session with it. The symptom is a session that quietly
// disappeared — no error, no failure, nothing in the log.
//
// An empty binDir is fatal, not tolerated: filepath.Join would fold it to the
// bare name and silently restore the PATH lookup this exists to replace, so a
// caller that has not staged a binary yet would pass against whatever release is
// installed on the machine.
func StagedHydrateExe(t *testing.T, binDir string) restore.ExecutableResolver {
	t.Helper()
	return stagedHydrateExe(t, binDir)
}

func stagedHydrateExe(t harnesstest.NamingT, binDir string) restore.ExecutableResolver {
	t.Helper()
	if binDir == "" {
		t.Fatalf("StagedHydrateExe: empty binDir; stage one with BuildPortalBinaryDir first")
		return nil
	}
	path := filepath.Join(binDir, "portal")
	return func() (string, error) { return path, nil }
}

// PrependPATH stages binDir ahead of the ambient PATH for a fixture that has
// something invoking `portal` by name: the global hook bodies (`portal state
// notify` and siblings, run through run-shell) and the `_portal-saver` pane's
// `portal state daemon`. It is NOT the route for the hydrate helper a restore
// arms its panes with — that is pinned by path through StagedHydrateExe, so a
// fixture whose only portal subprocess is that helper needs no PATH staging.
//
// It goes through t.Setenv, so subprocesses — notably tmux server forks —
// inherit the change and it is restored on test exit.
func PrependPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// DriveSignalHydrate mimics `portal state signal-hydrate <session>` by writing
// the FIFO byte directly, for cases DriveSignalHydrateBinary cannot cover. Its
// retry budget is far longer than production's on purpose: these tests arm a
// fresh server under parallel load, where the in-pane fork+exec can take seconds
// to reach its open(O_RDONLY).
func DriveSignalHydrate(t *testing.T, client *tmux.Client, stateDir string, sessions []string) {
	t.Helper()
	const (
		retryDelay = 50 * time.Millisecond
		budget     = 10 * time.Second
	)
	markers, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	if len(markers) == 0 {
		t.Fatal("no skeleton markers; restore did not arm any panes")
	}
	for _, session := range sessions {
		panes, err := client.ListPanesInSession(session)
		if err != nil {
			t.Fatalf("ListPanesInSession %q: %v", session, err)
		}
		for _, p := range panes {
			liveKey := state.SanitizePaneKey(session, p.Window, p.Pane)
			if _, marked := markers[liveKey]; !marked {
				continue
			}
			fifo := state.FIFOPath(stateDir, liveKey)
			if err := openAndSignalFIFO(fifo, retryDelay, budget); err != nil {
				t.Errorf("signal FIFO %s: %v", fifo, err)
			}
		}
	}
}

// DriveSignalHydrateBinary execs the pre-built `portal state signal-hydrate
// <session>`, argv-identical to what the tmux hooks invoke via run-shell.
//
// env is mandatory and must come from portaltest.IsolateStateForTest: it is the
// structural guarantee that the spawned binary cannot inherit the developer's
// real XDG_CONFIG_HOME. The per-spawn overrides shadow it, last-write-wins.
func DriveSignalHydrateBinary(t *testing.T, portalBinaryDir, socketPath, stateDir, hooksFile string, sessions []string, env []string) {
	t.Helper()
	binary := filepath.Join(portalBinaryDir, "portal")
	for _, session := range sessions {
		// The `--` separator is load-bearing: without it pflag reads a
		// leading-dash session name as a short-flag cluster and exits before the
		// command body runs.
		cmd := exec.Command(binary, "state", "signal-hydrate", "--", session)
		cmd.Env = append(append([]string{}, env...),
			// TMUX is the only way a tmux CLI invocation without -S/-L can target
			// a non-default socket. Only the socket-path component is consulted.
			fmt.Sprintf("TMUX=%s,1,0", socketPath),
			xdg.StateDir.EnvVar+"="+stateDir,
			xdg.HooksFile.EnvVar+"="+hooksFile,
			"PATH="+portalBinaryDir+string(os.PathListSeparator)+pathFromEnv(env),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("portal state signal-hydrate %q: %v\n%s", session, err, out)
		}
	}
}

// PATH comes from env, not os.Getenv, so composition stays on the isolated
// baseline.
func pathFromEnv(env []string) string {
	const prefix = "PATH="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return e[len(prefix):]
		}
	}
	return ""
}

// openAndSignalFIFO retries only ENXIO and EAGAIN — any other open error aborts
// immediately, so a genuine permission or path fault surfaces at once instead of
// waiting out the budget.
func openAndSignalFIFO(path string, delay, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	var lastErr error
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			if _, werr := f.Write([]byte{1}); werr != nil {
				_ = f.Close()
				return fmt.Errorf("write byte: %w", werr)
			}
			_ = f.Close()
			return nil
		}
		if !errors.Is(err, syscall.ENXIO) && !errors.Is(err, syscall.EAGAIN) {
			return fmt.Errorf("open: %w", err)
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("retries exhausted after %s: %w", budget, lastErr)
		}
		time.Sleep(delay)
	}
}

// WaitForSkeletonMarkersCleared returns once every helper has reached its
// hook-or-shell exec step. A marker still set at expiry means that helper
// crashed before unsetting it.
func WaitForSkeletonMarkersCleared(t *testing.T, client *tmux.Client, timeout, tick time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		markers, err := state.ListSkeletonMarkers(client)
		if err != nil {
			t.Fatalf("ListSkeletonMarkers: %v", err)
		}
		if len(markers) == 0 {
			return
		}
		time.Sleep(tick)
	}
	markers, _ := state.ListSkeletonMarkers(client)
	t.Fatalf("skeleton markers still set after %s: %v", timeout, SortedKeySet(markers))
}

// SortedKeySet yields an empty, non-nil slice for an empty input, so callers
// can format the result uniformly.
func SortedKeySet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
