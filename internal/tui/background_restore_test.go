package tui

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
)

func initCmds(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, initCmds(t, c)...)
	}
	return out
}

func initReturnsBatch(t *testing.T, m Model) bool {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.BatchMsg)
	return ok
}

func TestInit_BatchesBackgroundColorQuery(t *testing.T) {
	t.Run("sessions path (no project store)", func(t *testing.T) {
		m := New(fakeLister{})
		if !initReturnsBatch(t, m) {
			t.Error("Init did not batch the background-color query on the sessions path")
		}
	})

	t.Run("command-pending path", func(t *testing.T) {
		m := New(fakeLister{}, WithProjectStore(stubProjectStore{}), WithSessionCreator(stubCreator{})).
			WithCommand([]string{"echo"})
		if !initReturnsBatch(t, m) {
			t.Error("Init did not batch the background-color query on the command-pending path")
		}
	})

	t.Run("loading path (server started)", func(t *testing.T) {
		m := New(fakeLister{}, WithBootstrapping(idleBootstrap))
		if !initReturnsBatch(t, m) {
			t.Error("Init did not batch the background-color query on the loading path")
		}
	})
}

func TestInit_BackgroundQueryYieldsOSC11(t *testing.T) {
	wantType := reflect.TypeOf(tea.Cmd(tea.RequestBackgroundColor)())

	m := New(fakeLister{})
	msgs := initCmds(t, m.Init())

	found := false
	for _, msg := range msgs {
		if reflect.TypeOf(msg) == wantType {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Init batch did not include the tea.RequestBackgroundColor query (no %v produced)", wantType)
	}
}

func TestBackgroundColorMsg_StoresHex(t *testing.T) {
	m := New(fakeLister{})
	if got := m.OriginalBackground(); got != "" {
		t.Fatalf("OriginalBackground() = %q before any query response, want empty", got)
	}

	updated, _ := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
	})
	got := updated.(Model).OriginalBackground()
	if got != "#1e1e2e" {
		t.Errorf("OriginalBackground() = %q, want %q", got, "#1e1e2e")
	}
}

func TestBackgroundColorMsg_NilColorLeavesEmpty(t *testing.T) {
	m := New(fakeLister{})
	updated, _ := m.Update(tea.BackgroundColorMsg{Color: nil})
	if got := updated.(Model).OriginalBackground(); got != "" {
		t.Errorf("OriginalBackground() = %q after nil-color msg, want empty", got)
	}
}

func TestBackgroundColorMsg_DoesNotChangeRenderedFrame(t *testing.T) {
	for _, appearance := range []theme.Member{theme.MemberDark, theme.MemberLight} {
		const w, h = 90, 24
		base := newCanvasTestModel(t, w, h, appearance)
		before := base.View().Content

		updated, _ := base.Update(tea.BackgroundColorMsg{
			Color: color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
		})
		after := updated.(Model).View().Content

		if before != after {
			t.Errorf("canvas %v: View().Content changed after a BackgroundColorMsg — the async capture must not perturb the frame (determinism)", appearance)
		}
	}
}

func TestFirstPaint_NotGatedOnBackgroundQuery(t *testing.T) {
	const w, h = 90, 24

	noResponse := newCanvasTestModel(t, w, h, theme.MemberDark).View().Content

	m := newCanvasTestModel(t, w, h, theme.MemberDark)
	updated, _ := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
	})
	withResponse := updated.(Model).View().Content

	if noResponse != withResponse {
		t.Error("first View differs with vs without a BackgroundColorMsg — the first paint is gated on the OSC 11 response (it must not be)")
	}
	if updated.(Model).OriginalBackground() == "" {
		t.Error("OriginalBackground() empty after a response — capture did not store the original bg")
	}
}

func TestRestoreTerminalBackground_WritesSetBack(t *testing.T) {
	m := New(fakeLister{})
	updated, _ := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
	})

	var buf strings.Builder
	RestoreTerminalBackground(&buf, updated.(Model))

	want := ansi.SetBackgroundColor("#1e1e2e")
	if got := buf.String(); got != want {
		t.Errorf("RestoreTerminalBackground wrote %q, want %q", got, want)
	}
}

func TestRestoreTerminalBackground_EmptyWritesNothing(t *testing.T) {
	m := New(fakeLister{})

	var buf strings.Builder
	RestoreTerminalBackground(&buf, m)

	if got := buf.String(); got != "" {
		t.Errorf("RestoreTerminalBackground wrote %q for an empty original, want nothing", got)
	}
}

type stubProjectStore struct{}

func (stubProjectStore) List() ([]project.Project, error)       { return nil, nil }
func (stubProjectStore) CleanStale() ([]project.Project, error) { return nil, nil }
func (stubProjectStore) Remove(path, via string) error          { return nil }

type stubCreator struct{}

func (stubCreator) CreateFromDir(dir string, command []string) (string, error) { return "", nil }
