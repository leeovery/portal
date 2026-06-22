package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/portal/internal/project"
)

// === Test doubles ====================================================

// smProjectEditor records Rename / AddTag / RemoveTag calls so the
// state-machine tests can assert immediate-persist behaviour. Errors are
// configurable per method.
type smProjectEditor struct {
	renames   []smRename
	added     []smTagCall
	removed   []smTagCall
	renameErr error
	addErr    error
	removeErr error
}

type smRename struct{ path, name, via string }
type smTagCall struct{ path, tag string }

func (e *smProjectEditor) Rename(path, name, via string) error {
	if e.renameErr != nil {
		return e.renameErr
	}
	e.renames = append(e.renames, smRename{path, name, via})
	return nil
}

func (e *smProjectEditor) AddTag(path, tag string) error {
	if e.addErr != nil {
		return e.addErr
	}
	e.added = append(e.added, smTagCall{path, tag})
	return nil
}

func (e *smProjectEditor) RemoveTag(path, tag string) error {
	if e.removeErr != nil {
		return e.removeErr
	}
	e.removed = append(e.removed, smTagCall{path, tag})
	return nil
}

// smAliasEditor records SetAndSave / DeleteAndSave calls and serves a
// configurable alias map for the collision pre-check.
type smAliasEditor struct {
	aliases   map[string]string
	setCalls  []smAliasSet
	deleted   []string
	loadErr   error
	setErr    error
	deleteErr error
}

type smAliasSet struct{ name, path, via string }

// smProjectStore is a minimal ProjectStore for the Esc-close refresh test: only
// List/CleanStale/Remove need to be satisfied so loadProjects() builds a cmd.
type smProjectStore struct {
	projects []project.Project
}

func (s smProjectStore) List() ([]project.Project, error)       { return s.projects, nil }
func (s smProjectStore) CleanStale() ([]project.Project, error) { return s.projects, nil }
func (s smProjectStore) Remove(_, _ string) error               { return nil }

func (e *smAliasEditor) Load() (map[string]string, error) {
	if e.loadErr != nil {
		return nil, e.loadErr
	}
	out := make(map[string]string, len(e.aliases))
	for k, v := range e.aliases {
		out[k] = v
	}
	return out, nil
}

func (e *smAliasEditor) SetAndSave(name, path, via string) error {
	if e.setErr != nil {
		return e.setErr
	}
	e.setCalls = append(e.setCalls, smAliasSet{name, path, via})
	if e.aliases == nil {
		e.aliases = map[string]string{}
	}
	e.aliases[name] = path
	return nil
}

func (e *smAliasEditor) DeleteAndSave(name, via string) (bool, error) {
	if e.deleteErr != nil {
		return false, e.deleteErr
	}
	e.deleted = append(e.deleted, name)
	_, ok := e.aliases[name]
	delete(e.aliases, name)
	return ok, nil
}

// === Helpers =========================================================

// smModel builds a Model with the edit modal open in navigate mode, the
// given chip buffers seeded, and the supplied editors wired.
func smModel(ed *smProjectEditor, al *smAliasEditor, aliases, tags []string) Model {
	return Model{
		modal:         modalEditProject,
		editMode:      editModeNavigate,
		editFocus:     editFieldName,
		editName:      "proj",
		editProject:   project.Project{Path: "/p/one", Name: "proj"},
		editAliases:   aliases,
		editTags:      tags,
		projectEditor: ed,
		aliasEditor:   al,
	}
}

func smKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(msg)
	return updated.(Model)
}

func smKeyCmd(t *testing.T, m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.updateEditProjectModal(msg)
	return updated.(Model), cmd
}

var (
	keyTab      = tea.KeyPressMsg{Code: tea.KeyTab}
	keyShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	keyLeft     = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyRight    = tea.KeyPressMsg{Code: tea.KeyRight}
	keyEnter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc      = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func runeKey(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyExtended, Text: s}
}

func typeRunes(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = smKey(t, m, runeKey(string(r)))
	}
	return m
}

// === Navigate mode: field movement ===================================

