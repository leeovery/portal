package tmux_test

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// expectedSaveTriggerEvents is the canonical save-trigger event list, in
// registration order. Mirrors the save-trigger (non-hydration) prefix of the
// managedEvents table in hooks_register.go; kept here so tests can assert
// order without exporting the internal table.
var expectedSaveTriggerEvents = []string{
	"session-created",
	"session-closed",
	"session-renamed",
	"window-linked",
	"window-unlinked",
	"window-layout-changed",
	"pane-focus-out",
}

// notifyFingerprint is the per-event content fingerprint for the six
// `portal state notify` save-trigger events. Mirrors notifySubstring in
// hooks_register.go and is the single source from which expectedNotifyCommand
// is composed.
const notifyFingerprint = "portal state notify"

// commitNowFingerprint is the per-event content fingerprint for the
// session-closed commit-now event. Mirrors commitNowSubstring in
// hooks_register.go and is the single source from which expectedCommitNowCommand
// is composed.
const commitNowFingerprint = "portal state commit-now"

// signalHydrateFingerprint is the per-event content fingerprint for the two
// hydration-trigger events. Mirrors signalHydrateMarker in hooks_register.go
// and is the single source from which expectedSignalHydrateCommand is composed.
const signalHydrateFingerprint = "portal state signal-hydrate"

// expectedNotifyCommand is the exact full command Portal registers on each of
// the six non-session-closed save-trigger events. Mirrors notifyCommand in
// hooks_register.go, composed from notifyFingerprint so the fingerprint
// substring is the single source. The seventh save-trigger event
// (session-closed) carries commitNowCommand; see expectedCommitNowCommand.
const expectedNotifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && ` + notifyFingerprint + `"`

// expectedCommitNowCommand is the exact full command Portal registers on
// session-closed. Mirrors commitNowCommand in hooks_register.go, composed from
// commitNowFingerprint. session-closed is the single tmux-side seam that fires
// uniformly across every kill path; this command invokes a synchronous
// sessions.json commit to close the resurrection window between kill and the
// daemon's next tick.
const expectedCommitNowCommand = `run-shell "command -v portal >/dev/null 2>&1 && ` + commitNowFingerprint + `"`

// expectedSignalHydrateCommand is the exact full command Portal registers on
// every hydration-trigger event. Mirrors signalHydrateCommand in
// hooks_register.go, composed from signalHydrateFingerprint. The literal
// #{session_name} is preserved verbatim — tmux expands it at hook-fire time.
// The `--` end-of-flags separator before #{session_name} prevents cobra/pflag
// from misparsing leading-dash session names (e.g. `-dotfiles-HM9Zhw`) as
// short-flag clusters.
const expectedSignalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && ` + signalHydrateFingerprint + ` -- #{session_name}"`

// expectedManagedEventCount is the total number of Portal-managed events the
// convergence engine registers on a fresh table: seven save-trigger events +
// two hydration-trigger events.
var expectedManagedEventCount = len(expectedSaveTriggerEvents) + len(tmux.HydrationTriggerEvents)

// nonSessionClosedSaveTriggerEvents is the canonical save-trigger event list
// minus session-closed — the six events that converge to notifyCommand.
// Mirrors the implicit split in hooks_register.go.
var nonSessionClosedSaveTriggerEvents = []string{
	"session-created",
	"session-renamed",
	"window-linked",
	"window-unlinked",
	"window-layout-changed",
	"pane-focus-out",
}

// perEventDispatch builds a RunFunc that answers the PER-EVENT read shape the
// convergence engine now uses: `show-hooks -g <event>` (len(args) >= 3) returns
// ONLY the seeded lines for args[2], filtered out of the full seeded table.
// `set-hook -ga` is dispatched to setHookErrFor (per-event error map, nil =
// success); `set-hook -gu` succeeds. The no-arg global `show-hooks -g` read is
// no longer issued by the engine; if it ever appears the test fails loudly.
//
// It is the zero-fault convenience wrapper over perEventDispatchWithFaults — the
// single owner of the per-event read/dispatch skeleton and its no-arg-global-read
// fatal guard. Call sites that only vary set-hook -ga errors stay one-liners.
func perEventDispatch(t *testing.T, seededTable string, setHookErrFor map[string]error) func(args ...string) (string, error) {
	t.Helper()
	return perEventDispatchWithFaults(t, seededTable, setHookErrFor, nil, nil)
}

