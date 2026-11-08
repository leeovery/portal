package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

func newFailedCommitModel(t *testing.T) (Model, *fakeThemePersister) {
	t.Helper()

	m, persister := newCommitFailureFixture(t)
	persister.err = errThemeCommitFailed
	return m, persister
}

// The cursor is arrowed off both slots' rows: committing a slug a slot already
// holds leaves keys and badges identical whether the write landed or not.
func newCommitFailureFixture(t *testing.T) (Model, *fakeThemePersister) {
	t.Helper()

	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	m, persister := newSlotPairPanelModel(t, dir, theme.DefaultLightSlug, "aurora")
	requireBadge(t, m, "aurora", theme.BadgeDark)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)

	m = arrowToThemeRow(t, m, commitFailureTarget)
	return m, persister
}

const commitFailureTarget = "sunset"

func requireCommitFailedMessage(t *testing.T, m Model) {
	t.Helper()

	if got, want := m.themePanel.message, (themePanelMessage{Kind: themeMessageCommitFailed}); got != want {
		t.Errorf("the message slot holds %+v, want %+v — a failed commit reports in the panel's message slot", got, want)
	}
	if got := themePanelMessageRow(m); got != messageTestFailedCopy {
		t.Errorf("the message slot reads %q, want %q", got, messageTestFailedCopy)
	}
}

func themePanelMessageRow(m Model) string {
	lines := themePanelLines(renderRecomputePanel(m))
	row := lines[len(lines)-themePanelFooterHeight(themePanelKeymap())-1]
	return strings.TrimRight(strings.TrimPrefix(row, themePanelContentPrefix()), " ")
}

func badgeRows(m Model) []string {
	var rows []string
	for i, line := range themePanelLines(renderRecomputePanel(m)) {
		if strings.Contains(line, "●") {
			rows = append(rows, fmt.Sprintf("%d:%s", i, strings.TrimRight(line, " ")))
		}
	}
	return rows
}

func TestCommitFailure_MessageCopy(t *testing.T) {
	m, persister := newFailedCommitModel(t)

	m, cmd := pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: commitFailureTarget, member: theme.MemberDark})
	requireCommitFailedMessage(t, m)
	if !m.themePanel.open {
		t.Error("a failed commit closed the panel; `Esc` is the only way out")
	}
	if cmd != nil {
		t.Errorf("a failed commit scheduled %T, want nothing", cmd)
	}

	landed, landedPersister := newCommitFailureFixture(t)
	landed, _ = pressSlotKey(t, landed, slotDarkPress)
	requireSlotCommits(t, landedPersister, slotCommit{slug: commitFailureTarget, member: theme.MemberDark})
	if got := landed.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("a successful commit raised the message %+v, want an empty slot", got)
	}
}

func TestCommitFailure_BadgeDoesNotMove(t *testing.T) {
	m, persister := newFailedCommitModel(t)
	keys := m.themeState.keys
	badges := maps.Clone(m.themePanel.badges)
	labels := themePanelRowLabels(m)
	rendered := badgeRows(m)

	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: commitFailureTarget, member: theme.MemberDark})
	if got := badgeRows(m); !slices.Equal(got, rendered) {
		t.Errorf("a failed commit rendered the markers on %v, want the untouched %v — the `●` means what is PERSISTED", got, rendered)
	}
	requireBadge(t, m, "aurora", theme.BadgeDark)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)
	requireBadge(t, m, commitFailureTarget, theme.BadgeNone)
	if m.themeState.keys != keys {
		t.Errorf("a failed commit left keys %+v, want the untouched %+v — the badges derive from them", m.themeState.keys, keys)
	}
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("a failed commit left badges %v, want the untouched %v", got, badges)
	}
	if got := themePanelRowLabels(m); !slices.Equal(got, labels) {
		t.Errorf("a failed commit left rows %v, want the untouched %v — only a SUCCESSFUL commit recomputes", got, labels)
	}

	landed, _ := newCommitFailureFixture(t)
	landed, _ = pressSlotKey(t, landed, slotDarkPress)
	requireBadge(t, landed, commitFailureTarget, theme.BadgeDark)
	if got := badgeRows(landed); slices.Equal(got, rendered) {
		t.Errorf("a successful commit rendered the markers on %v too; the comparison above proves nothing", got)
	}
}

