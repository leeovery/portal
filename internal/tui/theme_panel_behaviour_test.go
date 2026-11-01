package tui

import (
	"go/ast"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
)

// The production DirThemeSource with only Open replaced, so the unoverridden
// derivations stay production's own.
type behaviourThemeSource struct {
	theme.DirThemeSource
	enumeration theme.Enumeration
}

func newBehaviourThemeSource(entries []theme.Entry) *behaviourThemeSource {
	return &behaviourThemeSource{
		DirThemeSource: theme.DirThemeSource{Loader: theme.NewSilentLoader()},
		enumeration:    theme.Enumeration{Entries: entries},
	}
}

func (e *behaviourThemeSource) Open(keys theme.RawKeys) (theme.Enumeration, theme.Union) {
	return e.enumeration, e.Reassemble(e.enumeration, keys)
}

func behaviourFile(t *testing.T, slug string, palette int) theme.Entry {
	t.Helper()
	return theme.Entry{Filename: slug + ".theme", Slug: slug, Theme: arrowPalette(t, palette)}
}

func behaviourRejected(slug string, reason theme.Reason) theme.Entry {
	return theme.Entry{
		Filename:  slug + ".theme",
		Slug:      slug,
		Rejection: &theme.Rejection{Reason: reason},
	}
}

func behaviourBadName(filename string) theme.Entry {
	return theme.Entry{Filename: filename, Rejection: &theme.Rejection{Reason: theme.ReasonBadName}}
}

const (
	behaviourContentW = 96
	behaviourContentH = 26
)

func behaviourPanel(t *testing.T, entries []theme.Entry, keys theme.RawKeys) (Model, *fakeThemePersister) {
	t.Helper()
	return behaviourPanelAt(t, entries, keys, behaviourContentW, behaviourContentH)
}

func behaviourPanelAt(t *testing.T, entries []theme.Entry, keys theme.RawKeys, contentW, contentH int) (Model, *fakeThemePersister) {
	t.Helper()

	enumerator := newBehaviourThemeSource(entries)
	persister := &fakeThemePersister{}
	m := Build(Deps{
		Lister:         fakeLister{},
		Theme:          behaviourNomination(t, enumerator, keys),
		ThemeSource:    enumerator,
		ThemeKeys:      keys,
		ThemePersister: persister,
	})
	return openPanelForTest(t, m, contentW, contentH), persister
}

func behaviourNomination(t *testing.T, e *behaviourThemeSource, keys theme.RawKeys) theme.Nomination {
	t.Helper()

	resolution, err := e.Resolve(e.enumeration, keys)
	if err != nil {
		t.Fatalf("construction-time resolution of %+v: %v", keys, err)
	}
	inForce, ok := inForceSlot(resolution, theme.MemberDark)
	if !ok {
		t.Fatalf("construction-time resolution %+v names no slot in force", resolution)
	}
	return theme.ConstantNomination(inForce.Theme)
}

const themeBadgeGlyph = "●"

type renderedPanelRow struct {
	label   string
	trailer string
	cursor  bool
}

func renderedPanelRows(t *testing.T, m Model) []renderedPanelRow {
	t.Helper()

	if got := m.themePanel.list.Paginator.TotalPages; got != 1 {
		t.Fatalf("fixture: the panel paginates over %d pages, so the drawn body is not the whole union", got)
	}
	lines := themePanelLines(renderRecomputePanel(m))
	from := panelHeaderRowsOf(m) + themePanelDirRowHeight(m.themePanel.union.DirUnusable)

	rows := make([]renderedPanelRow, 0, len(m.themePanel.union.Rows))
	for i := range m.themePanel.union.Rows {
		if from+i >= len(lines) {
			t.Fatalf("the panel drew %d lines, want a body row at %d for union row %d", len(lines), from+i, i)
		}
		rows = append(rows, parseRenderedPanelRow(t, lines[from+i]))
	}
	return rows
}

