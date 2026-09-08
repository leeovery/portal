package restore_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// The fixtures in this package script the argv the restorer issues, so an argv
// it was not expected to issue must fail the test rather than be answered. This
// pins that the fixture form they are built in does that.
func TestConvertedFixturesReportAnUnscriptedArgv(t *testing.T) {
	rec := &harnesstest.Recorder{}
	mock := commandertest.New(rec, commandertest.Returns("", "set-option"))
	r := &restore.SessionRestorer{Client: tmux.NewClient(mock)}

	sess := state.Session{Name: "work", Windows: []state.Window{
		{Index: 0, Layout: "L", Panes: []state.Pane{{Index: 0, Active: true}}},
	}}

	r.ApplyWindowGeometry(sess, []tmux.PaneCoord{{Window: 0, Pane: 0}})

	if len(rec.Errors) == 0 {
		t.Fatal("the fixture answered every geometry argv without reporting one it never scripted")
	}
	if !strings.Contains(strings.Join(rec.Errors, "\n"), "select-layout") {
		t.Errorf("reports %v do not name the unscripted argv", rec.Errors)
	}
}
