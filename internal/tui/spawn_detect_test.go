package tui

// restore-host-terminal-windows-6-1 — Async terminal-detection lifecycle + caching.
//
// These white-box (package tui) tests drive the Model through both routes that
// reach PageSessions — the cold loading→Sessions transition and the warm direct
// Sessions entry — and assert the detection lifecycle: exactly ONE async Detect()
// dispatch guarded by the detectDispatched latch, an in-flight window distinct
// from the resolved state, a cached identity + resolution delivered by the
// injected config-aware resolve seam, and no re-dispatch / re-resolve on any
// rebuild path (s-toggle, SessionsMsg refresh, filter apply/clear,
// projects-edit→Sessions return).
//
// No t.Parallel: consistent with the rest of the tui test surface (which mutates
// list state through Update via package-level fakes).

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeDetector is a scripted TerminalDetector that records how many times
// Detect() is invoked and returns a fixed identity. The picker calls Detect()
// exactly once on the command goroutine across the whole lifecycle, so calls
// must never exceed 1.
type fakeDetector struct {
	identity spawn.Identity
	calls    int
}

func (f *fakeDetector) Detect() spawn.Identity {
	f.calls++
	return f.identity
}

// countingResolve wraps a resolve func and counts invocations so a test can
// assert the terminalDetectedMsg arm resolves exactly once and no rebuild
// re-resolves.
type countingResolve struct {
	calls int
	fn    func(spawn.Identity) (spawn.Adapter, spawn.Resolution)
}

func (c *countingResolve) resolve(id spawn.Identity) (spawn.Adapter, spawn.Resolution) {
	c.calls++
	return c.fn(id)
}

// nativeResolve returns the real config-aware resolve seam over an EMPTY
// terminals.json config, so resolution reduces to the built-in native →
// unsupported precedence (ghostty → native; apple.Terminal / NULL → unsupported).
// Using the production resolver keeps DetectUnsupported's truth table honest:
// a recognised-but-undriven terminal (com.apple.Terminal) is non-NULL yet
// resolves unsupported, which IsNull() alone would miss.
func nativeResolve() func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
	return spawn.NewResolver(spawn.TerminalsConfig{}).Resolve
}

func ghosttyIdentity() spawn.Identity { return spawn.NewIdentity("com.mitchellh.ghostty", "Ghostty") }
func appleTerminalIdentity() spawn.Identity {
	return spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")
}