// perEventDispatchWithFaults is the single owner of the per-event
// read/dispatch skeleton, including the no-arg-global-read t.Fatalf guard that
// pins the convergence engine's per-event-read invariant in exactly one place.
// It generalises perEventDispatch with two optional fault-injection maps so the
// previously-bespoke per-event RunFuncs collapse to one-line calls that vary
// only the injected fault:
//
//   - readErrFor: keyed by event (args[2] of `show-hooks -g <event>`). When the
//     key matches, the per-event read returns that error instead of the seeded
//     table — the channel for both a plain read sentinel and a *CommandError.
//   - unsetErrFor: keyed by indexed hook target (args[2] of
//     `set-hook -gu <event[idx]>`). When the key matches, that single per-index
//     unset returns the error; all other unsets succeed.
//
// setHookErrFor retains its existing semantics (per-event `set-hook -ga` error
// map). A nil map disables that fault channel.
func perEventDispatchWithFaults(t *testing.T, seededTable string, setHookErrFor, readErrFor, unsetErrFor map[string]error) func(args ...string) (string, error) {
	t.Helper()
	byEvent := parseSeededTableByEvent(seededTable)
	return func(args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "show-hooks" && args[1] == "-g" {
			if readErrFor != nil {
				if err, ok := readErrFor[args[2]]; ok {
					return "", err
				}
			}
			return byEvent[args[2]], nil
		}
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			t.Fatalf("convergence engine must read per-event, not the no-arg global show-hooks -g: %v", args)
			return "", nil
		}
		if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
			if setHookErrFor != nil {
				if err, ok := setHookErrFor[args[2]]; ok {
					return "", err
				}
			}
			return "", nil
		}
		if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
			if unsetErrFor != nil {
				if err, ok := unsetErrFor[args[2]]; ok {
					return "", err
				}
			}
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
}

// readErrForAllManagedEvents builds a readErrFor map that returns err for the
// per-event read of EVERY managed event, in lockstep with the production
// managedEvents table (via tmux.ManagedEventNames). It is the fail-every-read
// fault used by the "no set-hook when reads fail everywhere" tests; deriving the
// key set from the production table keeps the fault total in step with any
// future event added to managedEvents.
func readErrForAllManagedEvents(err error) map[string]error {
	m := map[string]error{}
	for _, ev := range tmux.ManagedEventNames() {
		m[ev] = err
	}
	return m
}

// assertNoSetHookCalls fails the test if any set-hook call (either -ga append or
// -gu unset) was recorded. Pins the "set-hook must not be dispatched when the
// per-event read fails" invariant as an observable post-condition on the mock's
// call log — replacing the bespoke RunFuncs' inline t.Fatalf("set-hook must not
// be called ...") tripwire with an equivalent assertion that survives collapse
// onto the shared dispatch helper.
func assertNoSetHookCalls(t *testing.T, calls [][]string) {
	t.Helper()
	for _, c := range calls {
		if len(c) >= 2 && c[0] == "set-hook" {
			t.Errorf("set-hook must not be called when show-hooks fails: %v", c)
		}
	}
}

// parseSeededTableByEvent splits a multi-line seeded show-hooks table (each
// line prefixed by "<event>[<idx>] ...") into a per-event sub-table so the
// per-event dispatch can answer `show-hooks -g <event>` with only that event's
// lines. The event name is the text before the first '['.
func parseSeededTableByEvent(table string) map[string]string {
	byEvent := map[string]string{}
	for line := range strings.SplitSeq(table, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		open := strings.IndexByte(line, '[')
		if open <= 0 {
			continue
		}
		ev := line[:open]
		byEvent[ev] += line + "\n"
	}
	return byEvent
}

// setHookCalls extracts the set-hook -ga calls from a MockCommander's call
// log, in invocation order, returning each as [event, command].
func setHookCalls(calls [][]string) [][2]string {
	var out [][2]string
	for _, c := range calls {
		if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" {
			out = append(out, [2]string{c[2], c[3]})
		}
	}
	return out
}

// setHookEvent is one entry in the ordered, cross-verb set-hook projection
// produced by setHookEvents. Verb is the tmux flag ("-ga" append or "-gu"
// unset) and Target is argv[2] — the bare event for an append (e.g.
// "pane-focus-out") or the indexed target for an unset (e.g.
// "pane-focus-out[3]").
type setHookEvent struct {
	Verb   string
	Target string
}

// setHookEvents projects a MockCommander's call log to the ordered cross-verb
// list of set-hook calls (both -ga appends and -gu unsets), in invocation
// order, dropping the interleaved show-hooks reads. It is the single accessor
// for tests that assert append-vs-unset ORDERING relative to each other — the
// relative position of each entry in the returned slice mirrors its relative
// position in the raw call log, so "the append follows every unset" is
// preserved without re-encoding the set-hook argv guards per test.
func setHookEvents(calls [][]string) []setHookEvent {
	var out []setHookEvent
	for _, c := range calls {
		switch {
		case len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga":
			out = append(out, setHookEvent{Verb: "-ga", Target: c[2]})
		case len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu":
			out = append(out, setHookEvent{Verb: "-gu", Target: c[2]})
		}
	}
	return out
}

