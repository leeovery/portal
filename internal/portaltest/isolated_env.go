// Test-only. Importing this package from non-*_test.go files is
// prohibited — the *testing.T parameter enforces this structurally.

package portaltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NewIsolatedStateEnv returns an env slice and stateDir scoped to a
// per-test t.TempDir(). Callers assign env directly to exec.Cmd.Env
// when launching subprocesses (typically `portal state daemon`) so
// the spawned process resolves XDG_CONFIG_HOME — and therefore all
// portal state paths — under the isolated temp directory.
//
// The returned env is derived from os.Environ(), filtered to remove
// any inherited XDG_CONFIG_HOME entry (including XDG_CONFIG_HOME="")
// and with a single XDG_CONFIG_HOME=<tempDir>/config appended. All
// other inherited variables (HOME, PATH, etc.) are preserved
// verbatim.
//
// stateDir resolves to <XDG_CONFIG_HOME>/portal/state and is
// MkdirAll'd before return (0o700) so callers that immediately stat
// or read from it do not race the subprocess. The configDir itself
// is also created with 0o700.
//
// Errors from MkdirAll are fatal via t.Fatalf — this helper exists
// solely to make daemon tests safe, and a failure here means the
// test cannot proceed.
//
// The *testing.T parameter is intentional: it enforces test-only
// usage structurally. Production code cannot import this package
// without dragging in the testing stdlib package, which would fail
// `go build .` for the main binary.
func NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string) {
	t.Helper()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir configDir: %v", err)
	}

	stateDir = filepath.Join(configDir, "portal", "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("portaltest: mkdir stateDir: %v", err)
	}

	env = filterXDGConfigHome(os.Environ())
	env = append(env, "XDG_CONFIG_HOME="+configDir)

	return env, stateDir
}

// filterXDGConfigHome returns a copy of env with every entry whose
// key equals XDG_CONFIG_HOME removed. Matches the empty-value edge
// case (XDG_CONFIG_HOME="") because the prefix check uses the full
// "KEY=" form.
func filterXDGConfigHome(env []string) []string {
	const prefix = "XDG_CONFIG_HOME="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}
