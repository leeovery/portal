package tmux_test

// Teardown-side show-hooks-failure WARN shape. Mirrors the register-side
// coverage in hooks_register_warn_test.go (assertShowHooksWarnShape): the
// per-event read failure UnregisterPortalHooks emits must carry the same
// canonical WARN — message "show-hooks failed", error_class=unexpected,
// component=bootstrap, with the underlying error reachable on the "error"
// attr — and it must be observable through the same recording-logger seam the
// register side uses. Before the option-(b) injected-logger inner variant
// (unregisterPortalHooks) existed, the teardown WARN went to the package-level
// bootstrapLogger and was untestable through a recording sink; this pins that
// gap closed.

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestUnregisterPortalHooks_ShowHooksFailureEmitsCanonicalWarn drives the
// injected-logger teardown variant with a per-event read fault and asserts the
// canonical show-hooks-failed WARN is emitted through the recording-logger
// seam for every failing event, with the no-double-log invariant (exactly one
// WARN per failing event, no aggregate WARN).
func TestUnregisterPortalHooks_ShowHooksFailureEmitsCanonicalWarn(t *testing.T) {
	sentinel := errors.New("tmux show-hooks failure (teardown)")
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "show-hooks" && args[1] == "-g" {
				return "", sentinel
			}
			t.Fatalf("set-hook -gu must not be called when every read fails: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	rec := &recordingSlogHandler{}
	injected := slog.New(rec).With("component", "bootstrap")

	err := tmux.UnregisterPortalHooksWithLogger(client, injected)
	if err == nil {
		t.Fatal("expected aggregate error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("aggregate error %v does not wrap sentinel %v", err, sentinel)
	}

	// One WARN per teardown event whose per-event read failed; every event
	// fails once. No aggregate WARN from the errors.Join fold.
	warns := showHooksWarnRecords(rec.records)
	wantWarns := len(tmux.PortalTeardownEvents())
	if len(warns) != wantWarns {
		t.Fatalf("expected exactly %d %q WARNs (one per teardown event, no aggregate double-log), got %d: %v",
			wantWarns, showHooksWarnMessage, len(warns), rec.records)
	}
	for i, w := range warns {
		t.Run("warn-"+string(rune('0'+i)), func(t *testing.T) {
			assertShowHooksWarnShape(t, w, sentinel)
		})
	}
}
