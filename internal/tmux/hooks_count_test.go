package tmux_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestPortalHookCountsByEvent_CountsOnlyPortalFingerprintEntries proves the
// per-event counter classifies entries with the SAME predicate convergeEvent
// uses (containsAny over each event's fingerprints): a foreign non-Portal entry
// is ignored, a stacked Portal duplicate counts as 2, every managed event is
// present in the returned map, and the read is per-event
// (ShowGlobalHooksForEvent), never the no-arg global show-hooks -g.
func TestPortalHookCountsByEvent_CountsOnlyPortalFingerprintEntries(t *testing.T) {
	const foreign = `run-shell "tmux-resurrect save"`
	seeded := convergedTable() +
		fmt.Sprintf("pane-focus-out[1] => '%s'\n", foreign) +
		fmt.Sprintf("window-layout-changed[1] => '%s'\n", expectedNotifyCommand)

	mock := &MockCommander{RunFunc: perEventDispatch(t, seeded, nil)}
	client := tmux.NewClient(mock)

	counts, err := tmux.PortalHookCountsByEvent(client)
	if err != nil {
		t.Fatalf("PortalHookCountsByEvent: %v", err)
	}

	// Every managed event is present in the map (a zero-count event must still
	// appear so the caller can detect a not-registered event).
	for _, ev := range tmux.ManagedEventNames() {
		if _, ok := counts[ev]; !ok {
			t.Errorf("event %q missing from counts map %v", ev, counts)
		}
	}

	// The foreign entry on pane-focus-out is NOT Portal-fingerprinted → ignored,
	// leaving pane-focus-out's single Portal entry (count 1).
	if counts["pane-focus-out"] != 1 {
		t.Errorf("pane-focus-out count = %d, want 1 (foreign entry must be ignored)", counts["pane-focus-out"])
	}

	// The stacked Portal duplicate on window-layout-changed → count 2.
	if counts["window-layout-changed"] != 2 {
		t.Errorf("window-layout-changed count = %d, want 2 (stacked Portal duplicate)", counts["window-layout-changed"])
	}

	// Every other managed event carries exactly one Portal entry.
	for _, ev := range tmux.ManagedEventNames() {
		if ev == "window-layout-changed" {
			continue
		}
		if counts[ev] != 1 {
			t.Errorf("event %q count = %d, want 1", ev, counts[ev])
		}
	}
}

// TestPortalHookCountsByEvent_PerEventReadFailurePropagates proves a
// ShowGlobalHooksForEvent read failure on one event returns the wrapped error
// (naming the failed event, wrapping the underlying error so errors.Is
// recovers the sentinel) with a nil map — the transient path a caller reports
// as not-evaluable rather than a false health verdict.
func TestPortalHookCountsByEvent_PerEventReadFailurePropagates(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure on pane-focus-out")
	mock := &MockCommander{RunFunc: perEventDispatchWithFaults(t, convergedTable(), nil,
		map[string]error{"pane-focus-out": sentinel}, nil)}
	client := tmux.NewClient(mock)

	counts, err := tmux.PortalHookCountsByEvent(client)
	if counts != nil {
		t.Errorf("counts = %v, want nil on read failure", counts)
	}
	if err == nil {
		t.Fatal("expected wrapped error on per-event read failure, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "show-hooks failed on pane-focus-out") {
		t.Errorf("error %q missing the 'show-hooks failed on pane-focus-out' wrap", err.Error())
	}
}