func TestSM_TabMovesBetweenFields(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)

	m = smKey(t, m, keyTab)
	if m.editFocus != editFieldAliases {
		t.Fatalf("after Tab from Name, focus = %d, want Aliases", m.editFocus)
	}
	m = smKey(t, m, keyTab)
	if m.editFocus != editFieldTags {
		t.Fatalf("after Tab from Aliases, focus = %d, want Tags", m.editFocus)
	}
	m = smKey(t, m, keyTab)
	if m.editFocus != editFieldName {
		t.Fatalf("after Tab from Tags, focus = %d, want Name (wrap)", m.editFocus)
	}
}

func TestSM_ShiftTabMovesBackwards(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)

	m = smKey(t, m, keyShiftTab)
	if m.editFocus != editFieldTags {
		t.Fatalf("after Shift+Tab from Name, focus = %d, want Tags (wrap back)", m.editFocus)
	}
	m = smKey(t, m, keyShiftTab)
	if m.editFocus != editFieldAliases {
		t.Fatalf("after Shift+Tab, focus = %d, want Aliases", m.editFocus)
	}
	m = smKey(t, m, keyShiftTab)
	if m.editFocus != editFieldName {
		t.Fatalf("after Shift+Tab, focus = %d, want Name", m.editFocus)
	}
}

func TestSM_EnteringChipFieldLandsOnAddSlot(t *testing.T) {
	// Tab into Aliases (2 chips) must land on the trailing + add slot (index 2).
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a", "b"}, nil)
	m.editAliasCursor = 0 // dirty value reset on entry

	m = smKey(t, m, keyTab) // Name -> Aliases
	if m.editFocus != editFieldAliases {
		t.Fatalf("focus = %d, want Aliases", m.editFocus)
	}
	if m.editAliasCursor != len(m.editAliases) {
		t.Errorf("alias element index = %d, want %d (+ add slot)", m.editAliasCursor, len(m.editAliases))
	}

	// Tab into Tags (1 chip) lands on the add slot (index 1).
	m = smModel(&smProjectEditor{}, &smAliasEditor{}, nil, []string{"work"})
	m.editFocus = editFieldAliases
	m.editTagCursor = 0
	m = smKey(t, m, keyTab) // Aliases -> Tags
	if m.editFocus != editFieldTags {
		t.Fatalf("focus = %d, want Tags", m.editFocus)
	}
	if m.editTagCursor != len(m.editTags) {
		t.Errorf("tag element index = %d, want %d (+ add slot)", m.editTagCursor, len(m.editTags))
	}
}

func TestSM_DownMovesBetweenFields(t *testing.T) {
	// ↓ is an alias for Tab: Name -> Aliases -> Tags -> Name (wrap).
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)

	m = smKey(t, m, keyDown)
	if m.editFocus != editFieldAliases {
		t.Fatalf("after Down from Name, focus = %d, want Aliases", m.editFocus)
	}
	m = smKey(t, m, keyDown)
	if m.editFocus != editFieldTags {
		t.Fatalf("after Down from Aliases, focus = %d, want Tags", m.editFocus)
	}
	m = smKey(t, m, keyDown)
	if m.editFocus != editFieldName {
		t.Fatalf("after Down from Tags, focus = %d, want Name (wrap)", m.editFocus)
	}
}

func TestSM_UpMovesBetweenFieldsBackwards(t *testing.T) {
	// ↑ is an alias for Shift+Tab: Name -> Tags -> Aliases -> Name.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)

	m = smKey(t, m, keyUp)
	if m.editFocus != editFieldTags {
		t.Fatalf("after Up from Name, focus = %d, want Tags (wrap back)", m.editFocus)
	}
	m = smKey(t, m, keyUp)
	if m.editFocus != editFieldAliases {
		t.Fatalf("after Up, focus = %d, want Aliases", m.editFocus)
	}
	m = smKey(t, m, keyUp)
	if m.editFocus != editFieldName {
		t.Fatalf("after Up, focus = %d, want Name", m.editFocus)
	}
}

func TestSM_DownEnteringChipFieldLandsOnAddSlot(t *testing.T) {
	// ↓ into a chip field lands on the trailing + add slot, mirroring Tab.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a", "b"}, nil)
	m.editAliasCursor = 0 // dirty value reset on entry

	m = smKey(t, m, keyDown) // Name -> Aliases
	if m.editFocus != editFieldAliases {
		t.Fatalf("focus = %d, want Aliases", m.editFocus)
	}
	if m.editAliasCursor != len(m.editAliases) {
		t.Errorf("alias element index = %d, want %d (+ add slot)", m.editAliasCursor, len(m.editAliases))
	}
}