// No fixture in this file carries whitespace in its label, so the first field is
// the label and the rest is the trailer. Labels in general hold no such
// guarantee — a bad-name row is labelled by its filename, rendered verbatim — so
// a filename fixture with a space would mis-split here.
func parseRenderedPanelRow(t *testing.T, line string) renderedPanelRow {
	t.Helper()

	prefix := themePanelContentPrefix()
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("panel line %q does not open with the border-and-gutter prefix %q", line, prefix)
	}
	content := []rune(strings.TrimPrefix(line, prefix))
	if len(content) < leftBarColumnWidth {
		t.Fatalf("panel line %q is shorter than the %d-cell cursor column", line, leftBarColumnWidth)
	}

	fields := strings.Fields(string(content[leftBarColumnWidth:]))
	row := renderedPanelRow{cursor: string(content[0]) == selectorBar}
	if len(fields) > 0 {
		row.label, row.trailer = fields[0], strings.Join(fields[1:], " ")
	}
	return row
}

func renderedPanelLabels(t *testing.T, m Model) []string {
	t.Helper()

	rows := renderedPanelRows(t, m)
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.label)
	}
	return labels
}

func requireRenderedOrder(t *testing.T, m Model, want ...string) {
	t.Helper()

	if got := renderedPanelLabels(t, m); !slices.Equal(got, want) {
		t.Errorf("the panel draws %v, want %v", got, want)
	}
}

func renderedPanelRowFor(t *testing.T, m Model, label string) renderedPanelRow {
	t.Helper()

	rows := renderedPanelRows(t, m)
	at := slices.IndexFunc(rows, func(row renderedPanelRow) bool { return row.label == label })
	if at < 0 {
		t.Fatalf("the panel draws no row labelled %q; it draws %v", label, renderedPanelLabels(t, m))
	}
	return rows[at]
}

func requireRenderedBadge(t *testing.T, m Model, label string, want theme.Badge) {
	t.Helper()

	row := renderedPanelRowFor(t, m, label)
	if want == theme.BadgeNone {
		if strings.Contains(row.trailer, themeBadgeGlyph) {
			t.Errorf("the %q row draws a badge in %q, want none — the `●` marks what is SET", label, row.trailer)
		}
		return
	}
	if !strings.HasSuffix(row.trailer, themePanelBadgeText(want)) {
		t.Errorf("the %q row draws %q, want it to end with the right-aligned %q", label, row.trailer, themePanelBadgeText(want))
	}
}

func requireRenderedBadgeCount(t *testing.T, m Model, want int) {
	t.Helper()

	rows := renderedPanelRows(t, m)
	got := 0
	for _, row := range rows {
		got += strings.Count(row.trailer, themeBadgeGlyph)
	}
	if got != want {
		t.Errorf("the panel draws %d `●` markers, want %d; rows: %+v", got, want, rows)
	}
}

func requireRenderedCursorOn(t *testing.T, m Model, label string) {
	t.Helper()

	var on []string
	for _, row := range renderedPanelRows(t, m) {
		if row.cursor {
			on = append(on, row.label)
		}
	}
	if !slices.Equal(on, []string{label}) {
		t.Errorf("the panel draws the cursor bar on %v, want it on %q alone", on, label)
	}
}

