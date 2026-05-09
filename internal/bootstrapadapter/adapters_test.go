package bootstrapadapter_test

// Integration tests for the production-shape bootstrap adapters. The two
// adapters are pure pass-throughs to *tmux.Client / tmux.RegisterPortalHooks,
// so the goal here is a thin smoke layer: prove that Set/Clear toggle the
// expected server option on a live tmux server and that RegisterPortalHooks
// runs to completion. Heavy hook-table semantics are owned by
// internal/tmux/hooks_register_test.go; the orchestrator-level wiring is
// owned by cmd/bootstrap/phase5_integration_test.go. This file only proves
// the adapters' shaping.

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// Compile-time: the production adapter satisfies the orchestrator's
// MarkerCleaner seam. A drift here (e.g. a renamed method) would surface as
// a build failure rather than a runtime panic at bootstrap step 7.
var _ bootstrap.MarkerCleaner = (*bootstrapadapter.StaleMarkerCleaner)(nil)

// listerStub satisfies state.ServerOptionLister with a canned (output, err).
// Used to exercise FIFOSweeper.Sweep's marker-enumeration failure path
// without standing up a live tmux server.
type listerStub struct {
	out string
	err error
}

func (s *listerStub) ShowAllServerOptions() (string, error) {
	return s.out, s.err
}

// TestFIFOSweeper_PropagatesListSkeletonMarkersError proves that a
// ShowAllServerOptions failure surfaces from FIFOSweeper.Sweep wrapped with
// the "list skeleton markers" prefix so the orchestrator's step-7
// Warn-and-swallow path can log it uniformly. Pre-cycle-4 the adapter
// silently returned nil on this path, hiding transient tmux failures from
// portal.log; this test is the regression guard.
func TestFIFOSweeper_PropagatesListSkeletonMarkersError(t *testing.T) {
	sentinel := errors.New("show-options boom")
	s := &bootstrapadapter.FIFOSweeper{
		Client:   &listerStub{err: sentinel},
		StateDir: t.TempDir(),
		Logger:   nil, // *state.Logger is nil-safe; the sweep never reaches it.
	}

	err := s.Sweep()
	if err == nil {
		t.Fatal("Sweep returned nil; want wrapped error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Sweep err = %v; want errors.Is(err, sentinel)=true", err)
	}
	if got := err.Error(); got == "" || got == sentinel.Error() {
		t.Errorf("Sweep err = %q; want a wrapped message containing the cause", got)
	}
}

// staleClientStub satisfies the StaleMarkerCleaner adapter's tmux client
// seam with canned (output, err) values for each of the three primitives the
// adapter consumes:
//
//   - ShowAllServerOptions  — feeds state.ListSkeletonMarkers
//   - ListAllPanesWithFormat — feeds the live-pane enumeration
//   - UnsetServerOption     — clears one stale marker per call
//
// The stub also records the format string requested and every option name
// unset, so tests can assert the canonical format literal is used and that
// the mass-unset hazard guard never fires under failure paths.
type staleClientStub struct {
	showOut string
	showErr error

	listOut    string
	listErr    error
	listFormat string

	unsetCalls []string
	unsetErr   error
}

func (s *staleClientStub) ShowAllServerOptions() (string, error) {
	return s.showOut, s.showErr
}

func (s *staleClientStub) ListAllPanesWithFormat(format string) (string, error) {
	s.listFormat = format
	return s.listOut, s.listErr
}

func (s *staleClientStub) UnsetServerOption(name string) error {
	s.unsetCalls = append(s.unsetCalls, name)
	return s.unsetErr
}