func TestSM_UpDownIgnoredInEditMode(t *testing.T) {
	// In edit mode (one element live) ↑/↓ are ignored — focus and buffer untouched.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags)
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "ab")

	m = smKey(t, m, keyDown)
	m = smKey(t, m, keyUp)

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit (↑/↓ must not exit edit)", m.editMode)
	}
	if m.editFocus != editFieldTags {
		t.Errorf("editFocus = %d, want Tags (↑/↓ must not move fields in edit)", m.editFocus)
	}
	if m.editBuffer != "ab" {
		t.Errorf("editBuffer = %q, want %q (↑/↓ must not alter the buffer)", m.editBuffer, "ab")
	}
}

// === Navigate mode: element movement within a chip field =============

func TestSM_LeftRightMovesAcrossChipsAndAddSlot(t *testing.T) {
	// Aliases focused, on the + add slot (index 2). Left reaches the chips,
	// right returns to the add slot, bounded at both ends.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a", "b"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = len(m.editAliases) // + add slot (2)

	m = smKey(t, m, keyLeft)
	if m.editAliasCursor != 1 {
		t.Fatalf("after left from add slot, index = %d, want 1", m.editAliasCursor)
	}
	m = smKey(t, m, keyLeft)
	if m.editAliasCursor != 0 {
		t.Fatalf("after left, index = %d, want 0", m.editAliasCursor)
	}
	m = smKey(t, m, keyLeft)
	if m.editAliasCursor != 0 {
		t.Fatalf("left bounded at 0, index = %d", m.editAliasCursor)
	}
	m = smKey(t, m, keyRight)
	if m.editAliasCursor != 1 {
		t.Fatalf("after right, index = %d, want 1", m.editAliasCursor)
	}
	m = smKey(t, m, keyRight)
	if m.editAliasCursor != 2 {
		t.Fatalf("after right, index = %d, want 2 (add slot)", m.editAliasCursor)
	}
	m = smKey(t, m, keyRight)
	if m.editAliasCursor != 2 {
		t.Fatalf("right bounded at add slot, index = %d", m.editAliasCursor)
	}
}

// === Navigate mode: x deletes a focused chip immediately =============

func TestSM_XDeletesFocusedAliasImmediately(t *testing.T) {
	al := &smAliasEditor{aliases: map[string]string{"a": "/p/one", "b": "/p/one"}}
	m := smModel(&smProjectEditor{}, al, []string{"a", "b"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0 // on chip "a"

	m = smKey(t, m, runeKey("x"))

	if !reflect.DeepEqual(m.editAliases, []string{"b"}) {
		t.Errorf("editAliases = %v, want [b]", m.editAliases)
	}
	if !reflect.DeepEqual(al.deleted, []string{"a"}) {
		t.Errorf("DeleteAndSave calls = %v, want [a]", al.deleted)
	}
}

func TestSM_XDeletesFocusedTagImmediately(t *testing.T) {
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, []string{"work", "home"})
	m.editFocus = editFieldTags
	m.editTagCursor = 1 // on chip "home"

	m = smKey(t, m, runeKey("x"))

	if !reflect.DeepEqual(m.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work]", m.editTags)
	}
	wantRm := []smTagCall{{"/p/one", "home"}}
	if !reflect.DeepEqual(ed.removed, wantRm) {
		t.Errorf("RemoveTag calls = %v, want %v", ed.removed, wantRm)
	}
}

func TestSM_XOnAddSlotIsNoOpInNavigate(t *testing.T) {
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, []string{"work"})
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags) // + add slot

	m = smKey(t, m, runeKey("x"))

	if !reflect.DeepEqual(m.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work] (x on add slot deletes nothing)", m.editTags)
	}
	if len(ed.removed) != 0 {
		t.Errorf("RemoveTag calls = %v, want none", ed.removed)
	}
}

// === Entering edit mode ==============================================

func TestSM_EnterOnNameEntersEditMode(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldName

	m = smKey(t, m, keyEnter)

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit after Enter on Name", m.editMode)
	}
}