// The stub-backed pair, whose synthetic palettes carry a distinct value per
// token: the one-colour drop-ins would make the comparison below vacuous.
func TestCommitFailure_ThemeStaysApplied(t *testing.T) {
	rows := arrowValidRows(t, 4)
	m, persister := newCommitPairPanelModel(t, rows)
	m = arrowToThemeRow(t, m, rows[2].Slug)
	previewed := m.themeState.active
	if previewed != rows[2].Theme {
		t.Fatalf("fixture: the frame renders %s, want the arrowed-to row's palette", themeLabel(previewed))
	}
	before := frameColours(m.View().Content)
	persister.err = errThemeCommitFailed

	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: rows[2].Slug, member: theme.MemberDark})
	requireCommitFailedMessage(t, m)
	if m.themeState.active != previewed {
		t.Errorf("a failed commit rendered %s, want the previewed %s — a failed commit KEEPS the theme applied in memory", themeLabel(m.themeState.active), themeLabel(previewed))
	}

	frame := m.View().Content
	if seq := canvasSeq(t, previewed); !strings.Contains(frame, seq) {
		t.Errorf("the composed frame no longer paints the previewed canvas %q", seq)
	}
	after := frameColours(frame)
	if gone := colourDiff(before, after); len(gone) != 0 {
		t.Errorf("a failed commit dropped the colours %v from the frame", gone)
	}
	attention := tokenFgSeq(t, previewed.AccentAttention)
	carries := func(seq string) bool { return strings.Contains(seq, attention) }
	if !slices.ContainsFunc(after, carries) {
		t.Errorf("the frame paints no accent.attention run; the commit-failure line renders in that token")
	}
	for _, seq := range colourDiff(after, before) {
		if !carries(seq) {
			t.Errorf("a failed commit added the colour %q to the frame; only the commit-failure line may appear", escSeq(seq))
		}
	}
}

func colourDiff(a, b []string) []string {
	var only []string
	for _, seq := range a {
		if !slices.Contains(b, seq) {
			only = append(only, seq)
		}
	}
	return only
}

func TestCommitFailure_MessageClearsOnNextKeyAndFallsThrough(t *testing.T) {
	m, _ := newFailedCommitModel(t)

	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireCommitFailedMessage(t, m)
	index := m.themePanel.list.Index()
	previewed := m.themeState.active
	body := m.themePanel.list.Height()

	m = pressPanelKey(t, m, arrowDown)

	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the next keypress left the message %+v, want the slot empty", got)
	}
	if strings.Contains(themePanelMessageRow(m), messageTestFailedCopy) {
		t.Errorf("the panel still renders the commit-failure line after the next keypress:\n%s", renderRecomputePanel(m))
	}
	if got := m.themePanel.list.Index(); got == index {
		t.Errorf("the arrow left the cursor on row %d; the keystroke continues to its normal handler — one key, one intent", got)
	}
	if m.themeState.active == previewed {
		t.Errorf("the arrow left %s painting the frame; it moved the cursor, so it previews the new row", themeLabel(previewed))
	}
	if got := m.themePanel.list.Height(); got != body+1 {
		t.Errorf("the list body is %d rows once the message cleared, want the row back (%d) — the slot is not reserved when empty", got, body+1)
	}
}

func TestCommitFailure_MessageSurvivesWindowSize(t *testing.T) {
	m, _ := newFailedCommitModel(t)

	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireCommitFailedMessage(t, m)

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: arrowTermW - 4, Height: arrowTermH - 2})
	m = updated.(Model)

	if !m.themePanel.open {
		t.Fatal("the resize force-closed the panel; the fixture must stay above the panel's render floor for this to assert anything")
	}
	requireCommitFailedMessage(t, m)
	if !m.themeState.commitFailed {
		t.Error("the resize discharged the outstanding failure; only a successful commit does")
	}
	if cmd != nil {
		t.Errorf("the resize scheduled %T, want nothing", cmd)
	}
}