// eventOfUnsetTarget splits the event-name prefix out of an indexed unset
// target (the argv[2] of a set-hook -gu call, e.g. "client-attached[3]"),
// returning the text before the first '['. A target with no '[' is returned
// unchanged. Co-located with the set-hook extractors so the indexed-target
// shape is decoded in exactly one place.
func eventOfUnsetTarget(target string) string {
	if i := strings.IndexByte(target, '['); i > 0 {
		return target[:i]
	}
	return target
}

// convergedTable builds a per-event seeded show-hooks table in the
// post-convergence shape: notifyCommand on the six non-session-closed
// save-trigger events, commitNowCommand on session-closed, and
// signalHydrateCommand on the two hydration-trigger events. Each line is
// single-outer-quoted to mirror tmux's actual show-hooks output format
// (Portal bodies contain literal double quotes, so tmux wraps them in single
// quotes); ParseShowHooks strips the outer single quotes, so the parsed
// command equals the desired-body constant — which the fast-path equality
// check relies on.
func convergedTable() string {
	var b strings.Builder
	for _, e := range expectedSaveTriggerEvents {
		cmd := expectedNotifyCommand
		if e == "session-closed" {
			cmd = expectedCommitNowCommand
		}
		fmt.Fprintf(&b, "%s[0] => '%s'\n", e, cmd)
	}
	for _, e := range tmux.HydrationTriggerEvents {
		fmt.Fprintf(&b, "%s[0] => '%s'\n", e, expectedSignalHydrateCommand)
	}
	return b.String()
}

