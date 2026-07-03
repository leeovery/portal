// Tests in this file drive the package-level bootstrapWarnings sink and mutate
// package-level tmux seams (tmux.BootstrapAliveCheck, tmux.PortalSaverRetryDelay).
// They MUST NOT use t.Parallel — see cmd/bootstrap_warnings.go.
package cmd

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/tmux"
)

// resetBootstrapWarnings clears the package-level bootstrapWarnings sink before
// the test body and again on cleanup so warnings never bleed across tests
// (the sink is package-global and other cmd tests write to it too).
func resetBootstrapWarnings(t *testing.T) {
	t.Helper()
	bootstrapWarnings.Drain()
	t.Cleanup(func() { bootstrapWarnings.Drain() })
}

// shrinkSaverRetryDelay collapses the inter-attempt sleep in
// createPortalSaverWithRetry so the "revive fails across all retries" path
// completes without real wall-clock delay. Restored on cleanup.
func shrinkSaverRetryDelay(t *testing.T) {
	t.Helper()
	prev := tmux.PortalSaverRetryDelay
	tmux.PortalSaverRetryDelay = time.Millisecond
	t.Cleanup(func() { tmux.PortalSaverRetryDelay = prev })
}

// stubSaverAliveCheck pins tmux.BootstrapAliveCheck for the test so the revive
// path does not consult real on-disk daemon.pid files. Restored on cleanup.
func stubSaverAliveCheck(t *testing.T, alive bool) {
	t.Helper()
	prev := tmux.BootstrapAliveCheck
	tmux.BootstrapAliveCheck = func(string) bool { return alive }
	t.Cleanup(func() { tmux.BootstrapAliveCheck = prev })
}

// isPanePIDProbe reports whether a list-panes argv carries the "#{pane_pid}"
// format — the shape used by BOTH the presence probe (SaverPanePIDOrAbsent) and
// the best-effort pane-pid reads inside BootstrapPortalSaver. The pane-id read
// (SaverPaneID) carries "#{pane_id}" instead and is dispatched separately.
func isPanePIDProbe(args []string) bool {
	return len(args) > 0 && args[len(args)-1] == "#{pane_pid}"
}

// countOp returns how many recorded calls have argv[0] == op.
func countOp(calls [][]string, op string) int {
	n := 0
	for _, c := range calls {
		if len(c) > 0 && c[0] == op {
			n++
		}
	}
	return n
}

// noSuchSessionErr is a *tmux.CommandError whose stderr carries tmux's
// canonical lowercase "no such session" phrasing, so SaverPanePIDOrAbsent
// collapses it to (0, false, nil) — the "absent" classification.
func noSuchSessionErr() error {
	return &tmux.CommandError{
		Stderr: "no such session: " + tmux.PortalSaverName,
		Err:    errors.New("exit status 1"),
	}
}

func TestEnsureSaverLiveness_NoOpWhenSaverPresentAndAlive(t *testing.T) {
	resetBootstrapWarnings(t)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "list-panes" && isPanePIDProbe(args) {
				return "12345\n", nil // present pane, parseable pid -> alive
			}
			t.Fatalf("unexpected tmux call for present+alive saver: %v", args)
			return "", nil
		},
	}

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	// Exactly one presence round-trip; no revive / version-gate calls.
	if got := countOp(cmder.Calls, "list-panes"); got != 1 {
		t.Errorf("expected exactly 1 presence probe, got %d: %v", got, cmder.Calls)
	}
	for _, op := range []string{"has-session", "new-session", "respawn-pane", "kill-session"} {
		if n := countOp(cmder.Calls, op); n != 0 {
			t.Errorf("expected no %q call for alive saver, got %d: %v", op, n, cmder.Calls)
		}
	}
	if got := bootstrapWarnings.Drain(); len(got) != 0 {
		t.Errorf("expected empty warnings sink for alive saver, got %v", got)
	}
}

func TestEnsureSaverLiveness_RevivesViaBootstrapPortalSaverWhenAbsent(t *testing.T) {
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "list-panes":
				if isPanePIDProbe(args) {
					// Presence probe (and best-effort pid reads) -> absent.
					return "", noSuchSessionErr()
				}
				return "%1\n", nil // SaverPaneID (#{pane_id}), best-effort
			case "has-session":
				return "", errors.New("can't find session") // absent -> create branch
			case "new-session":
				return "", nil // createPortalSaverWithRetry succeeds
			case "set-option":
				return "", nil
			case "respawn-pane":
				// Fail here to return before BootstrapPortalSaver's daemon-
				// readiness barrier (no exported seam reachable from package
				// cmd). new-session is already recorded, proving the create
				// path ran. The revive failure funnels a warning that is
				// exercised directly in the warning test below.
				return "", errors.New("respawn failed")
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	if got := countOp(cmder.Calls, "new-session"); got != 1 {
		t.Errorf("expected BootstrapPortalSaver create path (1 new-session), got %d: %v", got, cmder.Calls)
	}
}