func TestCommitFailure_MessageHasNoTickLifecycle(t *testing.T) {
	m, _ := newFailedCommitModel(t)
	gen := m.flashGen

	m, cmd := pressSlotKey(t, m, slotDarkPress)

	requireCommitFailedMessage(t, m)
	if cmd != nil {
		t.Errorf("a failed commit scheduled %T; the commit-failure line takes no tick lifecycle", cmd)
	}
	if m.flashGen != gen {
		t.Errorf("a failed commit bumped the flash generation to %d, want the untouched %d — it raises no flash", m.flashGen, gen)
	}
	if m.flashText != "" {
		t.Errorf("a failed commit set the main-screen flash %q; a failed commit reports in the PANEL's slot", m.flashText)
	}

	updated, tickCmd := m.Update(flashTickMsg{Gen: m.flashGen})
	m = updated.(Model)

	requireCommitFailedMessage(t, m)
	if tickCmd != nil {
		t.Errorf("the tick scheduled %T, want nothing", tickCmd)
	}
	if !m.themeState.commitFailed {
		t.Error("the tick discharged the outstanding failure; only a successful commit does")
	}
}

func TestCommitFailure_StateOutlivesTheMessage(t *testing.T) {
	t.Run("an arrow", func(t *testing.T) {
		m, _ := newFailedCommitModel(t)
		m, _ = pressSlotKey(t, m, slotDarkPress)

		m = pressPanelKey(t, m, arrowDown)

		requireOutstandingWithNoMessage(t, m)
	})

	t.Run("a page", func(t *testing.T) {
		rows := arrowValidRows(t, 30)
		m, persister := newCommitPairPanelModel(t, rows)
		if got := m.themePanel.list.Paginator.TotalPages; got < 2 {
			t.Fatalf("fixture: the panel's list is %d page(s), want it paginating so `Ctrl+↓` has somewhere to go", got)
		}
		persister.err = errThemeCommitFailed
		m, _ = pressSlotKey(t, m, slotDarkPress)
		requireCommitFailedMessage(t, m)
		page := m.themePanel.list.Paginator.Page

		m = pressPanelKey(t, m, arrowPageDown)

		if got := m.themePanel.list.Paginator.Page; got == page {
			t.Errorf("the page key left the list on page %d; the keystroke continues to its normal handler", got)
		}
		requireOutstandingWithNoMessage(t, m)
	})
}

func requireOutstandingWithNoMessage(t *testing.T, m Model) {
	t.Helper()

	if got := m.themePanel.message; got.Kind == themeMessageCommitFailed {
		t.Errorf("the message slot still holds %+v; it persists only until the next keypress", got)
	}
	if !m.themeState.commitFailed {
		t.Error("the failure is no longer outstanding; dismissing the MESSAGE must leave the STATE, or the very next `Esc` reinstates the silent revert")
	}
}

func TestCommitFailure_SuccessDischargesTheState(t *testing.T) {
	m, persister := newFailedCommitModel(t)

	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireCommitFailedMessage(t, m)
	if !m.themeState.commitFailed {
		t.Fatal("the failed commit left nothing outstanding, so the discharge below proves nothing")
	}
	gen := m.flashGen
	persister.err = nil

	m, cmd := pressSlotKey(t, m, slotLightPress)

	requireSlotCommits(t, persister,
		slotCommit{slug: commitFailureTarget, member: theme.MemberDark},
		slotCommit{slug: commitFailureTarget, member: theme.MemberLight},
	)
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the successful commit left the message %+v, want the slot empty", got)
	}
	if m.themeState.commitFailed {
		t.Error("the successful commit left the failure outstanding; a `d` that fails followed by an `l` that succeeds reports nothing")
	}
	if m.flashText != "" || m.flashGen != gen {
		t.Errorf("the successful commit raised the flash %q (gen %d, want %d); there is nothing left to report", m.flashText, m.flashGen, gen)
	}
	requirePairKeys(t, m, commitFailureTarget, "aurora")
	requireBadge(t, m, commitFailureTarget, theme.BadgeLight)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeNone)
	if cmd != nil {
		t.Errorf("the successful commit scheduled %T, want nothing", cmd)
	}
}