// TestRegisterPortalHooks_FreshTable proves the convergence engine appends
// exactly one entry per managed event with the correct desired body when the
// table is empty — and never appends portal state migrate-rename.
func TestRegisterPortalHooks_FreshTable(t *testing.T) {
	mock := &MockCommander{RunFunc: perEventDispatch(t, "", nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := setHookCalls(mock.Calls)
	if len(got) != expectedManagedEventCount {
		t.Fatalf("set-hook -ga call count = %d, want %d: %v", len(got), expectedManagedEventCount, got)
	}

	// Each event appended exactly once with its correct desired body.
	wantBody := map[string]string{}
	for _, ev := range nonSessionClosedSaveTriggerEvents {
		wantBody[ev] = expectedNotifyCommand
	}
	wantBody["session-closed"] = expectedCommitNowCommand
	for _, ev := range tmux.HydrationTriggerEvents {
		wantBody[ev] = expectedSignalHydrateCommand
	}

	seen := map[string]int{}
	for _, c := range got {
		seen[c[0]]++
		if want, ok := wantBody[c[0]]; !ok {
			t.Errorf("unexpected event appended: %q", c[0])
		} else if c[1] != want {
			t.Errorf("event %q body = %q, want %q", c[0], c[1], want)
		}
		if strings.Contains(c[1], "portal state migrate-rename") {
			t.Errorf("event %q registered migrate-rename: %q", c[0], c[1])
		}
	}
	for ev := range wantBody {
		if seen[ev] != 1 {
			t.Errorf("event %q appended %d times, want exactly 1", ev, seen[ev])
		}
	}

	// No unset on an empty table.
	if unsets := unsetHookCalls(mock.Calls); len(unsets) != 0 {
		t.Errorf("expected 0 set-hook -gu on empty table, got %d: %v", len(unsets), unsets)
	}
}

// TestRegisterPortalHooks_IdempotentFastPath proves the churn-free signal: a
// registration against an already-converged table issues zero set-hook -ga and
// zero set-hook -gu, and emits no eviction INFO.
func TestRegisterPortalHooks_IdempotentFastPath(t *testing.T) {
	mock := &MockCommander{RunFunc: perEventDispatch(t, convergedTable(), nil)}
	client := tmux.NewClient(mock)

	logger := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if appends := setHookCalls(mock.Calls); len(appends) != 0 {
		t.Errorf("expected 0 set-hook -ga on converged table, got %d: %v", len(appends), appends)
	}
	if unsets := unsetHookCalls(mock.Calls); len(unsets) != 0 {
		t.Errorf("expected 0 set-hook -gu on converged table, got %d: %v", len(unsets), unsets)
	}
	for _, line := range logger.infos() {
		if strings.Contains(line, "reaped") || strings.Contains(line, "collapsed") {
			t.Errorf("unexpected eviction INFO on idempotent fast path: %q", line)
		}
	}
}

// TestRegisterPortalHooks_KDeepStackCollapse proves an event pre-seeded with K
// identical Portal entries collapses to exactly one: K set-hook -gu in
// descending index order, then one set-hook -ga carrying the desired body.
func TestRegisterPortalHooks_KDeepStackCollapse(t *testing.T) {
	const k = 5
	var b strings.Builder
	for i := range k {
		fmt.Fprintf(&b, "pane-focus-out[%d] => '%s'\n", i, expectedNotifyCommand)
	}
	mock := &MockCommander{RunFunc: perEventDispatch(t, b.String(), nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// K unsets on pane-focus-out, descending order.
	var paneFocusUnsets []string
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "pane-focus-out[") {
			paneFocusUnsets = append(paneFocusUnsets, u)
		}
	}
	if len(paneFocusUnsets) != k {
		t.Fatalf("pane-focus-out unset count = %d, want %d: %v", len(paneFocusUnsets), k, paneFocusUnsets)
	}
	for i, u := range paneFocusUnsets {
		want := fmt.Sprintf("pane-focus-out[%d]", k-1-i)
		if u != want {
			t.Errorf("unset[%d] = %q, want %q (descending-index order required)", i, u, want)
		}
	}

	// Exactly one append on pane-focus-out, after all the unsets, carrying
	// notifyCommand.
	appendIdx, lastUnsetIdx := -1, -1
	var appendBody string
	var paneFocusAppends int
	for i, e := range setHookEvents(mock.Calls) {
		if e.Verb == "-ga" && e.Target == "pane-focus-out" {
			paneFocusAppends++
			appendIdx = i
		}
		if e.Verb == "-gu" && strings.HasPrefix(e.Target, "pane-focus-out[") {
			lastUnsetIdx = i
		}
	}
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "pane-focus-out" {
			appendBody = c[1]
		}
	}
	if paneFocusAppends != 1 {
		t.Fatalf("pane-focus-out append count = %d, want 1", paneFocusAppends)
	}
	if appendBody != expectedNotifyCommand {
		t.Errorf("pane-focus-out append body = %q, want %q", appendBody, expectedNotifyCommand)
	}
	if appendIdx <= lastUnsetIdx {
		t.Errorf("append (event[%d]) must follow the unsets (last at event[%d])", appendIdx, lastUnsetIdx)
	}
}

// TestRegisterPortalHooks_StaleSignalHydrateMigratesInPlace proves the legacy
// un-separated signal-hydrate body is evicted and replaced by the current
// `--`-separated body, leaving exactly one entry on the event (count → 1).
func TestRegisterPortalHooks_StaleSignalHydrateMigratesInPlace(t *testing.T) {
	raw := fmt.Sprintf("client-attached[0] => '%s'\n", staleSignalHydrateCommand)
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw, nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Exactly one unset on client-attached[0] (the stale entry).
	var attachedUnsets []string
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "client-attached[") {
			attachedUnsets = append(attachedUnsets, u)
		}
	}
	if len(attachedUnsets) != 1 || attachedUnsets[0] != "client-attached[0]" {
		t.Fatalf("client-attached unsets = %v, want [client-attached[0]]", attachedUnsets)
	}

	// Exactly one append on client-attached carrying the `--` form.
	var appendCount int
	var appendBody string
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "client-attached" {
			appendCount++
			appendBody = c[1]
		}
	}
	if appendCount != 1 {
		t.Fatalf("client-attached append count = %d, want 1", appendCount)
	}
	if appendBody != expectedSignalHydrateCommand {
		t.Errorf("client-attached append body = %q, want %q (the -- form)", appendBody, expectedSignalHydrateCommand)
	}
}

// TestRegisterPortalHooks_StaleNotifyOnSessionClosedMigratesToCommitNow proves
// the pre-fix notifyCommand on session-closed is evicted and replaced by
// commitNowCommand (count → 1) via the union fingerprint set.
func TestRegisterPortalHooks_StaleNotifyOnSessionClosedMigratesToCommitNow(t *testing.T) {
	raw := fmt.Sprintf("session-closed[0] => '%s'\n", expectedNotifyCommand)
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw, nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Exactly one unset on session-closed[0].
	var closedUnsets []string
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "session-closed[") {
			closedUnsets = append(closedUnsets, u)
		}
	}
	if len(closedUnsets) != 1 || closedUnsets[0] != "session-closed[0]" {
		t.Fatalf("session-closed unsets = %v, want [session-closed[0]]", closedUnsets)
	}

	// Exactly one append on session-closed carrying commitNowCommand, after the
	// unset (evict-then-register avoids a transient duplicate window).
	unsetIdx, appendIdx := -1, -1
	var appendBody string
	var closedAppends int
	for i, e := range setHookEvents(mock.Calls) {
		if e.Verb == "-gu" && e.Target == "session-closed[0]" {
			unsetIdx = i
		}
		if e.Verb == "-ga" && e.Target == "session-closed" {
			closedAppends++
			appendIdx = i
		}
	}
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "session-closed" {
			appendBody = c[1]
		}
	}
	if closedAppends != 1 {
		t.Fatalf("session-closed append count = %d, want 1", closedAppends)
	}
	if appendBody != expectedCommitNowCommand {
		t.Errorf("session-closed append body = %q, want %q", appendBody, expectedCommitNowCommand)
	}
	if unsetIdx < 0 || appendIdx < 0 || unsetIdx >= appendIdx {
		t.Errorf("unset (event[%d]) must precede append (event[%d])", unsetIdx, appendIdx)
	}
}

