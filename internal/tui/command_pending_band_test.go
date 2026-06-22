package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui/theme"
)

// recordingCreator captures the dir + command CreateFromDir is called with, so the
// dispatch-parity test can confirm Enter/n route to the unchanged handlers.
type recordingCreator struct {
	dir     string
	command []string
}

func (c *recordingCreator) CreateFromDir(dir string, command []string) (string, error) {
	c.dir = dir
	c.command = command
	return "myapp-abc123", nil
}

// Tests for task 4-4: the §11.4 command-pending banner reskin. The plain
// `Select project to run: <cmd>` status line is replaced by a §11 violet INFO
// band — a violet `▌` left-bar + a `▸` caret + the fixed text `Pick a project to
// run` + the joined command in an accent.orange chip — on a subtle tinted band,
// over the FULL Projects chrome (green `Projects N` header + `/ to filter`). The
// footer swaps to `enter run here · n run in cwd · esc cancel`. Dispatch is
// unchanged (Enter → run here, n → run in cwd, Esc → cancel).
//
// No t.Parallel() — the package's shared canvas/mock helpers make parallelism
// unsafe across these tests.

// newCommandPendingTestModel builds a Model on the command-pending Projects page
// seeded with the given projects + command, sized so the band/footer budgets are
// applied. WithCommand sets m.commandPending and the active page to PageProjects.
func newCommandPendingTestModel(t *testing.T, w, h int, projects []project.Project, command []string) Model {
	t.Helper()
	m := New(fakeLister{}).WithCommand(command)
	m.termWidth = w
	m.termHeight = h
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	return m
}

// TestCommandBand_VioletBarCaretTextAndOrangeChip asserts the command band renders
// as a violet `▌` left-bar + a `▸` caret + the fixed `Pick a project to run` text +
// the joined command in an accent.orange chip.
func TestCommandBand_VioletBarCaretTextAndOrangeChip(t *testing.T) {
	const w = 80
	band := renderCommandBand([]string{"npm", "run", "dev"}, w, theme.Dark, false)

	stripped := ansi.Strip(band)
	// Far-left ▌ bar.
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("command band does not start with the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	// The `▸` caret follows the bar.
	if !strings.Contains(stripped, commandBandCaret) {
		t.Errorf("command band missing the %q caret: %q", commandBandCaret, stripped)
	}
	// The fixed banner text.
	if !strings.Contains(stripped, commandBandText) {
		t.Errorf("command band missing the fixed text %q: %q", commandBandText, stripped)
	}
	// The joined command renders in the band (the chip).
	if !strings.Contains(stripped, "npm run dev") {
		t.Errorf("command band missing the joined command %q: %q", "npm run dev", stripped)
	}

	// Bar colour = accent.violet (§2.9).
	violetSeq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark)
	if !strings.Contains(band, violetSeq) {
		t.Errorf("command band missing the accent.violet bar foreground sequence %q:\n%s", violetSeq, band)
	}
	// Chip command colour = accent.orange (§2.9 / §11.4).
	orangeSeq := tokenFgSeq(t, theme.MV.AccentOrange, theme.Dark)
	if !strings.Contains(band, orangeSeq) {
		t.Errorf("command band missing the accent.orange chip foreground sequence %q:\n%s", orangeSeq, band)
	}
}

// TestCommandBand_JoinsCommandSlice asserts the command slice joins on spaces into
// the orange chip (strings.Join(command, " ")).
func TestCommandBand_JoinsCommandSlice(t *testing.T) {
	const w = 90
	band := renderCommandBand([]string{"go", "test", "./..."}, w, theme.Dark, false)
	stripped := ansi.Strip(band)
	if !strings.Contains(stripped, "go test ./...") {
		t.Errorf("command band must join the slice on spaces: %q", stripped)
	}
}

