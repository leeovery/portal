package hookstest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/xdg"
)

const xdgConfigHome = "XDG_CONFIG_HOME"

// ResolveHooksFilePathFromEnv answers where a binary run with this env slice
// will look for hooks.json, by the same rule and the same file identity that
// binary resolves it with: xdg.ConfigFilePath over xdg.HooksFile, read against
// the slice rather than the process environment. Delegating rather than
// restating either of them is what keeps a seeded file and a read file the same
// file — a seeder resolving by its own rule seeds where nothing reads and
// passes on it.
//
// The rule's home fallback is deliberately out of reach here: a slice carrying
// neither variable means the test's isolation has regressed, which is a fatal
// rather than a path. That tripwire is this helper's own, not the precedence's
// — production reading the same slice would legitimately fall back to home.
//
// It resolves and nothing more, so no migration runs and nothing is created.
func ResolveHooksFilePathFromEnv(t *testing.T, env []string) string {
	t.Helper()
	lookup := xdg.EnvSlice(env)
	if lookup(xdg.HooksFile.EnvVar) == "" && lookup(xdgConfigHome) == "" {
		t.Fatalf("hookstest.ResolveHooksFilePathFromEnv: env slice contains neither %s nor %s — IsolateStateForTest isolation regression", xdg.HooksFile.EnvVar, xdgConfigHome)
	}
	resolved, err := xdg.ConfigFilePath(lookup, xdg.HooksFile)
	if err != nil {
		t.Fatalf("hookstest.ResolveHooksFilePathFromEnv: resolve %s: %v", xdg.HooksFile.Filename, err)
	}
	return resolved.Path
}

// SeedHooksJSON writes a hooks.json of {hookKey: onResumeCommand} entries via
// the production store, so the on-disk layout matches what the CLI produces.
func SeedHooksJSON(t *testing.T, env []string, entries map[string]string) {
	t.Helper()
	path := ResolveHooksFilePathFromEnv(t, env)
	t.Logf("hookstest.SeedHooksJSON: resolved hooks.json path = %s", path)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("hookstest.SeedHooksJSON: mkdir %s: %v", filepath.Dir(path), err)
	}

	store := hooks.NewStore(path)
	for key, cmd := range entries {
		if err := store.Set(key, hooks.EventOnResume, hooks.Registration{Command: cmd}, hooks.ViaCLI); err != nil {
			t.Fatalf("hookstest.SeedHooksJSON: set %s=%q: %v", key, cmd, err)
		}
	}
}

// HooksJSONBytes returns the raw on-disk bytes of the hooks.json the env
// resolves to, or nil when the file is absent. Any other read error fails the
// test.
func HooksJSONBytes(t *testing.T, env []string) []byte {
	t.Helper()
	return HooksFileBytes(t, ResolveHooksFilePathFromEnv(t, env))
}

// HooksFileBytes returns the raw on-disk bytes of the hooks.json at path, or
// nil when the file is absent, so a caller can compare a route's before and
// after without an absent file — a state the route may legitimately leave —
// fatalling. Any other read error fails the test.
func HooksFileBytes(t harnesstest.TestingT, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("hookstest.HooksFileBytes: read %s: %v", path, err)
	}
	return data
}

// AssertHooksFileUnchanged proves a route left the hooks.json at path byte for
// byte as it found it. An optional context names what the route must not have
// done in place of the default, so a caller reporting something more specific
// than a failing write keeps its own words in the failure.
func AssertHooksFileUnchanged(t harnesstest.TestingT, path string, before []byte, context ...string) {
	t.Helper()
	what := "changed on a failing route"
	if len(context) > 0 {
		what = context[0]
	}
	if after := HooksFileBytes(t, path); !bytes.Equal(before, after) {
		t.Errorf("hooks.json %s:\nbefore %s\nafter  %s", what, before, after)
	}
}

// sessionNameKey returns a seed hook key: the name of the session it keys,
// legible in test output by its role. n orders the seeds, so keys sort as
// their n do across roles, and distinct n give distinct keys.
func sessionNameKey(n int, role string) string {
	return fmt.Sprintf("seed%02d-%s", n, role)
}

// The named seed keys the suites share, and the only route to one: a fixture
// names the seed whose role it means — reapable, live, tmux-era or plain
// subject — so a name cannot drift onto a key of another half, and no package
// re-derives a key of its own.
var (
	// The reapable half: session names a fixture neither lists nor indexes, so
	// an entry under one is reaped. A fixture keying its subject on one of
	// these is measuring the reap.
	ReapableSeedA = sessionNameKey(0, "reapable")
	ReapableSeedB = sessionNameKey(1, "reapable")
	ReapableSeedC = sessionNameKey(2, "reapable")
	ReapableSeedD = sessionNameKey(3, "reapable")

	// The live half: names reserved for the sessions a fixture lists or
	// indexes. An entry under one survives because a session or an entry
	// names it, which is the whole point of keeping them apart from the
	// reapable half rather than reusing an index.
	LiveSeedA = sessionNameKey(4, "live")
	LiveSeedB = sessionNameKey(5, "live")
	LiveSeedC = sessionNameKey(6, "live")

	// The tmux-era half: keys the tmux version wrote, a pane token and the
	// older `<session>:<window>.<pane>` coordinates. No zmx session carries
	// one, so the rule judges them as it judges any other key.
	TmuxEraSeedA = "k3f9qz"
	TmuxEraSeedB = "old-session:0.0"
	TmuxEraSeedC = "old-session:1.2"

	// The subject half: names carrying no staleness role at all, for a fixture
	// measuring something else — a listing's ordering, a registration's write,
	// a removal's exit status.
	SubjectSeedA = sessionNameKey(7, "subject")
	SubjectSeedB = sessionNameKey(8, "subject")
	SubjectSeedC = sessionNameKey(9, "subject")
	SubjectSeedD = sessionNameKey(10, "subject")
)

// StaleHookSeed is a hooks.json body holding one genuinely stale entry beside
// one live one, so a sweep that runs reaps exactly the stale key and is
// measurably distinguishable from one that stood down. The stale entry is
// keyed on ReapableSeedA and the live one on LiveSeedA, so a fixture staging
// it asserts the outcome under those two names.
var StaleHookSeed = fmt.Sprintf(`{
  %q: {"on-resume": "cmd-gone"},
  %q: {"on-resume": "cmd-live"}
}`, ReapableSeedA, LiveSeedA)