// TestRegisterPortalHooks_SessionClosedUnionFastPath proves the union-count
// fast path: an already-converged session-closed holding a single
// commitNowCommand (matching the portal state commit-now fingerprint, union
// count 1, body equals desired) takes the fast path — no unset, no append.
func TestRegisterPortalHooks_SessionClosedUnionFastPath(t *testing.T) {
	raw := fmt.Sprintf("session-closed[0] => '%s'\n", expectedCommitNowCommand)
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw, nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "session-closed" {
			t.Errorf("unexpected set-hook -ga on already-converged session-closed: %q", c[1])
		}
	}
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "session-closed[") {
			t.Errorf("unexpected set-hook -gu on already-converged session-closed: %q", u)
		}
	}
}

// TestRegisterPortalHooks_SessionClosedSubstringEvictsPortalStateNotifyBody
// documents the spec's "One behavioral change to record": the unified
// convergence path identifies Portal-owned entries by SUBSTRING match
// (`portal state notify`), not the historical exact-string match. A body that
// merely CONTAINS `portal state notify` on session-closed — e.g. the legacy
// `portal state notify --debug` shape an exact-match would have preserved — is
// now treated as Portal-owned and evicted, converging to one commitNowCommand.
//
// This is the deliberate consequence of deleting migrateSessionClosedHook
// (whose exact-string match could never remove such a body) in favour of the
// shared substring predicate that the teardown path already uses.
func TestRegisterPortalHooks_SessionClosedSubstringEvictsPortalStateNotifyBody(t *testing.T) {
	// A body containing `portal state notify` but NOT byte-equal to the
	// historical notifyCommand literal — the exact case the old exact-match
	// path deliberately preserved, now evicted under the substring predicate.
	const notifyDebugBody = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify --debug"`
	if !strings.Contains(notifyDebugBody, "portal state notify") {
		t.Fatalf("test fixture %q does not contain the substring fingerprint", notifyDebugBody)
	}

	raw := fmt.Sprintf("session-closed[0] => '%s'\n", notifyDebugBody)
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw, nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The substring-matching body must be evicted at session-closed[0].
	var closedUnsets []string
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "session-closed[") {
			closedUnsets = append(closedUnsets, u)
		}
	}
	if len(closedUnsets) != 1 || closedUnsets[0] != "session-closed[0]" {
		t.Fatalf("session-closed unsets = %v, want [session-closed[0]] (substring predicate must evict the notify body)", closedUnsets)
	}

	// Exactly one append on session-closed carrying commitNowCommand.
	var appends int
	var body string
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "session-closed" {
			appends++
			body = c[1]
		}
	}
	if appends != 1 {
		t.Fatalf("session-closed append count = %d, want 1", appends)
	}
	if body != expectedCommitNowCommand {
		t.Errorf("session-closed append body = %q, want %q", body, expectedCommitNowCommand)
	}
}

// TestRegisterPortalHooks_SessionClosedNonMatchingUserHookSurvives replaces the
// old "--debug preserved" assertion (which no longer holds under the substring
// predicate — see TestRegisterPortalHooks_SessionClosedSubstringEvictsPortalStateNotifyBody).
// A genuinely non-matching user hook on session-closed — one whose body
// contains NONE of the event's fingerprints (`portal state notify`,
// `portal state commit-now`) — is never Portal-owned, so it survives untouched
// while the Portal commitNowCommand is appended alongside it.
func TestRegisterPortalHooks_SessionClosedNonMatchingUserHookSurvives(t *testing.T) {
	const userHook = `run-shell "tmux-resurrect save"`
	if strings.Contains(userHook, "portal state notify") || strings.Contains(userHook, "portal state commit-now") {
		t.Fatalf("test fixture %q unexpectedly contains a Portal fingerprint", userHook)
	}

	raw := fmt.Sprintf("session-closed[0] => '%s'\n", userHook)
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw, nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No unset on session-closed: the user hook is not Portal-fingerprinted.
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "session-closed[") {
			t.Errorf("unexpected unset on non-matching user hook: %q", u)
		}
	}

	// Exactly one append on session-closed carrying commitNowCommand, alongside
	// the surviving user hook.
	var appends int
	var body string
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "session-closed" {
			appends++
			body = c[1]
		}
	}
	if appends != 1 {
		t.Fatalf("session-closed append count = %d, want 1 (alongside the user hook)", appends)
	}
	if body != expectedCommitNowCommand {
		t.Errorf("session-closed append body = %q, want %q", body, expectedCommitNowCommand)
	}
}

