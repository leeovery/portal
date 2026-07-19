package cmd

// Tests for the multi-target routing gate + aggregated atomic pre-flight abort
// (Task 3-4). They drive openCmd.RunE through cobra, injecting the ordered raw
// argv via the openRawArgs seam and capturing the burst dispatch via a
// runOpenBurstFunc override. MUST NOT use t.Parallel (package cmd mutates
// package-level state).

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

// burstCapture records which dispatch arm openCmd.RunE took for a multi-target
// test: the burst (with the surfaces it was handed), a single connect
// (openSessionFunc / openPathFunc), or the picker.
type burstCapture struct {
	burstCalled   bool
	burstSurfaces []spawn.Surface
	sessionCalled bool
	sessionName   string
	pathCalled    bool
	tuiCalled     bool
}

// installOpenMultiTargetSeams wires the standard cmd-level seams for a
// multi-target open test: a nop bootstrap, the injected resolver deps, an
// injected raw argv (openRawArgs), and captured overrides of runOpenBurstFunc /
// openSessionFunc / openPathFunc / openTUIFunc. Every override is restored via
// t.Cleanup.
func installOpenMultiTargetSeams(t *testing.T, deps *OpenDeps, rawArgs []string) *burstCapture {
	t.Helper()

	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	openDeps = deps
	t.Cleanup(func() { openDeps = nil })

	bc := &burstCapture{}

	origBurst := runOpenBurstFunc
	runOpenBurstFunc = func(_ *cobra.Command, surfaces []spawn.Surface, _ []string) error {
		bc.burstCalled = true
		bc.burstSurfaces = surfaces
		return nil
	}
	t.Cleanup(func() { runOpenBurstFunc = origBurst })

	origSession := openSessionFunc
	openSessionFunc = func(_ *cobra.Command, name string) error {
		bc.sessionCalled = true
		bc.sessionName = name
		return nil
	}
	t.Cleanup(func() { openSessionFunc = origSession })

	origPath := openPathFunc
	openPathFunc = func(_ *cobra.Command, _ string, _ []string) error {
		bc.pathCalled = true
		return nil
	}
	t.Cleanup(func() { openPathFunc = origPath })

	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		bc.tuiCalled = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	origRaw := openRawArgs
	openRawArgs = func() []string { return rawArgs }
	t.Cleanup(func() { openRawArgs = origRaw })

	return bc
}

func TestOpenCommand_MultiTarget_MixedSetTwoMisses_ReportsBothAtomically(t *testing.T) {
	// A mixed set with one hit (api-1) and two misses (gone1, gone2) aborts the
	// WHOLE set atomically: nothing opens (no burst, no connector), and the error
	// reports EVERY unresolvable target so one re-run fixes all.
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{"api-1"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "api-1", "gone1", "gone2"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api-1", "gone1", "gone2"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected an aggregated miss error, got nil")
	}
	if got, want := err.Error(), "nothing resolved for: 'gone1', 'gone2'"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		t.Error("aggregated miss must be a plain error (exit 1), not a UsageError")
	}
	if bc.burstCalled {
		t.Error("runOpenBurstFunc must not be called on an atomic miss abort")
	}
	if bc.sessionCalled || bc.pathCalled || bc.tuiCalled {
		t.Error("no connector/creator/picker may be called on an atomic miss abort")
	}
}

func TestOpenCommand_MultiTarget_SingleMissInThreeSet_AbortsAtomically(t *testing.T) {
	// A single miss anywhere in a 3-target set (api-1 hit, gone miss, api-2 hit)
	// aborts atomically — nothing opens, the hits do NOT connect.
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{"api-1", "api-2"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "api-1", "gone", "api-2"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api-1", "gone", "api-2"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected an aggregated miss error, got nil")
	}
	if got, want := err.Error(), "nothing resolved for: 'gone'"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if bc.burstCalled {
		t.Error("runOpenBurstFunc must not be called on an atomic miss abort")
	}
	if bc.sessionCalled || bc.pathCalled {
		t.Error("no connector/creator may be called on an atomic miss abort")
	}
}

