package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
)

// The target is the selected row, never m.themeState.keys — committing the
// persisted slug would write back the theme being replaced.
func (m *Model) commitSelected(commit func(slug string) error) error {
	slug, ok := committableThemeSlug(m.themePanel.list)
	if !ok {
		return nil
	}
	return commit(slug)
}

func committableThemeSlug(l list.Model) (string, bool) {
	row, ok := selectedThemeRow(l)
	if !ok || !row.Selectable() || row.Slug == "" {
		return "", false
	}
	return row.Slug, true
}

// A nil persister is inert, not failed — the write is never reached, so no
// closure dereferences it. The mirror applies prefs' rule to the keys in hand
// rather than re-reading — the persister's read-modify-write holds other
// instances' writes. On error nothing moves, so the `●` cannot move.
func (m *Model) commit(write func() error, mirror func(theme.RawKeys) theme.RawKeys) error {
	if m.themeState.persister == nil {
		return nil
	}
	err := write()
	m.applyCommitResult(err)
	if err != nil {
		return err
	}
	m.themeState.keys = mirror(m.themeState.keys)
	m.recomputeThemePanel()
	return nil
}

func (m *Model) commitConstant(slug string) error {
	return m.commit(
		func() error { return m.themeState.persister.CommitTheme(slug) },
		func(keys theme.RawKeys) theme.RawKeys { return keys.WithConstant(slug) },
	)
}

// The message lasts until the next keypress, the state until a later commit
// succeeds or reportOutstandingCommitFailure discharges it into the close's
// flash. No logging: the persister owns the `theme: commit failed` emission.
func (m *Model) applyCommitResult(err error) {
	if err != nil {
		m.reportCommitFailure()
		return
	}
	m.clearThemePanelCommitFailed()
	m.themeState.commitFailed = false
}

// Raised whole: a line without the state dies on the next arrow and the close
// silently reverts; a state without the line reports nothing visible.
func (m *Model) reportCommitFailure() {
	m.raiseThemePanelCommitFailed()
	m.themeState.commitFailed = true
}

// The shape comes from themeSetting so it cannot disagree with what the panel
// resolves.
func (m Model) handleSlotCommitKey(member theme.Member) (tea.Model, tea.Cmd) {
	commit := (&m).commitSlot
	if m.themeSetting().IsConstant {
		commit = (&m).commitSlotOverConstant
	}
	_ = (&m).commitSelected(func(slug string) error { return commit(slug, member) })
	return m, nil
}

// A slot write over a constant clears it, bringing the other slot into force
// with it. The persister nil-check cannot be inferred from the nil error —
// commitSlot returns nil on the writer-less path too, where the keys still hold
// the constant.
func (m *Model) commitSlotOverConstant(slug string, member theme.Member) error {
	if err := m.commitSlot(slug, member); err != nil {
		return err
	}
	if m.themeState.persister == nil {
		return nil
	}
	m.loadNewlyLiveSlot(member)
	return nil
}

// The classification is assigned first and unconditionally — gating it on the
// load would let a failed palette decide which half is in force, so the load's
// error is nothing this site can act on. The newly-live slot's palette stays out
// of the nomination too: applyCommittedSetting is its single owner, and a second
// writer would put two sources of truth behind one screen. Never ApplyTheme —
// the screen keeps previewing the cursor's row.
func (m *Model) loadNewlyLiveSlot(assigned theme.Member) {
	m.themeState.adoptRetainedReply()

	newlyLive := assigned.Opposite()
	_ = m.themeState.source.LoadSlot(m.themePanel.enumeration, newlyLive.Slot(), m.themeState.keys)
}

// The untouched other slot is what makes `● both` reachable via `d` then `l` on
// one row.
func (m *Model) commitSlot(slug string, member theme.Member) error {
	return m.commit(
		func() error { return m.themeState.persister.CommitThemeSlot(slug, member) },
		func(keys theme.RawKeys) theme.RawKeys { return keys.WithMember(member, slug) },
	)
}

// Landed writes only: badges and nomination derive from the raw keys, so
// recomputing after a failed write would move the `●` off what is persisted.
// Inputs are the retained enumeration and this instance's keys — not the
// persister's merged bytes and not a fresh read. The cursor re-anchors last.
func (m *Model) recomputeThemePanel() {
	previewed := previewedThemeIdentity(m.themePanel.list)

	m.themePanel.union = m.themeState.source.Reassemble(m.themePanel.enumeration, m.themeState.keys)
	m.applyCommittedSetting()

	// Always nil: SetItems schedules work only when filtering is enabled.
	_ = m.themePanel.list.SetItems(m.themePanel.rowItems())

	m.anchorThemePanelCursor(previewed)
}

func previewedThemeIdentity(l list.Model) string {
	row, ok := selectedThemeRow(l)
	if !ok {
		return ""
	}
	return row.Identity()
}

// Never ApplyTheme — a commit is a write, not a navigation, and re-theming
// would swap the screen off the live preview. On error nothing moves: a zero
// Resolution would wipe every `●` at the moment the user committed one.
func (m *Model) applyCommittedSetting() {
	resolution, err := m.themeState.source.Resolve(m.themePanel.enumeration, m.themeState.keys)
	if err != nil {
		return
	}
	m.themePanel.badges = theme.Badges(resolution.Slots)
	m.themeState.nomination = resolution.Nomination
}
