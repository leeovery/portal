// The hook-key subject vocabulary shared across the cmd test suites: the
// hooks.json bodies the seed keys are written into, and the install a
// stale-hooks fixture judges them against. The seed keys themselves — and the
// stale-beside-live body — are named once in internal/hookstest, so no suite
// can re-point a name at a different key. Staging — how a test is set up and
// driven, including the store and file fixtures these bodies are staged into
// — lives in testhelpers_test.go.
package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/sessionindex"
)

// hooksBody renders a hooks.json body registering one on-resume entry per key,
// so a fixture that cares only about which keys are present says exactly that.
// With no keys it renders the empty registry: a file that exists and holds
// nothing.
func hooksBody(keys ...string) string {
	if len(keys) == 0 {
		return "{}"
	}
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, fmt.Sprintf("  %q: {\"on-resume\": \"echo hi\"}", key))
	}
	return "{\n" + strings.Join(entries, ",\n") + "\n}"
}

func keysOf(m hooks.Snapshot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// staleHooksInstall is a healthy install for the stale-hooks check and its
// repair: hooks.json staged from seed, the backend listing LiveSeedA and
// sessions.json indexing LiveSeedB, so StaleHookSeed holds one stale key.
type staleHooksInstall struct {
	deps      *DoctorDeps
	backend   *backendtest.Fake
	hooksPath string
}

func newStaleHooksInstall(t *testing.T, seed string) staleHooksInstall {
	t.Helper()
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})
	fake := backendtest.New(backend.Session{Name: hookstest.LiveSeedA})
	dir := t.TempDir()
	writeSessionsJSON(t, dir, indexJSON(t, []sessionindex.Entry{{Name: hookstest.LiveSeedB, Dir: "/srv/live"}}))
	projectStore, _ := seedProjectsJSON(t, t.TempDir())
	deps := withHealthyRuntime(t, &DoctorDeps{StateDir: dir, HookStore: store, ProjectStore: projectStore, ListSessions: fake.List})
	return staleHooksInstall{deps: deps, backend: fake, hooksPath: path}
}