// press builds a rune KeyPressMsg matching the established tui-test pattern
// (Code carries the rune so the bubbles/list keymap — e.g. its "/" filter
// binding — matches, and Text drives isRuneKey / textinput).
func press(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// oneNamedSession is a single-session snapshot whose name contains "a" so a
// "/a" filter keeps a non-empty match. Folded into one place so the filter
// regression test's fixture and its filter char cannot silently drift.
func oneNamedSession() []tmux.Session { return []tmux.Session{{Name: "alpha", Windows: 1}} }

// dispatchWarmDetection builds a warm-direct model (no serverStarted → opens on
// PageSessions) wired with the given detection seams, feeds the first SessionsMsg
// (the warm entry point), and returns the model plus the batched cmd the
// SessionsMsg arm emitted — WITHOUT draining it, so the caller can observe the
// in-flight window before the async Detect() resolves.
func dispatchWarmDetection(t *testing.T, det TerminalDetector, res func(spawn.Identity) (spawn.Adapter, spawn.Resolution)) (Model, tea.Cmd) {
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

// warmResolvedModel drives the warm-direct route all the way through the async
// Detect() resolution and returns the model with detection cached.
func warmResolvedModel(t *testing.T, det TerminalDetector, res func(spawn.Identity) (spawn.Adapter, spawn.Resolution)) Model {
	t.Helper()
	m, cmd := dispatchWarmDetection(t, det, res)
	return drainBatchToModel(t, m, cmd)
}

// TestDetection_WarmSessionsEntry_DispatchesOnce covers the warm direct Sessions
// entry (no loading page): the first SessionsMsg landing PageSessions dispatches
// exactly one detection command; DetectDispatched flips true and the resolved
// terminalDetectedMsg caches the identity.
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

// TestDetection_InFlightVsResolvedNull pins the two distinguishable states: after
// dispatch but before the terminalDetectedMsg (in-flight: dispatched && !resolved),
// versus after a NULL identity resolves (resolved && IsNull()).
func TestDetection_InFlightVsResolvedNull(t *testing.T) {
	det := &fakeDetector{identity: spawn.Identity{}} // NULL (remote/mosh / no host-local)

	m, cmd := dispatchWarmDetection(t, det, nativeResolve())

	// In-flight: dispatched, not yet resolved.
	if !m.DetectDispatched() || m.DetectResolved() {
		t.Fatalf("in-flight window must be dispatched && !resolved; dispatched=%v resolved=%v", m.DetectDispatched(), m.DetectResolved())
	}

	final := drainBatchToModel(t, m, cmd)

	// Resolved NULL: resolved && IsNull() — distinct from the in-flight window.
	if !final.DetectResolved() {
		t.Error("resolved state must have DetectResolved=true")
	}
	if !final.DetectedIdentity().IsNull() {
		t.Error("a NULL identity must resolve to an IsNull() cached identity")
	}
}

// TestDetection_Unsupported_Predicate is the DetectUnsupported truth table via
// the injected config-aware resolve seam: TRUE for a NULL identity AND a
// non-NULL recognised-but-undriven identity (com.apple.Terminal), FALSE for a
// native-driven identity (ghostty). IsNull() alone is NOT the test.
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

// TestDetection_TransientError_CachesUnsupported: a transient Detect() failure
// surfaces as the NULL identity (spawn.Detect folds it there), which the model
// caches as unsupported (IsNull() true). The model itself emits no WARN — the
// transient WARN is owned by spawn.Detector.Detect, not the picker.
func TestDetection_TransientError_CachesUnsupported(t *testing.T) {
	det := &fakeDetector{identity: spawn.Identity{}} // transient → NULL shape

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

// TestDetection_SToggle_NoReDispatch: the s-key grouping-mode toggle rebuilds the
// session list but must never re-dispatch detection nor reset the cached state.
func TestDetection_SToggle_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	updated, _ := m.Update(press('s'))
	assertNoReDispatch(t, updated.(Model), det, res, "s-toggle")
}

// TestDetection_SessionsMsgRefresh_NoReDispatch: a subsequent SessionsMsg (a
// kill/rename/preview refresh) re-enters the SessionsMsg arm — which calls
// maybeDispatchDetectionCmd — but the detectDispatched latch makes it a no-op.
func TestDetection_SessionsMsgRefresh_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	updated, cmd := m.Update(SessionsMsg{Sessions: oneNamedSession()})
	final := drainBatchToModel(t, updated.(Model), cmd)
	assertNoReDispatch(t, final, det, res, "SessionsMsg refresh")
}

// TestDetection_FilterApplyClear_NoReDispatch: applying and clearing the list
// filter rebuilds the visible set (flatten-on-filter) but never re-dispatches.
func TestDetection_FilterApplyClear_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	var model tea.Model = m
	model, _ = model.Update(press('/'))                           // start filtering
	model, _ = model.Update(press('a'))                           // type a matching char
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})  // apply → browse
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) // clear

	assertNoReDispatch(t, model.(Model), det, res, "filter apply/clear")
}

