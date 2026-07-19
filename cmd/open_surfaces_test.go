package cmd

// Tests for the read-only classify engine resolveOpenSurfaces. They call the
// engine directly with read-only resolver fakes (reusing testSessionLister /
// testAliasLookup / testZoxideQuerier / testDirValidator from open_test.go), never
// through cobra — the engine takes a *resolver.QueryResolver + []Target and returns
// ordered surfaces + collected misses. MUST NOT use t.Parallel (package cmd).

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

// newSurfaceResolver builds a QueryResolver from read-only fakes for the engine
// tests. Path targets stat the real filesystem via ResolvePath, so -p tests pass
// real temp dirs; the DirValidator fake backs only alias/zoxide validation.
func newSurfaceResolver(names []string, aliases map[string]string, zoxideResult string, zoxideErr error, existing map[string]bool) *resolver.QueryResolver {
	return resolver.NewQueryResolver(
		&testSessionLister{names: names},
		&testAliasLookup{aliases: aliases},
		&testZoxideQuerier{result: zoxideResult, err: zoxideErr},
		&testDirValidator{existing: existing},
	)
}

func TestResolveOpenSurfaces_MixedOrderedSet(t *testing.T) {
	// A mixed ordered target set resolves to ordered attach/mint surfaces in the
	// exact target order — one surface per single-result target.
	dir := t.TempDir()

	qr := newSurfaceResolver(
		[]string{"dev"},
		map[string]string{"myapp": "/code/myapp", "blog": "/code/blog"},
		"/code/zoxide-prj",
		nil,
		map[string]bool{"/code/myapp": true, "/code/blog": true, "/code/zoxide-prj": true},
	)

	targets := []Target{
		{Value: "dev", Domain: "session"},
		{Value: dir, Domain: "path"},
		{Value: "myapp", Domain: "alias"},
		{Value: "prj", Domain: "zoxide"},
		{Value: "blog", Domain: "bare"},
	}

	surfaces, misses, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(misses) != 0 {
		t.Fatalf("misses = %v, want none", misses)
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "dev"},
		{Kind: spawn.SurfaceMint, Value: dir},
		{Kind: spawn.SurfaceMint, Value: "/code/myapp"},
		{Kind: spawn.SurfaceMint, Value: "/code/zoxide-prj"},
		{Kind: spawn.SurfaceMint, Value: "/code/blog"},
	}
	assertSurfaces(t, surfaces, want)
}

func TestResolveOpenSurfaces_SessionGlobExpandsInPlace(t *testing.T) {
	// A bare session glob expands to K attach surfaces that JOIN THE LIST IN PLACE,
	// between the surfaces of the targets around it.
	qr := newSurfaceResolver(
		[]string{"lead", "tail", "api-1", "api-2"},
		map[string]string{},
		"",
		resolver.ErrNoMatch,
		map[string]bool{},
	)

	targets := []Target{
		{Value: "lead", Domain: "session"},
		{Value: "api-*", Domain: "bare"},
		{Value: "tail", Domain: "session"},
	}

	surfaces, misses, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(misses) != 0 {
		t.Fatalf("misses = %v, want none", misses)
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "lead"},
		{Kind: spawn.SurfaceAttach, Value: "api-1"},
		{Kind: spawn.SurfaceAttach, Value: "api-2"},
		{Kind: spawn.SurfaceAttach, Value: "tail"},
	}
	assertSurfaces(t, surfaces, want)
}

func TestResolveOpenSurfaces_AliasKeyGlobExpandsToMints(t *testing.T) {
	// A -a key glob expands to K mint surfaces, each reduced to the aliased literal
	// dir. Keys() is sorted, so order is deterministic.
	qr := newSurfaceResolver(
		nil,
		map[string]string{"workflow-a": "/code/wa", "workflow-b": "/code/wb", "blog": "/code/blog"},
		"",
		resolver.ErrNoMatch,
		map[string]bool{"/code/wa": true, "/code/wb": true},
	)

	targets := []Target{{Value: "workflow-*", Domain: "alias"}}

	surfaces, misses, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(misses) != 0 {
		t.Fatalf("misses = %v, want none", misses)
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceMint, Value: "/code/wa"},
		{Kind: spawn.SurfaceMint, Value: "/code/wb"},
	}
	assertSurfaces(t, surfaces, want)
}

