package tmux_test

// Real-tmux regression guards for the per-event hook convergence fix.
//
// The defect these guard against is a tmux-output-SHAPE issue: tmux 3.6b's
// no-arg `show-hooks -g` does not enumerate an entire class of events
// (pane-* and the geometry/rename window-* events such as
// window-layout-changed), even though those hooks ARE set and DO fire. A
// string-fixture / mock commander returns whatever output the test author
// wrote, so it cannot expose this blind spot — only a real tmux server is a
// faithful oracle. These tests therefore drive the internal/tmuxtest socket
// fixtures against a real tmux and carry NO build tag; they are gated only by
// SkipIfNoTmux(t) at runtime so environments without tmux skip cleanly.
//
// Spec: .workflows/state-notify-cascade-on-binary-upgrade/specification/
// state-notify-cascade-on-binary-upgrade/specification.md §§ Testing
// Requirements (1, 2), Acceptance Criteria (1, 8).

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// managedEventFingerprint pairs a Portal-managed tmux event with the Portal
// content fingerprint substring used to identify a Portal-authored entry on
// that event, mirroring the unexported managedEvents table in
// hooks_register.go. The fingerprints are the exact substrings the production
// convergence engine matches on (notifySubstring / commitNowSubstring /
// signalHydrateMarker), kept in sync here because they live below the
// external-test import boundary as unexported constants.
type managedEventFingerprint struct {
	event       string
	fingerprint string
}

// notifyFingerprint is the per-event content fingerprint for the six
// `portal state notify` save-trigger events (matches notifySubstring).
const notifyFingerprint = "portal state notify"

// commitNowFingerprint is the per-event content fingerprint for the
// session-closed commit-now event (matches commitNowSubstring).
const commitNowFingerprint = "portal state commit-now"

// signalHydrateFingerprint is the per-event content fingerprint for the two
// hydration-trigger events (matches signalHydrateMarker).
const signalHydrateFingerprint = "portal state signal-hydrate"

// notifyCommandBody is the full wrapped body the six notify save-trigger
// events converge to (mirrors the unexported notifyCommand constant). Used to
// cross-check the desired body on the two blind events.
const notifyCommandBody = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

// managedEventFingerprints enumerates every Portal-managed event with the
// fingerprint that identifies a Portal-authored entry on it. This mirrors the
// nine-entry managedEvents table: six notify save-trigger events,
// session-closed (commit-now), and the two hydration events.
var managedEventFingerprints = []managedEventFingerprint{
	{event: "session-created", fingerprint: notifyFingerprint},
	{event: "session-closed", fingerprint: commitNowFingerprint},
	{event: "session-renamed", fingerprint: notifyFingerprint},
	{event: "window-linked", fingerprint: notifyFingerprint},
	{event: "window-unlinked", fingerprint: notifyFingerprint},
	{event: "window-layout-changed", fingerprint: notifyFingerprint},
	{event: "pane-focus-out", fingerprint: notifyFingerprint},
	{event: "client-attached", fingerprint: signalHydrateFingerprint},
	{event: "client-session-changed", fingerprint: signalHydrateFingerprint},
}

// countPortalEntriesForEvent reads event's hook array PER-EVENT (via the
// ShowGlobalHooksForEvent seam, never the no-arg global form) and returns the
// number of entries whose command body contains fingerprint. Reading
// per-event is load-bearing: a no-arg global read would itself be blind to
// pane-focus-out / window-layout-changed, so the count assertion would be
// vacuously satisfied. The per-event read is the only oracle that is not
// itself blind.
func countPortalEntriesForEvent(t *testing.T, client *tmux.Client, event, fingerprint string) int {
	t.Helper()
	raw, err := client.ShowGlobalHooksForEvent(event)
	if err != nil {
		t.Fatalf("ShowGlobalHooksForEvent(%s): %v", event, err)
	}
	parsed := tmux.ParseShowHooks(raw)
	count := 0
	for _, e := range parsed[event] {
		if strings.Contains(e.Command, fingerprint) {
			count++
		}
	}
	return count
}

// TestRegisterPortalHooks_NoGrowthAcrossBootstraps is the direct regression
// guard for the cascade bug (Acceptance Criteria 1 and 8). It runs
// RegisterPortalHooks N times (N=3, N>=2 required) against a real tmux server
// and asserts that every managed event's hook array stays at EXACTLY ONE
// Portal entry after every run — naming pane-focus-out and
// window-layout-changed explicitly, since those are the two events a no-arg
// global read cannot see and the events that grew unbounded pre-fix.
//
// Against a pre-fix binary the two blind events would accumulate one extra
// copy per run (the no-arg global idempotency check concludes "absent"); this
// test fails there and passes after the per-event fix.
//
// The per-event count assertion (count == 1 across N>=2 bootstraps) is the
// structural guard for the cascade outcome: a single tmux event firing a
// managed hook spawns exactly one `portal state notify`, not N. No separate
// process-count test is required (Acceptance Criterion 8).
func TestRegisterPortalHooks_NoGrowthAcrossBootstraps(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hooks-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const runs = 3 // N >= 2 required; 3 gives margin to expose linear growth.
	for run := 1; run <= runs; run++ {
		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("run %d: RegisterPortalHooks: %v", run, err)
		}

		// Every managed event must hold exactly one Portal entry after each
		// run. Reading per-event keeps the assertion oracle un-blind.
		for _, me := range managedEventFingerprints {
			got := countPortalEntriesForEvent(t, client, me.event, me.fingerprint)
			if got != 1 {
				t.Errorf("run %d: event %q: Portal entry count = %d, want 1", run, me.event, got)
			}
		}

		// Explicit, by-name regression target: the two blind events that grew
		// unbounded pre-fix must each stay at exactly one entry on every run.
		if got := countPortalEntriesForEvent(t, client, "pane-focus-out", notifyFingerprint); got != 1 {
			t.Errorf("run %d: pane-focus-out (blind event): Portal entry count = %d, want 1 (no growth)", run, got)
		}
		if got := countPortalEntriesForEvent(t, client, "window-layout-changed", notifyFingerprint); got != 1 {
			t.Errorf("run %d: window-layout-changed (blind event): Portal entry count = %d, want 1 (no growth)", run, got)
		}
	}

	// Cross-check: the single surviving entry on each blind event carries the
	// expected desired body byte-for-byte (modulo ParseShowHooks' outer-quote
	// stripping), confirming convergence landed the correct command, not just
	// some Portal-fingerprinted body.
	for _, ev := range []string{"pane-focus-out", "window-layout-changed"} {
		raw, err := client.ShowGlobalHooksForEvent(ev)
		if err != nil {
			t.Fatalf("ShowGlobalHooksForEvent(%s): %v", ev, err)
		}
		entries := tmux.ParseShowHooks(raw)[ev]
		if len(entries) != 1 {
			t.Fatalf("event %q: entry count = %d, want exactly 1; entries=%v", ev, len(entries), entries)
		}
		if entries[0].Command != notifyCommandBody {
			t.Errorf("event %q: desired body = %q, want %q", ev, entries[0].Command, notifyCommandBody)
		}
	}
}