// TestStaleMarkerCleaner_PropagatesListAllPanesWithFormatError proves that a
// ListAllPanesWithFormat failure surfaces from CleanStaleMarkers as a non-nil
// error wrapping the underlying cause. Pre-fix the adapter would have used
// (*tmux.Client).ListAllPanes which silently returns ([]string{}, nil) on
// error — that path would land in the cleanup with an empty live set and
// mass-unset every marker. The cleanup's zero-panes hazard guard relies on
// the error-propagating ListAllPanesWithFormat variant so a transient tmux
// failure surfaces as a soft warning rather than destabilising the server.
func TestStaleMarkerCleaner_PropagatesListAllPanesWithFormatError(t *testing.T) {
	sentinel := errors.New("list-panes boom")
	// Seed at least one marker so the cleanup actually reaches the live-pane
	// call (an empty marker set would short-circuit to nil before the
	// list-panes call lands).
	stub := &staleClientStub{
		showOut: state.SkeletonMarkerPrefix + "stale__0.0 \"1\"\n",
		listErr: sentinel,
	}
	c := bootstrapadapter.NewStaleMarkerCleaner(stub, nil) // *state.Logger is nil-safe.

	err := c.CleanStaleMarkers()
	if err == nil {
		t.Fatal("CleanStaleMarkers returned nil; want wrapped error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("CleanStaleMarkers err = %v; want errors.Is(err, sentinel)=true", err)
	}
	if len(stub.unsetCalls) != 0 {
		t.Errorf("unset calls = %v; want zero (mass-unset hazard guard)", stub.unsetCalls)
	}
	wantFormat := "#{session_name}:#{window_index}.#{pane_index}"
	if stub.listFormat != wantFormat {
		t.Errorf("ListAllPanesWithFormat format = %q; want %q", stub.listFormat, wantFormat)
	}
}

// TestStaleMarkerCleaner_PropagatesListSkeletonMarkersError proves that a
// ShowAllServerOptions failure surfaces from CleanStaleMarkers as a non-nil
// error so the orchestrator's step-7 Warn-and-swallow path logs it uniformly.
// Mirrors TestFIFOSweeper_PropagatesListSkeletonMarkersError.
func TestStaleMarkerCleaner_PropagatesListSkeletonMarkersError(t *testing.T) {
	sentinel := errors.New("show-options boom")
	stub := &staleClientStub{showErr: sentinel}
	c := bootstrapadapter.NewStaleMarkerCleaner(stub, nil)

	err := c.CleanStaleMarkers()
	if err == nil {
		t.Fatal("CleanStaleMarkers returned nil; want wrapped error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("CleanStaleMarkers err = %v; want errors.Is(err, sentinel)=true", err)
	}
	if len(stub.unsetCalls) != 0 {
		t.Errorf("unset calls = %v; want zero (marker enumeration failed)", stub.unsetCalls)
	}
}

// TestStaleMarkerCleaner_LiveTmuxStaleVsLive is the production-shape smoke
// test: against a live tmux server, set one skeleton marker for a paneKey
// that is NOT represented by a live pane, and one for a paneKey that IS,
// then invoke CleanStaleMarkers. The stale marker MUST be unset; the live
// marker MUST be preserved. This is the regression guard for the spec
// invariant that the adapter wires through to the canonical-paneKey diff
// against the canonical literal format string.
func TestStaleMarkerCleaner_LiveTmuxStaleVsLive(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-bsa-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Create a real session so list-panes -a returns at least one row, both
	// guarding against the zero-panes hazard guard and giving us a known
	// "live" paneKey for the preserved-marker assertion.
	if err := client.NewSession("live-sess", t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	livePaneKey := state.SanitizePaneKey("live-sess", 0, 0)
	stalePaneKey := state.SanitizePaneKey("ghost-sess", 0, 0)

	// Seed both markers via state.SetSkeletonMarker so the option-name
	// composition matches the canonical SkeletonMarkerPrefix layout.
	if err := state.SetSkeletonMarker(client, livePaneKey); err != nil {
		t.Fatalf("SetSkeletonMarker(live): %v", err)
	}
	if err := state.SetSkeletonMarker(client, stalePaneKey); err != nil {
		t.Fatalf("SetSkeletonMarker(stale): %v", err)
	}

	c := bootstrapadapter.NewStaleMarkerCleaner(client, nil)
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers: %v", err)
	}

	// Stale marker MUST be absent.
	_, found, err := client.TryGetServerOption(state.SkeletonMarkerPrefix + stalePaneKey)
	if err != nil {
		t.Fatalf("TryGetServerOption(stale): %v", err)
	}
	if found {
		t.Errorf("stale marker still set after CleanStaleMarkers; want absent")
	}

	// Live marker MUST still be present.
	_, found, err = client.TryGetServerOption(state.SkeletonMarkerPrefix + livePaneKey)
	if err != nil {
		t.Fatalf("TryGetServerOption(live): %v", err)
	}
	if !found {
		t.Errorf("live marker unexpectedly unset by CleanStaleMarkers; want preserved")
	}
}

// TestRestoringMarker_SetClearsTogglesServerOption proves that Set writes
// @portal-restoring="1" and Clear removes it, both observable on the live
// tmux server. The literal name comes from state.RestoringMarkerName so the
// adapter cannot drift from the canonical constant.
func TestRestoringMarker_SetClearsTogglesServerOption(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-bsa-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	m := &bootstrapadapter.RestoringMarker{Client: client}

	// Pre-condition: marker MUST be absent.
	if _, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption pre-Set: %v", err)
	} else if found {
		t.Fatal("@portal-restoring unexpectedly set before Set()")
	}

	// Set: marker MUST be "1".
	if err := m.Set(); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, found, err := client.TryGetServerOption(state.RestoringMarkerName)
	if err != nil {
		t.Fatalf("TryGetServerOption post-Set: %v", err)
	}
	if !found || val != "1" {
		t.Errorf("post-Set: found=%v value=%q; want found=true value=%q", found, val, "1")
	}

	// Clear: marker MUST be absent again.
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption post-Clear: %v", err)
	} else if found {
		t.Error("@portal-restoring still present after Clear()")
	}

	// Clear is idempotent: a second invocation MUST NOT error.
	if err := m.Clear(); err != nil {
		t.Errorf("Clear (second invocation): %v", err)
	}
}

// TestHookRegistrar_RegistersPortalHooks proves that RegisterPortalHooks
// runs without error against a live tmux server. The hook-table contents
// are exercised in detail by internal/tmux/hooks_register_test.go and at
// the orchestrator level by phase5_integration_test.go; this test only
// confirms the adapter shape.
func TestHookRegistrar_RegistersPortalHooks(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-bsa-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	r := &bootstrapadapter.HookRegistrar{Client: client}
	if err := r.RegisterPortalHooks(); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// Idempotent: a second invocation MUST NOT error or duplicate entries.
	if err := r.RegisterPortalHooks(); err != nil {
		t.Errorf("RegisterPortalHooks (second invocation): %v", err)
	}
}