func TestResolveOpenSurfaces_OverlappingGlobsDuplicate(t *testing.T) {
	// Overlapping globs may produce a duplicate surface; it is honored, never
	// deduped (spec § No dedup).
	qr := newSurfaceResolver(
		[]string{"api-1", "api-2"},
		map[string]string{},
		"",
		resolver.ErrNoMatch,
		map[string]bool{},
	)

	targets := []Target{
		{Value: "api-*", Domain: "bare"},
		{Value: "api-1", Domain: "session"},
	}

	surfaces, misses, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(misses) != 0 {
		t.Fatalf("misses = %v, want none", misses)
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "api-1"},
		{Kind: spawn.SurfaceAttach, Value: "api-2"},
		{Kind: spawn.SurfaceAttach, Value: "api-1"},
	}
	assertSurfaces(t, surfaces, want)
}

func TestResolveOpenSurfaces_MintReducedToLiteralDir(t *testing.T) {
	// A mint surface's Value is the resolved literal dir, never the alias key /
	// zoxide query / -p input — the query never travels to the spawned window.
	qr := newSurfaceResolver(
		nil,
		map[string]string{"myapp": "/code/myapp"},
		"/code/zoxide-prj",
		nil,
		map[string]bool{"/code/myapp": true, "/code/zoxide-prj": true},
	)

	targets := []Target{
		{Value: "myapp", Domain: "alias"},
		{Value: "prj", Domain: "zoxide"},
	}

	surfaces, _, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceMint, Value: "/code/myapp"},
		{Kind: spawn.SurfaceMint, Value: "/code/zoxide-prj"},
	}
	assertSurfaces(t, surfaces, want)
	// Explicitly assert the query strings did NOT travel as surface values.
	for _, s := range surfaces {
		if s.Value == "myapp" || s.Value == "prj" {
			t.Errorf("surface value %q is the raw query — mint must reduce to the literal dir", s.Value)
		}
	}
}

