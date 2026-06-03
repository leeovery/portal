package tmux_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// expectedSaveTriggerEvents is the canonical save-trigger event list, in
// registration order. Mirrors saveTriggerEvents in hooks_register.go; kept
// here so tests can assert order without exporting the internal slice.
var expectedSaveTriggerEvents = []string{
	"session-created",
	"session-closed",
	"session-renamed",
	"window-linked",
	"window-unlinked",
	"window-layout-changed",
	"pane-focus-out",
}

// expectedNotifyCommand is the exact full command Portal registers on each of
// the six non-session-closed save-trigger events. Mirrors notifyCommand in
// hooks_register.go. The seventh save-trigger event (session-closed) carries
// commitNowCommand; see expectedCommitNowCommand.
const expectedNotifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`

// expectedCommitNowCommand is the exact full command Portal registers on
// session-closed. Mirrors commitNowCommand in hooks_register.go. session-closed
// is the single tmux-side seam that fires uniformly across every kill path;
// this command invokes a synchronous sessions.json commit to close the
// resurrection window between kill and the daemon's next tick.
const expectedCommitNowCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`

// expectedSignalHydrateCommand is the exact full command Portal registers on
// every hydration-trigger event. Mirrors signalHydrateCommand in
// hooks_register.go. The literal #{session_name} is preserved verbatim — tmux
// expands it at hook-fire time. The `--` end-of-flags separator before
// #{session_name} prevents cobra/pflag from misparsing leading-dash session
// names (e.g. `-dotfiles-HM9Zhw`) as short-flag clusters.
const expectedSignalHydrateCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`

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
func perEventDispatch(t *testing.T, seededTable string, setHookErrFor map[string]error) func(args ...string) (string, error) {
	t.Helper()
	byEvent := parseSeededTableByEvent(seededTable)
	return func(args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "show-hooks" && args[1] == "-g" {
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
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
}

// parseSeededTableByEvent splits a multi-line seeded show-hooks table (each
// line prefixed by "<event>[<idx>] ...") into a per-event sub-table so the
// per-event dispatch can answer `show-hooks -g <event>` with only that event's
// lines. The event name is the text before the first '['.
func parseSeededTableByEvent(table string) map[string]string {
	byEvent := map[string]string{}
	for _, line := range strings.Split(table, "\n") {
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
	for _, line := range logger.infos {
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
	for i := 0; i < k; i++ {
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
	for i, c := range mock.Calls {
		if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "pane-focus-out" {
			paneFocusAppends++
			appendIdx = i
			appendBody = c[3]
		}
		if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" && strings.HasPrefix(c[2], "pane-focus-out[") {
			lastUnsetIdx = i
		}
	}
	if paneFocusAppends != 1 {
		t.Fatalf("pane-focus-out append count = %d, want 1", paneFocusAppends)
	}
	if appendBody != expectedNotifyCommand {
		t.Errorf("pane-focus-out append body = %q, want %q", appendBody, expectedNotifyCommand)
	}
	if appendIdx <= lastUnsetIdx {
		t.Errorf("append (call[%d]) must follow the unsets (last at call[%d])", appendIdx, lastUnsetIdx)
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
	for i, c := range mock.Calls {
		if len(c) >= 3 && c[0] == "set-hook" && c[1] == "-gu" && c[2] == "session-closed[0]" {
			unsetIdx = i
		}
		if len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga" && c[2] == "session-closed" {
			closedAppends++
			appendIdx = i
			appendBody = c[3]
		}
	}
	if closedAppends != 1 {
		t.Fatalf("session-closed append count = %d, want 1", closedAppends)
	}
	if appendBody != expectedCommitNowCommand {
		t.Errorf("session-closed append body = %q, want %q", appendBody, expectedCommitNowCommand)
	}
	if unsetIdx < 0 || appendIdx < 0 || unsetIdx >= appendIdx {
		t.Errorf("unset (call[%d]) must precede append (call[%d])", unsetIdx, appendIdx)
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

	for _, c := range mock.Calls {
		if c[0] != "set-hook" {
			continue
		}
		if c[1] == "-ga" && len(c) >= 4 && c[2] == "session-closed" {
			t.Errorf("unexpected set-hook -ga on already-converged session-closed: %q", c[3])
		}
		if c[1] == "-gu" && len(c) >= 3 && strings.HasPrefix(c[2], "session-closed[") {
			t.Errorf("unexpected set-hook -gu on already-converged session-closed: %q", c[2])
		}
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
	runFunc := func(args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "show-hooks" && args[1] == "-g" {
			if args[2] == "session-renamed" {
				return "", sentinel
			}
			return "", nil
		}
		if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
			t.Fatalf("convergence engine must read per-event: %v", args)
			return "", nil
		}
		if len(args) >= 2 && args[0] == "set-hook" && (args[1] == "-ga" || args[1] == "-gu") {
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
	mock := &MockCommander{RunFunc: runFunc}
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
	if len(logger.warns) == 0 {
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
	byEvent := parseSeededTableByEvent(raw)
	runFunc := func(args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "show-hooks" && args[1] == "-g" {
			return byEvent[args[2]], nil
		}
		if len(args) >= 3 && args[0] == "set-hook" && args[1] == "-gu" {
			if args[2] == "pane-focus-out[1]" {
				return "", sentinel
			}
			return "", nil
		}
		if len(args) >= 4 && args[0] == "set-hook" && args[1] == "-ga" {
			return "", nil
		}
		t.Fatalf("unexpected command: %v", args)
		return "", nil
	}
	mock := &MockCommander{RunFunc: runFunc}
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
	if len(logger.warns) == 0 {
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

	if len(logger.infos) != 1 {
		t.Fatalf("INFO line count = %d, want 1; infos=%v", len(logger.infos), logger.infos)
	}
	if !strings.HasPrefix(logger.infos[0], "[bootstrap] ") {
		t.Errorf("INFO line %q not bound to the bootstrap component", logger.infos[0])
	}
	if logger.infoReaped[0] != 3 {
		t.Errorf("reaped attr = %d, want 3 (2 window-linked + 1 session-created)", logger.infoReaped[0])
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
		if len(logger.infos) != 0 {
			t.Errorf("expected 0 INFO lines on zero-eviction fresh table, got %d: %v", len(logger.infos), logger.infos)
		}
	})

	t.Run("converged table (all fast-path, no evictions)", func(t *testing.T) {
		mock := &MockCommander{RunFunc: perEventDispatch(t, convergedTable(), nil)}
		client := tmux.NewClient(mock)

		logger := &recordingMigrationLogger{}
		if err := tmux.RegisterPortalHooks(client, logger.Logger().With("component", "bootstrap")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logger.infos) != 0 {
			t.Errorf("expected 0 INFO lines on all-fast-path converged table, got %d: %v", len(logger.infos), logger.infos)
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

// recordingMigrationLogger is a slog.Handler that captures Info / Warn records
// for assertion in convergence-flow tests. Each captured Info record is
// rendered as "[<component>] <message>" (so component-binding can be asserted)
// and its `reaped` attr value is captured positionally in infoReaped (-1 when
// absent). Use Logger() to obtain a *slog.Logger to pass into
// RegisterPortalHooks.
type recordingMigrationLogger struct {
	infos      []string
	infoReaped []int64
	warns      []string
	// shared points at the slice-owning recorder so handlers derived via
	// WithAttrs/WithGroup (notably .With("component", ...)) record into the
	// same slices; nil on the root.
	shared *recordingMigrationLogger
	bound  []slog.Attr
}

// Logger returns a *slog.Logger whose records are captured by this recorder.
func (r *recordingMigrationLogger) Logger() *slog.Logger { return slog.New(r) }

func (r *recordingMigrationLogger) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (r *recordingMigrationLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(r.bound)+len(attrs))
	next = append(next, r.bound...)
	next = append(next, attrs...)
	return &recordingMigrationLogger{shared: r.owner(), bound: next}
}

func (r *recordingMigrationLogger) WithGroup(_ string) slog.Handler {
	return &recordingMigrationLogger{shared: r.owner(), bound: r.bound}
}

func (r *recordingMigrationLogger) owner() *recordingMigrationLogger {
	if r.shared != nil {
		return r.shared
	}
	return r
}

func (r *recordingMigrationLogger) Handle(_ context.Context, rec slog.Record) error {
	component := ""
	reaped := int64(-1)
	read := func(a slog.Attr) bool {
		switch a.Key {
		case "component":
			component = a.Value.String()
		case "reaped":
			reaped = a.Value.Int64()
		}
		return true
	}
	for _, a := range r.bound {
		read(a)
	}
	rec.Attrs(read)
	line := "[" + component + "] " + rec.Message
	owner := r.owner()
	switch rec.Level {
	case slog.LevelInfo:
		owner.infos = append(owner.infos, line)
		owner.infoReaped = append(owner.infoReaped, reaped)
	case slog.LevelWarn:
		owner.warns = append(owner.warns, line)
	}
	return nil
}
