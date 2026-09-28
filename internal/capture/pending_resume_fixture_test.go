package capture_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

const (
	pendingResumeFixture           = "sessions-pending-resume"
	pendingResumeColourlessFixture = "sessions-pending-resume-colourless"
)

// One session per attached × pending combination the fixture seeds.
const (
	comboNeither      = "fabric-lk26UG"
	comboAttachedOnly = "agentic-workflows-codify"
	comboPendingOnly  = "evvi-sync-engine"
	comboBoth         = "folio-Jiz4el"
)

func pendingFixture(t *testing.T, name string) *capture.Fixture {
	t.Helper()
	fx, err := capture.FixtureByName(name)
	if err != nil {
		t.Fatalf("FixtureByName(%s): %v", name, err)
	}
	return fx
}

func renderDeps(t *testing.T, fx *capture.Fixture, deps tui.Deps) string {
	t.Helper()
	sessions, err := fx.Lister.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	var pending map[string]struct{}
	if deps.PendingReader != nil {
		view, err := deps.PendingReader.ListPendingResumePanes()
		if err != nil {
			t.Fatalf("ListPendingResumePanes: %v", err)
		}
		pending = view.Sessions
	}
	var model tea.Model = tui.Build(deps)
	next, _ := model.Update(tea.WindowSizeMsg{Width: harnessWidth, Height: harnessHeight})
	next, _ = next.Update(tui.SessionsMsg{Sessions: sessions, Pending: pending})
	return next.(tui.Model).View().Content
}

