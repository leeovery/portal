package hookstest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/xdg"
)

// seedKeyPrefix keeps a seed key legible in test output. It is fitted to
// whatever room the token width leaves beside the n disambiguator.
const seedKeyPrefix = "seed"

// disambiguatorWidth is the number of trailing bytes a token-shaped seed key
// spends on distinguishing one key from the next.
const disambiguatorWidth = 2

// paneTokenWidth is taken from the mint rather than declared here, so a seed
// key is authored at whatever width recognition reads.
var paneTokenWidth = sync.OnceValue(func() int {
	token, err := nanoid.NewPaneTokenGenerator()()
	if err != nil {
		panic(fmt.Sprintf("hookstest: mint a pane token to size the seed vocabulary: %v", err))
	}
	return len(token)
})

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

// tokenShapedHookKey returns a seed hook key the staleness rule can judge. The
// returned value is authored at whatever width the mint reads, so a width move
// carries it along, and it is asserted token-shaped, so an id charset change
// panics here rather than silently turning a reap fixture into a retention one.
// n only disambiguates — distinct n give distinct keys.
func tokenShapedHookKey(n int) string {
	radix := len(nanoid.Alphabet)
	if n < 0 || n >= radix*radix {
		panic(fmt.Sprintf("hookstest: seed key n = %d out of range [0,%d)", n, radix*radix))
	}
	key := fitPrefix(paneTokenWidth()-disambiguatorWidth) + string([]byte{
		nanoid.Alphabet[n/radix],
		nanoid.Alphabet[n%radix],
	})
	if !nanoid.IsTokenShaped(key) {
		panic(fmt.Sprintf("hookstest: seed key %q is not token-shaped — the seed vocabulary has drifted from nanoid.IsTokenShaped", key))
	}
	return key
}

// fitPrefix returns the legible seed prefix as exactly width bytes, padded from
// the alphabet when the token width leaves more room than the prefix fills.
func fitPrefix(width int) string {
	if width < 1 {
		panic(fmt.Sprintf("hookstest: a pane token of %d bytes leaves no room for a legible seed prefix", paneTokenWidth()))
	}
	if width <= len(seedKeyPrefix) {
		return seedKeyPrefix[:width]
	}
	return seedKeyPrefix + strings.Repeat(string(nanoid.Alphabet[0]), width-len(seedKeyPrefix))
}

// unjudgeableHookKey returns a seed hook key of the legacy `<name>:window.pane`
// shape, which the staleness rule cannot judge and therefore retains even when
// absent from the live pane set. n only disambiguates.
func unjudgeableHookKey(n int) string {
	return fmt.Sprintf("unjudgeable-session-%d:0.0", n)
}

// The named seed keys the suites share, and the only route to one: a fixture
// names the seed whose role it means — reapable, live, unjudgeable or plain
// subject — so a name cannot drift onto a key of another half, and no package
// re-derives a key of its own.
var (
	// The reapable half: keys the staleness rule can judge, so an entry under
	// one is reaped once it is absent from the live pane set. A fixture keying
	// its subject on one of these is measuring the reap.
	ReapableSeedA = tokenShapedHookKey(0)
	ReapableSeedB = tokenShapedHookKey(1)
	ReapableSeedC = tokenShapedHookKey(2)
	ReapableSeedD = tokenShapedHookKey(3)

	// The live half: keys of the same judgeable shape, reserved for the entries
	// a fixture's enumeration reports as live. An entry under one survives
	// because its pane is live and not because the reaper cannot judge its
	// shape — which is the whole point of keeping them apart from the reapable
	// half rather than reusing an index.
	LiveSeedA = tokenShapedHookKey(4)
	LiveSeedB = tokenShapedHookKey(5)
	LiveSeedC = tokenShapedHookKey(6)

	// The unjudgeable half: legacy-shaped keys the staleness rule cannot judge,
	// so an entry under one is retained whatever the live set says.
	UnjudgeableSeedA = unjudgeableHookKey(0)
	UnjudgeableSeedB = unjudgeableHookKey(1)
	UnjudgeableSeedC = unjudgeableHookKey(2)

	// The subject half: token-shaped keys carrying no staleness role at all,
	// for a fixture measuring something else — a listing's ordering, a
	// registration's write, a removal's exit status. They come off the same
	// mint as the rest, so a width move carries them along like the others.
	SubjectSeedA = tokenShapedHookKey(7)
	SubjectSeedB = tokenShapedHookKey(8)
	SubjectSeedC = tokenShapedHookKey(9)
	SubjectSeedD = tokenShapedHookKey(10)
)

// StaleHookSeed is a hooks.json body holding one genuinely stale token-shaped
// entry beside one live one, so a sweep that runs reaps exactly the stale key
// and is measurably distinguishable from one that stood down. The stale entry
// is keyed on ReapableSeedA and the live one on LiveSeedA, so a fixture staging
// it asserts the outcome under those two names.
var StaleHookSeed = fmt.Sprintf(`{
  %q: {"on-resume": "cmd-gone"},
  %q: {"on-resume": "cmd-live"}
}`, ReapableSeedA, LiveSeedA)