func TestSM_EOnNameEntersEditMode(t *testing.T) {
	// `e` on the NAME field enters edit mode (mirroring Enter) — matching the
	// `⏎/e edit` footer hint. Regression guard: `e` was previously wired only for
	// chips, leaving the Name field reachable by Enter alone.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldName

	m = smKey(t, m, runeKey("e"))

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit after e on Name", m.editMode)
	}
	if m.editBuffer != "proj" {
		t.Errorf("editBuffer = %q, want %q (seeded from the name)", m.editBuffer, "proj")
	}
}

func TestSM_EOnNameInEditModeIsLiteralChar(t *testing.T) {
	// Once editing the Name, `e` is a literal character (the navigate-mode `e`
	// shortcut must not fire), so it lands in the buffer at the cursor.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldName
	m = smKey(t, m, keyEnter) // enter edit on Name (buffer="proj")

	m = typeRunes(t, m, "e") // -> "proje"

	if m.editBuffer != "proje" {
		t.Errorf("editBuffer = %q, want %q (e is a literal char while editing the name)", m.editBuffer, "proje")
	}
}

func TestSM_EnterOnChipEntersEditMode(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0 // on chip "a"

	m = smKey(t, m, keyEnter)

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit after Enter on chip", m.editMode)
	}
	if m.editBuffer != "a" {
		t.Errorf("editBuffer = %q, want %q (seeded from the chip)", m.editBuffer, "a")
	}
}

func TestSM_EOnChipEntersEditMode(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0

	m = smKey(t, m, runeKey("e"))

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit after e on chip", m.editMode)
	}
	if m.editBuffer != "a" {
		t.Errorf("editBuffer = %q, want %q", m.editBuffer, "a")
	}
}

func TestSM_EnterOnAddSlotSpawnsNewEmptyChipInEditMode(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = len(m.editAliases) // + add slot

	m = smKey(t, m, keyEnter)

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit", m.editMode)
	}
	if m.editBuffer != "" {
		t.Errorf("editBuffer = %q, want empty (brand-new chip)", m.editBuffer)
	}
	if !m.editIsNewChip {
		t.Errorf("editIsNewChip = false, want true (Enter on add slot spawns a new chip)")
	}
}

func TestSM_PlusOnAddSlotSpawnsNewEmptyChipInEditMode(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags) // + add slot

	m = smKey(t, m, runeKey("+"))

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit after + on add slot", m.editMode)
	}
	if !m.editIsNewChip {
		t.Errorf("editIsNewChip = false, want true")
	}
}

func TestSM_LandingOnAddSlotIsNavigateOnly(t *testing.T) {
	// Tab onto a chip field lands on the add slot but stays in NAVIGATE mode —
	// it never auto-enters edit.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m = smKey(t, m, keyTab) // Name -> Aliases (add slot)
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate (landing on add slot must not auto-edit)", m.editMode)
	}

	// And arriving via right onto the add slot is also navigate-only.
	m = smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0
	m = smKey(t, m, keyRight) // chip -> add slot
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate after right onto add slot", m.editMode)
	}
}

// === Edit mode: text cursor + typing =================================

func TestSM_TypingEditsTheLiveValue(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags)
	m = smKey(t, m, keyEnter) // spawn new chip in edit mode

	m = typeRunes(t, m, "abc")

	if m.editBuffer != "abc" {
		t.Errorf("editBuffer = %q, want %q", m.editBuffer, "abc")
	}
	if m.editCursor != 3 {
		t.Errorf("editCursor = %d, want 3", m.editCursor)
	}
}

func TestSM_LeftRightMovesTextCursorInEditMode(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags)
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "abc") // cursor at 3

	m = smKey(t, m, keyLeft)
	if m.editCursor != 2 {
		t.Fatalf("after left, editCursor = %d, want 2", m.editCursor)
	}
	m = smKey(t, m, keyLeft)
	m = smKey(t, m, keyLeft)
	if m.editCursor != 0 {
		t.Fatalf("after 3 lefts, editCursor = %d, want 0", m.editCursor)
	}
	m = smKey(t, m, keyLeft)
	if m.editCursor != 0 {
		t.Fatalf("left bounded at 0, editCursor = %d", m.editCursor)
	}
	m = smKey(t, m, keyRight)
	if m.editCursor != 1 {
		t.Fatalf("after right, editCursor = %d, want 1", m.editCursor)
	}
	// Insert at the cursor position.
	m = typeRunes(t, m, "X")
	if m.editBuffer != "aXbc" {
		t.Errorf("editBuffer = %q, want %q (insert at cursor)", m.editBuffer, "aXbc")
	}
}

