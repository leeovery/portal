package transienttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
)

// ResolveHooksFilePathFromEnv mirrors cmd/config.go's configFilePath
// resolution chain but consumes the env slice returned by
// portaltest.IsolateStateForTest rather than os.Getenv. The chain:
//
//  1. If env contains PORTAL_HOOKS_FILE=<path>, return <path> verbatim.
//  2. Otherwise scan env for XDG_CONFIG_HOME=<dir> and return
//     <dir>/portal/hooks.json.
//  3. Otherwise t.Fatalf — signals isolation regression.
//
// Returning early on PORTAL_HOOKS_FILE matches the production behaviour
// where the env-var override takes precedence over XDG-derived defaults.
func ResolveHooksFilePathFromEnv(t *testing.T, env []string) string {
	t.Helper()
	const (
		hooksFileKey = "PORTAL_HOOKS_FILE="
		xdgKey       = "XDG_CONFIG_HOME="
	)
	var xdg string
	for _, e := range env {
		if after, ok := strings.CutPrefix(e, hooksFileKey); ok {
			return after
		}
		if after, ok := strings.CutPrefix(e, xdgKey); ok {
			xdg = after
		}
	}
	if xdg == "" {
		t.Fatalf("transienttest.ResolveHooksFilePathFromEnv: env slice contains neither PORTAL_HOOKS_FILE nor XDG_CONFIG_HOME — IsolateStateForTest isolation regression")
	}
	return filepath.Join(xdg, "portal", "hooks.json")
}

// SeedHooksJSON writes a populated hooks.json under the resolved config
// path. The supplied entries map is interpreted as
// {structuralKey: onResumeCommand} — for each entry, a single on-resume
// hook is registered via the production hooks.Store so the on-disk JSON
// layout matches what `portal hooks set --on-resume` would produce at
// runtime.
//
// The resolved path is t.Logf'd to verify the seed lands under the
// isolated tree, per the project's daemon-test isolation rule.
func SeedHooksJSON(t *testing.T, env []string, entries map[string]string) {
	t.Helper()
	path := ResolveHooksFilePathFromEnv(t, env)
	t.Logf("transienttest.SeedHooksJSON: resolved hooks.json path = %s", path)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("transienttest.SeedHooksJSON: mkdir %s: %v", filepath.Dir(path), err)
	}

	store := hooks.NewStore(path)
	for key, cmd := range entries {
		if err := store.Set(key, "on-resume", cmd, "cli"); err != nil {
			t.Fatalf("transienttest.SeedHooksJSON: set %s=%q: %v", key, cmd, err)
		}
	}
}

// HooksJSONBytes returns the raw on-disk bytes of hooks.json resolved
// from the test-isolated env. Used for byte-identical before/after
// comparisons that pin the "no wipe" invariant. Fails the test on any
// read error other than ENOENT — callers asserting on byte identity have
// no meaningful answer if the file is unreadable.
//
// ENOENT returns a nil slice so a missing-file precondition can be
// distinguished from a present-but-empty file (bytes.Equal handles both
// cases the same way, but the caller may want to check the distinction).
func HooksJSONBytes(t *testing.T, env []string) []byte {
	t.Helper()
	path := ResolveHooksFilePathFromEnv(t, env)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("transienttest.HooksJSONBytes: read %s: %v", path, err)
	}
	return data
}
