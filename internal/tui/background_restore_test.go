package tui

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui/theme"
)

// initCmds executes a tea.Cmd and returns every leaf message it produces,
// recursively draining tea.BatchMsg. Used to inspect what Init batched without
// coupling to the batch shape.
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

// initReturnsBatch reports whether the model's Init returns a tea.BatchMsg —
// the observable structural consequence of folding the async OSC 11
// background-color query into Init. Before the query, several Init paths
// returned a bare single-shot data-load cmd; now every path batches (query +
// data load), so a tea.BatchMsg top-level result is the necessary signature of
// the query being present. TestInit_BackgroundQueryYieldsOSC11 then confirms
// the batched cmd actually IS the OSC 11 query (not some other addition).
func initReturnsBatch(t *testing.T, m Model) bool {
	t.Helper()
	cmd := m.Init()
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.BatchMsg)
	return ok
}

// TestInit_BatchesBackgroundColorQuery asserts every Init path returns a batch
// (never a bare single-shot data-load cmd), which is the observable consequence
// of folding the async OSC 11 background-color query (tea.RequestBackgroundColor)
// into Init. The query is fire-and-forget capture for restore-on-exit; it must
// always be issued so the original background can be restored on quit.
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
		m := New(fakeLister{}, WithServerStarted(true))
		if !initReturnsBatch(t, m) {
			t.Error("Init did not batch the background-color query on the loading path")
		}
	})
}

// TestInit_BackgroundQueryYieldsOSC11 runs the cmds Init batched and asserts one
// of them IS the tea.RequestBackgroundColor query, pinning that the OSC 11 query
// Cmd (not some other addition) is what was batched.
//
// tea.RequestBackgroundColor's cmd emits an UNEXPORTED internal request marker
// (tea.backgroundColorMsg{}) — it is the program runtime, not the cmd, that
// turns that marker into the public tea.BackgroundColorMsg response. So a
// headless harness can only match the marker by its type string. We assert the
// query against a reference taken from tea.RequestBackgroundColor itself, so the
// match cannot drift if the marker's name changes.
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

// TestBackgroundColorMsg_StoresHex routes a tea.BackgroundColorMsg through Update
// and asserts the terminal's reported background is stored as a hex string on
// the model, surfaced via OriginalBackground().
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

// TestBackgroundColorMsg_NilColorLeavesEmpty asserts a no-answer (nil color)
// BackgroundColorMsg leaves OriginalBackground() empty — the helper then writes
// nothing and Bubble Tea's own OSC 111 reset stands.
func TestBackgroundColorMsg_NilColorLeavesEmpty(t *testing.T) {
	m := New(fakeLister{})
	updated, _ := m.Update(tea.BackgroundColorMsg{Color: nil})
	if got := updated.(Model).OriginalBackground(); got != "" {
		t.Errorf("OriginalBackground() = %q after nil-color msg, want empty", got)
	}
}

// TestBackgroundColorMsg_DoesNotChangeRenderedFrame is the determinism guard:
// the async OSC 11 capture must NOT alter the rendered frame, or the vhs
// captures would stop being byte-deterministic. The composed View().Content is
// asserted byte-identical before and after a BackgroundColorMsg is routed
// through Update.
func TestBackgroundColorMsg_DoesNotChangeRenderedFrame(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		const w, h = 90, 24
		base := newCanvasTestModel(t, w, h, mode)
		before := base.View().Content

		updated, _ := base.Update(tea.BackgroundColorMsg{
			Color: color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
		})
		after := updated.(Model).View().Content

		if before != after {
			t.Errorf("mode %v: View().Content changed after a BackgroundColorMsg — the async capture must not perturb the frame (determinism)", mode)
		}
	}
}

// TestFirstPaint_NotGatedOnBackgroundQuery is the non-gating parity proof: the
// model produces its first View identically whether or not a BackgroundColorMsg
// has arrived. The first paint must NOT wait on the OSC 11 response (the
// detect-or-timeout first-paint gate is a later task), so a model with no
// response and a model that received one render the same frame.
func TestFirstPaint_NotGatedOnBackgroundQuery(t *testing.T) {
	const w, h = 90, 24

	// No response ever arrives.
	noResponse := newCanvasTestModel(t, w, h, theme.Dark).View().Content

	// A response arrives (the only difference is originalBg is captured).
	m := newCanvasTestModel(t, w, h, theme.Dark)
	updated, _ := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
	})
	withResponse := updated.(Model).View().Content

	if noResponse != withResponse {
		t.Error("first View differs with vs without a BackgroundColorMsg — the first paint is gated on the OSC 11 response (it must not be)")
	}
	// And the captured value is distinct from the painted canvas: originalBg is
	// the terminal's actual bg (for restore), canvasMode drives the canvas.
	if updated.(Model).OriginalBackground() == "" {
		t.Error("OriginalBackground() empty after a response — capture did not store the original bg")
	}
}

// TestRestoreTerminalBackground_WritesSetBack asserts the shared restore helper
// writes exactly the OSC 11 SET sequence for the captured original background —
// restoring by setting the original back (not relying on Bubble Tea's OSC 111
// reset, which mosh/Blink ignore).
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

// TestRestoreTerminalBackground_EmptyWritesNothing asserts the helper is a no-op
// when no OSC 11 response was captured — best-effort fallback to Bubble Tea's
// own OSC 111 reset, never a stray write.
func TestRestoreTerminalBackground_EmptyWritesNothing(t *testing.T) {
	m := New(fakeLister{}) // never received a BackgroundColorMsg

	var buf strings.Builder
	RestoreTerminalBackground(&buf, m)

	if got := buf.String(); got != "" {
		t.Errorf("RestoreTerminalBackground wrote %q for an empty original, want nothing", got)
	}
}

// stubProjectStore / stubCreator are minimal no-op seams letting New build a
// command-pending model (which needs a project store + creator wired).
type stubProjectStore struct{}

func (stubProjectStore) List() ([]project.Project, error)       { return nil, nil }
func (stubProjectStore) CleanStale() ([]project.Project, error) { return nil, nil }
func (stubProjectStore) Remove(path, via string) error          { return nil }

type stubCreator struct{}

func (stubCreator) CreateFromDir(dir string, command []string) (string, error) { return "", nil }