func TestSM_TabIsIgnoredInEditMode(t *testing.T) {
	// §8.2 treats Tab/Shift+Tab as navigate-mode field moves; in edit mode (one
	// element live) they are ignored — focus and buffer are untouched.
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags)
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "ab")

	m = smKey(t, m, keyTab)
	m = smKey(t, m, keyShiftTab)

	if m.editMode != editModeEdit {
		t.Errorf("editMode = %d, want editModeEdit (Tab must not exit edit)", m.editMode)
	}
	if m.editFocus != editFieldTags {
		t.Errorf("editFocus = %d, want Tags (Tab must not move fields in edit)", m.editFocus)
	}
	if m.editBuffer != "ab" {
		t.Errorf("editBuffer = %q, want %q (Tab must not alter the buffer)", m.editBuffer, "ab")
	}
}

func TestSM_XInEditModeIsALiteralChar(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags)
	m = smKey(t, m, keyEnter)

	m = typeRunes(t, m, "x")

	if m.editBuffer != "x" {
		t.Errorf("editBuffer = %q, want %q (x is a literal char in edit mode)", m.editBuffer, "x")
	}
}

// === Edit mode: commit & persist =====================================

func TestSM_CommitNamePersistsViaRename(t *testing.T) {
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldName
	m = smKey(t, m, keyEnter) // enter edit on Name (buffer="proj")
	m = typeRunes(t, m, "X")  // -> "projX"

	m = smKey(t, m, keyEnter) // commit

	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate after commit", m.editMode)
	}
	if m.editName != "projX" {
		t.Errorf("editName = %q, want projX", m.editName)
	}
	wantRn := []smRename{{"/p/one", "projX", "cli"}}
	if !reflect.DeepEqual(ed.renames, wantRn) {
		t.Errorf("Rename calls = %v, want %v", ed.renames, wantRn)
	}
}

func TestSM_CommitNewAliasPersistsViaSetAndSave(t *testing.T) {
	al := &smAliasEditor{aliases: map[string]string{}}
	m := smModel(&smProjectEditor{}, al, nil, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0 // add slot (no chips)
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "my")

	m, _ = smKeyCmd(t, m, keyEnter) // commit

	if !reflect.DeepEqual(m.editAliases, []string{"my"}) {
		t.Errorf("editAliases = %v, want [my]", m.editAliases)
	}
	wantSet := []smAliasSet{{"my", "/p/one", "cli"}}
	if !reflect.DeepEqual(al.setCalls, wantSet) {
		t.Errorf("SetAndSave calls = %v, want %v", al.setCalls, wantSet)
	}
}

func TestSM_NewAliasCollisionWithAnotherProjectIsSilentRevert(t *testing.T) {
	// "w" already maps to a DIFFERENT project path. The collision pre-check
	// (Load then reject) must skip the SetAndSave and silently drop the chip.
	al := &smAliasEditor{aliases: map[string]string{"w": "/p/other"}}
	m := smModel(&smProjectEditor{}, al, nil, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "w")

	m = smKey(t, m, keyEnter) // commit -> collision

	if len(al.setCalls) != 0 {
		t.Errorf("SetAndSave calls = %v, want none (collision pre-check rejects)", al.setCalls)
	}
	if len(m.editAliases) != 0 {
		t.Errorf("editAliases = %v, want empty (collision chip dropped silently)", m.editAliases)
	}
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate (no blocking modal)", m.editMode)
	}
}

func TestSM_CommitNewTagPersistsViaAddTag(t *testing.T) {
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldTags
	m.editTagCursor = 0 // add slot
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "design")

	m = smKey(t, m, keyEnter) // commit

	if !reflect.DeepEqual(m.editTags, []string{"design"}) {
		t.Errorf("editTags = %v, want [design]", m.editTags)
	}
	wantAdd := []smTagCall{{"/p/one", "design"}}
	if !reflect.DeepEqual(ed.added, wantAdd) {
		t.Errorf("AddTag calls = %v, want %v", ed.added, wantAdd)
	}
}

// === Falling-out rules ===============================================

