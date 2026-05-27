package bootstrap

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeMarkerLister is a lightweight in-memory state.ServerOptionLister for
// unit tests. It synthesises the tmux `show-options -s` output that
// state.ListSkeletonMarkers parses from the seeded markers map, so test
// authors keep the ergonomic "give me a paneKey set" seeding shape while the
// production code path is exercised end-to-end through the real parser.
type fakeMarkerLister struct {
	markers map[string]struct{}
	err     error
	calls   int
}

func (f *fakeMarkerLister) ShowAllServerOptions() (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if len(f.markers) == 0 {
		return "", nil
	}
	var b strings.Builder
	for k := range f.markers {
		b.WriteString(state.SkeletonMarkerPrefix)
		b.WriteString(k)
		b.WriteString(" \"1\"\n")
	}
	return b.String(), nil
}

// fakeLivePaneLister is a lightweight in-memory LivePaneLister for unit tests.
// It records the format string requested so tests can assert the canonical
// tmux.StructuralKeyFormat constant is used verbatim.
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
//
// errs lets tests inject per-call errors keyed by the (1-based) invocation
// index. A missing key returns nil. errOn keys tests that want a specific
// option name to fail. err is the legacy single-error-for-every-call hook;
// when set, it overrides errs/errOn and returns on every call. Used to
// drive the per-unset-failure soft-warning posture tests.
type fakeMarkerUnsetter struct {
	calls []string
	err   error
	errs  map[int]error
	errOn map[string]error
}

func (f *fakeMarkerUnsetter) UnsetServerOption(name string) error {
	f.calls = append(f.calls, name)
	if f.err != nil {
		return f.err
	}
	if e, ok := f.errs[len(f.calls)]; ok {
		return e
	}
	if e, ok := f.errOn[name]; ok {
		return e
	}
	return nil
}

func TestCleanStaleMarkers_unsetsMarkerWhosePaneKeyIsNotInLiveSet(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"stale__0.0": {},
		"live__0.0":  {},
	}}
	live := &fakeLivePaneLister{output: "live:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &MarkerCleanupCore{
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

	c := &MarkerCleanupCore{
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

	c := &MarkerCleanupCore{
		Markers:  lister,
		Panes:    live,
		Unsetter: unsetter,
	}
	if err := c.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers returned error: %v", err)
	}

	if live.gotFormat != tmux.StructuralKeyFormat {
		t.Errorf("ListAllPanesWithFormat format = %q, want %q", live.gotFormat, tmux.StructuralKeyFormat)
	}
}

