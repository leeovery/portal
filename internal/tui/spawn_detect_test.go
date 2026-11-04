package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
)

type fakeDetector struct {
	identity spawn.Identity
	calls    int
}

func (f *fakeDetector) Detect() spawn.Identity {
	f.calls++
	return f.identity
}

type countingResolve struct {
	calls int
	fn    spawn.AdapterResolver
}

func (c *countingResolve) resolve(id spawn.Identity) (spawn.Adapter, spawn.Resolution) {
	c.calls++
	return c.fn(id)
}

func nativeResolve() spawn.AdapterResolver {
	return spawn.NewResolver(spawn.TerminalsConfig{}).Resolve
}

func ghosttyIdentity() spawn.Identity { return spawn.NewIdentity("com.mitchellh.ghostty", "Ghostty") }
func appleTerminalIdentity() spawn.Identity {
	return spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")
}

func press(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func oneNamedSession() []Session { return []Session{{Name: "alpha"}} }

func dispatchWarmDetection(t *testing.T, det TerminalDetector, res spawn.AdapterResolver) (Model, tea.Cmd) {
	t.Helper()
	m := New(fakeLister{},
		WithProjectStore(stubProjectStore{}),
		WithTerminalDetector(det),
		WithResolve(res),
	)
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, cmd := model.Update(SessionsMsg{Sessions: oneNamedSession()})
	return model.(Model), cmd
}

func warmResolvedModel(t *testing.T, det TerminalDetector, res spawn.AdapterResolver) Model {
	t.Helper()
	m, cmd := dispatchWarmDetection(t, det, res)
	return drainBatchToModel(t, m, cmd)
}

func TestDetection_WarmSessionsEntry_DispatchesOnce(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}

	m, cmd := dispatchWarmDetection(t, det, nativeResolve())
	if !m.DetectDispatched() {
		t.Fatal("reaching PageSessions must dispatch detection (DetectDispatched=true)")
	}
	if det.calls != 0 {
		t.Fatalf("Detect() must run async on the command goroutine, not inside Update; calls=%d", det.calls)
	}

	final := drainBatchToModel(t, m, cmd)
	if det.calls != 1 {
		t.Errorf("expected exactly one Detect() call across the lifecycle, got %d", det.calls)
	}
	if !final.DetectResolved() {
		t.Error("after the terminalDetectedMsg lands, DetectResolved must be true")
	}
	if got := final.DetectedIdentity(); got != ghosttyIdentity() {
		t.Errorf("DetectedIdentity must cache the detected identity, want %v got %v", ghosttyIdentity(), got)
	}
}

func TestDetection_InFlightVsResolvedNull(t *testing.T) {
	det := &fakeDetector{identity: spawn.Identity{}}

	m, cmd := dispatchWarmDetection(t, det, nativeResolve())

	if !m.DetectDispatched() || m.DetectResolved() {
		t.Fatalf("in-flight window must be dispatched && !resolved; dispatched=%v resolved=%v", m.DetectDispatched(), m.DetectResolved())
	}

	final := drainBatchToModel(t, m, cmd)

	if !final.DetectResolved() {
		t.Error("resolved state must have DetectResolved=true")
	}
	if !final.DetectedIdentity().IsNull() {
		t.Error("a NULL identity must resolve to an IsNull() cached identity")
	}
}

func TestDetection_Unsupported_Predicate(t *testing.T) {
	cases := []struct {
		name            string
		identity        spawn.Identity
		wantUnsupported bool
		wantResolution  spawn.Resolution
	}{
		{"null remote/mosh", spawn.Identity{}, true, spawn.ResolutionUnsupported},
		{"recognised-but-undriven apple terminal", appleTerminalIdentity(), true, spawn.ResolutionUnsupported},
		{"native ghostty", ghosttyIdentity(), false, spawn.ResolutionNative},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			det := &fakeDetector{identity: tc.identity}
			m := warmResolvedModel(t, det, nativeResolve())

			if got := m.DetectUnsupported(); got != tc.wantUnsupported {
				t.Errorf("DetectUnsupported()=%v, want %v (identity %v)", got, tc.wantUnsupported, tc.identity)
			}
			if got := m.DetectedResolution(); got != tc.wantResolution {
				t.Errorf("DetectedResolution()=%q, want %q", got, tc.wantResolution)
			}
		})
	}
}

func TestDetection_TransientError_CachesUnsupported(t *testing.T) {
	det := &fakeDetector{identity: spawn.Identity{}}

	m := warmResolvedModel(t, det, nativeResolve())

	if !m.DetectResolved() {
		t.Fatal("a transient (NULL-shaped) detection must still resolve")
	}
	if !m.DetectedIdentity().IsNull() {
		t.Error("a transient detection caches as the NULL identity (IsNull() true)")
	}
	if !m.DetectUnsupported() {
		t.Error("a transient (NULL) detection must classify as unsupported")
	}
}

func TestDetection_SToggle_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	updated, _ := m.Update(press('s'))
	assertNoReDispatch(t, updated.(Model), det, res, "s-toggle")
}

func TestDetection_SessionsMsgRefresh_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	updated, cmd := m.Update(SessionsMsg{Sessions: oneNamedSession()})
	final := drainBatchToModel(t, updated.(Model), cmd)
	assertNoReDispatch(t, final, det, res, "SessionsMsg refresh")
}

