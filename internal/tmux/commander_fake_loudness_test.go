package tmux_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/tmux"
)

// The fixtures in this package script the argv their subject issues, so an argv
// the subject was not expected to issue must fail the test rather than be
// answered. This pins that the fixture form they are built in does that.
func TestConvertedFixturesReportAnUnscriptedArgv(t *testing.T) {
	rec := &harnesstest.Recorder{}
	mock := commandertest.New(rec, commandertest.Returns("dev|1|0|", "list-sessions"))
	client := tmux.NewClient(mock)

	err := client.KillSession("work")

	if err == nil {
		t.Error("KillSession returned a nil error for an argv the fixture never scripted")
	}
	if len(rec.Errors) != 1 {
		t.Fatalf("the fixture reported %d failures, want 1: %v", len(rec.Errors), rec.Errors)
	}
	if !strings.Contains(rec.Errors[0], "kill-session") {
		t.Errorf("report %q does not name the unscripted argv", rec.Errors[0])
	}
}
