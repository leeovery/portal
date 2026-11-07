package capture_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// stillFrame is the fixture's captured screen as text, with the model it was
// drawn from for the page it landed on.
func stillFrame(t *testing.T, fixture string) (tui.Model, string) {
	t.Helper()
	fx, err := capture.FixtureByName(fixture)
	if err != nil {
		t.Fatalf("FixtureByName(%s): %v", fixture, err)
	}
	m := fx.ModelAt(darkBuiltinTheme(t), harnessWidth, harnessHeight)
	return m, ansi.Strip(m.View().Content)
}

// assertLineHolds fails unless one line of the frame carries every part, in
// order, so a key and its description are known to share a row.
func assertLineHolds(t *testing.T, frame string, parts ...string) {
	t.Helper()
	for line := range strings.SplitSeq(frame, "\n") {
		if holdsInOrder(line, parts) {
			return
		}
	}
	t.Errorf("no line carries %q in order:\n%s", parts, frame)
}

func holdsInOrder(line string, parts []string) bool {
	for _, part := range parts {
		i := strings.Index(line, part)
		if i < 0 {
			return false
		}
		line = line[i+len(part):]
	}
	return true
}

func TestSessionsHelpFixture_DrawsTheKeybindingsModal(t *testing.T) {
	_, frame := stillFrame(t, "sessions-help")

	assertLineHolds(t, frame, "? Keybindings", "esc close")
	assertLineHolds(t, frame, "/", "Filter sessions")
	assertLineHolds(t, frame, "x", "Switch to Projects")
	if strings.Contains(frame, "orchard-pipeline-code-based") {
		t.Errorf("the modal draws over the sessions page alone, yet a session row shows:\n%s", frame)
	}
}

func TestProjectsEditFixture_DrawsTheSeededProjectsChips(t *testing.T) {
	m, frame := stillFrame(t, "projects-edit")

	if m.ActivePage() != tui.PageProjects {
		t.Errorf("ActivePage() = %d, want the Projects page the modal opens over", m.ActivePage())
	}
	assertLineHolds(t, frame, "Edit Project", "relay-v1-api")
	assertLineHolds(t, frame, "│ relay-v1-api", "│")
	assertLineHolds(t, frame, "│ rapi │", "│ v1 │", "+ add")
	assertLineHolds(t, frame, "│ Loom │", "│ api │", "+ add")
}

func TestSessionsFilteringFixture_DrawsTheTypedTermAndItsMatches(t *testing.T) {
	m, frame := stillFrame(t, "sessions-filtering")

	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want the Sessions page being filtered", m.ActivePage())
	}
	assertLineHolds(t, frame, "/ relay")
	assertLineHolds(t, frame, "type to filter")
	for _, row := range []string{"relay-v1-api-Hq7wNe", "relayx-3RJDWA", "kiln-relayx-explore"} {
		if !strings.Contains(frame, row) {
			t.Errorf("the filtered list is missing the matching row %q:\n%s", row, frame)
		}
	}
	for _, row := range []string{"orchard-pipeline-code-based", "lumen-sync-engine", "kiln-aws-migration"} {
		if strings.Contains(frame, row) {
			t.Errorf("the filtered list still shows %q, which the term does not match:\n%s", row, frame)
		}
	}
}

// The README's still of the panel lists the six built-ins and nothing else,
// the constant tokyo-night marked and under the cursor, over the flat page.
func TestThemePanelOpenFixture_DrawsTheBuiltInsWithTheCursorOnTheThemePainted(t *testing.T) {
	m, frame := stillFrame(t, "theme-panel-open")

	if m.ActivePage() != tui.PageSessions {
		t.Errorf("ActivePage() = %d, want the Sessions page the panel opens over", m.ActivePage())
	}
	assertLineHolds(t, frame, "Sessions 12", "│ Themes")
	if listed := panelListRows(frame); !slices.Equal(listed, theme.BuiltinSlugs()) {
		t.Errorf("the panel lists %q, want the built-ins alone, %q:\n%s", listed, theme.BuiltinSlugs(), frame)
	}
	assertLineHolds(t, frame, "│ ▌ tokyo-night ", "●")
	if strings.Contains(frame, "⚠") || strings.Contains(frame, "● light") || strings.Contains(frame, "● dark") {
		t.Errorf("the panel carries a rejection or a pair's badge, want a constant on a clean list:\n%s", frame)
	}
}

// panelListRows are the names the theme panel lists, in order: its rows from
// the blank line under its heading to the next blank one.
func panelListRows(frame string) []string {
	var rows []string
	headed, opened := false, false
	for line := range strings.SplitSeq(frame, "\n") {
		_, panel, ok := strings.Cut(line, "│")
		cells := strings.Fields(strings.TrimPrefix(strings.TrimSpace(panel), "▌"))
		switch {
		case !ok:
		case !headed:
			headed = len(cells) > 0 && cells[0] == "Themes"
		case len(cells) == 0 && opened:
			return rows
		case len(cells) == 0:
			opened = true
		default:
			rows = append(rows, cells[0])
		}
	}
	return rows
}
