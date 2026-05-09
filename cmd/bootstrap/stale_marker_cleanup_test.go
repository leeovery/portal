package bootstrap

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// fakeMarkerLister is a lightweight in-memory MarkerLister for unit tests.
type fakeMarkerLister struct {
	markers map[string]struct{}
	err     error
	calls   int
}

func (f *fakeMarkerLister) ListSkeletonMarkers() (map[string]struct{}, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]struct{}, len(f.markers))
	for k := range f.markers {
		out[k] = struct{}{}
	}
	return out, nil
}

// fakeLivePaneLister is a lightweight in-memory LivePaneLister for unit tests.
// It records the format string requested so tests can assert the canonical
// `#{session_name}:#{window_index}.#{pane_index}` literal is used verbatim.
type fakeLivePaneLister struct {
	output      string
	err         error
	gotFormat   string
	formatCalls int
}

func (f *fakeLivePaneLister) ListAllPanesWithFormat(format string) (string, error) {
	f.formatCalls++
	f.gotFormat = format
	if f.err != nil {
		return "", f.err
	}
	return f.output, nil
}

// fakeMarkerUnsetter records every UnsetServerOption call in invocation order
// so tests can assert which option names were unset and how many times.
type fakeMarkerUnsetter struct {
	calls []string
	err   error
}

func (f *fakeMarkerUnsetter) UnsetServerOption(name string) error {
	f.calls = append(f.calls, name)
	return f.err
}

func TestCleanStaleMarkers_unsetsMarkerWhosePaneKeyIsNotInLiveSet(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"stale__0.0": {},
		"live__0.0":  {},
	}}
	live := &fakeLivePaneLister{output: "live:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if len(unsetter.calls) != 1 {
		t.Fatalf("expected exactly 1 unset call, got %d (%v)", len(unsetter.calls), unsetter.calls)
	}
	want := state.SkeletonMarkerPrefix + "stale__0.0"
	if unsetter.calls[0] != want {
		t.Errorf("unset name = %q, want %q", unsetter.calls[0], want)
	}
}

func TestCleanStaleMarkers_leavesLiveMarkerAlone(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"live__0.0": {},
	}}
	live := &fakeLivePaneLister{output: "live:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if len(unsetter.calls) != 0 {
		t.Errorf("expected zero unset calls, got %d (%v)", len(unsetter.calls), unsetter.calls)
	}
}

func TestCleanStaleMarkers_requestsLivePanesWithCanonicalFormat(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{}}
	live := &fakeLivePaneLister{output: "live:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	wantFormat := "#{session_name}:#{window_index}.#{pane_index}"
	if live.gotFormat != wantFormat {
		t.Errorf("ListAllPanesWithFormat format = %q, want %q", live.gotFormat, wantFormat)
	}
}

func TestCleanStaleMarkers_composesOptionNameFromSkeletonMarkerPrefix(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"bar__1.2": {},
	}}
	live := &fakeLivePaneLister{output: "foo:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if len(unsetter.calls) != 1 {
		t.Fatalf("expected 1 unset call, got %v", unsetter.calls)
	}
	want := state.SkeletonMarkerPrefix + "bar__1.2"
	if unsetter.calls[0] != want {
		t.Errorf("unset name = %q, want %q (must be SkeletonMarkerPrefix + paneKey)", unsetter.calls[0], want)
	}
}

func TestCleanStaleMarkers_emptyMarkerSet(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{}}
	live := &fakeLivePaneLister{output: "foo:0.0\nbar:1.2\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if len(unsetter.calls) != 0 {
		t.Errorf("expected zero unset calls for empty marker set, got %v", unsetter.calls)
	}
}

func TestCleanStaleMarkers_fullOverlapNoUnsetCalls(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"foo__0.0": {},
		"bar__1.2": {},
	}}
	live := &fakeLivePaneLister{output: "foo:0.0\nbar:1.2\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if len(unsetter.calls) != 0 {
		t.Errorf("expected zero unset calls when all markers are live, got %v", unsetter.calls)
	}
}