func TestCommitFailure_RetryIsJustPressingAgain(t *testing.T) {
	m, persister := newFailedCommitModel(t)
	failed := slotCommit{slug: commitFailureTarget, member: theme.MemberDark}

	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireCommitFailedMessage(t, m)

	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, failed, failed)
	requireCommitFailedMessage(t, m)
	if !m.themeState.commitFailed {
		t.Error("the retry discharged the outstanding failure; it failed again")
	}

	persister.err = nil
	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, failed, failed, failed)
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the successful retry left the message %+v, want the slot empty", got)
	}
	if m.themeState.commitFailed {
		t.Error("the successful retry left the failure outstanding")
	}
	requirePairKeys(t, m, theme.DefaultLightSlug, commitFailureTarget)
	requireBadge(t, m, commitFailureTarget, theme.BadgeDark)
}

func TestCommitFailure_OverAConstant(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	requireBadge(t, m, "aurora", theme.BadgeConstant)
	labels := themePanelRowLabels(m)
	badges := maps.Clone(m.themePanel.badges)

	m = arrowToThemeRow(t, m, "nord")
	previewed := m.themeState.active
	persister.err = errThemeCommitFailed
	m, cmd := pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
	requireCommitFailedMessage(t, m)
	if !m.themeState.commitFailed {
		t.Error("a failed commit over a constant left nothing outstanding")
	}
	requireConstantKeys(t, m, "aurora")
	requireBadge(t, m, "aurora", theme.BadgeConstant)
	requireBadgeText(t, m, 1, 0, 0)
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("a failed commit over a constant left badges %v, want the untouched %v", got, badges)
	}
	if got := themePanelRowLabels(m); !slices.Equal(got, labels) {
		t.Errorf("a failed commit over a constant left rows %v, want the untouched %v — only a SUCCESSFUL commit recomputes", got, labels)
	}
	if m.themeState.active != previewed {
		t.Errorf("a failed commit over a constant rendered %s, want the previewed %s left alone", themeLabel(m.themeState.active), themeLabel(previewed))
	}
	if cmd != nil {
		t.Errorf("a failed commit over a constant scheduled %T, want nothing", cmd)
	}
}

func TestCommitFailure_PanelEmitsNoThemeRecord(t *testing.T) {
	sink := logtest.Install(t)
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))

	m, persister := newLoadPanelModel(t, dir, theme.RawKeys{Light: theme.DefaultLightSlug, Dark: "aurora"}, theme.NewLoader(theme.NewEventLogger(log.For("theme"))))
	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	if len(sink.Records()) == 0 {
		t.Fatal("the open emitted nothing at all, so the silence below is a sink that was never wired")
	}
	opened := len(sink.Records())
	m = arrowToThemeRow(t, m, "nord")
	persister.err = errThemeCommitFailed

	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
	requireCommitFailedMessage(t, m)
	if got := sink.Records(); len(got) != opened {
		t.Errorf("the failure path emitted %d record(s):\n%s\nthe persister is the SINGLE site for `theme: commit failed`", len(got)-opened, sink.Body())
	}
}

func TestCommitFailure_NilPersisterRaisesNothing(t *testing.T) {
	rows := arrowValidRows(t, 4)
	m := openCommitPanel(t, commitPairPanelDeps(t, rows), PageSessions, rows[1].Slug)
	if m.themeState.persister != nil {
		t.Fatalf("fixture: the model holds persister %#v, want none", m.themeState.persister)
	}
	m = arrowToThemeRow(t, m, rows[2].Slug)
	before := m.View().Content

	for _, tc := range []struct {
		name string
		run  func(*testing.T, Model) Model
	}{
		{name: "Enter", run: func(t *testing.T, m Model) Model { m, _ = pressCommitKey(t, m); return m }},
		{name: "d", run: func(t *testing.T, m Model) Model { m, _ = pressSlotKey(t, m, slotDarkPress); return m }},
		{name: "l", run: func(t *testing.T, m Model) Model { m, _ = pressSlotKey(t, m, slotLightPress); return m }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pressed := tc.run(t, m)

			if got := pressed.themePanel.message; got.Kind != themeMessageNone {
				t.Errorf("a nil persister raised the message %+v; it is the absence of a WRITER, not a failed write", got)
			}
			if pressed.themeState.commitFailed {
				t.Error("a nil persister left a commit failure outstanding; nothing was written and nothing failed")
			}
			if got := pressed.View().Content; got != before {
				t.Errorf("%q over a nil persister changed the frame\nbefore: %q\nafter:  %q", tc.name, escSeq(before), escSeq(got))
			}
		})
	}
}