func TestOpenCommand_MultiTargetMiss_OmitsMinusF(t *testing.T) {
	// A two-target all-miss set reports both, joined by spawn.QuoteJoin, and omits
	// the -f suggestion (-f cannot carry a multi-target intent).
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "a", "b"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "a", "b"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected an aggregated miss error, got nil")
	}
	if got, want := err.Error(), "nothing resolved for: 'a', 'b'"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if bc.burstCalled {
		t.Error("runOpenBurstFunc must not be called on an atomic miss abort")
	}
}

func TestOpenCommand_SingleTargetMiss_KeepsMinusFSuggestion(t *testing.T) {
	// A single non-glob bare miss stays on the EXISTING single-target path and
	// keeps the Phase-1 -f escape-hatch suggestion (NOT the aggregated wording).
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "blog"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "blog"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected a single-target miss error, got nil")
	}
	if got, want := err.Error(), "nothing resolved for 'blog' — try -f blog"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if bc.burstCalled {
		t.Error("runOpenBurstFunc must not be called for a single non-glob target")
	}
}

func TestOpenCommand_SingleGlobExpandingToZero_KeepsMinusF(t *testing.T) {
	// A single session glob that expands to ZERO matches is N=1 arity: it routes
	// through the burst resolver (glob may expand to K≥2) but, expanding to zero,
	// hard-fails with the single-target -f suggestion, not the aggregated wording.
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "nomatch-*"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "nomatch-*"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected a zero-match glob miss error, got nil")
	}
	if got, want := err.Error(), "nothing resolved for 'nomatch-*' — try -f nomatch-*"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if bc.burstCalled {
		t.Error("runOpenBurstFunc must not be called when a glob expands to zero")
	}
}

func TestOpenCommand_SingleGlobExpandingToMany_Bursts(t *testing.T) {
	// A single bare session glob expanding to K≥2 matches routes to the burst with
	// K attach surfaces (overrides Phase-1's single-glob first-match).
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{"api-1", "api-2"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "api-*"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api-*"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bc.burstCalled {
		t.Fatal("runOpenBurstFunc must be called for a glob expanding to K≥2")
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "api-1"},
		{Kind: spawn.SurfaceAttach, Value: "api-2"},
	}
	assertBurstSurfaces(t, bc.burstSurfaces, want)
	if bc.sessionCalled {
		t.Error("openSessionFunc must not be called on the burst path")
	}
}

func TestOpenCommand_MultiTarget_AllHitRepeatedPin_Bursts(t *testing.T) {
	// `open -s a -s b` — a repeated same-flag pin — is TWO targets, proving the
	// raw-args scan preserves repeats cobra collapses. Both hit → burst with the
	// ordered attach surfaces.
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{"a", "b"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "-s", "a", "-s", "b"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "-s", "a", "-s", "b"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bc.burstCalled {
		t.Fatal("runOpenBurstFunc must be called for two session pins")
	}
	want := []spawn.Surface{
		{Kind: spawn.SurfaceAttach, Value: "a"},
		{Kind: spawn.SurfaceAttach, Value: "b"},
	}
	assertBurstSurfaces(t, bc.burstSurfaces, want)
}

func TestOpenCommand_SingleGlobExpandingToOne_SingleConnectNotBurst(t *testing.T) {
	// A single glob expanding to exactly ONE surface degenerates to a single
	// connect through openResolved (openSessionFunc), NOT the burst.
	bc := installOpenMultiTargetSeams(t,
		&OpenDeps{
			SessionLister: &testSessionLister{names: []string{"api-1"}},
			AliasLookup:   &testAliasLookup{aliases: map[string]string{}},
			Zoxide:        &testZoxideQuerier{err: resolver.ErrNoMatch},
			DirValidator:  &testDirValidator{existing: map[string]bool{}},
		},
		[]string{"portal", "open", "api-*"},
	)

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", "api-*"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bc.burstCalled {
		t.Error("runOpenBurstFunc must not be called when a glob expands to a single surface")
	}
	if !bc.sessionCalled {
		t.Fatal("openSessionFunc must be called for the single-surface connect")
	}
	if bc.sessionName != "api-1" {
		t.Errorf("openSessionFunc called with %q, want %q", bc.sessionName, "api-1")
	}
}

// assertBurstSurfaces compares captured surfaces against the expected ordered
// slice element-by-element.
func assertBurstSurfaces(t *testing.T, got, want []spawn.Surface) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("surfaces = %v, want %v (len %d != %d)", got, want, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("surface[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
