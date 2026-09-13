package cmd

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

type burstCapture struct {
	burstCalled   bool
	burstSurfaces []spawn.Surface
	sessionCalled bool
	sessionName   string
	pathCalled    bool
	tuiCalled     bool
}

func installOpenMultiTargetSeams(t *testing.T, deps *OpenDeps, rawArgs []string) *burstCapture {
	t.Helper()

	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

	withOpenDeps(t, *deps)

	bc := &burstCapture{}

	withFuncSeam(t, &runOpenBurstFunc, func(_ *cobra.Command, surfaces []spawn.Surface, _ []string) error {
		bc.burstCalled = true
		bc.burstSurfaces = surfaces
		return nil
	})

	withFuncSeam(t, &openSessionFunc, func(_ *cobra.Command, name string) error {
		bc.sessionCalled = true
		bc.sessionName = name
		return nil
	})

	withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, _ string, _ []string) error {
		bc.pathCalled = true
		return nil
	})

	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		bc.tuiCalled = true
		return nil
	})

	withFuncSeam(t, &openRawArgs, func() []string { return rawArgs })

	return bc
}

func TestOpenCommand_MultiTarget_MixedSetTwoMisses_ReportsBothAtomically(t *testing.T) {
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
	if _, ok := errors.AsType[*UsageError](err); ok {
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

func TestSingleMissError_ByteIdenticalFormat(t *testing.T) {
	if got, want := singleMissError("blog").Error(), "nothing resolved for 'blog' — try -f blog"; got != want {
		t.Errorf("singleMissError = %q, want %q", got, want)
	}
}

func TestOpenCommand_SingleGlobExpandingToZero_KeepsMinusF(t *testing.T) {
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
	// A repeated same-flag pin is two targets: the raw-args scan preserves the
	// repeats cobra collapses.
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