func TestResolveOpenSurfaces_GlobNamedDir_BareIsMiss_PathIsMint(t *testing.T) {
	// A directory whose name contains glob metacharacters is UNREACHABLE as a bare
	// positional (glob pre-check → zero session matches → miss), reachable only via
	// -p (ResolvePathPin stats the literal path → mint).
	tmp := t.TempDir()
	globDir := filepath.Join(tmp, "foo[1]")
	if err := os.Mkdir(globDir, 0o755); err != nil {
		t.Fatalf("failed to create glob-named dir: %v", err)
	}

	qr := newSurfaceResolver(nil, map[string]string{}, "", resolver.ErrNoMatch, map[string]bool{})

	t.Run("bare positional is a collected miss", func(t *testing.T) {
		surfaces, misses, err := resolveOpenSurfaces(qr, []Target{{Value: globDir, Domain: "bare"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(surfaces) != 0 {
			t.Errorf("surfaces = %v, want none for a glob-named bare positional", surfaces)
		}
		if len(misses) != 1 || misses[0] != globDir {
			t.Errorf("misses = %v, want [%q]", misses, globDir)
		}
	})

	t.Run("-p pin mints the literal dir", func(t *testing.T) {
		surfaces, misses, err := resolveOpenSurfaces(qr, []Target{{Value: globDir, Domain: "path"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(misses) != 0 {
			t.Errorf("misses = %v, want none", misses)
		}
		assertSurfaces(t, surfaces, []spawn.Surface{{Kind: spawn.SurfaceMint, Value: globDir}})
	})
}

func TestResolveOpenSurfaces_ZoxideNotInstalled_ImmediateHardError(t *testing.T) {
	// ErrZoxideNotInstalled is an environment fault: it aborts the WHOLE resolve
	// immediately (returns the error), with nil surfaces and nil misses — even
	// though an earlier target resolved.
	qr := newSurfaceResolver(
		[]string{"dev"},
		map[string]string{},
		"",
		resolver.ErrZoxideNotInstalled,
		map[string]bool{},
	)

	targets := []Target{
		{Value: "dev", Domain: "session"},
		{Value: "prj", Domain: "zoxide"},
	}

	surfaces, misses, err := resolveOpenSurfaces(qr, targets)
	if err == nil {
		t.Fatal("expected an immediate hard error for ErrZoxideNotInstalled, got nil")
	}
	if !errors.Is(err, resolver.ErrZoxideNotInstalled) {
		t.Fatalf("err = %v, want ErrZoxideNotInstalled", err)
	}
	if surfaces != nil {
		t.Errorf("surfaces = %v, want nil on the immediate hard-error abort", surfaces)
	}
	if misses != nil {
		t.Errorf("misses = %v, want nil on the immediate hard-error abort", misses)
	}
}

func TestResolveOpenSurfaces_CollectedMisses(t *testing.T) {
	// -z no-match, -p non-existent dir, and -p non-directory file are all COLLECTED
	// MISSES (raw target appended to misses), NOT immediate hard errors — only
	// ErrZoxideNotInstalled aborts. The non-directory -p folding is this task's
	// documented classification decision.
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	qr := newSurfaceResolver(nil, map[string]string{}, "", resolver.ErrNoMatch, map[string]bool{})

	targets := []Target{
		{Value: "nope", Domain: "zoxide"},
		{Value: "/nonexistent/dir", Domain: "path"},
		{Value: filePath, Domain: "path"},
	}

	surfaces, misses, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error (collected misses must not hard-fail): %v", err)
	}
	if len(surfaces) != 0 {
		t.Errorf("surfaces = %v, want none", surfaces)
	}
	want := []string{"nope", "/nonexistent/dir", filePath}
	if len(misses) != len(want) {
		t.Fatalf("misses = %v, want %v", misses, want)
	}
	for i := range want {
		if misses[i] != want[i] {
			t.Errorf("misses[%d] = %q, want %q", i, misses[i], want[i])
		}
	}
}

func TestResolveOpenSurfaces_ReadOnly_NoMintOrAttach(t *testing.T) {
	// The engine is strictly read-only: it produces surfaces but never opens/mints.
	// Guard by making the outcome funcs fatal-if-called; resolving a mixed set must
	// not invoke either.
	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		t.Fatal("openPathFunc must not be called during a read-only resolve")
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, _ string) error {
		t.Fatal("openSessionFunc must not be called during a read-only resolve")
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	dir := t.TempDir()
	qr := newSurfaceResolver(
		[]string{"dev"},
		map[string]string{"myapp": "/code/myapp"},
		"",
		resolver.ErrNoMatch,
		map[string]bool{"/code/myapp": true},
	)

	targets := []Target{
		{Value: "dev", Domain: "session"},
		{Value: dir, Domain: "path"},
		{Value: "myapp", Domain: "alias"},
	}

	surfaces, _, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(surfaces) != 3 {
		t.Fatalf("surfaces = %v, want 3 (resolve still classifies, just never opens)", surfaces)
	}
}

func TestResolveOpenSurfaces_ResolveLog_BareNonGlobOnly(t *testing.T) {
	// Exactly one resolve decision line per bare non-glob (guessing-chain) target —
	// including a bare miss (domain=miss). Pins and globs emit NO line.
	h := newCapturingHandler()
	log.SetTestHandler(t, h)

	qr := newSurfaceResolver(
		[]string{"dev", "web", "api-1", "api-2"},
		map[string]string{"blog": "/code/blog"},
		"",
		resolver.ErrNoMatch,
		map[string]bool{"/code/blog": true},
	)

	targets := []Target{
		{Value: "dev", Domain: "session"}, // pin: no line
		{Value: "dev", Domain: "bare"},    // bare session hit: line
		{Value: "api-*", Domain: "bare"},  // bare glob: no line
		{Value: "web", Domain: "session"}, // pin: no line
		{Value: "blog", Domain: "bare"},   // bare alias mint: line
		{Value: "gone", Domain: "bare"},   // bare total miss: line
	}

	_, _, err := resolveOpenSurfaces(qr, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs := h.resolveRecords()
	if len(recs) != 3 {
		t.Fatalf("expected exactly 3 resolve records (one per bare non-glob target), got %d", len(recs))
	}
	for _, r := range recs {
		if r.record.Level != slog.LevelInfo {
			t.Errorf("resolve record level = %v, want INFO", r.record.Level)
		}
	}
	assertResolveAttr(t, recs[0], "target", "dev")
	assertResolveAttr(t, recs[0], "domain", "session")
	assertResolveAttr(t, recs[0], "resolved_path", "dev")

	assertResolveAttr(t, recs[1], "target", "blog")
	assertResolveAttr(t, recs[1], "domain", "alias")
	assertResolveAttr(t, recs[1], "resolved_path", "/code/blog")

	assertResolveAttr(t, recs[2], "target", "gone")
	assertResolveAttr(t, recs[2], "domain", "miss")
	assertResolveAttr(t, recs[2], "resolved_path", "")
}

// assertSurfaces asserts got equals want, element by element, in order.
func assertSurfaces(t *testing.T, got, want []spawn.Surface) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("surfaces = %v, want %v (len %d != %d)", got, want, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("surface[%d] = {%s %q}, want {%s %q}",
				i, got[i].Kind, got[i].Value, want[i].Kind, want[i].Value)
		}
	}
}