// rowLine is the raw frame line holding the named session's row.
func rowLine(t *testing.T, frame, session string) string {
	t.Helper()
	var found []string
	for line := range strings.SplitSeq(frame, "\n") {
		if strings.Contains(ansi.Strip(line), " "+session+" ") {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one row for %s, found %d in:\n%s", session, len(found), ansi.Strip(frame))
	}
	return found[0]
}

// trailingIndicators is the run of indicator cells closing the row's visible
// text, after its window count.
func trailingIndicators(line string) string {
	visible := strings.TrimRight(ansi.Strip(line), " ")
	_, after, ok := strings.Cut(visible, "window")
	if !ok {
		return visible
	}
	after = strings.TrimPrefix(after, "s")
	return strings.TrimLeft(after, " ")
}

func glyphColumn(line string) int {
	visible := ansi.Strip(line)
	return ansi.StringWidth(visible[:strings.Index(visible, "●")])
}

var sgrSequence = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// indicatorSGRs is, per ● in the raw line, the last SGR sequence emitted
// before it — the style that painted that glyph.
func indicatorSGRs(line string) []string {
	var out []string
	last := ""
	for rest := line; rest != ""; {
		loc := sgrSequence.FindStringIndex(rest)
		glyphAt := strings.Index(rest, "●")
		if glyphAt < 0 {
			break
		}
		if loc != nil && loc[0] < glyphAt {
			last = rest[loc[0]:loc[1]]
			rest = rest[loc[1]:]
			continue
		}
		out = append(out, last)
		rest = rest[glyphAt+len("●"):]
	}
	return out
}

func TestPendingResumeFixtures_EnumeratedByName(t *testing.T) {
	for _, name := range []string{pendingResumeFixture, pendingResumeColourlessFixture} {
		if !slices.Contains(capture.FixtureNames(), name) {
			t.Errorf("FixtureNames() %v does not include %s", capture.FixtureNames(), name)
		}
		if capture.IsStandalone(name) {
			t.Errorf("%s is listed as standalone; it must be an ordinary registry fixture", name)
		}
		if fx := pendingFixture(t, name); fx.Name() != name {
			t.Errorf("FixtureByName(%s) returned the fixture named %s", name, fx.Name())
		}
	}
}

func TestPendingResumeFixture_RendersAllFourCombinationsInOneFrame(t *testing.T) {
	th := darkBuiltinTheme(t)
	frame := pendingFixture(t, pendingResumeFixture).ModelAt(th, harnessWidth, harnessHeight).View().Content
	positive, attention := fgSeq(t, th.StatePositive), fgSeq(t, th.AccentAttention)

	cases := []struct {
		session string
		glyphs  string
		colours []string
	}{
		{comboNeither, "", nil},
		{comboAttachedOnly, "●", []string{positive}},
		{comboPendingOnly, "●", []string{attention}},
		{comboBoth, "●●", []string{positive, attention}},
	}
	for _, c := range cases {
		line := rowLine(t, frame, c.session)
		if got := trailingIndicators(line); got != c.glyphs {
			t.Errorf("%s: trailing indicators = %q, want %q: %q", c.session, got, c.glyphs, ansi.Strip(line))
		}
		sgrs := indicatorSGRs(line)
		if len(sgrs) != len(c.colours) {
			t.Errorf("%s: %d painted indicators, want %d", c.session, len(sgrs), len(c.colours))
			continue
		}
		for i, run := range c.colours {
			if !carriesRun(sgrs[i], run) {
				t.Errorf("%s: indicator %d painted by %q, want the fg run %q", c.session, i, sgrs[i], run)
			}
		}
	}

	attachedCol := glyphColumn(rowLine(t, frame, comboAttachedOnly))
	pendingCol := glyphColumn(rowLine(t, frame, comboPendingOnly))
	if attachedCol != pendingCol {
		t.Errorf("a lone attached dot sits at column %d and a lone pending dot at %d; both take the rightmost cell", attachedCol, pendingCol)
	}
}

func TestPendingResumeFixture_RendersTheProductionDelegate(t *testing.T) {
	th := darkBuiltinTheme(t)
	fx := pendingFixture(t, pendingResumeFixture)
	deps := fx.Deps(th)
	if deps.PendingReader == nil {
		t.Fatal("Deps().PendingReader is nil; the pending state must reach the model through the production seam")
	}
	if deps.Capture.Flash != "" || deps.Command != nil || len(deps.Capture.MultiSelect) != 0 {
		t.Errorf("the fixture seeds text or selection beside the pending state: flash=%q command=%v multi=%v", deps.Capture.Flash, deps.Command, deps.Capture.MultiSelect)
	}

	withReader := fx.ModelAt(th, harnessWidth, harnessHeight).View().Content
	if got := renderDeps(t, fx, deps); got != withReader {
		t.Errorf("the fixture frame differs from tui.Build over its own Deps:\nfixture:\n%s\nbuild:\n%s", ansi.Strip(withReader), ansi.Strip(got))
	}

	deps.PendingReader = nil
	withoutReader := renderDeps(t, fx, deps)
	for _, session := range []string{comboNeither, comboAttachedOnly} {
		if a, b := rowLine(t, withReader, session), rowLine(t, withoutReader, session); a != b {
			t.Errorf("%s: row changes when the reader is withdrawn, but it is not pending:\n%q\n%q", session, a, b)
		}
	}
	for _, session := range []string{comboPendingOnly, comboBoth} {
		if a, b := rowLine(t, withReader, session), rowLine(t, withoutReader, session); a == b {
			t.Errorf("%s: row is unchanged when the reader is withdrawn; its pending dot did not come through the reader", session)
		}
	}
	if got := trailingIndicators(rowLine(t, withoutReader, comboBoth)); got != "●" {
		t.Errorf("%s without the reader carries %q, want the attached dot alone", comboBoth, got)
	}
}

func TestPendingResumeColourlessFixture_RendersLetters(t *testing.T) {
	fx := pendingFixture(t, pendingResumeColourlessFixture)
	frame := fx.ModelAt(darkBuiltinTheme(t), harnessWidth, harnessHeight).View().Content

	cases := []struct {
		session string
		letters string
	}{
		{comboNeither, ""},
		{comboAttachedOnly, "A"},
		{comboPendingOnly, "P"},
		{comboBoth, "AP"},
	}
	for _, c := range cases {
		line := rowLine(t, frame, c.session)
		if got := trailingIndicators(line); got != c.letters {
			t.Errorf("%s: trailing indicators = %q, want %q: %q", c.session, got, c.letters, ansi.Strip(line))
		}
	}
	if strings.Contains(ansi.Strip(frame), "●") {
		t.Errorf("colourless frame carries a ● indicator:\n%s", ansi.Strip(frame))
	}
}

func TestPendingResumeColourlessFixture_ReportsColourless(t *testing.T) {
	colourless := pendingFixture(t, pendingResumeColourlessFixture)
	if !colourless.Colourless() || !colourless.Deps(theme.Theme{}).NoColor {
		t.Errorf("%s: Colourless() = %t, Deps().NoColor = %t; want both true", pendingResumeColourlessFixture, colourless.Colourless(), colourless.Deps(theme.Theme{}).NoColor)
	}
	coloured := pendingFixture(t, pendingResumeFixture)
	if coloured.Colourless() {
		t.Errorf("%s reports colourless; it must join the palette-swap guard", pendingResumeFixture)
	}
}

func TestPendingResumeFixtures_SeedNoToolName(t *testing.T) {
	for _, name := range []string{pendingResumeFixture, pendingResumeColourlessFixture} {
		sessions, err := pendingFixture(t, name).Lister.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		for _, s := range sessions {
			for _, banned := range []string{"claude", "fable", "brewed", "codex", "cursor", "aider"} {
				if strings.Contains(strings.ToLower(s.Name), banned) {
					t.Errorf("%s: session %q names a tool", name, s.Name)
				}
			}
			if s.Dir == "" {
				t.Errorf("%s: session %q carries no Dir; the grouped fallback would issue a pane read", name, s.Name)
			}
		}
	}
}

func TestFixtures_WithoutPendingSessionsWireNoReader(t *testing.T) {
	pendingBearing := []string{pendingResumeFixture, pendingResumeColourlessFixture}
	for _, name := range buildBackedFixtureNames() {
		if slices.Contains(pendingBearing, name) {
			continue
		}
		if reader := pendingFixture(t, name).Deps(theme.Theme{}).PendingReader; reader != nil {
			t.Errorf("%s declares no pending sessions but wires the reader %#v", name, reader)
		}
	}
}