func TestThemePanelBehaviour_Union(t *testing.T) {
	t.Run("a persisted built-in slug is that built-in's row", func(t *testing.T) {
		m, _ := behaviourPanel(t, nil, theme.RawKeys{Theme: "nord"})

		requireRenderedOrder(t, m, theme.BuiltinSlugs()...)
		row := themePanelRowFor(t, m, "nord")
		if !row.Row.Selectable() {
			t.Errorf("the `nord` row is unselectable (%v); a persisted slug naming a built-in IS that built-in's row", row.Row.Rejection)
		}
		if row.Row.Source != theme.SourceBuiltin {
			t.Errorf("the `nord` row is a %v, want the built-in's own row rather than a minted persisted one", row.Row.Source)
		}
		requireRenderedBadge(t, m, "nord", theme.BadgeConstant)
		requireRenderedBadgeCount(t, m, 1)
		for _, drawn := range renderedPanelRows(t, m) {
			if strings.Contains(drawn.trailer, string(theme.ReasonNotFound)) {
				t.Errorf("the panel drew a %q row (%+v); the persisted slug RESOLVES, so no second row is minted for it", theme.ReasonNotFound, drawn)
			}
		}
	})

	t.Run("a persisted invalid file is that file's row, with both the reason and the badge", func(t *testing.T) {
		m, _ := behaviourPanel(t,
			[]theme.Entry{behaviourRejected("sunset", theme.ReasonBadColour)},
			theme.RawKeys{Theme: "sunset"})

		requireRenderedOrder(t, m, "gitlab-dark-grey", "horizon", "nord", "nord-wave", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
		row := themePanelRowFor(t, m, "sunset")
		if row.Row.Selectable() {
			t.Error("the `sunset` row is selectable; an invalid file is present and named but not committable")
		}
		if got := row.Row.Rejection.Reason; got != theme.ReasonBadColour {
			t.Errorf("the `sunset` row carries reason %q, want the file's own %q — nothing re-derives it", got, theme.ReasonBadColour)
		}
		requireRenderedBadge(t, m, "sunset", theme.BadgeConstant)
		requireRenderedBadgeCount(t, m, 1)
		requireRenderedCursorOn(t, m, theme.DefaultDarkSlug)
	})

	t.Run("a reserved name file is the one two-rows-for-one-slug case", func(t *testing.T) {
		m, _ := behaviourPanel(t,
			[]theme.Entry{behaviourRejected("nord", theme.ReasonReservedName)},
			theme.RawKeys{Theme: "nord"})

		requireRenderedOrder(t, m, "gitlab-dark-grey", "horizon", "nord", "nord.theme", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug)
		collided := themePanelRowFor(t, m, "nord.theme")
		if got := collided.Row.Slug; got != "nord" {
			t.Errorf("the `nord.theme` row's slug is %q, want the built-in's %q — the collision IS the reason", got, "nord")
		}
		if collided.Row.Selectable() {
			t.Error("the `nord.theme` row is selectable; a reserved name is a rejection")
		}
		requireRenderedBadge(t, m, "nord", theme.BadgeConstant)
		requireRenderedBadge(t, m, "nord.theme", theme.BadgeNone)
		requireRenderedBadgeCount(t, m, 1)
	})
}

func TestThemePanelBehaviour_Ordering(t *testing.T) {
	t.Run("it draws the union's order rather than the assembly's", func(t *testing.T) {
		m, _ := behaviourPanel(t, []theme.Entry{
			behaviourFile(t, "aurora", 0),
			behaviourRejected("zebra", theme.ReasonBadSyntax),
		}, theme.RawKeys{})

		requireRenderedOrder(t, m, "aurora", "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug, "zebra")
	})

	t.Run("it compares case-insensitively with a byte-wise tie-break", func(t *testing.T) {
		m, _ := behaviourPanel(t, []theme.Entry{
			behaviourBadName("alpha.THEME"),
			behaviourBadName("Zed.theme"),
			behaviourBadName("Alpha.theme"),
		}, theme.RawKeys{})

		requireRenderedOrder(t, m,
			"Alpha.theme", "alpha.THEME", "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug, "Zed.theme")
	})

	t.Run("the guaranteed tie puts the built-in first and the cursor on it", func(t *testing.T) {
		m, _ := behaviourPanel(t,
			[]theme.Entry{behaviourRejected("nord", theme.ReasonReservedName)},
			theme.RawKeys{Theme: "nord"})

		rows := renderedPanelLabels(t, m)
		builtin := slices.Index(rows, "nord")
		collided := slices.Index(rows, "nord.theme")
		if builtin < 0 || collided != builtin+1 {
			t.Fatalf("the panel draws %v, want `nord.theme` immediately after `nord` — the valid thing first, then the row explaining why theirs is not it", rows)
		}
		requireRenderedCursorOn(t, m, "nord")
		if got, want := m.themePanel.list.Index(), builtin; got != want {
			t.Errorf("the cursor sits at index %d, want the built-in's %d — the anchor resolves a shared identity to the SELECTABLE row", got, want)
		}
	})
}

const behaviourLongSlug = "aurora-midnight-drifting-forever-and-ever"

func TestThemePanelBehaviour_RowComposition(t *testing.T) {
	const narrowContentW = 40

	entries := []theme.Entry{
		behaviourFile(t, behaviourLongSlug, 0),
		behaviourRejected("sunset", theme.ReasonBadColour),
	}
	order := []string{"gitlab-dark-grey", "horizon", "nord", "nord-wave", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug}

	badged, _ := behaviourPanelAt(t, entries, theme.RawKeys{Theme: "sunset"}, narrowContentW, behaviourContentH)
	if got := badged.themePanel.width; got != themePanelMinWidth {
		t.Fatalf("fixture: the panel opened %d columns wide, want the ladder's minimum %d", got, themePanelMinWidth)
	}

	t.Run("every row is exactly one line at the panel's width", func(t *testing.T) {
		block := themePanelLines(renderRecomputePanel(badged))
		from := panelHeaderRowsOf(badged)
		for i := range badged.themePanel.union.Rows {
			line := block[from+i]
			if strings.Contains(line, "\n") {
				t.Errorf("body line %d carries a newline: %q", i, line)
			}
			if got := lipgloss.Width(line); got != themePanelMinWidth {
				t.Errorf("body line %d is %d cells, want the panel's %d: %q", i, got, themePanelMinWidth, line)
			}
		}
		wantOrder := append([]string{truncatedBehaviourLabel(t, badged)}, order...)
		requireRenderedOrder(t, badged, wantOrder...)
	})

	t.Run("the reason is dropped before the badge", func(t *testing.T) {
		row := renderedPanelRowFor(t, badged, "sunset")
		if !strings.Contains(row.trailer, flashWarningGlyph) {
			t.Errorf("the badged invalid row drew %q, want the `⚠` invalidity signal — it survives every competitor", row.trailer)
		}
		if strings.Contains(row.trailer, string(theme.ReasonBadColour)) {
			t.Errorf("the badged invalid row drew the reason in %q; the badge outranks it, so the marker always has a home", row.trailer)
		}
		requireRenderedBadge(t, badged, "sunset", theme.BadgeConstant)

		bare, _ := behaviourPanelAt(t, entries, theme.RawKeys{}, narrowContentW, behaviourContentH)
		control := renderedPanelRowFor(t, bare, "sunset")
		if !strings.Contains(control.trailer, string(theme.ReasonBadColour)) {
			t.Errorf("the unbadged invalid row drew %q, want the terse reason %q — the comparison above proves nothing without it", control.trailer, theme.ReasonBadColour)
		}
		requireRenderedBadge(t, bare, "sunset", theme.BadgeNone)
	})

	t.Run("an over-long label truncates and holds the floor", func(t *testing.T) {
		label := truncatedBehaviourLabel(t, badged)
		if label == behaviourLongSlug {
			t.Fatalf("the label drew in full at the panel's minimum width: %q", label)
		}
		if !strings.HasSuffix(label, themeRowEllipsis) {
			t.Errorf("the truncated label %q carries no ellipsis", label)
		}
		if got := len([]rune(label)); got < themeRowLabelFloor {
			t.Errorf("the label drew %d cells, below the floor of three characters plus the ellipsis (%d)", got, themeRowLabelFloor)
		}
	})
}

func truncatedBehaviourLabel(t *testing.T, m Model) string {
	t.Helper()

	for _, row := range renderedPanelRows(t, m) {
		stem := strings.TrimSuffix(row.label, themeRowEllipsis)
		if stem != "" && strings.HasPrefix(behaviourLongSlug, stem) {
			return row.label
		}
	}
	t.Fatalf("the panel drew no row for %q; it drew %v", behaviourLongSlug, renderedPanelLabels(t, m))
	return ""
}

func TestThemePanelBehaviour_Badges(t *testing.T) {
	t.Run("set and loadable badges the persisted slug", func(t *testing.T) {
		m, _ := behaviourPanel(t, nil, theme.RawKeys{Light: theme.DefaultLightSlug, Dark: "nord"})

		requireRenderedBadge(t, m, "nord", theme.BadgeDark)
		requireRenderedBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)
		requireRenderedBadge(t, m, theme.DefaultDarkSlug, theme.BadgeNone)
		requireRenderedBadgeCount(t, m, 2)
	})

	t.Run("set but unloadable still badges the persisted slug", func(t *testing.T) {
		m, _ := behaviourPanel(t, nil, theme.RawKeys{Light: "gone-light", Dark: "nord"})

		requireRenderedBadge(t, m, "gone-light", theme.BadgeLight)
		requireRenderedBadge(t, m, "nord", theme.BadgeDark)
		requireRenderedBadge(t, m, theme.DefaultLightSlug, theme.BadgeNone)
		requireRenderedBadgeCount(t, m, 2)

		fallback := renderedPanelRowFor(t, m, "gone-light")
		if !strings.Contains(fallback.trailer, flashWarningGlyph) {
			t.Errorf("the `gone-light` row drew %q, want the `⚠` — the user sees what is set AND why it is not applying", fallback.trailer)
		}
	})

	t.Run("a never-set slot badges the shipped default", func(t *testing.T) {
		m, _ := behaviourPanel(t, []theme.Entry{behaviourFile(t, "aurora", 0)}, theme.RawKeys{})

		requireRenderedBadge(t, m, theme.DefaultDarkSlug, theme.BadgeDark)
		requireRenderedBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)
		requireRenderedBadge(t, m, "nord", theme.BadgeNone)
		requireRenderedBadge(t, m, "aurora", theme.BadgeNone)
		requireRenderedBadgeCount(t, m, 2)
	})
}

func TestThemePanelBehaviour_CommitRecompute(t *testing.T) {
	keys := theme.RawKeys{Theme: "sunset", Light: "ghost", Dark: "sunset"}
	m, persister := behaviourPanel(t, []theme.Entry{behaviourFile(t, "sunset", 0)}, keys)

	requireRenderedOrder(t, m, "gitlab-dark-grey", "horizon", "nord", "nord-wave", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireRenderedCursorOn(t, m, "sunset")
	requireRenderedBadge(t, m, "sunset", theme.BadgeConstant)
	requireRenderedBadgeCount(t, m, 1)
	previewed := m.themeState.active
	before := m.themePanel.list.Index()

	m, _ = pressSlotKey(t, m, slotDarkPress)
	if !m.themePanel.confirming() {
		t.Fatal("`d` over a constant did not raise the confirm, so the commit below is not the one the confirm gates")
	}
	m, _ = pressConfirmKey(t, m, confirmYes)

	requireSlotCommits(t, persister, slotCommit{slug: "sunset", member: theme.MemberDark})
	requirePairKeys(t, m, "ghost", "sunset")
	requireRenderedOrder(t, m, "ghost", "gitlab-dark-grey", "horizon", "nord", "nord-wave", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireRenderedBadge(t, m, "sunset", theme.BadgeDark)
	requireRenderedBadge(t, m, "ghost", theme.BadgeLight)
	requireRenderedBadgeCount(t, m, 2)
	requireRenderedCursorOn(t, m, "sunset")
	if got, want := m.themePanel.list.Index(), before+1; got != want {
		t.Errorf("the cursor sits at index %d, want %d — the row inserted above it pushed the previewed row down", got, want)
	}
	if m.themeState.active != previewed {
		t.Errorf("the commit rendered %s, want the previewed %s — a commit is a WRITE, not a navigation", themeLabel(m.themeState.active), themeLabel(previewed))
	}

	m, _ = pressCommitKey(t, m)

	if got := persister.constants; !slices.Equal(got, []string{"sunset"}) {
		t.Fatalf("`Enter` recorded the constants %v, want [sunset]", got)
	}
	if got := len(persister.slotCommits); got != 1 {
		t.Errorf("`Enter` recorded %d slot commits, want the 1 the confirm made — it writes the CONSTANT and clears both slots", got)
	}
	requireConstantKeys(t, m, "sunset")
	requireRenderedOrder(t, m, "gitlab-dark-grey", "horizon", "nord", "nord-wave", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireRenderedBadge(t, m, "sunset", theme.BadgeConstant)
	requireRenderedBadgeCount(t, m, 1)
	requireRenderedCursorOn(t, m, "sunset")
	if got := m.themePanel.list.Index(); got != before {
		t.Errorf("the cursor sits at index %d, want %d — the removed row above it pulled the previewed row back up", got, before)
	}
}

var behaviourConfirmAnswers = []tea.KeyPressMsg{
	confirmYes, confirmYesShift, confirmNo, confirmNoShift, confirmEsc,
}

func TestThemePanelBehaviour_Confirm(t *testing.T) {
	t.Run("the three answers resolve it", func(t *testing.T) {
		for _, press := range behaviourConfirmAnswers {
			t.Run(press.String(), func(t *testing.T) {
				m := behaviourConfirmModel(t)

				m, _ = pressConfirmKey(t, m, press)

				if m.themePanel.confirming() {
					t.Errorf("%v left the question standing; it is one of the three resolving inputs", press)
				}
				if !m.themePanel.open {
					t.Errorf("%v closed the panel; the confirm resolves, and only an `Esc` with no confirm live closes", press)
				}
			})
		}
	})

	t.Run("every other key the panel binds is swallowed", func(t *testing.T) {
		swallowed := 0
		for name, probe := range themePanelProbes(livePanelDispatch) {
			if slices.Contains(behaviourConfirmAnswers, probe.press) {
				continue
			}
			swallowed++
			t.Run(name, func(t *testing.T) {
				m := behaviourConfirmModel(t)
				pending, index := m.themePanel.pending, m.themePanel.list.Index()
				persister, ok := m.themeState.persister.(*fakeThemePersister)
				if !ok {
					t.Fatalf("the fixture wired a %T persister, want the recording fake", m.themeState.persister)
				}

				got, cmd := pressConfirmKey(t, m, probe.press)

				if !got.themePanel.confirming() || got.themePanel.pending != pending {
					t.Errorf("%v resolved the question (pending %+v, want the standing %+v); only the three resolving inputs may", probe.press, got.themePanel.pending, pending)
				}
				if len(persister.slugs) != 0 {
					t.Errorf("%v wrote %v while the question was live; nothing is written until `y`", probe.press, persister.slugs)
				}
				if got.themePanel.list.Index() != index {
					t.Errorf("%v moved the cursor to row %d, want it left on %d — a move mid-question re-themes the screen behind the answer", probe.press, got.themePanel.list.Index(), index)
				}
				if cmd != nil {
					t.Errorf("%v scheduled %T while the question was live, want nothing", probe.press, cmd)
				}
			})
		}
		if want := len(themePanelKeymap()) - 1; swallowed != want {
			t.Errorf("the partition covered %d swallowed panel keys, want %d — every descriptor entry but `esc`, which is an answer", swallowed, want)
		}
	})
}

func behaviourConfirmModel(t *testing.T) Model {
	t.Helper()

	m, _ := behaviourPanel(t, []theme.Entry{
		behaviourFile(t, "aurora", 0),
		behaviourFile(t, "sunset", 1),
	}, theme.RawKeys{Theme: "aurora"})
	m = arrowToThemeRow(t, m, "sunset")

	m, _ = pressSlotKey(t, m, slotLightPress)
	if !m.themePanel.confirming() {
		t.Fatal("fixture: `l` over a constant raised no confirm, so there is no question to resolve")
	}
	if m.themePanel.pending != (themeSlotConfirm{slug: "sunset", member: theme.MemberLight}) {
		t.Fatalf("fixture: the pending assignment is %+v, want `sunset` into the light slot", m.themePanel.pending)
	}
	return m
}

func TestThemePanelBehaviour_FailureStateMachine(t *testing.T) {
	t.Run("the message clears on a keypress and the state stands until a commit succeeds", func(t *testing.T) {
		m, persister := behaviourFailureModel(t)
		persister.err = errThemeCommitFailed

		m, _ = pressSlotKey(t, m, slotDarkPress)

		if got := themePanelMessageRow(m); got != messageTestFailedCopy {
			t.Fatalf("the failed commit drew %q in the message slot, want %q", got, messageTestFailedCopy)
		}
		if !m.themeState.commitFailed {
			t.Fatal("the failed commit left nothing outstanding")
		}

		index := m.themePanel.list.Index()
		m = pressPanelKey(t, m, arrowDown)
		if got := themePanelMessageRow(m); got != "" {
			t.Errorf("the message slot still reads %q after the next keypress, want it cleared", got)
		}
		if m.themePanel.list.Index() == index {
			t.Error("the keypress that cleared the message was swallowed; it clears AND acts")
		}
		if !m.themeState.commitFailed {
			t.Fatal("arrowing away discharged the state; only a SUCCESSFUL commit does")
		}

		persister.err = nil
		m, _ = pressSlotKey(t, m, slotLightPress)
		if m.themeState.commitFailed {
			t.Error("the successful commit left the failure outstanding")
		}

		m, _ = closePanelForTest(t, m)
		if got := m.flashText; got != "" {
			t.Errorf("the close raised %q after a successful retry, want nothing", got)
		}
	})

	t.Run("closing with a failure outstanding reports once and discharges", func(t *testing.T) {
		m, persister := behaviourFailureModel(t)
		persister.err = errThemeCommitFailed
		m, _ = pressSlotKey(t, m, slotDarkPress)

		m, _ = closePanelForTest(t, m)

		if got := m.flashText; got != wantThemeNotSavedFlash {
			t.Errorf("the close raised %q, want %q", got, wantThemeNotSavedFlash)
		}
		requireFlashBandVisible(t, m, wantThemeNotSavedFlash)
		if m.themeState.commitFailed {
			t.Error("the report was raised with the failure still outstanding; raising it DISCHARGES the state")
		}

		m = pressThemeKey(t, m)
		if !m.themePanel.open {
			t.Fatal("`t` did not re-open the panel, so the second close proves nothing")
		}
		m, _ = closePanelForTest(t, m)
		if got := m.flashText; got != "" {
			t.Errorf("the second close raised %q, want nothing — the first discharged the state", got)
		}
	})

	t.Run("a forced close reports the failure over the geometry event", func(t *testing.T) {
		m, persister := behaviourFailureModel(t)
		persister.err = errThemeCommitFailed
		m, _ = pressSlotKey(t, m, slotDarkPress)

		contentW, contentH := geometryBelowWidthFloor()
		m = resizeForTest(t, m, contentW, contentH)

		if m.themePanel.open {
			t.Fatal("the resize below the floor left the panel open")
		}
		if got := m.flashText; got != wantThemeNotSavedFlash {
			t.Errorf("the forced close raised %q, want the commit-failure report to win the single band slot over %q", got, wantNarrowClosedFlash)
		}
		if m.themeState.commitFailed {
			t.Error("the forced close left the failure outstanding; the report was made, so the state is discharged")
		}

		control, _ := behaviourFailureModel(t)
		control = resizeForTest(t, control, contentW, contentH)
		if got := control.flashText; got != wantNarrowClosedFlash {
			t.Errorf("the control's forced close raised %q, want %q", got, wantNarrowClosedFlash)
		}
	})
}

func behaviourFailureModel(t *testing.T) (Model, *fakeThemePersister) {
	t.Helper()

	m, persister := behaviourPanel(t, []theme.Entry{
		behaviourFile(t, "aurora", 0),
		behaviourFile(t, "sunset", 1),
	}, theme.RawKeys{Light: "aurora", Dark: "sunset"})
	if m.themeSetting().IsConstant {
		t.Fatal("fixture: the setting resolved as a constant, so `d`/`l` would ask rather than write")
	}
	return m, persister
}

const behaviourSuiteFile = "theme_panel_behaviour_test.go"

var behaviourBannedImports = []string{"os", "io/fs", "path", "path/filepath"}

// These reach the filesystem or env with no bannable import: t.TempDir and
// t.Setenv are testing's own, NewStore arrives through the prefs package.
var behaviourBannedCalls = []string{
	"TempDir", "Setenv", "MkdirTemp", "WriteFile", "ReadFile", "ReadDir", "NewStore",
}

func TestThemePanelBehaviour_NoConfigAccess(t *testing.T) {
	t.Run("it renders an invalid row with no themes directory", func(t *testing.T) {
		m, _ := behaviourPanel(t,
			[]theme.Entry{behaviourRejected("sunset", theme.ReasonMissingTokens)},
			theme.RawKeys{})

		if got := m.themePanel.enumeration.DirPath; got != "" {
			t.Errorf("the retained enumeration names the directory %q, want none — the seam returns the finished union, not a directory listing", got)
		}
		row := renderedPanelRowFor(t, m, "sunset")
		if !strings.Contains(row.trailer, flashWarningGlyph) || !strings.Contains(row.trailer, string(theme.ReasonMissingTokens)) {
			t.Errorf("the rejected row drew %q, want the `⚠` and the terse reason %q", row.trailer, theme.ReasonMissingTokens)
		}
	})

	t.Run("the suite names no filesystem or config entry point", func(t *testing.T) {
		file := sourceguardtest.PackageSource(t, ".", behaviourSuiteFile).File

		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if slices.Contains(behaviourBannedImports, path) {
				t.Errorf("%s imports %q; the seam is declared data and the persister is a recorder, so this suite needs no path at all", behaviourSuiteFile, path)
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if slices.Contains(behaviourBannedCalls, sel.Sel.Name) {
				t.Errorf("%s calls %s; this suite opens no directory, reads no prefs.json and constructs no real store", behaviourSuiteFile, sel.Sel.Name)
			}
			return true
		})
	})
}