// TestCommandBand_FixedTextConstant pins the banner wording to the spec-exact §11.4
// string, sourced as a single constant (commandBandText).
func TestCommandBand_FixedTextConstant(t *testing.T) {
	const want = "Pick a project to run"
	if commandBandText != want {
		t.Errorf("commandBandText = %q, want the spec-exact wording %q", commandBandText, want)
	}
}

// TestCommandBand_Tinted asserts the band carries a tint (the bg.selection subtle
// surface, distinguishing the §11.4 command band from the tint-less §11.3 signpost).
func TestCommandBand_Tinted(t *testing.T) {
	const w = 80
	band := renderCommandBand([]string{"npm", "run", "dev"}, w, theme.Dark, false)
	tintSeq := tokenBgSeq(t, theme.MV.BgSelection, theme.Dark)
	if !strings.Contains(band, tintSeq) {
		t.Errorf("command band missing the bg.selection tint %q:\n%s", tintSeq, band)
	}
	// Single full-width line for a short command.
	if got := lipgloss.Width(band); got != w {
		t.Errorf("command band width = %d, want %d (full width)", got, w)
	}
}

// TestCommandBand_NoColorKeepsBarCaretAndChip asserts the NO_COLOR carve-out
// (§2.5): under colourless the band drops the tint + bar colour + chip colour but
// keeps the `▌` bar, the `▸` caret, the text, and the chip command — and carries no
// SGR colour sequences at all.
func TestCommandBand_NoColorKeepsBarCaretAndChip(t *testing.T) {
	const w = 80
	band := renderCommandBand([]string{"npm", "run", "dev"}, w, theme.Dark, true)

	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("NO_COLOR command band must keep the far-left %q bar: %q", noticeBarGlyph, stripped)
	}
	if !strings.Contains(stripped, commandBandCaret) {
		t.Errorf("NO_COLOR command band must keep the %q caret: %q", commandBandCaret, stripped)
	}
	if !strings.Contains(stripped, commandBandText) {
		t.Errorf("NO_COLOR command band must keep the text %q: %q", commandBandText, stripped)
	}
	if !strings.Contains(stripped, "npm run dev") {
		t.Errorf("NO_COLOR command band must keep the chip command %q: %q", "npm run dev", stripped)
	}
	if band != stripped {
		t.Errorf("NO_COLOR command band must carry no SGR colour sequences; got raw %q", band)
	}
}

// TestCommandBandRole_BarAndTintTokens asserts the bandCommand role selects the
// accent.violet bar token and the bg.selection tint token from the closed §2.9
// vocabulary (no literal hex at the call site).
func TestCommandBandRole_BarAndTintTokens(t *testing.T) {
	if got := bandCommand.barToken().Name; got != theme.MV.AccentViolet.Name {
		t.Errorf("bandCommand bar token = %q, want accent.violet", got)
	}
	if got := bandCommand.tintToken().Name; got != theme.MV.BgSelection.Name {
		t.Errorf("bandCommand tint token = %q, want bg.selection", got)
	}
}

// TestViewProjectList_CommandPendingBandOverFullChrome asserts the command-pending
// Projects view renders the band (violet bar + caret + text + chip) AND keeps the
// FULL Projects chrome (green `Projects N` header + `/ to filter`) — the page is not
// stripped, the banner sits on top under the title separator.
func TestViewProjectList_CommandPendingBandOverFullChrome(t *testing.T) {
	m := newCommandPendingTestModel(t, 90, 30, sampleProjects(), []string{"npm", "run", "dev"})
	view := m.viewProjectList()
	visible := ansi.Strip(view)

	// The banner: fixed text + the joined command chip.
	if !strings.Contains(visible, commandBandText) {
		t.Errorf("command-pending view missing the banner text %q:\n%s", commandBandText, visible)
	}
	if !strings.Contains(visible, "npm run dev") {
		t.Errorf("command-pending view missing the joined command chip %q:\n%s", "npm run dev", visible)
	}
	// FULL Projects chrome preserved: green section label + count + filter hint.
	if !strings.Contains(visible, "Projects") {
		t.Errorf("command-pending view missing the Projects section header (chrome stripped?):\n%s", visible)
	}
	if seq := tokenFgSeq(t, theme.MV.StateGreen, theme.Dark); !strings.Contains(view, seq) {
		t.Errorf("command-pending view missing the state.green section label role sequence %q", seq)
	}
	if !strings.Contains(visible, sectionFilterHint) {
		t.Errorf("command-pending view missing the %q hint (chrome stripped?):\n%s", sectionFilterHint, visible)
	}
	// PORTAL header block preserved.
	if !strings.Contains(visible, "P O R T A L") {
		t.Errorf("command-pending view missing the PORTAL wordmark (chrome stripped?):\n%s", visible)
	}
	// The legacy plain status-line wording must be gone.
	if strings.Contains(visible, "Select project to run") {
		t.Errorf("command-pending view leaked the legacy plain status line:\n%s", visible)
	}
}

