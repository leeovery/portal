package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/project"
)

// TestRenderEditProjectContent_ByteExact pins the full rendered output of the
// edit-project modal's interim (pre-3-9) render across the navigate-mode focus
// states (Name/Aliases/Tags focus, empty "(none)" state, populated with the
// element index on a chip, the index on the + add slot) plus one edit-mode state
// (a brand-new chip showing its live buffer in the Add slot). The §8.2 MV chip
// render is task 3-9; this oracle guards the interim render against the new
// two-mode state model.
func TestRenderEditProjectContent_ByteExact(t *testing.T) {
	tests := []struct {
		name      string
		mode      editMode
		focus     editField
		aliases   []string
		aliasCur  int
		tags      []string
		tagCur    int
		buffer    string
		isNewChip bool
		want      string
	}{
		{
			name:  "name-focus-empty",
			focus: editFieldName,
			want:  "Edit: Portal\n\n> Name: MyName\n\n  Aliases:\n    (none)\n    Add: \n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] edit/save  [Esc] back  [Tab] next field",
		},
		{
			name:     "aliases-focus-addslot",
			focus:    editFieldAliases,
			aliasCur: 0,
			want:     "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n    (none)\n  > Add: \n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] edit/save  [Esc] back  [Tab] next field",
		},
		{
			name:     "aliases-focus-chip",
			focus:    editFieldAliases,
			aliases:  []string{"a1", "a2"},
			aliasCur: 1,
			want:     "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n    [x] a1\n  > [x] a2\n    Add: \n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] edit/save  [Esc] back  [Tab] next field",
		},
		{
			name:     "aliases-focus-addslot-with-chip",
			focus:    editFieldAliases,
			aliases:  []string{"a1"},
			aliasCur: 1,
			want:     "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n    [x] a1\n  > Add: \n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] edit/save  [Esc] back  [Tab] next field",
		},
		{
			name:   "tags-focus-chip",
			focus:  editFieldTags,
			tags:   []string{"t1", "t2"},
			tagCur: 0,
			want:   "Edit: Portal\n\n  Name: MyName\n\n  Aliases:\n    (none)\n    Add: \n\n> Tags:\n  > [x] t1\n    [x] t2\n    Add: \n\n  [Enter] edit/save  [Esc] back  [Tab] next field",
		},
		{
			name:      "tags-edit-new-chip-shows-buffer",
			mode:      editModeEdit,
			focus:     editFieldTags,
			aliases:   []string{"a1"},
			tags:      []string{"t1"},
			tagCur:    1,
			buffer:    "addtag",
			isNewChip: true,
			want:      "Edit: Portal\n\n  Name: MyName\n\n  Aliases:\n    [x] a1\n    Add: \n\n> Tags:\n    [x] t1\n  > Add: addtag\n\n  [Enter] edit/save  [Esc] back  [Tab] next field",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				modal:           modalEditProject,
				editProject:     project.Project{Name: "Portal"},
				editMode:        tc.mode,
				editFocus:       tc.focus,
				editName:        "MyName",
				editAliases:     tc.aliases,
				editAliasCursor: tc.aliasCur,
				editTags:        tc.tags,
				editTagCursor:   tc.tagCur,
				editBuffer:      tc.buffer,
				editIsNewChip:   tc.isNewChip,
			}
			got := m.renderEditProjectContent()
			if got != tc.want {
				t.Errorf("render mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}
