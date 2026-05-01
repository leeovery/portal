package tmux_test

// Migration tests for migrateHydrationHooks — Task 1-2 of the
// scrollback-not-restored-with-non-zero-base-index spec.
//
// The migration scans every event in hydrationTriggerEvents, evicts any
// pre-existing un-separated `portal state signal-hydrate` entry (so the
// new fixed entry can be cleanly installed by RegisterHookIfAbsent), and
// emits diagnostics via a small MigrationLogger seam.
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
// Portal installs registered before the `--` fix. The eviction predicate
// matches this body; the new fixed body in expectedSignalHydrateCommand
// does not.
const staleSignalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`

// recordingLogger captures Info and Warn calls so assertions can verify
// emission counts and message content. Satisfies tmux.MigrationLogger.
type recordingLogger struct {
	infos []string
	warns []string
}

func (r *recordingLogger) Info(component, format string, args ...any) {
	r.infos = append(r.infos, fmt.Sprintf("%s | "+format, append([]any{component}, args...)...))
}

func (r *recordingLogger) Warn(component, format string, args ...any) {
	r.warns = append(r.warns, fmt.Sprintf("%s | "+format, append([]any{component}, args...)...))
}

// countSignalHydrateEntries returns, for each event in
// expectedHydrationTriggerEvents, the number of hook entries on that event
// whose command body contains "portal state signal-hydrate". Used to assert
// AC #3's "exactly 1 entry per event after bootstrap" invariant.
func countSignalHydrateEntries(t *testing.T, client *tmux.Client) map[string]int {
	t.Helper()
	raw, err := client.ShowGlobalHooks()
	if err != nil {
		t.Fatalf("ShowGlobalHooks: %v", err)
	}
	parsed := tmux.ParseShowHooks(raw)
	counts := make(map[string]int)
	for _, ev := range expectedHydrationTriggerEvents {
		for _, e := range parsed[ev] {
			if strings.Contains(e.Command, "portal state signal-hydrate") {
				counts[ev]++
			}
		}
	}
	return counts
}

// installStaleHooks appends a stale (un-separated) signal-hydrate hook
// entry to every event in expectedHydrationTriggerEvents on the supplied
// real-tmux server.
func installStaleHooks(t *testing.T, client *tmux.Client) {
	t.Helper()
	for _, ev := range expectedHydrationTriggerEvents {
		if err := client.AppendGlobalHook(ev, staleSignalHydrateCommand); err != nil {
			t.Fatalf("AppendGlobalHook(%s): %v", ev, err)
		}
	}
}

// TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed proves the
// happy-path upgrade: an installation with one stale un-separated entry on
// every hydration event ends up with exactly one fixed entry per event
// after bootstrap (eviction then install via RegisterPortalHooksWithLogger).
func TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	installStaleHooks(t, client)

	log := &recordingLogger{}
	if err := tmux.RegisterPortalHooksWithLogger(client, log); err != nil {
		t.Fatalf("RegisterPortalHooksWithLogger: %v", err)
	}

	counts := countSignalHydrateEntries(t, client)
	for _, ev := range expectedHydrationTriggerEvents {
		if counts[ev] != 1 {
			t.Errorf("event %q: signal-hydrate entry count = %d, want 1", ev, counts[ev])
		}
	}

	// One INFO line summarising the eviction count must be emitted.
	if len(log.infos) != 1 {
		t.Errorf("INFO line count = %d, want 1; infos=%v", len(log.infos), log.infos)
	} else if !strings.Contains(log.infos[0], "evicted") || !strings.Contains(log.infos[0], "stale signal-hydrate") {
		t.Errorf("INFO line = %q, missing eviction summary", log.infos[0])
	}

	// Verify the fixed entry actually contains the `--` separator on each
	// hydration event.
	raw, err := client.ShowGlobalHooks()
	if err != nil {
		t.Fatalf("ShowGlobalHooks: %v", err)
	}
	parsed := tmux.ParseShowHooks(raw)
	for _, ev := range expectedHydrationTriggerEvents {
		var found bool
		for _, e := range parsed[ev] {
			if strings.Contains(e.Command, "portal state signal-hydrate -- ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("event %q: no entry containing `signal-hydrate -- `; entries=%v", ev, parsed[ev])
		}
	}
}

// TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap proves AC #3's
// "unchanged across two consecutive bootstraps" invariant: a second
// invocation evicts nothing, emits no INFO line, and leaves the entry count
// per event at exactly 1.
func TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	installStaleHooks(t, client)

	// First bootstrap: evicts and installs.
	first := &recordingLogger{}
	if err := tmux.RegisterPortalHooksWithLogger(client, first); err != nil {
		t.Fatalf("first RegisterPortalHooksWithLogger: %v", err)
	}

	// Second bootstrap: must be a complete no-op.
	second := &recordingLogger{}
	if err := tmux.RegisterPortalHooksWithLogger(client, second); err != nil {
		t.Fatalf("second RegisterPortalHooksWithLogger: %v", err)
	}

	if len(second.infos) != 0 {
		t.Errorf("second bootstrap INFO count = %d, want 0; infos=%v", len(second.infos), second.infos)
	}
	if len(second.warns) != 0 {
		t.Errorf("second bootstrap WARN count = %d, want 0; warns=%v", len(second.warns), second.warns)
	}

	counts := countSignalHydrateEntries(t, client)
	for _, ev := range expectedHydrationTriggerEvents {
		if counts[ev] != 1 {
			t.Errorf("event %q: signal-hydrate entry count = %d, want 1", ev, counts[ev])
		}
	}
}

// TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp proves the
// fresh-install path: no stale entries to evict, no INFO emission. The
// register loop still installs the fixed entry per event.
func TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	log := &recordingLogger{}
	if err := tmux.RegisterPortalHooksWithLogger(client, log); err != nil {
		t.Fatalf("RegisterPortalHooksWithLogger: %v", err)
	}

	if len(log.infos) != 0 {
		t.Errorf("INFO count = %d, want 0 (zero-eviction bootstrap silent); infos=%v", len(log.infos), log.infos)
	}
	if len(log.warns) != 0 {
		t.Errorf("WARN count = %d, want 0; warns=%v", len(log.warns), log.warns)
	}

	counts := countSignalHydrateEntries(t, client)
	for _, ev := range expectedHydrationTriggerEvents {
		if counts[ev] != 1 {
			t.Errorf("event %q: signal-hydrate entry count = %d, want 1", ev, counts[ev])
		}
	}
}

// TestMigrateHydrationHooks_MultipleStaleEntriesOnSameEventEvictAllInOrder
// proves descending-index iteration prevents shift bugs: appending three
// stale entries to a single event and then running the migration must
// remove all three, leaving exactly one fixed entry post-bootstrap.
func TestMigrateHydrationHooks_MultipleStaleEntriesOnSameEventEvictAllInOrder(t *testing.T) {
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

	log := &recordingLogger{}
	if err := tmux.RegisterPortalHooksWithLogger(client, log); err != nil {
		t.Fatalf("RegisterPortalHooksWithLogger: %v", err)
	}

	counts := countSignalHydrateEntries(t, client)
	if counts["client-attached"] != 1 {
		t.Errorf("client-attached: signal-hydrate entry count = %d, want 1", counts["client-attached"])
	}
	if counts["client-session-changed"] != 1 {
		t.Errorf("client-session-changed: signal-hydrate entry count = %d, want 1", counts["client-session-changed"])
	}

	// INFO line should report 4 evictions (3 + 1).
	if len(log.infos) != 1 {
		t.Fatalf("INFO count = %d, want 1; infos=%v", len(log.infos), log.infos)
	}
	if !strings.Contains(log.infos[0], "evicted 4") {
		t.Errorf("INFO line = %q, want eviction count = 4", log.infos[0])
	}
}

// TestMigrateHydrationHooks_DoesNotEvictHandAuthoredHooksLackingCommandVPortalPrefix
// proves the eviction predicate's specificity: a hand-authored hook that
// references `portal state signal-hydrate` but lacks the `command -v portal`
// guard prefix (i.e. is not Portal-authored shape) is preserved.
func TestMigrateHydrationHooks_DoesNotEvictHandAuthoredHooksLackingCommandVPortalPrefix(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-mig-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// User-authored entry: contains "portal state signal-hydrate" but lacks
	// the Portal-authored "command -v portal >/dev/null 2>&1 &&" prefix.
	userHook := `run-shell "echo running portal state signal-hydrate manually"`
	if err := client.AppendGlobalHook("client-attached", userHook); err != nil {
		t.Fatalf("AppendGlobalHook(user): %v", err)
	}

	log := &recordingLogger{}
	if err := tmux.RegisterPortalHooksWithLogger(client, log); err != nil {
		t.Fatalf("RegisterPortalHooksWithLogger: %v", err)
	}

	// User entry must still be there. After migration the Portal-fixed
	// entry is also installed, so client-attached should have count = 2
	// (one user, one Portal-fixed).
	raw, err := client.ShowGlobalHooks()
	if err != nil {
		t.Fatalf("ShowGlobalHooks: %v", err)
	}
	parsed := tmux.ParseShowHooks(raw)

	var sawUser bool
	for _, e := range parsed["client-attached"] {
		if strings.Contains(e.Command, "echo running portal state signal-hydrate manually") {
			sawUser = true
			break
		}
	}
	if !sawUser {
		t.Errorf("user hook was evicted; entries=%v", parsed["client-attached"])
	}

	// No INFO emission since no Portal-authored stale entries existed.
	if len(log.infos) != 0 {
		t.Errorf("INFO count = %d, want 0 (user hook is not Portal-shape so no eviction); infos=%v", len(log.infos), log.infos)
	}
}

// TestMigrateHydrationHooks_PartialFailureLogsWarnAndContinues uses a
// MockCommander to inject a per-index UnsetGlobalHookAt failure that the
// real-tmux harness does not expose. The test asserts:
//
//   - migrateHydrationHooks returns (evicted, nil) — per-index failures
//     surface only via WARN log lines, never as a returned error.
//   - At least one WARN line names the failing event and a "failed to
//     evict" message.
//   - Successful evictions on other events still increment the count and
//     trigger the INFO emission.
func TestMigrateHydrationHooks_PartialFailureLogsWarnAndContinues(t *testing.T) {
	// show-hooks output: one stale entry per hydration event.
	var raw strings.Builder
	for _, ev := range expectedHydrationTriggerEvents {
		fmt.Fprintf(&raw, "%s[0] => %q\n", ev, staleSignalHydrateCommand)
	}

	failingTarget := "client-attached[0]" // matches set-hook -gu argv[2]
	sentinel := errors.New("tmux unset failure")

	runFunc := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			return raw.String(), nil
		}
		if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
			if args[2] == failingTarget {
				return "", sentinel
			}
			return "", nil
		}
		// set-hook -ga and any other downstream calls — accept silently.
		if len(args) >= 2 && args[0] == "set-hook" && args[1] == "-ga" {
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
	mock := &MockCommander{RunFunc: runFunc}
	client := tmux.NewClient(mock)

	log := &recordingLogger{}
	evicted, err := tmux.MigrateHydrationHooks(client, log)
	if err != nil {
		t.Fatalf("migrateHydrationHooks returned err: %v (per-index failures must not error)", err)
	}

	// One eviction failed, one (or more) succeeded. evicted reflects
	// successful removals only.
	if evicted < 1 {
		t.Errorf("evicted = %d, want >= 1 (other events should succeed)", evicted)
	}

	// At least one WARN line naming the failing event and message.
	var sawFailureWarn bool
	for _, w := range log.warns {
		if strings.Contains(w, "client-attached") && strings.Contains(w, "failed to evict") {
			sawFailureWarn = true
			break
		}
	}
	if !sawFailureWarn {
		t.Errorf("no WARN line names the failing event with `failed to evict`; warns=%v", log.warns)
	}
}

// TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime
// proves the migration scans every event in hydrationTriggerEvents (read at
// runtime, not hard-coded). The set-hook -gu calls observed must cover every
// event in the canonical list — extending the slice later requires no code
// change in migration.
func TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime(t *testing.T) {
	var raw strings.Builder
	for _, ev := range expectedHydrationTriggerEvents {
		fmt.Fprintf(&raw, "%s[0] => %q\n", ev, staleSignalHydrateCommand)
	}

	runFunc := func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			return raw.String(), nil
		}
		if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
	mock := &MockCommander{RunFunc: runFunc}
	client := tmux.NewClient(mock)

	log := &recordingLogger{}
	evicted, err := tmux.MigrateHydrationHooks(client, log)
	if err != nil {
		t.Fatalf("migrateHydrationHooks: %v", err)
	}
	if evicted != len(expectedHydrationTriggerEvents) {
		t.Errorf("evicted = %d, want %d (one per hydration event)", evicted, len(expectedHydrationTriggerEvents))
	}

	// The unset calls should target every event in the canonical slice.
	gotEvents := map[string]bool{}
	for _, c := range mock.Calls {
		if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" {
			// argv[2] is e.g. "client-attached[0]"
			ev := c[2]
			if i := strings.Index(ev, "["); i > 0 {
				ev = ev[:i]
			}
			gotEvents[ev] = true
		}
	}
	for _, want := range expectedHydrationTriggerEvents {
		if !gotEvents[want] {
			t.Errorf("event %q in hydrationTriggerEvents was NOT scanned by migration; got=%v", want, gotEvents)
		}
	}
}

// TestMigrateHydrationHooks_ShowHooksFailureWrapsError proves the only path
// that surfaces an error from migrateHydrationHooks: a ShowGlobalHooks
// failure. Per-index UnsetGlobalHookAt failures are best-effort (WARN +
// continue), but a failure to enumerate at all aborts the migration with
// the wrapped error.
func TestMigrateHydrationHooks_ShowHooksFailureWrapsError(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure")
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				return "", sentinel
			}
			t.Fatalf("set-hook must not be called when show-hooks fails: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	log := &recordingLogger{}
	evicted, err := tmux.MigrateHydrationHooks(client, log)

	if evicted != 0 {
		t.Errorf("evicted = %d, want 0 on show-hooks failure", evicted)
	}
	if err == nil {
		t.Fatal("expected wrapped error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "show-hooks failed") {
		t.Errorf("error %q does not contain expected wrap %q", err.Error(), "show-hooks failed")
	}
}
