package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/project"
)

// TestRenderEditProjectContent_ByteExact is the byte-identical regression oracle
// for the renderEditListField extraction: it pins the full rendered output of the
// edit-project modal across every relevant state (Name/Aliases/Tags focus, empty
// "(none)" state, populated with cursor on an entry, cursor on the Add row, and
// the error footer). The expected strings were captured from the pre-refactor
// inline implementation; the helper must reproduce them exactly.
func TestRenderEditProjectContent_ByteExact(t *testing.T) {
	tests := []struct {
		name      string
		focus     editField
		aliases   []string
		aliasCur  int
		newAlias  string
		tags      []string
		tagCur    int
		newTag    string
		editError string
		want      string
	}{
		{
			name:  "name-focus-empty",
			focus: editFieldName,
			want:  "Edit: Portal\n\n> Name: MyName\n\n  Aliases:\n    (none)\n    Add: \n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:     "aliases-focus-empty-add",
			focus:    editFieldAliases,
			newAlias: "draftA",
			want:     "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n    (none)\n  > Add: draftA\n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:     "aliases-focus-entry",
			focus:    editFieldAliases,
			aliases:  []string{"a1", "a2"},
			aliasCur: 1,
			newAlias: "na",
			want:     "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n    [x] a1\n  > [x] a2\n    Add: na\n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:     "aliases-focus-addrow",
			focus:    editFieldAliases,
			aliases:  []string{"a1"},
			aliasCur: 1,
			newAlias: "newone",
			want:     "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n    [x] a1\n  > Add: newone\n\n  Tags:\n    (none)\n    Add: \n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:   "tags-focus-empty",
			focus:  editFieldTags,
			newTag: "tg",
			want:   "Edit: Portal\n\n  Name: MyName\n\n  Aliases:\n    (none)\n    Add: \n\n> Tags:\n    (none)\n  > Add: tg\n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:   "tags-focus-entry",
			focus:  editFieldTags,
			tags:   []string{"t1", "t2"},
			tagCur: 0,
			want:   "Edit: Portal\n\n  Name: MyName\n\n  Aliases:\n    (none)\n    Add: \n\n> Tags:\n  > [x] t1\n    [x] t2\n    Add: \n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:    "tags-focus-addrow",
			focus:   editFieldTags,
			aliases: []string{"a1"},
			tags:    []string{"t1"},
			tagCur:  1,
			newTag:  "addtag",
			want:    "Edit: Portal\n\n  Name: MyName\n\n  Aliases:\n    [x] a1\n    Add: \n\n> Tags:\n    [x] t1\n  > Add: addtag\n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
		{
			name:      "with-error",
			focus:     editFieldAliases,
			aliases:   []string{"a1"},
			tags:      []string{"t1"},
			editError: "boom",
			want:      "Edit: Portal\n\n  Name: MyName\n\n> Aliases:\n  > [x] a1\n    Add: \n\n  Tags:\n    [x] t1\n    Add: \n\n  Error: boom\n\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				modal:           modalEditProject,
				editProject:     project.Project{Name: "Portal"},
				editFocus:       tc.focus,
				editName:        "MyName",
				editAliases:     tc.aliases,
				editAliasCursor: tc.aliasCur,
				editNewAlias:    tc.newAlias,
				editTags:        tc.tags,
				editTagCursor:   tc.tagCur,
				editNewTag:      tc.newTag,
				editError:       tc.editError,
			}
			got := m.renderEditProjectContent()
			if got != tc.want {
				t.Errorf("render mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}
