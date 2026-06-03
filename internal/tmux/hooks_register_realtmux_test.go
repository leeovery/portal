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

// The per-event content fingerprints (notifyFingerprint, commitNowFingerprint,
// signalHydrateFingerprint) and the full wrapped bodies they compose
// (expectedNotifyCommand etc.) are declared once in hooks_register_test.go, the
// single test-package home for the desired-body literals. They mirror the
// unexported notifySubstring / commitNowSubstring / signalHydrateMarker and
// notifyCommand constants and are referenced here to identify Portal-authored
// entries and cross-check the desired body on the two blind events.

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

// portalEntryCommandsForEvent reads event's hook array PER-EVENT (via the
// ShowGlobalHooksForEvent seam, never the no-arg global form) and returns the
// command bodies of every entry whose body contains fingerprint, in the order
// ParseShowHooks yields them. It is the single tmux_test-package primitive for
// "read one event, parse, select entries matching a fingerprint" — both the
// count-based callers (via len) and the body-inspecting callers (which need
// the surviving command text) route through it.
//
// Reading per-event is load-bearing: a no-arg global read would itself be
// blind to pane-focus-out / window-layout-changed, so a count/select assertion
// built on it would be vacuously satisfied. The per-event read is the only
// oracle that is not itself blind, so this helper MUST stay on
// ShowGlobalHooksForEvent and never revert to the no-arg global form.
func portalEntryCommandsForEvent(t *testing.T, client *tmux.Client, event, fingerprint string) []string {
	t.Helper()
	raw, err := client.ShowGlobalHooksForEvent(event)
	if err != nil {
		t.Fatalf("ShowGlobalHooksForEvent(%s): %v", event, err)
	}
	parsed := tmux.ParseShowHooks(raw)
	var commands []string
	for _, e := range parsed[event] {
		if strings.Contains(e.Command, fingerprint) {
			commands = append(commands, e.Command)
		}
	}
	return commands
}