// TestShowHooksGlobalEnumeration_OmitsPaneAndGeometryEvents locks the tmux
// 3.6b reality the per-event fix is built on (Testing Requirement 2 /
// Acceptance Criterion 1's premise). It appends a Portal-shape hook directly
// onto a pane-scoped event (pane-focus-out), a geometry window event
// (window-layout-changed), and a control event known to be enumerated
// globally (session-created), then asserts:
//
//   - the no-arg `show-hooks -g` enumeration INCLUDES session-created but
//     OMITS pane-focus-out and window-layout-changed (the documented blind
//     spot — confirmed live on tmux 3.6b during authoring), and
//   - the per-event `show-hooks -g <event>` seam RETURNS each omitted event's
//     entry, proving the seam the production code now reads through is not
//     blind.
//
// A failure here means the tmux blind-spot assumption changed (e.g. a future
// tmux version that enumerates pane/geometry events globally) — NOT
// necessarily a Portal bug. The per-event fix remains correct regardless of
// whether the global form is blind; this test documents WHY the per-event
// seam is mandatory and catches a silent tmux behaviour change.
func TestShowHooksGlobalEnumeration_OmitsPaneAndGeometryEvents(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hooks-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// A control event that the global enumeration DOES surface, plus the two
	// events the global enumeration is blind to.
	const enumeratedEvent = "session-created"
	blindEvents := []string{"pane-focus-out", "window-layout-changed"}

	// Append a Portal-shape body onto each event directly (not via
	// RegisterPortalHooks — this test is about tmux enumeration shape, not the
	// convergence engine).
	allEvents := append([]string{enumeratedEvent}, blindEvents...)
	for _, ev := range allEvents {
		if err := client.AppendGlobalHook(ev, notifyCommandBody); err != nil {
			t.Fatalf("AppendGlobalHook(%s): %v", ev, err)
		}
	}

	// Read the no-arg global table by driving raw tmux through the socket. The
	// no-arg show-hooks -g client method was deliberately deleted in an
	// earlier task, so the only way to observe the blind form is the raw
	// socket invocation.
	globalRaw := ts.Run(t, "show-hooks", "-g")
	globalParsed := tmux.ParseShowHooks(globalRaw)

	// The control event must be present in the global enumeration with its
	// Portal entry.
	if !hasPortalEntry(globalParsed[enumeratedEvent], notifyFingerprint) {
		t.Errorf("no-arg `show-hooks -g` omitted %q, but tmux 3.6b is expected to enumerate it; global entries=%v",
			enumeratedEvent, globalParsed[enumeratedEvent])
	}

	// The two blind events must be ABSENT from the global enumeration even
	// though their hooks are set and fire normally (the tmux 3.6b blind spot).
	for _, ev := range blindEvents {
		if len(globalParsed[ev]) != 0 {
			t.Errorf("no-arg `show-hooks -g` enumerated %q (entries=%v); tmux 3.6b is expected to OMIT it — "+
				"the blind-spot assumption may have changed (not necessarily a Portal bug)", ev, globalParsed[ev])
		}
	}

	// The per-event seam must NOT be blind: each omitted event returns its
	// entry when read by name. This is exactly the seam the production
	// convergence engine reads through.
	for _, ev := range blindEvents {
		raw, err := client.ShowGlobalHooksForEvent(ev)
		if err != nil {
			t.Fatalf("ShowGlobalHooksForEvent(%s): %v", ev, err)
		}
		parsed := tmux.ParseShowHooks(raw)
		if !hasPortalEntry(parsed[ev], notifyFingerprint) {
			t.Errorf("per-event `show-hooks -g %s` did not return the Portal entry; per-event entries=%v — "+
				"the per-event seam must never be blind", ev, parsed[ev])
		}
	}
}

// hasPortalEntry reports whether any entry's command body contains the Portal
// fingerprint substring.
func hasPortalEntry(entries []tmux.HookEntry, fingerprint string) bool {
	for _, e := range entries {
		if strings.Contains(e.Command, fingerprint) {
			return true
		}
	}
	return false
}