// TestRegisterPortalHooks_UserHookUntouched proves a co-resident hook matching
// none of the event's fingerprints survives convergence: the user entry is
// never unset, and the Portal desired body is still appended alongside it.
func TestRegisterPortalHooks_UserHookUntouched(t *testing.T) {
	const userHook = `run-shell "tmux-resurrect save"`
	raw := fmt.Sprintf("pane-focus-out[0] => '%s'\n", userHook)
	mock := &MockCommander{RunFunc: perEventDispatch(t, raw, nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No unset on pane-focus-out: the user hook is not Portal-authored.
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "pane-focus-out[") {
			t.Errorf("unexpected unset on user hook: %q", u)
		}
	}

	// Exactly one append on pane-focus-out carrying notifyCommand.
	var appends int
	var body string
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "pane-focus-out" {
			appends++
			body = c[1]
		}
	}
	if appends != 1 {
		t.Fatalf("pane-focus-out append count = %d, want 1 (alongside the user hook)", appends)
	}
	if body != expectedNotifyCommand {
		t.Errorf("pane-focus-out append body = %q, want %q", body, expectedNotifyCommand)
	}
}

// TestRegisterPortalHooks_PerEventReadFailureFolds proves a
// ShowGlobalHooksForEvent failure on one event folds into the errors.Join
// aggregate, emits the canonical show-hooks-failed WARN, and does NOT prevent
// the other events from converging.
func TestRegisterPortalHooks_PerEventReadFailureFolds(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure on session-renamed")
	mock := &MockCommander{RunFunc: perEventDispatchWithFaults(t, "", nil,
		map[string]error{"session-renamed": sentinel}, nil)}
	client := tmux.NewClient(mock)

	logger := &recordingMigrationLogger{}
	err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap"))

	if err == nil {
		t.Fatal("expected aggregate error wrapping the sentinel, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "session-renamed") {
		t.Errorf("error %q does not name the failed event session-renamed", err.Error())
	}
	if !strings.Contains(err.Error(), "show-hooks failed") {
		t.Errorf("error %q missing the show-hooks-failed wrap", err.Error())
	}

	// The canonical WARN must have been emitted for the failure.
	if len(logger.warns()) == 0 {
		t.Errorf("expected at least one WARN for the show-hooks failure, got none")
	}

	// session-renamed must NOT have been appended (its convergence was skipped).
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "session-renamed" {
			t.Errorf("session-renamed must not be appended when its read fails: %v", c)
		}
	}

	// Every other managed event must still have been appended exactly once.
	got := map[string]int{}
	for _, c := range setHookCalls(mock.Calls) {
		got[c[0]]++
	}
	for _, ev := range expectedSaveTriggerEvents {
		if ev == "session-renamed" {
			continue
		}
		if got[ev] != 1 {
			t.Errorf("event %q append count = %d, want 1 (must still converge)", ev, got[ev])
		}
	}
	for _, ev := range tmux.HydrationTriggerEvents {
		if got[ev] != 1 {
			t.Errorf("event %q append count = %d, want 1 (must still converge)", ev, got[ev])
		}
	}
}

