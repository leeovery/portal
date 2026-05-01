//go:build integration

// Package restoretest provides shared test scaffolding for portal's
// reboot round-trip integration tests. It exists so the helpers
// originally duplicated across:
//
//   - cmd/bootstrap/reboot_roundtrip_test.go
//   - internal/restore/integration_full_test.go
//   - cmd/reattach_integration_test.go
//
// have a single canonical implementation and cannot drift. The package is
// gated `//go:build integration` because every consumer is also gated —
// keeping the gate here means the package contributes zero compile cost
// and zero surface to default `go test ./...` runs, while still being
// importable from the integration test files that need it.
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
//
// The package depends only on internal/tmux + internal/state + stdlib +
// testing — no import cycles with internal/restore or cmd/bootstrap.
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

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// ProjectRoot walks up from the current working directory until it finds
// a directory containing go.mod. Returns the absolute path of that
// directory. Used to anchor `go build` invocations regardless of the
// caller test binary's runtime CWD (cmd/, internal/restore/, etc.).
//
// Returns an error rather than fatalling so it can be reused by helpers
// that also return error (BuildPortalBinaryStable) without dragging
// *testing.T into pure plumbing.
//
// Exported because internal/restoretest/restoretest_test.go (external
// _test package) exercises this helper's contract directly.
func ProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

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
	if err := buildPortalBinaryInto(dir); err != nil {
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
	if err := buildPortalBinaryInto(dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// buildPortalBinaryInto compiles the portal CLI into dir/portal. Shared
// by BuildPortalBinaryDir and BuildPortalBinaryStable so the underlying
// `go build` invocation lives in one place.
func buildPortalBinaryInto(dir string) error {
	binary := filepath.Join(dir, "portal")
	root, err := ProjectRoot()
	if err != nil {
		return fmt.Errorf("locate project root: %w", err)
	}
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
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
// byte-equivalent to writeFIFOSignal in cmd/state_signal_hydrate.go;
// retain only when the binary path cannot be exercised.
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
// On per-session failure (build missing, exit non-zero, output drift) the
// test fails via t.Errorf — every session's invocation is reported
// independently so a failing test pinpoints which session's hook pipeline
// regressed.
func DriveSignalHydrateBinary(t *testing.T, portalBinaryDir, socketPath, stateDir, hooksFile string, sessions []string) {
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
		// Env construction: prepend os.Environ() then append overrides.
		// exec.Cmd honours last-write-wins for duplicate keys, so any
		// inherited TMUX/PORTAL_STATE_DIR/PORTAL_HOOKS_FILE/PATH from the
		// outer test process is shadowed by the explicit values below.
		cmd.Env = append(os.Environ(),
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
			// same `portal` we just spawned.
			"PATH="+portalBinaryDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("portal state signal-hydrate %q: %v\n%s", session, err, out)
		}
	}
}

// openAndSignalFIFO opens path O_WRONLY|O_NONBLOCK, retries ENXIO and
// EAGAIN at delay intervals until budget elapses, then writes a single
// byte. Byte-equivalent to cmd/state_signal_hydrate.writeFIFOSignal;
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
// hook-or-shell exec step. timeout is the deadline; on expiry the test
// fails with a sorted list of stuck keys for stable diagnostics — a
// stuck marker indicates the helper crashed before unsetting it.
func WaitForSkeletonMarkersCleared(t *testing.T, client *tmux.Client, timeout time.Duration) {
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
		time.Sleep(50 * time.Millisecond)
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
