package tmux_test

// Hydration-event convergence tests, driven through the unified
// RegisterPortalHooks per-event ensure-exactly-one path.
//
// These were originally the migrateHydrationHooks suite; that helper was
// deleted once the convergence engine subsumed its job (matching on
// `portal state signal-hydrate` evicts the legacy un-separated body AND any
// duplicate, converging to the `--` form as an ordinary side effect). The
// tests now pin that convergence behaviour against RegisterPortalHooks
// directly. The capture helper is recordingMigrationLogger (the single
// source of truth declared in hooks_register_test.go).
//
// Real-tmux fixtures (internal/tmuxtest) drive the tests where the eviction
// touches `set-hook -gu` index semantics; mock-based tests cover the
// per-index failure path (which would require fault injection on a real
// tmux that the test harness does not expose).

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// staleSignalHydrateCommand is the legacy un-separated hook body older
// Portal installs registered before the `--` fix. The convergence engine's
// hydration fingerprint (`portal state signal-hydrate`) matches this body; the
// new fixed body in expectedSignalHydrateCommand carries the `--` separator.
const staleSignalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`

// countSignalHydrateEntries returns, for each event in
// tmux.HydrationTriggerEvents, the number of hook entries on that event
// whose command body contains "portal state signal-hydrate". Used to assert
// AC #3's "exactly 1 entry per event after bootstrap" invariant. It is a thin
// map-builder over the canonical countPortalEntriesForEvent helper, so the
// per-event read/parse/match body lives in exactly one tmux_test-package place.
func countSignalHydrateEntries(t *testing.T, client *tmux.Client) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	for _, ev := range tmux.HydrationTriggerEvents {
		counts[ev] = countPortalEntriesForEvent(t, client, ev, "portal state signal-hydrate")
	}
	return counts
}

// installStaleHooks appends a stale (un-separated) signal-hydrate hook
// entry to every event in tmux.HydrationTriggerEvents on the supplied
// real-tmux server.
func installStaleHooks(t *testing.T, client *tmux.Client) {
	t.Helper()
	for _, ev := range tmux.HydrationTriggerEvents {
		if err := client.AppendGlobalHook(ev, staleSignalHydrateCommand); err != nil {
			t.Fatalf("AppendGlobalHook(%s): %v", ev, err)
		}
	}
}

// TestRegisterPortalHooks_HydrationConvergesUnSeparatedToDashForm proves the
// happy-path upgrade: an installation with one stale un-separated entry on
// every hydration event ends up with exactly one fixed entry per event
// after bootstrap (eviction then install via RegisterPortalHooks).
func TestRegisterPortalHooks_HydrationConvergesUnSeparatedToDashForm(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	installStaleHooks(t, client)

	log := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	counts := countSignalHydrateEntries(t, client)
	for _, ev := range tmux.HydrationTriggerEvents {
		if counts[ev] != 1 {
			t.Errorf("event %q: signal-hydrate entry count = %d, want 1", ev, counts[ev])
		}
	}

	// One INFO line summarising the eviction count must be emitted. The
	// unified convergence path emits a single "collapsed stacked portal hooks"
	// summary carrying the reaped count across all events.
	if infos := log.infos(); len(infos) != 1 {
		t.Errorf("INFO line count = %d, want 1; infos=%v", len(infos), infos)
	} else if !strings.Contains(infos[0], "collapsed stacked portal hooks") || log.infoReaped()[0] < 1 {
		t.Errorf("INFO line = %q reaped=%d, missing eviction summary", infos[0], log.infoReaped()[0])
	}

	// Verify the fixed entry actually contains the `--` separator on each
	// hydration event. Read each event's own table via the per-event seam
	// (the canonical helper), matching on the fixed `-- ` body.
	for _, ev := range tmux.HydrationTriggerEvents {
		fixed := portalEntryCommandsForEvent(t, client, ev, "portal state signal-hydrate -- ")
		if len(fixed) == 0 {
			t.Errorf("event %q: no entry containing `signal-hydrate -- `; fixed entries=%v", ev, fixed)
		}
	}
}

// TestRegisterPortalHooks_HydrationSecondBootstrapIsSilentNoOp proves AC #3's
// "unchanged across two consecutive bootstraps" invariant: a second
// invocation evicts nothing, emits no INFO line, and leaves the entry count
// per event at exactly 1.
func TestRegisterPortalHooks_HydrationSecondBootstrapIsSilentNoOp(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	installStaleHooks(t, client)

	// First bootstrap: evicts and installs.
	first := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, first.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("first RegisterPortalHooks: %v", err)
	}

	// Second bootstrap: must be a complete no-op.
	second := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, second.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("second RegisterPortalHooks: %v", err)
	}

	if len(second.infos()) != 0 {
		t.Errorf("second bootstrap INFO count = %d, want 0; infos=%v", len(second.infos()), second.infos())
	}
	if len(second.warns()) != 0 {
		t.Errorf("second bootstrap WARN count = %d, want 0; warns=%v", len(second.warns()), second.warns())
	}

	counts := countSignalHydrateEntries(t, client)
	for _, ev := range tmux.HydrationTriggerEvents {
		if counts[ev] != 1 {
			t.Errorf("event %q: signal-hydrate entry count = %d, want 1", ev, counts[ev])
		}
	}
}

// TestRegisterPortalHooks_HydrationFreshInstallIsSilentAndInstallsFixed proves the
// fresh-install path: no stale entries to evict, no INFO emission. The
// register loop still installs the fixed entry per event.
func TestRegisterPortalHooks_HydrationFreshInstallIsSilentAndInstallsFixed(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	log := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	if len(log.infos()) != 0 {
		t.Errorf("INFO count = %d, want 0 (zero-eviction bootstrap silent); infos=%v", len(log.infos()), log.infos())
	}
	if len(log.warns()) != 0 {
		t.Errorf("WARN count = %d, want 0; warns=%v", len(log.warns()), log.warns())
	}

	counts := countSignalHydrateEntries(t, client)
	for _, ev := range tmux.HydrationTriggerEvents {
		if counts[ev] != 1 {
			t.Errorf("event %q: signal-hydrate entry count = %d, want 1", ev, counts[ev])
		}
	}
}

// TestRegisterPortalHooks_HydrationCollapsesMultipleStaleEntriesOnOneEvent
// proves descending-index iteration prevents shift bugs: appending three
// stale entries to a single event and then running the migration must
// remove all three, leaving exactly one fixed entry post-bootstrap.
func TestRegisterPortalHooks_HydrationCollapsesMultipleStaleEntriesOnOneEvent(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Three stale entries on client-attached. Other hydration event also
	// gets one stale entry so the migration's per-event loop is exercised.
	for i := 0; i < 3; i++ {
		if err := client.AppendGlobalHook("client-attached", staleSignalHydrateCommand); err != nil {
			t.Fatalf("AppendGlobalHook[client-attached][%d]: %v", i, err)
		}
	}
	if err := client.AppendGlobalHook("client-session-changed", staleSignalHydrateCommand); err != nil {
		t.Fatalf("AppendGlobalHook[client-session-changed]: %v", err)
	}

	log := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	counts := countSignalHydrateEntries(t, client)
	if counts["client-attached"] != 1 {
		t.Errorf("client-attached: signal-hydrate entry count = %d, want 1", counts["client-attached"])
	}
	if counts["client-session-changed"] != 1 {
		t.Errorf("client-session-changed: signal-hydrate entry count = %d, want 1", counts["client-session-changed"])
	}

	// INFO line should report 4 evictions (3 + 1).
	if len(log.infos()) != 1 {
		t.Fatalf("INFO count = %d, want 1; infos=%v", len(log.infos()), log.infos())
	}
	if log.infoReaped()[0] != 4 {
		t.Errorf("reaped attr = %d, want eviction count 4", log.infoReaped()[0])
	}
}

// TestRegisterPortalHooks_HydrationPreservesUserHookLackingFingerprint
// proves the convergence engine's user-hook coexistence guarantee: a
// hand-authored hook on a managed event that does NOT contain the event's
// Portal fingerprint (`portal state signal-hydrate`) is never matched and
// survives untouched.
//
// (Per the spec's adopted substring predicate — see § "One behavioral change
// to record" — a user hook that *does* contain `portal state signal-hydrate`
// would now be treated as Portal-owned and evicted; this test deliberately
// uses a fingerprint-free user hook to assert the surviving case.)
func TestRegisterPortalHooks_HydrationPreservesUserHookLackingFingerprint(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// User-authored entry on client-attached that contains none of the event's
	// Portal fingerprints (no `portal state signal-hydrate`).
	userHook := `run-shell "tmux-resurrect restore"`
	if err := client.AppendGlobalHook("client-attached", userHook); err != nil {
		t.Fatalf("AppendGlobalHook(user): %v", err)
	}

	log := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// User entry must still be there. The convergence also appends the
	// Portal-fixed entry, so client-attached holds the user hook + one Portal
	// hook. Read client-attached's own table via the per-event seam (the
	// canonical helper), matching on the user hook's distinctive body.
	survivingUser := portalEntryCommandsForEvent(t, client, "client-attached", "tmux-resurrect restore")
	if len(survivingUser) == 0 {
		t.Errorf("user hook was evicted; surviving user entries=%v", survivingUser)
	}

	// No eviction INFO: the user hook is not Portal-authored, so nothing was
	// reaped on client-attached, and the only convergence action was an append.
	if len(log.infos()) != 0 {
		t.Errorf("INFO count = %d, want 0 (user hook not Portal-fingerprinted, no eviction); infos=%v", len(log.infos()), log.infos())
	}
}

// TestRegisterPortalHooks_HydrationPerIndexEvictFailureWarnsAndContinues uses a
// MockCommander to inject a per-index UnsetGlobalHookAt failure that the
// real-tmux harness does not expose. The test drives the convergence through
// the canonical entry point RegisterPortalHooks (the hydration migration is
// now an ordinary side effect of the per-event convergence path) and asserts:
//
//   - RegisterPortalHooks returns nil — per-index migration failures
//     surface only via WARN log lines, never as a returned error.
//   - At least one WARN line names the failing event and a "failed to
//     evict" message.
//   - Successful evictions on other events trigger the INFO emission.
func TestRegisterPortalHooks_HydrationPerIndexEvictFailureWarnsAndContinues(t *testing.T) {
	// One stale entry per hydration event, served per-event via the shared
	// perEventDispatchWithFaults helper (the single owner of the per-event
	// read/dispatch skeleton). The unset-failure is injected through its
	// unsetErrFor fault map keyed by the indexed hook target, mirroring
	// TestRegisterPortalHooks_PerIndexUnsetFailureWarnsAndContinues.
	var raw strings.Builder
	for _, ev := range tmux.HydrationTriggerEvents {
		fmt.Fprintf(&raw, "%s[0] => %q\n", ev, staleSignalHydrateCommand)
	}

	failingTarget := "client-attached[0]" // matches set-hook -gu argv[2]
	sentinel := errors.New("tmux unset failure")

	mock := &MockCommander{RunFunc: perEventDispatchWithFaults(t, raw.String(), nil, nil,
		map[string]error{failingTarget: sentinel})}
	client := tmux.NewClient(mock)

	log := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks returned err: %v (per-index migration failures must not error)", err)
	}

	// At least one WARN line reporting the eviction failure. (The failing
	// event name was previously interpolated into the message; post-migration
	// the event name has no closed attr key so the terse message no longer
	// carries it — the WARN signature is asserted on its own.)
	var sawFailureWarn bool
	for _, w := range log.warns() {
		if strings.Contains(w, "failed to evict") {
			sawFailureWarn = true
			break
		}
	}
	if !sawFailureWarn {
		t.Errorf("no WARN line with `failed to evict`; warns=%v", log.warns())
	}

	// Successful evictions on other hydration events should trigger the
	// single INFO summary line (count >= 1).
	if infos := log.infos(); len(infos) != 1 {
		t.Fatalf("INFO count = %d, want 1; infos=%v", len(infos), infos)
	} else if !strings.Contains(infos[0], "collapsed stacked portal hooks") || log.infoReaped()[0] < 1 {
		t.Errorf("INFO line = %q reaped=%d, missing eviction summary", infos[0], log.infoReaped()[0])
	}
}

// TestRegisterPortalHooks_HydrationScansEveryRuntimeTriggerEvent
// proves the migration scans every event in HydrationTriggerEvents (read at
// runtime, not hard-coded). Driving through RegisterPortalHooks, the
// set-hook -gu calls observed must cover every event in the canonical list
// — extending the slice later requires no code change in migration.
func TestRegisterPortalHooks_HydrationScansEveryRuntimeTriggerEvent(t *testing.T) {
	// Seed one stale entry per hydration event and serve it through the
	// canonical perEventDispatch (single source of truth for per-event-filtered
	// show-hooks output). The convergence engine must scan every event in the
	// runtime slice; extending HydrationTriggerEvents later widens coverage with
	// no code change.
	var raw strings.Builder
	for _, ev := range tmux.HydrationTriggerEvents {
		fmt.Fprintf(&raw, "%s[0] => %q\n", ev, staleSignalHydrateCommand)
	}
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw.String(), nil)}
	client := tmux.NewClient(mock)

	log := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// The unset calls should target every event in the canonical slice.
	// unsetHookCalls already filters to set-hook -gu only — set-hook -ga calls
	// from the register loop are unrelated to the migration's runtime-slice
	// invariant. eventOfUnsetTarget splits the bare event name out of the
	// indexed target (e.g. "client-attached[0]" -> "client-attached").
	gotEvents := map[string]bool{}
	for _, u := range unsetHookCalls(mock.Calls) {
		gotEvents[eventOfUnsetTarget(u)] = true
	}
	for _, want := range tmux.HydrationTriggerEvents {
		if !gotEvents[want] {
			t.Errorf("event %q in HydrationTriggerEvents was NOT scanned by migration; got=%v", want, gotEvents)
		}
	}

	// Exactly one INFO line summarising eviction count = len(HydrationTriggerEvents).
	if len(log.infos()) != 1 {
		t.Fatalf("INFO count = %d, want 1; infos=%v", len(log.infos()), log.infos())
	}
	if want := int64(len(tmux.HydrationTriggerEvents)); log.infoReaped()[0] != want {
		t.Errorf("reaped attr = %d, want eviction count = %d", log.infoReaped()[0], want)
	}
}

// TestRegisterPortalHooks_HydrationReadFailureWrapsErrorAndSkipsSetHook proves the only path
// that surfaces an error from a per-event convergence: a
// ShowGlobalHooksForEvent failure. Per-index UnsetGlobalHookAt failures are
// best-effort (WARN + continue), but a failure to read an event at all skips
// that event's convergence with the wrapped error folded into the errors.Join
// aggregate. With every per-event read failing, the aggregate names each
// managed event's leg ("register hook on <event>: show-hooks failed: ...").
// No set-hook call must ever be made when every read fails.
func TestRegisterPortalHooks_HydrationReadFailureWrapsErrorAndSkipsSetHook(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure")
	mock := &MockCommander{
		RunFunc: perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil),
	}
	client := tmux.NewClient(mock)

	log := &recordingMigrationLogger{}
	err := tmux.RegisterPortalHooks(client, log.Logger().With("component", "bootstrap"))

	if err == nil {
		t.Fatal("expected error from RegisterPortalHooks, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}

	// No set-hook may be dispatched when every per-event read fails.
	assertNoSetHookCalls(t, mock.Calls)
	if !strings.Contains(err.Error(), "show-hooks failed") {
		t.Errorf("error %q does not contain expected wrap %q", err.Error(), "show-hooks failed")
	}
	// The failing hydration event's convergence must contribute a
	// "register hook on client-attached:" leaf to the errors.Join aggregate.
	if !strings.Contains(err.Error(), "register hook on client-attached") {
		t.Errorf("error %q missing per-event leg wrap %q", err.Error(), "register hook on client-attached")
	}
}