// countPortalEntriesForEvent returns the number of entries on event whose
// command body contains fingerprint, derived from the canonical
// portalEntryCommandsForEvent primitive so the read-per-event/parse/match body
// lives in exactly one place.
func countPortalEntriesForEvent(t *testing.T, client *tmux.Client, event, fingerprint string) int {
	t.Helper()
	return len(portalEntryCommandsForEvent(t, client, event, fingerprint))
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
		if entries[0].Command != expectedNotifyCommand {
			t.Errorf("event %q: desired body = %q, want %q", ev, entries[0].Command, expectedNotifyCommand)
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
		if err := client.AppendGlobalHook(ev, expectedNotifyCommand); err != nil {
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

// stackDepth is the K used by the self-heal and teardown-at-depth guards. The
// live incident stacked 139 identical Portal entries on each blind event
// (pane-focus-out / window-layout-changed); 5 exercises the same depth-N
// collapse / reap path (the convergence and teardown loops are linear in the
// number of Portal-authored entries, so K=5 and K=139 traverse identical
// code) while keeping the per-test wall-clock bounded — each seeded entry is
// one real `set-hook -ga` round-trip against the live server.
const stackDepth = 5

// userHookFingerprint is the unique marker embedded in the co-resident user
// hook bodies. It contains NONE of the portalCommandSubstrings fingerprints
// (`portal state notify` / `portal state signal-hydrate` /
// `portal state migrate-rename`) nor the registration commit-now fingerprint,
// so both the registration convergence engine and the teardown reap classify
// it as non-Portal and leave it untouched.
const userHookFingerprint = "echo user pane-focus-out hook"

// userHookBody is the full run-shell-wrapped co-resident user hook seeded
// alongside the stacked Portal entries. A user `.tmux.conf` hook on a managed
// event must survive both registration and teardown (Acceptance Criterion 4).
const userHookBody = `run-shell "echo user pane-focus-out hook"`

// TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact is the
// real-tmux self-heal guard (Testing Requirement 3 / Acceptance Criteria 2
// and 4). It seeds a blind event (pane-focus-out) with K stacked identical
// Portal `notify` entries plus one co-resident non-Portal user hook, runs a
// single RegisterPortalHooks, and asserts the K-deep Portal stack collapses to
// exactly one entry carrying the desired notifyCommand while the user hook
// survives untouched.
//
// pane-focus-out is chosen deliberately: it is one of the two events the
// no-arg `show-hooks -g` enumeration is blind to, so this is the exact path
// that grew unbounded pre-fix. A mock commander cannot reproduce the
// stacked-array collapse — only a real tmux server faithfully models
// `set-hook -gu <event>[N]` index semantics and the per-event read shape.
func TestRegisterPortalHooks_SelfHealsKDeepStackLeavingUserHookIntact(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hooks-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const event = "pane-focus-out" // a blind event — the pre-fix growth path.

	// Seed K stacked identical Portal entries plus ONE co-resident user hook.
	// Live incident was 139-deep on this event; K=5 traverses the identical
	// collapse path (see stackDepth).
	for i := 0; i < stackDepth; i++ {
		if err := client.AppendGlobalHook(event, expectedNotifyCommand); err != nil {
			t.Fatalf("seed Portal entry %d: AppendGlobalHook(%s): %v", i, event, err)
		}
	}
	if err := client.AppendGlobalHook(event, userHookBody); err != nil {
		t.Fatalf("seed user hook: AppendGlobalHook(%s): %v", event, err)
	}

	// Pre-condition sanity: the stack really is K deep and the user hook is
	// present, so a green result cannot be a vacuous "nothing was there" pass.
	if got := countPortalEntriesForEvent(t, client, event, notifyFingerprint); got != stackDepth {
		t.Fatalf("pre-seed: Portal entry count = %d, want %d", got, stackDepth)
	}

	// One registration must collapse the whole stack to exactly one.
	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// Exactly one Portal entry survives, carrying the desired notifyCommand
	// byte-for-byte (modulo ParseShowHooks' outer-quote stripping).
	raw, err := client.ShowGlobalHooksForEvent(event)
	if err != nil {
		t.Fatalf("ShowGlobalHooksForEvent(%s): %v", event, err)
	}
	entries := tmux.ParseShowHooks(raw)[event]

	var portal []tmux.HookEntry
	for _, e := range entries {
		if strings.Contains(e.Command, notifyFingerprint) {
			portal = append(portal, e)
		}
	}
	if len(portal) != 1 {
		t.Fatalf("after self-heal: Portal entry count = %d, want 1; entries=%v", len(portal), entries)
	}
	if portal[0].Command != expectedNotifyCommand {
		t.Errorf("after self-heal: surviving body = %q, want %q", portal[0].Command, expectedNotifyCommand)
	}

	// The co-resident user hook is untouched — exactly one entry carrying its
	// fingerprint remains (Acceptance Criterion 4).
	if got := countPortalEntriesForEvent(t, client, event, userHookFingerprint); got != 1 {
		t.Errorf("after self-heal: user hook count = %d, want 1 (must survive untouched); entries=%v", got, entries)
	}
}

// TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact is
// the real-tmux teardown-at-depth guard (Testing Requirement 4 / Acceptance
// Criteria 4 and 5). On EACH blind event (pane-focus-out and
// window-layout-changed) it seeds K stacked Portal `notify` entries plus one
// co-resident non-Portal user hook, runs UnregisterPortalHooks once, and
// asserts every Portal entry is reaped (count → 0) while the user hook
// survives on each event.
//
// This is the path that NO-OPS pre-fix: the pre-fix teardown read through the
// no-arg `show-hooks -g`, which is blind to these two events, so it saw zero
// Portal entries on the K-deep arrays and removed nothing. It passes only
// after the teardown path moved to the per-event `show-hooks -g <event>` seam.
// UnregisterPortalHooks takes no logger (its WARN sink is a package-level
// bootstrapLogger), so correctness is asserted purely on the resulting tmux
// state via per-event reads.
func TestUnregisterPortalHooks_ReapsAtDepthOnBlindEventsLeavingUserHookIntact(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hooks-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	blindEvents := []string{"pane-focus-out", "window-layout-changed"}

	// Seed each blind event with a K-deep Portal stack plus one user hook.
	for _, event := range blindEvents {
		for i := 0; i < stackDepth; i++ {
			if err := client.AppendGlobalHook(event, expectedNotifyCommand); err != nil {
				t.Fatalf("seed Portal entry %d on %s: AppendGlobalHook: %v", i, event, err)
			}
		}
		if err := client.AppendGlobalHook(event, userHookBody); err != nil {
			t.Fatalf("seed user hook on %s: AppendGlobalHook: %v", event, err)
		}
		// Pre-condition sanity so a green pass cannot be vacuous.
		if got := countPortalEntriesForEvent(t, client, event, notifyFingerprint); got != stackDepth {
			t.Fatalf("pre-seed %s: Portal entry count = %d, want %d", event, got, stackDepth)
		}
	}

	// One teardown must reap every Portal entry on BOTH blind events.
	if err := tmux.UnregisterPortalHooks(client); err != nil {
		t.Fatalf("UnregisterPortalHooks: %v", err)
	}

	// Assert per blind event: zero entries carry ANY portalCommandSubstrings
	// fingerprint, AND the co-resident user hook survives intact.
	teardownFingerprints := []string{
		"portal state notify",
		"portal state commit-now",
		"portal state signal-hydrate",
		"portal state migrate-rename",
	}
	for _, event := range blindEvents {
		for _, fp := range teardownFingerprints {
			if got := countPortalEntriesForEvent(t, client, event, fp); got != 0 {
				t.Errorf("after teardown: event %q still holds %d entries matching %q, want 0", event, got, fp)
			}
		}
		if got := countPortalEntriesForEvent(t, client, event, userHookFingerprint); got != 1 {
			t.Errorf("after teardown: event %q user hook count = %d, want 1 (must survive untouched)", event, got)
		}
	}
}

// TestRegisterPortalHooks_SecondRegistrationIsChurnFree is the real-tmux
// idempotency / no-churn guard (Testing Requirement 5 / Acceptance Criterion
// 7). It converges the table with one RegisterPortalHooks, snapshots every
// managed event's parsed entry indices, runs a SECOND RegisterPortalHooks, and
// asserts:
//
//   - per-event entry indices are IDENTICAL across the two runs — the
//     load-bearing structural proof that the converged fast path performed no
//     unset+append (an unset+append would renumber the array). Real tmux is
//     the oracle: index stability, not mock call counts, proves no churn.
//   - the second run emits NO eviction INFO line (no `reaped` attr > 0 — the
//     absence is the asserted churn-free signal per the spec), and
//   - the second run emits NO WARN.
func TestRegisterPortalHooks_SecondRegistrationIsChurnFree(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-hooks-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// First registration converges the table (no pre-seeded stack — a clean
	// server still has nothing to evict, so this is the all-fast-path-after-one
	// append shape). Capture INFO/WARN to confirm the first run's behaviour is
	// what we expect before testing the second.
	r1 := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, r1.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("first RegisterPortalHooks: %v", err)
	}

	// Snapshot each managed event's parsed entry indices after convergence.
	before := snapshotEventIndices(t, client)

	// Second registration against the already-converged table.
	r2 := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, r2.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("second RegisterPortalHooks: %v", err)
	}

	after := snapshotEventIndices(t, client)

	// (a) Index stability: a converged fast path leaves every event's array
	// untouched, so indices must be byte-for-byte identical. Any difference
	// means the fast path regressed to unset+append (renumbering the array).
	for _, me := range managedEventFingerprints {
		b, a := before[me.event], after[me.event]
		if !equalInts(b, a) {
			t.Errorf("event %q: entry indices changed across churn-free run: before=%v after=%v "+
				"(an unset+append would renumber — the fast path regressed)", me.event, b, a)
		}
	}

	// (b) No eviction INFO line on the second run: no recorded INFO carries a
	// reaped attr > 0. (RegisterPortalHooks emits the `reaped` cycle-summary
	// INFO only when totalEvicted > 0.)
	infos := r2.infos()
	for i, reaped := range r2.infoReaped() {
		if reaped > 0 {
			t.Errorf("second run emitted an eviction INFO line %q with reaped=%d, want no eviction line",
				infos[i], reaped)
		}
	}

	// (c) No WARN on the second run.
	if len(r2.warns()) != 0 {
		t.Errorf("second run emitted %d WARN line(s): %v, want none", len(r2.warns()), r2.warns())
	}
}

// snapshotEventIndices reads every managed event per-event and returns its
// parsed HookEntry indices (ascending, as ParseShowHooks sorts them). Used by
// the churn-free guard to prove the converged fast path leaves the arrays
// unrenumbered across a second registration.
func snapshotEventIndices(t *testing.T, client *tmux.Client) map[string][]int {
	t.Helper()
	out := make(map[string][]int, len(managedEventFingerprints))
	for _, me := range managedEventFingerprints {
		raw, err := client.ShowGlobalHooksForEvent(me.event)
		if err != nil {
			t.Fatalf("ShowGlobalHooksForEvent(%s): %v", me.event, err)
		}
		var indices []int
		for _, e := range tmux.ParseShowHooks(raw)[me.event] {
			indices = append(indices, e.Index)
		}
		out[me.event] = indices
	}
	return out
}

// equalInts reports whether two int slices are element-wise equal (including
// length). nil and empty are treated as equal.
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