func TestSM_EditExistingAliasCommitsViaDeleteThenSet(t *testing.T) {
	// Editing an existing alias chip to a new value deletes the old name and
	// sets the new one (DeleteAndSave then SetAndSave).
	al := &smAliasEditor{aliases: map[string]string{"a": "/p/one"}}
	m := smModel(&smProjectEditor{}, al, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0
	m = smKey(t, m, keyEnter) // edit chip "a"
	// Replace "a" with "b": backspace then type.
	m = smKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeRunes(t, m, "b")

	m = smKey(t, m, keyEnter) // commit

	if !reflect.DeepEqual(m.editAliases, []string{"b"}) {
		t.Errorf("editAliases = %v, want [b]", m.editAliases)
	}
	if !reflect.DeepEqual(al.deleted, []string{"a"}) {
		t.Errorf("DeleteAndSave calls = %v, want [a]", al.deleted)
	}
	wantSet := []smAliasSet{{"b", "/p/one", "cli"}}
	if !reflect.DeepEqual(al.setCalls, wantSet) {
		t.Errorf("SetAndSave calls = %v, want %v", al.setCalls, wantSet)
	}
}

func TestSM_EditExistingTagCommitsViaRemoveThenAdd(t *testing.T) {
	// Editing an existing tag chip to a new value removes the old and adds the new.
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, []string{"work"})
	m.editFocus = editFieldTags
	m.editTagCursor = 0
	m = smKey(t, m, keyEnter) // edit chip "work"
	for range "work" {
		m = smKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m = typeRunes(t, m, "home")

	m = smKey(t, m, keyEnter) // commit

	if !reflect.DeepEqual(m.editTags, []string{"home"}) {
		t.Errorf("editTags = %v, want [home]", m.editTags)
	}
	wantRm := []smTagCall{{"/p/one", "work"}}
	if !reflect.DeepEqual(ed.removed, wantRm) {
		t.Errorf("RemoveTag calls = %v, want %v", ed.removed, wantRm)
	}
	wantAdd := []smTagCall{{"/p/one", "home"}}
	if !reflect.DeepEqual(ed.added, wantAdd) {
		t.Errorf("AddTag calls = %v, want %v", ed.added, wantAdd)
	}
}

func TestSM_ExistingChipCommittedEmptyIsDeleted(t *testing.T) {
	al := &smAliasEditor{aliases: map[string]string{"a": "/p/one"}}
	m := smModel(&smProjectEditor{}, al, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0
	m = smKey(t, m, keyEnter)     // edit chip "a" (buffer "a")
	m = smKey(t, m, runeKey("x")) // literal x -> "ax"
	m = smKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = smKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace}) // clear to ""

	m = smKey(t, m, keyEnter) // commit empty -> delete

	if len(m.editAliases) != 0 {
		t.Errorf("editAliases = %v, want empty (empty-on-commit = delete)", m.editAliases)
	}
	if !reflect.DeepEqual(al.deleted, []string{"a"}) {
		t.Errorf("DeleteAndSave calls = %v, want [a]", al.deleted)
	}
}

func TestSM_EmptyNameCommitRevertsToPrior(t *testing.T) {
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldName
	m = smKey(t, m, keyEnter) // edit Name (buffer "proj")
	// Clear the whole name.
	for range "proj" {
		m = smKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}

	m = smKey(t, m, keyEnter) // commit empty

	if m.editName != "proj" {
		t.Errorf("editName = %q, want proj (empty Name reverts to prior)", m.editName)
	}
	if len(ed.renames) != 0 {
		t.Errorf("Rename calls = %v, want none (empty Name does not persist)", ed.renames)
	}
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate (no blocking error)", m.editMode)
	}
}

func TestSM_DuplicateChipCommitIsSilentNoOp(t *testing.T) {
	// New tag chip committed with a value already present -> dedupe, no AddTag.
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, []string{"work"})
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags) // add slot
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "work")

	m = smKey(t, m, keyEnter) // commit duplicate

	if !reflect.DeepEqual(m.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work] (duplicate dedupes, nothing added)", m.editTags)
	}
	if len(ed.added) != 0 {
		t.Errorf("AddTag calls = %v, want none (duplicate is a no-op)", ed.added)
	}
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate", m.editMode)
	}
}