// TestRegisterPortalHooks_PerIndexUnsetFailureWarnsAndContinues proves a single
// UnsetGlobalHookAt failure during convergence emits a WARN, the loop
// continues to the remaining indices, and the append still fires.
func TestRegisterPortalHooks_PerIndexUnsetFailureWarnsAndContinues(t *testing.T) {
	// Two stale notify entries on pane-focus-out at indices 0 and 1. The first
	// unset (highest index — 1) fails; index 0 must still be attempted, and the
	// notifyCommand append must still fire after the partial eviction.
	raw := fmt.Sprintf("pane-focus-out[0] => '%s'\npane-focus-out[1] => '%s'\n",
		expectedNotifyCommand, expectedNotifyCommand)
	sentinel := errors.New("tmux unset failed at index 1")
	mock := &MockCommander{RunFunc: perEventDispatchWithFaults(t, raw, nil, nil,
		map[string]error{"pane-focus-out[1]": sentinel})}
	client := tmux.NewClient(mock)

	logger := &recordingMigrationLogger{}
	// A best-effort per-index unset failure must not propagate.
	if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("unexpected error from RegisterPortalHooks: %v", err)
	}

	// Both indices attempted, highest-first.
	var unsets []string
	for _, u := range unsetHookCalls(mock.Calls) {
		if strings.HasPrefix(u, "pane-focus-out[") {
			unsets = append(unsets, u)
		}
	}
	if len(unsets) != 2 || unsets[0] != "pane-focus-out[1]" || unsets[1] != "pane-focus-out[0]" {
		t.Fatalf("pane-focus-out unsets = %v, want [pane-focus-out[1] pane-focus-out[0]]", unsets)
	}

	// WARN logged for the failure.
	if len(logger.warns()) == 0 {
		t.Errorf("expected at least one WARN for the per-index unset failure, got none")
	}

	// The notifyCommand append still fired.
	var appended bool
	for _, c := range setHookCalls(mock.Calls) {
		if c[0] == "pane-focus-out" && c[1] == expectedNotifyCommand {
			appended = true
			break
		}
	}
	if !appended {
		t.Errorf("expected notifyCommand appended after partial eviction, none recorded")
	}
}

// TestRegisterPortalHooks_SingleReapedInfoOnEviction proves exactly one INFO
// line under the bootstrap component carrying the reaped total is emitted when
// evictions occur, summed across all events.
func TestRegisterPortalHooks_SingleReapedInfoOnEviction(t *testing.T) {
	// Stale notify entries on two events: 2 identical converged-body entries on
	// window-linked (a duplicate stack, both evicted) + 1 stale un-guarded
	// notify body on session-created (matches the `portal state notify`
	// fingerprint but is not byte-equal to notifyCommand, so it is not the
	// fast path — evicted and re-appended) → 3 evictions total.
	const staleNotify = `run-shell "portal state notify"`
	var b strings.Builder
	fmt.Fprintf(&b, "window-linked[0] => '%s'\n", expectedNotifyCommand)
	fmt.Fprintf(&b, "window-linked[1] => '%s'\n", expectedNotifyCommand)
	fmt.Fprintf(&b, "session-created[0] => '%s'\n", staleNotify)
	mock := &MockCommander{RunFunc: perEventDispatch(t, b.String(), nil)}
	client := tmux.NewClient(mock)

	logger := &recordingMigrationLogger{}
	if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	infos := logger.infos()
	if len(infos) != 1 {
		t.Fatalf("INFO line count = %d, want 1; infos=%v", len(infos), infos)
	}
	if !strings.HasPrefix(infos[0], "[bootstrap] ") {
		t.Errorf("INFO line %q not bound to the bootstrap component", infos[0])
	}
	if logger.infoReaped()[0] != 3 {
		t.Errorf("reaped attr = %d, want 3 (2 window-linked + 1 session-created)", logger.infoReaped()[0])
	}
}

// TestRegisterPortalHooks_NoReapedInfoOnZeroEviction proves no eviction INFO is
// emitted on a zero-eviction registration — covering both the fresh-table
// (all-append) path and the all-fast-path converged table. Absence of the line
// is the asserted churn-free signal.
func TestRegisterPortalHooks_NoReapedInfoOnZeroEviction(t *testing.T) {
	t.Run("fresh table (all appends, no evictions)", func(t *testing.T) {
		mock := &MockCommander{RunFunc: perEventDispatch(t, "", nil)}
		client := tmux.NewClient(mock)

		logger := &recordingMigrationLogger{}
		if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logger.infos()) != 0 {
			t.Errorf("expected 0 INFO lines on zero-eviction fresh table, got %d: %v", len(logger.infos()), logger.infos())
		}
	})

	t.Run("converged table (all fast-path, no evictions)", func(t *testing.T) {
		mock := &MockCommander{RunFunc: perEventDispatch(t, convergedTable(), nil)}
		client := tmux.NewClient(mock)

		logger := &recordingMigrationLogger{}
		if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logger.infos()) != 0 {
			t.Errorf("expected 0 INFO lines on all-fast-path converged table, got %d: %v", len(logger.infos()), logger.infos())
		}
	})
}