// TestViewProjectList_CommandBandUnderSeparatorAboveSectionHeader asserts the band
// sits directly under the title separator and ABOVE the green section header, with
// ONE blank breathing row between them (band → blank → section header).
func TestViewProjectList_CommandBandUnderSeparatorAboveSectionHeader(t *testing.T) {
	m := newCommandPendingTestModel(t, 90, 30, sampleProjects(), []string{"npm", "run", "dev"})
	lines := strings.Split(ansi.Strip(m.viewProjectList()), "\n")

	ruleIdx := -1
	for i, l := range lines {
		if strings.Contains(l, strings.Repeat(headerRuleGlyph, 4)) {
			ruleIdx = i
			break
		}
	}
	bandIdx := lineIndexContaining(lines, commandBandText)
	sectionIdx := lineIndexContaining(lines, "Projects")
	if ruleIdx < 0 || bandIdx < 0 || sectionIdx < 0 {
		t.Fatalf("missing a landmark: rule=%d band=%d section=%d\n%s", ruleIdx, bandIdx, sectionIdx, strings.Join(lines, "\n"))
	}
	if bandIdx <= ruleIdx {
		t.Errorf("band index %d must be > separator-rule index %d (band under the separator)", bandIdx, ruleIdx)
	}
	if sectionIdx <= bandIdx {
		t.Errorf("section header index %d must be > band index %d (band above the section header)", sectionIdx, bandIdx)
	}
	if sectionIdx-bandIdx != 2 {
		t.Errorf("section header is %d rows below the band, want 2 (band → blank → section header)", sectionIdx-bandIdx)
	}
	blank := lines[bandIdx+1]
	if strings.TrimSpace(blank) != "" {
		t.Errorf("row between the band and section header must be blank, got %q", blank)
	}
}

// TestProjectBandHeight_TracksRenderedSlot asserts projectBandHeight reserves the
// band + blank breathing row while command-pending, and zero otherwise — measured
// off the SAME slot the view composes (the F10 recompute).
func TestProjectBandHeight_TracksRenderedSlot(t *testing.T) {
	withBand := newCommandPendingTestModel(t, 90, 30, sampleProjects(), []string{"npm", "run", "dev"})
	slotHeight := lipgloss.Height(withBand.renderProjectBandSlot())
	if slotHeight < 2 {
		t.Fatalf("command band slot height = %d, want >=2 (band + blank)", slotHeight)
	}
	if got := withBand.projectBandHeight(); got != slotHeight {
		t.Errorf("projectBandHeight = %d, want %d (measured off the rendered slot)", got, slotHeight)
	}

	noBand := newProjectsPageTestModel(t, 90, 30, theme.Dark, sampleProjects())
	if got := noBand.projectBandHeight(); got != 0 {
		t.Errorf("projectBandHeight (not command-pending) = %d, want 0", got)
	}
}