func TestSM_DuplicateTagIsCaseSensitive(t *testing.T) {
	// "WORK" is distinct from a present "work" (case-sensitive) — appended.
	ed := &smProjectEditor{}
	m := smModel(ed, &smAliasEditor{}, nil, []string{"work"})
	m.editFocus = editFieldTags
	m.editTagCursor = len(m.editTags)
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "WORK")

	m = smKey(t, m, keyEnter) // commit

	if !reflect.DeepEqual(m.editTags, []string{"work", "WORK"}) {
		t.Errorf("editTags = %v, want [work WORK] (case-sensitive distinct)", m.editTags)
	}
	wantAdd := []smTagCall{{"/p/one", "WORK"}}
	if !reflect.DeepEqual(ed.added, wantAdd) {
		t.Errorf("AddTag calls = %v, want %v", ed.added, wantAdd)
	}
}

func TestSM_NewEmptyChipVanishesOnEsc(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = len(m.editAliases) // add slot
	m = smKey(t, m, keyEnter)              // spawn new empty chip in edit mode

	m = smKey(t, m, keyEsc) // discard

	if !reflect.DeepEqual(m.editAliases, []string{"a"}) {
		t.Errorf("editAliases = %v, want [a] (brand-new empty chip vanishes on Esc)", m.editAliases)
	}
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate after Esc", m.editMode)
	}
	if m.modal != modalEditProject {
		t.Errorf("modal = %v, want modalEditProject (Esc in edit must not close)", m.modal)
	}
}

func TestSM_EscOnExistingChipDiscardsEditKeepsPriorValue(t *testing.T) {
	al := &smAliasEditor{aliases: map[string]string{"a": "/p/one"}}
	m := smModel(&smProjectEditor{}, al, []string{"a"}, nil)
	m.editFocus = editFieldAliases
	m.editAliasCursor = 0
	m = smKey(t, m, keyEnter) // edit chip "a"
	m = typeRunes(t, m, "ZZ") // in-progress garbage

	m = smKey(t, m, keyEsc) // discard the in-progress edit

	if !reflect.DeepEqual(m.editAliases, []string{"a"}) {
		t.Errorf("editAliases = %v, want [a] (existing chip keeps its prior value)", m.editAliases)
	}
	if len(al.setCalls) != 0 || len(al.deleted) != 0 {
		t.Errorf("no persistence expected on discard; set=%v deleted=%v", al.setCalls, al.deleted)
	}
	if m.editMode != editModeNavigate {
		t.Errorf("editMode = %d, want navigate after Esc-discard", m.editMode)
	}
}

func TestSM_EscInNavigateClosesWithoutDiscardingSavedWork(t *testing.T) {
	// A tag was already persisted live (dirty). Esc in navigate must close and
	// trigger a refresh (loadProjects cmd) — never discard the saved work.
	ed := &smProjectEditor{}
	al := &smAliasEditor{}
	m := smModel(ed, al, nil, nil)
	m.projectStore = smProjectStore{projects: []project.Project{{Path: "/p/one", Name: "proj"}}}
	m.editFocus = editFieldTags
	m.editTagCursor = 0
	m = smKey(t, m, keyEnter)
	m = typeRunes(t, m, "work")
	m = smKey(t, m, keyEnter) // commit -> AddTag (dirty)

	m, cmd := smKeyCmd(t, m, keyEsc) // close

	if m.modal != modalNone {
		t.Errorf("modal = %v, want modalNone (Esc in navigate closes)", m.modal)
	}
	if len(ed.added) != 1 {
		t.Errorf("AddTag calls = %d, want 1 (saved work must survive the close)", len(ed.added))
	}
	if cmd == nil {
		t.Errorf("Esc-close after a live edit should refresh projects (non-nil cmd)")
	}
}

func TestSM_EscInNavigateWithNoChangesClosesWithoutRefresh(t *testing.T) {
	m := smModel(&smProjectEditor{}, &smAliasEditor{}, nil, nil)
	m.editFocus = editFieldName

	m, cmd := smKeyCmd(t, m, keyEsc)

	if m.modal != modalNone {
		t.Errorf("modal = %v, want modalNone", m.modal)
	}
	if cmd != nil {
		t.Errorf("Esc-close with no edits should not refresh (nil cmd), got non-nil")
	}
}