// TestSignalHydrateCommand_HasEndOfFlagsSeparator pins the shape of the
// hydration desired body. The `--` end-of-flags separator before
// #{session_name} is load-bearing: leading-dash session names would otherwise
// be parsed by cobra/pflag as short-flag clusters and the hook would exit
// non-zero before runSignalHydrate runs.
func TestSignalHydrateCommand_HasEndOfFlagsSeparator(t *testing.T) {
	t.Run("signalHydrateCommand resolves with -- before #{session_name}", func(t *testing.T) {
		want := `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`
		if expectedSignalHydrateCommand != want {
			t.Errorf("expectedSignalHydrateCommand = %q, want %q", expectedSignalHydrateCommand, want)
		}
		if !strings.Contains(expectedSignalHydrateCommand, " -- #{session_name}") {
			t.Errorf("expectedSignalHydrateCommand %q missing ` -- #{session_name}` separator", expectedSignalHydrateCommand)
		}
	})

	t.Run("RegisterPortalHooks emits the -- separator on every hydration event", func(t *testing.T) {
		mock := &MockCommander{RunFunc: perEventDispatch(t, "", nil)}
		client := tmux.NewClient(mock)

		if err := tmux.RegisterPortalHooks(client, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := map[string]string{}
		for _, c := range setHookCalls(mock.Calls) {
			got[c[0]] = c[1]
		}
		for _, ev := range tmux.HydrationTriggerEvents {
			cmd := got[ev]
			if !strings.Contains(cmd, "portal state signal-hydrate -- #{session_name}") {
				t.Errorf("event %q command = %q, missing `signal-hydrate -- #{session_name}`", ev, cmd)
			}
		}
	})
}

// TestRegisterPortalHooks_NoMigrateRename proves the convergence engine never
// registers a portal state migrate-rename hook on any event (the rename-key
// migration is deferred and is the teardown path's concern, not registration's).
func TestRegisterPortalHooks_NoMigrateRename(t *testing.T) {
	mock := &MockCommander{RunFunc: perEventDispatch(t, "", nil)}
	client := tmux.NewClient(mock)

	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range setHookCalls(mock.Calls) {
		if strings.Contains(c[1], "portal state migrate-rename") {
			t.Errorf("unexpected migrate-rename registration on event %q: %q", c[0], c[1])
		}
		// Every body must be one of the three Portal command literals.
		if c[1] != expectedNotifyCommand && c[1] != expectedCommitNowCommand && c[1] != expectedSignalHydrateCommand {
			t.Errorf("unexpected command body on event %q: %q", c[0], c[1])
		}
	}
}

// recordingMigrationLogger is a thin typed projection over the shared
// recordingSlogHandler (declared in portal_saver_test.go), which owns the
// slog.Handler capture scaffolding (Enabled/Handle/WithAttrs/WithGroup/owner
// and the shared records slice — including the WithAttrs-bound `component` it
// merges onto each stored record). recordingMigrationLogger adds the
// convergence-flow projections the migration tests assert on: each captured
// Info record is rendered as "[<component>] <message>" (so component-binding
// can be asserted) and its `reaped` attr value is projected positionally by
// infoReaped (-1 when absent). Use Logger() to obtain a *slog.Logger to pass
// into RegisterPortalHooks.
type recordingMigrationLogger struct {
	recordingSlogHandler
}

// Logger returns a *slog.Logger whose records are captured by this recorder.
func (r *recordingMigrationLogger) Logger() *slog.Logger { return slog.New(&r.recordingSlogHandler) }

// infos projects the captured Info records as "[<component>] <message>" lines,
// in capture order.
func (r *recordingMigrationLogger) infos() []string {
	var out []string
	for _, rec := range r.records {
		if rec.Level == slog.LevelInfo {
			out = append(out, migrationLine(rec))
		}
	}
	return out
}

// infoReaped projects the `reaped` attr of each captured Info record
// positionally (aligned with infos), using -1 when the attr is absent.
func (r *recordingMigrationLogger) infoReaped() []int64 {
	var out []int64
	for _, rec := range r.records {
		if rec.Level == slog.LevelInfo {
			out = append(out, migrationReaped(rec))
		}
	}
	return out
}

// warns projects the captured Warn records as "[<component>] <message>" lines,
// in capture order.
func (r *recordingMigrationLogger) warns() []string {
	var out []string
	for _, rec := range r.records {
		if rec.Level == slog.LevelWarn {
			out = append(out, migrationLine(rec))
		}
	}
	return out
}

// migrationLine renders a captured record as "[<component>] <message>", reading
// the component attr that recordingSlogHandler.Handle merged from the .With
// binding onto the stored record's attrs.
func migrationLine(rec slog.Record) string {
	component := ""
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			component = a.Value.String()
		}
		return true
	})
	return "[" + component + "] " + rec.Message
}

// migrationReaped extracts the `reaped` int64 attr from a captured record,
// returning -1 when the attr is absent.
func migrationReaped(rec slog.Record) int64 {
	reaped := int64(-1)
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "reaped" {
			reaped = a.Value.Int64()
		}
		return true
	})
	return reaped
}
