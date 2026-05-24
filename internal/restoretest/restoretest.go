//go:build integration

// Integration-only helpers for portal's reboot round-trip tests. The
// package-level doc lives in doc.go (untagged); this file holds only
// helpers that depend on tmux fixtures or the portal binary and so are
// gated `//go:build integration`.
//
// Two flavours of "build the portal CLI" are exposed:
//
//   - BuildPortalBinaryDir: t.TempDir-based, fatals via t.Fatal. Use when
//     the binary's lifetime should match a single test.
//   - BuildPortalBinaryStable: os.MkdirTemp-based, returns error. Use
//     under sync.Once.Do where the binary outlives the test that triggers
//     the build (the cmd/reattach pattern). Cleanup is the caller's
//     responsibility — typically skipped, since the dir lives under
//     $TMPDIR which the OS reaps.
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

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// BuildPortalBinaryDir compiles `portal` into a fresh t.TempDir and
// returns the directory containing the binary. The caller typically
// follows up with PrependPATH(t, dir) so the in-pane hydrate helper
// resolves the binary on PATH.
//
// The binary lives only for the duration of the test (t.TempDir's
// cleanup deletes it). Use BuildPortalBinaryStable for cases that need
// the binary to outlive a single test (e.g. sync.Once-cached builds
// shared across sub-tests in the same process).
func BuildPortalBinaryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := portalbintest.BuildPortalBinary(dir); err != nil {
		t.Fatalf("build portal binary: %v", err)
	}
	return dir
}

// BuildPortalBinaryStable compiles `portal` into a fresh os.MkdirTemp
// directory and returns the directory containing the binary. The dir is
// NOT cleaned up automatically — callers running this under sync.Once
// either skip cleanup (the dir lives in $TMPDIR which the OS reaps) or
// register an os.RemoveAll explicitly.
//
// The contract diverges from BuildPortalBinaryDir specifically because
// t.TempDir is removed on the test that triggered the once-Do exit; if
// later tests reuse the cached dir they would point at a deleted path.
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

// PrependPATH prefixes dir to the test process's PATH using t.Setenv,
// which guarantees the original PATH is restored on test exit. The
// modified PATH is inherited by any subprocess (notably tmux server
// forks) so the in-pane hydrate helper can resolve `portal` on PATH.
func PrependPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// DriveSignalHydrate is a fallback (CI flake-tolerant) test-side helper
// that mimics `portal state signal-hydrate <session>` by writing the FIFO
// byte directly. For each named session, it enumerates skeleton-marked
// panes and writes 1 byte to each pane's hydration FIFO, retrying
// ENXIO/EAGAIN at 50ms intervals up to a 10-second budget.
//
// Phase 13 task 13-2 made DriveSignalHydrateBinary the *primary* coverage
// surface for the production hook → run-shell → portal CLI argv → FIFO
// pipeline. This direct-FIFO helper remains as a fallback used by:
//
//   - The base-index drift round-trip variant where exec'ing the binary
//     against a divergent live tmux base-index would re-walk paths the
//     binary-driven primary path already covers.
//   - Any future CI lane where the binary path proves flaky and a
//     short-term fallback is preferable to skipping the test entirely.
//
// The 10-second budget is deliberately longer than production's 500ms:
// production runs from a tmux client-attached hook with a warm server,
// while the integration tests run `go build` and a fresh tmux server arm
// in the same process. Under parallel `go test ./...` load the in-pane
// fork+exec for the helper can take well over a second before reaching
// its open(O_RDONLY) — 10s absorbs CI scheduling without masking
// regressions (a stuck helper still times out).
//
// Driving the FIFO directly (rather than spawning the production CLI) is
// byte-equivalent to state.WriteFIFOSignal / state.SendHydrateSignal in
// internal/state; retain only when the binary path cannot be exercised.
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