// TestStaleMarkerCleanup_PaneKeyNormalisation pins the contract that the
// cleanup compares marker paneKeys (canonical `session__win.pane`) against
// live-pane paneKeys (tmux's raw `session:win.pane`) by normalising the live
// side via state.SanitizePaneKey BEFORE diffing. A regression that drops the
// conversion, applies it to the wrong side, or replaces the diff with naive
// string equality would re-introduce the mass-unset hazard from a different
// angle. See spec §Fix Component B (Adapter Wiring → PaneKey conversion,
// Parse contract) and §Testing Requirements (PaneKey normalisation
// correctness).
func TestStaleMarkerCleanup_PaneKeyNormalisation(t *testing.T) {
	t.Run("it recognises a marker in canonical form against a live pane in tmux session:win.pane form", func(t *testing.T) {
		// Marker side seeded canonical (`session__win.pane`); live side
		// supplies tmux's raw `session:win.pane`. After cleanup the marker
		// must NOT be unset — the cleanup must sanitise the live side via
		// state.SanitizePaneKey before diffing so the two representations
		// of the same logical pane match.
		canonical := state.SanitizePaneKey("my-session", 0, 1) // "my-session__0.1"
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			canonical: {},
		}}
		live := &fakeLivePaneLister{output: "my-session:0.1\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}

		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls (canonical marker should match live pane after sanitisation), got %v", unsetter.calls)
		}
	})

	t.Run("it does not treat raw session:win.pane and canonical session__win.pane as equivalent", func(t *testing.T) {
		// Marker side seeded with the RAW unsanitised form `session:win.pane`
		// — a buggy producer might persist this. Live side supplies the same
		// raw form; cleanup sanitises live to `session__win.pane`. The marker
		// raw form is NOT in the canonical live set, so the cleanup unsets it.
		// This proves the diff is NOT a naive string-equality shortcut: if it
		// were, the raw-vs-raw match would falsely preserve the marker.
		raw := "my-session:0.1"
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			raw: {},
		}}
		live := &fakeLivePaneLister{output: "my-session:0.1\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}

		if len(unsetter.calls) != 1 {
			t.Fatalf("expected exactly 1 unset call (raw marker form must NOT match canonical live set), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
		want := state.SkeletonMarkerPrefix + raw
		if unsetter.calls[0] != want {
			t.Errorf("unset name = %q, want %q", unsetter.calls[0], want)
		}
	})

	t.Run("it splits on the rightmost colon to recover session names containing colons", func(t *testing.T) {
		// Session name literally contains ':' (e.g. `host:1234`). Marker side
		// holds canonical `host:1234__0.0`; live side supplies tmux's raw
		// `host:1234:0.0`. The cleanup MUST split on the rightmost ':' to
		// recover (session=`host:1234`, window=0, pane=0); a leftmost-':'
		// split would corrupt the session name and produce a non-matching
		// canonical key, falsely unsetting the marker.
		canonical := state.SanitizePaneKey("host:1234", 0, 0) // "host:1234__0.0"
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			canonical: {},
		}}
		live := &fakeLivePaneLister{output: "host:1234:0.0\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}

		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls (rightmost-colon split must recover session name with colon), got %v", unsetter.calls)
		}
	})
}

func TestCleanStaleMarkers_noOverlapUnsetsEveryMarker(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"stale1__0.0": {},
		"stale2__1.2": {},
	}}
	// Live set is non-empty (mass-unset hazard guard does not trip).
	live := &fakeLivePaneLister{output: "alive:9.9\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &StaleMarkerCleaner{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if len(unsetter.calls) != 2 {
		t.Fatalf("expected 2 unset calls, got %d (%v)", len(unsetter.calls), unsetter.calls)
	}
	gotSet := map[string]struct{}{
		unsetter.calls[0]: {},
		unsetter.calls[1]: {},
	}
	wantSet := map[string]struct{}{
		state.SkeletonMarkerPrefix + "stale1__0.0": {},
		state.SkeletonMarkerPrefix + "stale2__1.2": {},
	}
	for k := range wantSet {
		if _, ok := gotSet[k]; !ok {
			t.Errorf("missing expected unset for %q; got %v", k, unsetter.calls)
		}
	}
}