// TestCommandPendingFooter_SwappedCopy asserts the command-pending Projects footer
// reads `enter run here · n run in cwd · esc cancel` (the §11.4 swapped copy), with
// the `? help` right anchor retained and `quit` deferred.
func TestCommandPendingFooter_SwappedCopy(t *testing.T) {
	m := newCommandPendingTestModel(t, 160, 30, sampleProjects(), []string{"npm", "run", "dev"})
	visible := ansi.Strip(m.viewProjectList())

	for _, want := range []string{"run here", "run in cwd", "cancel", "help"} {
		if !strings.Contains(visible, want) {
			t.Errorf("command-pending footer missing the §11.4 entry %q:\n%s", want, visible)
		}
	}
	// `quit` is deferred to ? help; the standard §6.3 copy must not leak in
	// command-pending mode.
	if strings.Contains(visible, "quit") {
		t.Errorf("command-pending footer must NOT contain 'quit' (deferred to ? help):\n%s", visible)
	}
	for _, banned := range []string{"new session", "new in cwd"} {
		if strings.Contains(visible, banned) {
			t.Errorf("command-pending footer leaked non-§11.4 copy %q:\n%s", banned, visible)
		}
	}
}

// TestCommandPending_DispatchParity asserts the reskin did NOT change dispatch:
// Enter still creates a session from the selected project (run here), n still
// creates a session in the cwd (run in cwd), and Esc still cancels (quits). The
// command slice is forwarded unchanged to CreateFromDir on both create paths.
func TestCommandPending_DispatchParity(t *testing.T) {
	command := []string{"npm", "run", "dev"}
	projects := []project.Project{{Path: "/code/myapp", Name: "myapp"}}

	build := func() (Model, *recordingCreator) {
		creator := &recordingCreator{}
		m := New(fakeLister{}, WithProjectStore(stubProjectStore{}), WithSessionCreator(creator)).WithCommand(command)
		m.cwd = "/code/cwd"
		m.setProjects(projects)
		m.projectList.SetItems(ProjectsToListItems(projects))
		return m, creator
	}

	t.Run("Enter dispatches run-here from the selected project", func(t *testing.T) {
		m, creator := build()
		_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Enter must dispatch a create command in command-pending mode")
		}
		cmd()
		if creator.dir != "/code/myapp" {
			t.Errorf("Enter ran here for dir %q, want the selected project /code/myapp", creator.dir)
		}
		if strings.Join(creator.command, " ") != strings.Join(command, " ") {
			t.Errorf("Enter forwarded command %v, want %v", creator.command, command)
		}
	})

	t.Run("n dispatches run-in-cwd", func(t *testing.T) {
		m, creator := build()
		_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("n must dispatch a create-in-cwd command in command-pending mode")
		}
		cmd()
		if creator.dir != "/code/cwd" {
			t.Errorf("n ran in dir %q, want the cwd /code/cwd", creator.dir)
		}
		if strings.Join(creator.command, " ") != strings.Join(command, " ") {
			t.Errorf("n forwarded command %v, want %v", creator.command, command)
		}
	})

	t.Run("Esc dispatches cancel (quit)", func(t *testing.T) {
		m, _ := build()
		_, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd == nil {
			t.Fatal("Esc must dispatch quit (cancel) in command-pending mode")
		}
		if msg := cmd(); msg == nil {
			t.Fatal("Esc cancel must produce a quit message")
		}
	})
}

// TestCommandPendingHelpKeys_Copy asserts commandPendingHelpKeys is the §11.4
// binding source for the swapped footer: enter run here · n run in cwd · esc cancel.
func TestCommandPendingHelpKeys_Copy(t *testing.T) {
	bindings := commandPendingHelpKeys()
	want := []struct{ key, help string }{
		{"enter", "run here"},
		{"n", "run in cwd"},
		{"esc", "cancel"},
	}
	if len(bindings) != len(want) {
		t.Fatalf("commandPendingHelpKeys returned %d bindings, want %d", len(bindings), len(want))
	}
	for i, w := range want {
		h := bindings[i].Help()
		if h.Key != w.key || h.Desc != w.help {
			t.Errorf("binding %d = (%q, %q), want (%q, %q)", i, h.Key, h.Desc, w.key, w.help)
		}
	}
}