// TestDetection_ProjectsEditReturn_NoReDispatch: the projects→Sessions return
// (x on the projects page) flips back to PageSessions and dispatches a mode-aware
// re-group refresh (previewSessionsRefreshedMsg), but must not re-dispatch
// detection nor reset the cached state.
func TestDetection_ProjectsEditReturn_NoReDispatch(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}
	m := warmResolvedModel(t, det, res.resolve)
	assertDetectionResolvedOnce(t, m, det, res)

	// Sessions → Projects (x), then Projects → Sessions (x) — the round trip that
	// re-enters PageSessions via the projects-edit return path.
	var model tea.Model = m
	model, _ = model.Update(press('x')) // Sessions → Projects
	if model.(Model).ActivePage() != PageProjects {
		t.Fatalf("setup: x on Sessions must land on PageProjects, got %v", model.(Model).ActivePage())
	}
	model, cmd := model.Update(press('x')) // Projects → Sessions (edit-return)
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("setup: x on Projects must return to PageSessions, got %v", model.(Model).ActivePage())
	}
	final := drainBatchToModel(t, model.(Model), cmd)
	assertNoReDispatch(t, final, det, res, "projects-edit→Sessions return")
}

// TestDetection_ColdLoadingToSessions_DispatchesOnce covers the cold concurrent
// route: the loading→Sessions transition (in the LoadingMinElapsedMsg /
// BootstrapCompleteMsg arm) dispatches detection exactly once, and the
// post-restore refetch SessionsMsg that follows does NOT dispatch a second.
func TestDetection_ColdLoadingToSessions_DispatchesOnce(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}
	res := &countingResolve{fn: nativeResolve()}

	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]tmux.Session{restored}}
	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithProjectStore(stubProjectStore{}),
		WithTerminalDetector(det),
		WithResolve(res.resolve),
	)

	interim, completeCmd := driveColdBootToTransition(t, m, []tmux.Session{})

	// The transition (cold interim PageSessions) must have dispatched detection.
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

// TestDetection_IndependentOfAppearanceGate proves the detection command is never
// part of the §2.6 first-paint appearance gate: the gate resolves (modeResolved)
// with no terminal detection, and detection is not even dispatched while on the
// loading page (guarded to PageSessions), so the first paint never waits on it.
func TestDetection_IndependentOfAppearanceGate(t *testing.T) {
	det := &fakeDetector{identity: ghosttyIdentity()}

	// A directly-constructed model resolves the appearance gate (auto → dark
	// fallback) from frame one — with detection unresolved and undispatched.
	m := New(fakeLister{}, WithTerminalDetector(det), WithResolve(nativeResolve()))
	if !m.modeResolved() {
		t.Fatal("appearance gate must resolve independently of terminal detection")
	}
	if m.DetectResolved() || m.DetectDispatched() {
		t.Fatal("constructing the model must not dispatch or resolve detection")
	}

	// On the loading page detection is guarded off (activePage != PageSessions),
	// so the gate's first-paint wait never depends on it.
	loading := New(fakeLister{},
		WithServerStarted(true),
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

// TestDetection_NilDetector_NeverDispatches: a model with no Detector wired (the
// capture harness and every existing test) never dispatches — maybeDispatch is a
// no-op and reaching PageSessions leaves DetectDispatched false.
func TestDetection_NilDetector_NeverDispatches(t *testing.T) {
	m := New(fakeLister{}, WithProjectStore(stubProjectStore{}))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: oneNamedSession()})
	if model.(Model).DetectDispatched() {
		t.Error("a nil detector must never dispatch detection")
	}
}

// TestBuild_WiresDetectorAndResolve pins the build.go seam wiring: Build threads
// Deps.Detector + Deps.Resolve onto the model, and tolerates their absence
// (nil-tolerant, matching the offline capture harness).
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

	// Nil-tolerant: omitting both leaves the seams unwired without panicking.
	bare := Build(Deps{Lister: fakeLister{}})
	if bare.detector != nil {
		t.Error("Build must leave detector nil when Deps.Detector is unset")
	}
	if bare.resolve != nil {
		t.Error("Build must leave resolve nil when Deps.Resolve is unset")
	}
}

// assertDetectionResolvedOnce is the shared precondition for the rebuild
// regression tests: detection dispatched, resolved, and both Detect()/resolve
// called exactly once.
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

// assertNoReDispatch asserts a rebuild path neither re-dispatched detection nor
// reset the cached state.
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