// TestStaleMarkerCleanup_MassUnsetHazardGuard pins the spec invariant that
// the cleanup must NEVER fall through to "live set is empty, therefore
// unset every marker". Two failure modes trigger the guard:
//
//  1. ListAllPanesWithFormat returns a non-nil error (tmux unavailable,
//     transient socket error). The error is propagated so the orchestrator
//     wires it as a soft warning per task 2-5; zero unset calls.
//  2. ListAllPanesWithFormat returns no error but zero parsed live panes
//     (whitespace-only output, all-malformed lines, or genuinely empty)
//     while at least one marker exists. A non-nil error is returned (no
//     unset calls) so the orchestrator surfaces a soft warning rather than
//     destabilising a still-live tmux server by mass-unsetting markers
//     that may protect legitimate hydrate-in-progress panes.
//
// The third sub-case — zero live panes AND zero markers — is a clean no-op
// (nil return, zero unset calls): no marker means no hazard.
//
// See spec §Fix Component B (Mass-unset hazard guard, Soft-Warning Posture).
func TestStaleMarkerCleanup_MassUnsetHazardGuard(t *testing.T) {
	t.Run("it skips unset and emits a warning when ListAllPanesWithFormat returns an error", func(t *testing.T) {
		sentinel := errors.New("tmux: connection refused")
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"protected__0.0": {},
		}}
		live := &fakeLivePaneLister{err: sentinel}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error from CleanStaleMarkers; got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("expected returned error to wrap sentinel %v, got %v", sentinel, err)
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls when ListAllPanesWithFormat fails, got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})

	t.Run("it skips unset and emits a warning when zero live panes are returned with markers present", func(t *testing.T) {
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"protected__0.0": {},
			"another__1.2":   {},
		}}
		// No error, but zero panes parsed.
		live := &fakeLivePaneLister{output: ""}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error when zero live panes returned with markers present; got nil")
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls under zero-panes guard, got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})

	t.Run("it is a clean no-op when zero live panes are returned with zero markers", func(t *testing.T) {
		lister := &fakeMarkerLister{markers: map[string]struct{}{}}
		live := &fakeLivePaneLister{output: ""}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("expected nil error for zero markers + zero live panes; got %v", err)
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls for empty markers + empty live, got %v", unsetter.calls)
		}
	})

	t.Run("the zero-panes guard runs before any unset", func(t *testing.T) {
		// Multiple markers — were the guard absent, every one would be
		// computed as stale and unset. Guard must short-circuit BEFORE any
		// UnsetServerOption call lands.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"a__0.0": {},
			"b__0.1": {},
			"c__1.0": {},
		}}
		live := &fakeLivePaneLister{output: "   \n  \n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error when whitespace-only output yields zero live panes with markers present; got nil")
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls (guard must run before any unset), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})

	t.Run("it never mass-unsets when ListAllPanesWithFormat fails", func(t *testing.T) {
		// Many markers; ListAllPanesWithFormat fails. The guard must
		// prevent EVERY marker from being unset on a tmux failure.
		sentinel := errors.New("tmux gone")
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"m1__0.0": {},
			"m2__0.1": {},
			"m3__1.0": {},
			"m4__1.1": {},
			"m5__2.0": {},
		}}
		live := &fakeLivePaneLister{err: sentinel}
		unsetter := &fakeMarkerUnsetter{}

		c := &StaleMarkerCleaner{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error from ListAllPanesWithFormat failure; got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("expected returned error to wrap sentinel %v, got %v", sentinel, err)
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls when ListAllPanesWithFormat fails (mass-unset hazard), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})
}