func TestCleanStaleMarkers_composesOptionNameFromSkeletonMarkerPrefix(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"bar__1.2": {},
	}}
	live := &fakeLivePaneLister{output: "foo:0.0\n"}
	unsetter := &fakeMarkerUnsetter{}

	c := &MarkerCleanupCore{
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

	c := &MarkerCleanupCore{
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

	c := &MarkerCleanupCore{
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

		c := &MarkerCleanupCore{
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

		c := &MarkerCleanupCore{
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

	t.Run("it recognises a canonical marker against a live pane whose session name contains a colon", func(t *testing.T) {
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

		c := &MarkerCleanupCore{
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

	c := &MarkerCleanupCore{
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

		c := &MarkerCleanupCore{
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

	t.Run("it returns nil and emits a Warn log entry when zero live panes are returned with markers present", func(t *testing.T) {
		// Spec change (task 3-6): the zero-panes-with-markers branch is a
		// successful soft outcome ("skip this run; next bootstrap retries"),
		// not a genuine failure. CleanStaleMarkers MUST return nil so the
		// orchestrator's error channel exclusively carries genuine failures;
		// the deferral signal moves to portal.log under ComponentBootstrap
		// via Logger.Warn — asserted in-memory via the package's
		// RecordingLogger fake.
		logger := &RecordingLogger{}
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"protected__0.0": {},
			"another__1.2":   {},
		}}
		// No error, but zero panes parsed.
		live := &fakeLivePaneLister{output: ""}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
			Logger:   logger,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers must return nil for zero-panes-with-markers deferral; got %v", err)
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls under zero-panes guard, got %d (%v)", len(unsetter.calls), unsetter.calls)
		}

		// Locate the deferral Warn entry in-memory. Component routing is
		// enforced at every Logger.Warn call site by supplying
		// state.ComponentBootstrap; the recording fake captures the
		// component alongside the formatted message body so we pin
		// both: the deferral signature (message body) and the
		// bootstrap routing (component).
		foundDeferral := false
		for i, msg := range logger.warnings {
			if strings.Contains(msg, "stale-marker cleanup") && strings.Contains(msg, "2 marker(s)") {
				if logger.warnComponents[i] != state.ComponentBootstrap {
					t.Errorf("deferral Warn component = %q, want %q", logger.warnComponents[i], state.ComponentBootstrap)
				}
				foundDeferral = true
				break
			}
		}
		if !foundDeferral {
			t.Errorf("expected a Warn entry identifying the stale-marker cleanup deferral with marker count \"2 marker(s)\"; got warnings=%v", logger.warnings)
		}
	})

	t.Run("it is a clean no-op when zero live panes are returned with zero markers", func(t *testing.T) {
		lister := &fakeMarkerLister{markers: map[string]struct{}{}}
		live := &fakeLivePaneLister{output: ""}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
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
		// UnsetServerOption call lands. Per task 3-6 the deferral surfaces
		// via Logger.Warn rather than a sentinel error.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"a__0.0": {},
			"b__0.1": {},
			"c__1.0": {},
		}}
		live := &fakeLivePaneLister{output: "   \n  \n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers must return nil for whitespace-only zero-panes deferral; got %v", err)
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

		c := &MarkerCleanupCore{
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

// TestStaleMarkerCleanup_SoftWarningPosture pins the spec invariant from
// §Fix Component B (Soft-Warning Posture): a single failed UnsetServerOption
// call must NEVER abort the cleanup loop, malformed live-pane lines must
// never propagate as a fatal, and the cleanup MUST NOT return *FatalError
// under any code path. Failures are recorded and aggregated; the orchestrator
// (task 2-5) wires the aggregate as a Warn.
func TestStaleMarkerCleanup_SoftWarningPosture(t *testing.T) {
	t.Run("it continues attempting unsets when one fails mid-loop", func(t *testing.T) {
		// Three stale markers; mock unset returns error on the second call.
		// All three calls must still be attempted; returned error is non-nil
		// and wraps the failing sentinel.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"a__0.0": {},
			"b__0.0": {},
			"c__0.0": {},
		}}
		// Live set non-empty so the zero-panes guard does not trip; the
		// live pane is unrelated to any marker so all three are stale.
		live := &fakeLivePaneLister{output: "alive:9.9\n"}
		sentinel := errors.New("tmux: option boom")
		unsetter := &fakeMarkerUnsetter{
			errs: map[int]error{2: sentinel},
		}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error when one unset fails mid-loop; got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("expected returned error to wrap sentinel %v, got %v", sentinel, err)
		}
		if len(unsetter.calls) != 3 {
			t.Errorf("expected all 3 unset calls attempted despite mid-loop failure, got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})

	t.Run("it attempts every unset when all fail", func(t *testing.T) {
		// Edge case: every unset fails. The cleanup must still attempt
		// each one; the returned error is non-nil; no fatal escalation.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"a__0.0": {},
			"b__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "alive:9.9\n"}
		sentinel := errors.New("every unset boom")
		unsetter := &fakeMarkerUnsetter{err: sentinel}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error when every unset fails; got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("expected returned error to wrap sentinel %v, got %v", sentinel, err)
		}
		if len(unsetter.calls) != 2 {
			t.Errorf("expected both unset calls attempted, got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
		var fatal *FatalError
		if errors.As(err, &fatal) {
			t.Errorf("expected non-fatal error; got *FatalError = %v", fatal)
		}
	})

	t.Run("it skips malformed live-pane lines without aborting cleanup", func(t *testing.T) {
		// Mix of well-formed and malformed lines. The well-formed lines
		// land in the live-pane set; the malformed line is skipped and
		// must NOT abort cleanup.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			// Both well-formed live entries are also marked → they are
			// alive and not unset.
			state.SanitizePaneKey("good", 0, 0):  {},
			state.SanitizePaneKey("good2", 1, 0): {},
			// This marker is stale: not present in the live set. Cleanup
			// MUST unset it after skipping the malformed line, proving
			// processing continued past the malformed line.
			"stale__9.9": {},
		}}
		live := &fakeLivePaneLister{output: "good:0.0\nmalformed-no-colon\ngood2:1.0\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}
		if len(unsetter.calls) != 1 {
			t.Fatalf("expected exactly 1 unset call (stale marker), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
		want := state.SkeletonMarkerPrefix + "stale__9.9"
		if unsetter.calls[0] != want {
			t.Errorf("unset name = %q, want %q", unsetter.calls[0], want)
		}
	})

	t.Run("it skips a line whose window index is not an integer", func(t *testing.T) {
		// `good:abc.0` — non-integer window index. The line is skipped;
		// the marker `good__0.0` is therefore stale and unset. This
		// proves the malformed line did NOT enter the live set.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"good__0.0": {},
		}}
		// Provide one well-formed unrelated live entry so the zero-panes
		// guard does not trip.
		live := &fakeLivePaneLister{output: "good:abc.0\nalive:9.9\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}
		if len(unsetter.calls) != 1 {
			t.Fatalf("expected 1 unset call (malformed window must NOT enter live set), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
		want := state.SkeletonMarkerPrefix + "good__0.0"
		if unsetter.calls[0] != want {
			t.Errorf("unset name = %q, want %q", unsetter.calls[0], want)
		}
	})

	t.Run("it skips a line whose pane index is not an integer", func(t *testing.T) {
		// `good:0.xyz` — non-integer pane index. Line skipped; marker is stale.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"good__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "good:0.xyz\nalive:9.9\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}
		if len(unsetter.calls) != 1 {
			t.Fatalf("expected 1 unset call (malformed pane must NOT enter live set), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
		want := state.SkeletonMarkerPrefix + "good__0.0"
		if unsetter.calls[0] != want {
			t.Errorf("unset name = %q, want %q", unsetter.calls[0], want)
		}
	})

	t.Run("it skips a line missing the dot separator", func(t *testing.T) {
		// `good:01` — the rest of the line after `:` has no `.` separator.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"good__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "good:01\nalive:9.9\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}
		if len(unsetter.calls) != 1 {
			t.Fatalf("expected 1 unset call (missing-dot line must NOT enter live set), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})

	t.Run("it skips a line missing the colon separator", func(t *testing.T) {
		// `goodonly` — no `:` at all.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"good__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "goodonly\nalive:9.9\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers returned error: %v", err)
		}
		if len(unsetter.calls) != 1 {
			t.Fatalf("expected 1 unset call (missing-colon line must NOT enter live set), got %d (%v)", len(unsetter.calls), unsetter.calls)
		}
	})

	t.Run("the cleanup never returns a fatal error", func(t *testing.T) {
		// Combined per-unset failure AND malformed-line conditions: the
		// returned error MUST NOT be wrappable to *FatalError. The
		// orchestrator (task 2-5) Warn-and-swallows a non-nil return; a
		// fatal escalation here would abort bootstrap.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			// All three are stale; the live entries are unrelated.
			"a__0.0": {},
			"b__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "malformed-no-colon\nalive:9.9\n"}
		sentinel := errors.New("unset boom")
		unsetter := &fakeMarkerUnsetter{err: sentinel}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
		}
		err := c.CleanStaleMarkers()
		if err == nil {
			t.Fatalf("expected non-nil error from per-unset failures; got nil")
		}
		var fatal *FatalError
		if errors.As(err, &fatal) {
			t.Errorf("CleanStaleMarkers returned *FatalError = %v; soft-warning posture forbids fatal escalation", fatal)
		}
	})

	t.Run("zero-panes guard fires when all lines are malformed and markers exist", func(t *testing.T) {
		// All live-pane lines malformed → live set parses to empty. With
		// markers present, the zero-panes guard MUST trigger so no
		// mass-unset lands. Per task 3-6 the deferral surfaces via
		// Logger.Warn (component=bootstrap) and CleanStaleMarkers returns
		// nil — the genuine-failure error channel must not carry the
		// soft-deferral signal.
		logger := &RecordingLogger{}
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"a__0.0": {},
			"b__0.0": {},
			"c__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "malformed1\nmalformed2:nope\nmalformed3:0.zzz\n"}
		unsetter := &fakeMarkerUnsetter{}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
			Logger:   logger,
		}
		if err := c.CleanStaleMarkers(); err != nil {
			t.Fatalf("CleanStaleMarkers must return nil under all-malformed + markers-exist deferral; got %v", err)
		}
		if len(unsetter.calls) != 0 {
			t.Errorf("expected zero unset calls under all-malformed + zero-panes guard, got %d (%v)", len(unsetter.calls), unsetter.calls)
		}

		// At least one Warn entry for the deferral itself; mentions the
		// stale-marker cleanup deferral signature. (Per-line malformed
		// Warns may also appear — the deferral Warn is what we're
		// pinning here.) The component is supplied at every Logger.Warn
		// call site as state.ComponentBootstrap; the recording fake
		// captures the component alongside the formatted message body
		// so we pin bootstrap routing here too.
		foundDeferral := false
		for i, msg := range logger.warnings {
			if strings.Contains(msg, "stale-marker cleanup") && strings.Contains(msg, "marker(s)") {
				if logger.warnComponents[i] != state.ComponentBootstrap {
					t.Errorf("deferral Warn component = %q, want %q", logger.warnComponents[i], state.ComponentBootstrap)
				}
				foundDeferral = true
				break
			}
		}
		if !foundDeferral {
			t.Errorf("expected a Warn entry identifying the all-malformed + zero-panes deferral; got warnings=%v", logger.warnings)
		}
	})

	t.Run("logger is nil-safe under per-unset failure and malformed lines", func(t *testing.T) {
		// Logger field nil: cleanup MUST NOT panic on either malformed
		// lines or per-unset failures. Mirrors bootstrapadapter.FIFOSweeper
		// nil-safety convention.
		lister := &fakeMarkerLister{markers: map[string]struct{}{
			"a__0.0": {},
		}}
		live := &fakeLivePaneLister{output: "malformed\nalive:9.9\n"}
		unsetter := &fakeMarkerUnsetter{err: errors.New("boom")}

		c := &MarkerCleanupCore{
			Markers:  lister,
			Panes:    live,
			Unsetter: unsetter,
			Logger:   nil,
		}
		// Should return non-nil (per-unset failure) but never panic.
		if err := c.CleanStaleMarkers(); err == nil {
			t.Fatalf("expected non-nil error; got nil")
		}
	})
}

// TestStaleMarkerCleanup_GenuineFailurePropagation pins task 3-6's
// reclassification: after the zero-panes-with-markers branch returns nil
// + Warn, the error channel of CleanStaleMarkers exclusively carries
// genuine failures. A non-nil error from a dependency MUST still
// propagate so the orchestrator's existing soft-warn-and-swallow posture
// surfaces it in portal.log.
func TestStaleMarkerCleanup_GenuineFailurePropagation(t *testing.T) {
	listSentinel := errors.New("list-panes: socket gone")
	markersSentinel := errors.New("show-options: tmux dead")

	cases := []struct {
		name     string
		markers  *fakeMarkerLister
		panes    *fakeLivePaneLister
		wantWrap error
	}{
		{
			name:     "ListAllPanesWithFormat error propagates with no unset calls",
			markers:  &fakeMarkerLister{markers: map[string]struct{}{"m__0.0": {}}},
			panes:    &fakeLivePaneLister{err: listSentinel},
			wantWrap: listSentinel,
		},
		{
			name:     "ListSkeletonMarkers error propagates with no unset calls",
			markers:  &fakeMarkerLister{err: markersSentinel},
			panes:    &fakeLivePaneLister{output: "live:0.0\n"},
			wantWrap: markersSentinel,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unsetter := &fakeMarkerUnsetter{}
			c := &MarkerCleanupCore{
				Markers:  tc.markers,
				Panes:    tc.panes,
				Unsetter: unsetter,
			}
			err := c.CleanStaleMarkers()
			if err == nil {
				t.Fatalf("expected non-nil error to propagate genuine failure; got nil")
			}
			if !errors.Is(err, tc.wantWrap) {
				t.Errorf("expected returned error to wrap %v, got %v", tc.wantWrap, err)
			}
			if len(unsetter.calls) != 0 {
				t.Errorf("expected zero unset calls when a dependency fails, got %d (%v)", len(unsetter.calls), unsetter.calls)
			}
		})
	}
}