func TestEnsureSaverLiveness_TreatsProbeTransientErrorAsAbsentAndRevives(t *testing.T) {
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "list-panes":
				if isPanePIDProbe(args) {
					// Non-sentinel transient: a parseable-looking line that is
					// not a base-10 pid -> ErrPanePIDParse -> (0,false,err).
					return "not-a-pid\n", nil
				}
				return "%1\n", nil
			case "has-session":
				return "", errors.New("can't find session")
			case "new-session":
				return "", nil
			case "set-option":
				return "", nil
			case "respawn-pane":
				return "", errors.New("respawn failed")
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	if got := countOp(cmder.Calls, "new-session"); got != 1 {
		t.Errorf("transient probe error must be treated as absent and revive (1 new-session), got %d: %v", got, cmder.Calls)
	}
}

func TestEnsureSaverLiveness_FunnelsSaverDownWarningWhenReviveFails(t *testing.T) {
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)
	shrinkSaverRetryDelay(t)

	cmder := saverAbsentReviveFailsCommander()

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	got := bootstrapWarnings.Drain()
	if len(got) != 1 {
		t.Fatalf("expected exactly one warning on revive failure, got %d: %v", len(got), got)
	}
	if !reflect.DeepEqual(got[0], bootstrap.SaverDownWarning()) {
		t.Errorf("warning = %#v, want %#v", got[0], bootstrap.SaverDownWarning())
	}
}

// TestEnsureSaverLiveness_LogsWarnWithUnderlyingErrorWhenReviveFails asserts
// that a failed BootstrapPortalSaver on the abridged path emits exactly one
// bootstrap-component WARN carrying the underlying error — restoring
// diagnosability parity with the full-bootstrap step-5 "step failed" WARN
// (cmd/bootstrap/bootstrap.go) — while the SaverDownWarning funnel and the
// proceed-anyway (no error return) posture stay unchanged. The package-level
// bootstrapLogger (log.For("bootstrap")) routes through the swapped sink.
func TestEnsureSaverLiveness_LogsWarnWithUnderlyingErrorWhenReviveFails(t *testing.T) {
	resetBootstrapWarnings(t)
	stubSaverAliveCheck(t, false)
	shrinkSaverRetryDelay(t)

	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	cmder := saverAbsentReviveFailsCommander()

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	if n := countLines(sink, "WARN", "component=bootstrap", "abridged EnsureSaver", "error="); n != 1 {
		t.Errorf("expected exactly one bootstrap-component WARN carrying the underlying error, got %d in:\n%s", n, sink.Body())
	}

	// The SaverDownWarning funnel and proceed-anyway posture are unchanged.
	got := bootstrapWarnings.Drain()
	if len(got) != 1 || !reflect.DeepEqual(got[0], bootstrap.SaverDownWarning()) {
		t.Errorf("expected exactly one SaverDownWarning still funneled, got %#v", got)
	}
}

// TestEnsureSaverLiveness_LogsNoWarnWhenSaverPresent asserts the successful
// presence early return emits no WARN (and adds no warning) — the breadcrumb
// is confined to the revive-failure branch.
func TestEnsureSaverLiveness_LogsNoWarnWhenSaverPresent(t *testing.T) {
	resetBootstrapWarnings(t)

	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "list-panes" && isPanePIDProbe(args) {
				return "12345\n", nil // present pane, parseable pid -> alive
			}
			t.Fatalf("unexpected tmux call for present+alive saver: %v", args)
			return "", nil
		},
	}

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	if n := countLines(sink, "WARN"); n != 0 {
		t.Errorf("expected no WARN on the present-saver early return, got %d in:\n%s", n, sink.Body())
	}
	if got := bootstrapWarnings.Drain(); len(got) != 0 {
		t.Errorf("expected empty warnings sink for present saver, got %v", got)
	}
}

func TestEnsureSaverLiveness_NeverInvokesVersionGate(t *testing.T) {
	resetBootstrapWarnings(t)

	cmder := &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "list-panes" && isPanePIDProbe(args) {
				return "12345\n", nil // present + alive -> no revive path at all
			}
			t.Fatalf("unexpected tmux call: %v", args)
			return "", nil
		},
	}

	ensureSaverLiveness(tmux.NewClient(cmder), t.TempDir())

	// No kill-barrier / kill-session is ever issued by the liveness-only helper.
	if n := countOp(cmder.Calls, "kill-session"); n != 0 {
		t.Errorf("liveness-only helper must never kill-session, got %d: %v", n, cmder.Calls)
	}

	// Source-level guard: the abridged helper must not INVOKE the version-gate.
	// The identifier is intentionally named in the docstring (to document the
	// contract), so we match a call — identifier immediately followed by "(" —
	// rather than any mention.
	src, err := os.ReadFile("abridged_saver.go")
	if err != nil {
		t.Fatalf("read abridged_saver.go: %v", err)
	}
	if strings.Contains(string(src), "EnsurePortalSaverVersion(") {
		t.Error("abridged_saver.go must not call EnsurePortalSaverVersion (version-gate lives only in the full-bootstrap orchestrator)")
	}
}