// DriveSignalHydrateBinary is the production-binary-equivalent
// counterpart to DriveSignalHydrate. For each named session it exec's the
// pre-built `portal state signal-hydrate <session>` subcommand — argv-
// identical to what the tmux `client-attached` / `client-session-changed`
// hooks invoke via `run-shell` in production. This exercises the full
// hook pipeline that the Phase 5 task 5-9 acceptance bullet calls out:
// tmux hook → run-shell → portal CLI argv → runSignalHydrate body →
// per-pane FIFO write → in-pane hydrate helper unblock.
//
// portalBinaryDir is the directory the binary lives in (typically the
// return of BuildPortalBinaryDir or BuildPortalBinaryStable). socketPath
// is the test's isolated tmux server socket — propagated to the spawned
// binary via `TMUX=<socket>,1,0` so the binary's tmux.DefaultClient
// targets the test's isolated server (without -S/-L, tmux respects the
// TMUX env's socket-path component).
//
// stateDir / hooksFile are propagated as PORTAL_STATE_DIR /
// PORTAL_HOOKS_FILE so the binary's state.EnsureDir and hooks store
// resolve to the test's isolated config locations.
//
// env is the isolated baseline env — callers MUST obtain it via
// portaltest.IsolateStateForTest(t) (or an equivalent helper that strips
// any inherited XDG_CONFIG_HOME and substitutes a per-test temp-dir
// value). This parameter is mandatory and has no env-less overload: it
// is the structural guarantee that the spawned binary cannot inherit
// the developer's real XDG_CONFIG_HOME (Component G of the
// slow-open-empty-previews-and-zombie-sessions work unit). The
// per-spawn overrides below are appended on top; exec.Cmd honours
// last-write-wins for duplicate keys, so any matching key in env is
// shadowed by the explicit override.
//
// On per-session failure (build missing, exit non-zero, output drift) the
// test fails via t.Errorf — every session's invocation is reported
// independently so a failing test pinpoints which session's hook pipeline
// regressed.
func DriveSignalHydrateBinary(t *testing.T, portalBinaryDir, socketPath, stateDir, hooksFile string, sessions []string, env []string) {
	t.Helper()
	binary := filepath.Join(portalBinaryDir, "portal")
	for _, session := range sessions {
		// Argv mirrors the production hook: `portal state signal-hydrate
		// <session-name>`. The hook itself wraps this in
		// `command -v portal >/dev/null 2>&1 && ...` for the not-on-PATH
		// case; we already control PATH (and pass an absolute binary
		// path here) so the wrap is unnecessary.
		// `--` end-of-flags separator before the session arg mirrors the
		// production hook command (signalHydrateCommand in
		// internal/tmux/hooks_register.go) and is load-bearing for
		// leading-dash session names: without it, cobra/pflag treats
		// `-dotfiles-test` as a short-flag cluster and exits non-zero
		// before runSignalHydrate runs. With `--`, every following token
		// is treated as a positional argument regardless of leading dashes.
		cmd := exec.Command(binary, "state", "signal-hydrate", "--", session)
		// Env construction: start from the caller's isolated baseline
		// (XDG_CONFIG_HOME scoped to a per-test temp dir) and append the
		// per-spawn overrides below. exec.Cmd honours last-write-wins
		// for duplicate keys, so any TMUX/PORTAL_STATE_DIR/
		// PORTAL_HOOKS_FILE/PATH already present in env is shadowed by
		// the explicit value here.
		cmd.Env = append(append([]string{}, env...),
			// TMUX is the only documented mechanism by which a tmux CLI
			// invocation without -S/-L can target a non-default socket.
			// Format: <socket-path>,<server-pid>,<session-id>; only the
			// first component is consulted by the client-side connect
			// path, and the literals `,1,0` are ignored. Production
			// signal-hydrate inherits this env from the run-shell parent
			// (the tmux server itself), so this exec mirrors the
			// production env shape.
			fmt.Sprintf("TMUX=%s,1,0", socketPath),
			"PORTAL_STATE_DIR="+stateDir,
			"PORTAL_HOOKS_FILE="+hooksFile,
			// PATH: keep prepended portalBinaryDir so any sub-exec the
			// binary performs (none today, but defensive) resolves the
			// same `portal` we just spawned. Reads PATH from the caller-
			// supplied env (which preserves the test process's PATH
			// verbatim) rather than os.Getenv so the composition stays
			// consistent with the isolated baseline.
			"PATH="+portalBinaryDir+string(os.PathListSeparator)+pathFromEnv(env),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("portal state signal-hydrate %q: %v\n%s", session, err, out)
		}
	}
}

// pathFromEnv returns the PATH value embedded in env, or empty if env
// does not contain a PATH entry. exec.Cmd does not auto-inherit PATH
// when cmd.Env is set, so DriveSignalHydrateBinary composes a PATH
// override using the caller-supplied env's PATH value rather than the
// live os.Getenv("PATH") — keeping the composition consistent with the
// isolated baseline.
func pathFromEnv(env []string) string {
	const prefix = "PATH="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return e[len(prefix):]
		}
	}
	return ""
}

// openAndSignalFIFO opens path O_WRONLY|O_NONBLOCK, retries ENXIO and
// EAGAIN at delay intervals until budget elapses, then writes a single
// byte. Byte-equivalent to state.WriteFIFOSignal in internal/state;
// internal helper for DriveSignalHydrate. Any open error other than
// ENXIO/EAGAIN aborts immediately so genuine permission / path errors
// surface clearly rather than waiting out the full budget.
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

// WaitForSkeletonMarkersCleared polls until every `@portal-skeleton-*`
// server option has been unset. Each helper unsets its own marker after
// scrollback dump + 100ms settle (or after the file-missing recovery
// path), so an empty marker set means every helper has reached the
// hook-or-shell exec step. timeout is the deadline; tick is the poll
// cadence. On expiry the test fails with a sorted list of stuck keys
// for stable diagnostics — a stuck marker indicates the helper crashed
// before unsetting it.
//
// tick is mandatory — call sites historically disagreed on cadence
// (AC1's 2s-budget poll used 50ms; the reboot round-trip's 10s-budget
// poll also used 50ms internally), so the consolidated helper requires
// the caller to be explicit.
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

// SortedKeySet flattens a presence-set to a sorted string slice for
// stable diagnostic output. The returned slice always sorts in
// ascending lexicographic order. An empty input map yields an empty
// (zero-length) slice rather than nil so callers can format the result
// uniformly.
func SortedKeySet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
