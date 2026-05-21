package tmux_test

// Per-event regression gate pinning the
// killed-session-resurrects-within-tick-window spec's "Six Other Events
// Untouched" contract.
//
// Spec references:
//   - § What Stays Eventually Consistent — the six non-session-closed
//     save-trigger events ("session-created", "session-renamed",
//     "window-linked", "window-unlinked", "window-layout-changed",
//     "pane-focus-out") keep the cheap notifyCommand dirty-flag touch.
//     "Raising their cost from a 2-syscall touch to a full structural
//     capture is gratuitous."
//   - § Acceptance Criteria item 9 — "Six other events untouched. ...
//     must continue to fire portal state notify (the cheap dirty-flag
//     touch). No structural capture cost is added to these events."
//   - § Testing Requirements → Regression Tests → "Six-event eventual
//     consistency".
//
// Why this gate exists alongside TestRegisterPortalHooks's table-style
// coverage:
// The existing TestRegisterPortalHooks_SessionClosedMigration's
// nonSessionClosedSaveTriggerEvents loop already asserts each of the
// six events resolves to notifyCommand on a fresh server. This file
// adds explicit per-event named sub-tests so a future regression that
// over-generalises the session-closed migration (e.g. accidentally
// routing all save-trigger events through commitNowCommand) is
// surfaced with a precise per-event failure message, rather than a
// collective loop assertion that names only one event. The named
// sub-tests also document the contract event-by-event for code reviewers
// who scan failing test names without reading the table-driven body.
//
// The companion firing-side invariant — "firing notifyCommand touches
// save.requested and writes nothing to sessions.json within a bounded
// window" — lives in cmd/state_notify_six_event_eventual_consistency_test.go
// because notifyCommand resolves to `portal state notify` whose
// behaviour is owned by the cmd package. The registration-level
// invariant here, combined with the cmd-package firing invariant,
// covers spec § Testing Requirements → Regression Tests → "Six-event
// eventual consistency" end-to-end without requiring a real-tmux
// fixture for each of the six events (`pane-focus-out` in particular
// is hard to drive deterministically from a test harness).

import (
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestRegisterPortalHooks_NonSessionClosedEventsRouteToNotifyCommand pins,
// per-event, that each of the six non-session-closed save-trigger events
// is registered with notifyCommand and NOT commitNowCommand on a fresh
// hook table. A regression that accidentally promoted any of these
// events onto commitNowCommand would surface here with a precise per-
// event failure message identifying the misrouted event.
//
// The sub-test names match the task's "Named assertions" deliverables
// so a failing test name in CI output is self-documenting.
func TestRegisterPortalHooks_NonSessionClosedEventsRouteToNotifyCommand(t *testing.T) {
	for _, ev := range nonSessionClosedSaveTriggerEvents {
		ev := ev // pin for closure
		t.Run(fmt.Sprintf("%s is registered with notifyCommand and not commitNowCommand", ev), func(t *testing.T) {
			mock := &MockCommander{RunFunc: dispatchPortalHooks(t, "", nil)}
			client := tmux.NewClient(mock)

			if err := tmux.RegisterPortalHooks(client, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Collect every set-hook -ga call targeting this event.
			// On a fresh hook table the registration loop appends
			// each event exactly once, so the expected shape is one
			// call with body == expectedNotifyCommand.
			var eventCalls [][2]string
			for _, c := range setHookCalls(mock.Calls) {
				if c[0] == ev {
					eventCalls = append(eventCalls, c)
				}
			}

			if len(eventCalls) != 1 {
				t.Fatalf(
					"expected exactly 1 set-hook -ga on %q, got %d: %v\n"+
						"--- full set-hook -ga call log ---\n%v",
					ev, len(eventCalls), eventCalls, setHookCalls(mock.Calls),
				)
			}

			got := eventCalls[0][1]
			if got != expectedNotifyCommand {
				t.Errorf(
					"event %q registered with %q, want %q "+
						"(notifyCommand — the cheap dirty-flag touch)",
					ev, got, expectedNotifyCommand,
				)
			}
			if got == expectedCommitNowCommand {
				t.Errorf(
					"event %q was REGRESSION-routed onto commitNowCommand %q; "+
						"only session-closed may carry commitNowCommand per spec § "+
						"Hook Registration Migration → Registration Strategy",
					ev, expectedCommitNowCommand,
				)
			}
		})
	}
}