func TestDetection_FilterApplyClear_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	var model tea.Model = m
	model, _ = model.Update(press('/'))
	model, _ = model.Update(press('a'))
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assertNoReDispatch(t, model.(Model), det, res, "filter apply/clear")
}

func TestDetection_ProjectsEditReturn_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	var model tea.Model = m
	model, _ = model.Update(press('x'))
	if model.(Model).ActivePage() != PageProjects {
		t.Fatalf("setup: x on Sessions must land on PageProjects, got %v", model.(Model).ActivePage())
	}
	model, cmd := model.Update(press('x'))
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("setup: x on Projects must return to PageSessions, got %v", model.(Model).ActivePage())
	}
	final := drainBatchToModel(t, model.(Model), cmd)
	assertNoReDispatch(t, final, det, res, "projects-edit→Sessions return")
}

func TestDetection_ColdLoadingToSessions_DispatchesOnce(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}

	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]Session{restored}}
	m := New(lister,
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
		WithTerminalDetector(det),
		WithResolve(res.resolve),
	)

	interim, completeCmd := driveColdBootToTransition(t, m, []Session{})

	if !interim.DetectDispatched() {
		t.Fatal("cold loading→Sessions transition must dispatch detection")
	}

	final := drainBatchToModel(t, interim, completeCmd)

	if det.calls != 1 {
		t.Errorf("cold route must dispatch exactly one Detect() (latch survives the post-restore refetch), got %d", det.calls)
	}
	if res.calls != 1 {
		t.Errorf("cold route must resolve exactly once, got %d", res.calls)
	}
	if !final.DetectResolved() {
		t.Error("cold route must resolve detection after the transition")
	}
	if got := final.DetectedIdentity(); got != ghosttyIdentity() {
		t.Errorf("cold route must cache the detected identity, want %v got %v", ghosttyIdentity(), got)
	}
	if final.ActivePage() != PageSessions {
		t.Fatalf("cold route with N>0 must land on PageSessions, got %v", final.ActivePage())
	}
}

func TestDetection_IndependentOfAppearanceGate(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}

	m := New(fakeLister{}, WithTerminalDetector(det), WithResolve(nativeResolve()))
	if !m.modeResolved() {
		t.Fatal("appearance gate must resolve independently of terminal detection")
	}
	if m.DetectResolved() || m.DetectDispatched() {
		t.Fatal("constructing the model must not dispatch or resolve detection")
	}

	loading := New(fakeLister{},
		WithBootstrapping(idleBootstrap),
		WithTerminalDetector(det),
		WithResolve(nativeResolve()),
	)
	if cmd := (&loading).maybeDispatchDetectionCmd(); cmd != nil {
		t.Error("detection must not dispatch while on PageLoading (activePage != PageSessions)")
	}
	if loading.DetectDispatched() {
		t.Error("a guarded-off dispatch attempt must not set the detectDispatched latch")
	}
	if det.calls != 0 {
		t.Errorf("no Detect() should run for a guarded-off dispatch, got %d", det.calls)
	}
}

func TestDetection_NilDetector_NeverDispatches(t *testing.T) {
	m := New(fakeLister{}, WithProjectStore(stubProjectStore{}))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: oneNamedSession()})
	if model.(Model).DetectDispatched() {
		t.Error("a nil detector must never dispatch detection")
	}
}

func TestBuild_WiresDetectorAndResolve(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	m := Build(Deps{
		Lister:   fakeLister{},
		Detector: det,
		Resolve:  nativeResolve(),
	})
	if m.detector == nil {
		t.Error("Build must wire Deps.Detector onto the model")
	}
	if m.resolve == nil {
		t.Error("Build must wire Deps.Resolve onto the model")
	}

	bare := Build(Deps{Lister: fakeLister{}})
	if bare.detector != nil {
		t.Error("Build must leave detector nil when Deps.Detector is unset")
	}
	if bare.resolve != nil {
		t.Error("Build must leave resolve nil when Deps.Resolve is unset")
	}
}

func assertDetectionResolvedOnce(t *testing.T, m Model, det *fakeDetector, res *countingResolve) {
	t.Helper()
	if !m.DetectDispatched() || !m.DetectResolved() {
		t.Fatalf("precondition: detection must be dispatched && resolved; dispatched=%v resolved=%v", m.DetectDispatched(), m.DetectResolved())
	}
	if det.calls != 1 {
		t.Fatalf("precondition: exactly one Detect() call, got %d", det.calls)
	}
	if res.calls != 1 {
		t.Fatalf("precondition: exactly one resolve() call, got %d", res.calls)
	}
}

func assertNoReDispatch(t *testing.T, m Model, det *fakeDetector, res *countingResolve, path string) {
	t.Helper()
	if det.calls != 1 {
		t.Errorf("%s must not re-dispatch detection: Detect() calls=%d, want 1", path, det.calls)
	}
	if res.calls != 1 {
		t.Errorf("%s must not re-resolve: resolve() calls=%d, want 1", path, res.calls)
	}
	if !m.DetectDispatched() {
		t.Errorf("%s must not reset detectDispatched", path)
	}
	if !m.DetectResolved() {
		t.Errorf("%s must not reset detectResolved", path)
	}
	if got := m.DetectedIdentity(); got != ghosttyIdentity() {
		t.Errorf("%s must not change the cached identity, want %v got %v", path, ghosttyIdentity(), got)
	}
}
