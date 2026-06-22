package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
)

// TestRenderEditProjectContent_ByteExact pins the full ANSI-stripped layout of the
// §13.1 MV edit-project modal across the navigate-name, chip-focused, and editing
// states. The colour-role assertions live in edit_modal_test.go; this oracle guards
// the STRUCTURE (panel frame, field labels, the rounded NAME box, the square chip
// boxes, the faint `+ add` slot, and the contextual footers) byte-for-byte so a
// layout regression (a box overrunning the frame, a missing inset, a wrong glyph)
// fails loudly. The render is deterministic once ANSI is stripped.
//
// All three states render at the SAME panel width: the header reserves the
// `◉ EDIT MODE` badge's slot in navigate (blank) and right-aligns the badge in the
// far corner while editing, and the panel width is anchored by the (fixed-width)
// header / name box / footer — so toggling navigate↔edit never resizes the panel
// (the "jaggedy" resize bug) even though an editing chip box is one cell wider for
// its live cursor (a navigate chip is sized to its content). The editing-in-place
// footer additionally right-aligns its `empty on save = delete` consequence note to
// the far-right corner (the same fixed-width-row + flexible-spacer technique as the
// header badge); the navigate footers stay left-packed.
func TestRenderEditProjectContent_ByteExact(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *Model)
		want  string
	}{
		{
			name:  "navigate-name-focused",
			setup: func(m *Model) {},
			want: "╭──────────────────────────────────────────────────────────────╮\n" +
				"│  Edit Project flow-v1-api                                    │\n" +
				"├──────────────────────────────────────────────────────────────┤\n" +
				"│  NAME                                                        │\n" +
				"│  ╭──────────────────────────────────────────────────────╮    │\n" +
				"│  │ flow-v1-api                                          │    │\n" +
				"│  ╰──────────────────────────────────────────────────────╯    │\n" +
				"│                                                              │\n" +
				"│  ALIASES                                                     │\n" +
				"│  ┌──────┐ ┌────┐                                             │\n" +
				"│  │ fapi │ │ v1 │ + add                                       │\n" +
				"│  └──────┘ └────┘                                             │\n" +
				"│                                                              │\n" +
				"│  TAGS                                                        │\n" +
				"│  ┌────────┐ ┌─────┐                                          │\n" +
				"│  │ Fabric │ │ api │ + add                                    │\n" +
				"│  └────────┘ └─────┘                                          │\n" +
				"├──────────────────────────────────────────────────────────────┤\n" +
				"│  ⏎/e edit · ⇥ next field · esc close                         │\n" +
				"╰──────────────────────────────────────────────────────────────╯",
		},
		{
			name: "navigate-tag-chip-focused",
			setup: func(m *Model) {
				m.editFocus = editFieldTags
				m.editTagCursor = 0
			},
			want: "╭──────────────────────────────────────────────────────────────╮\n" +
				"│  Edit Project flow-v1-api                                    │\n" +
				"├──────────────────────────────────────────────────────────────┤\n" +
				"│  NAME                                                        │\n" +
				"│  ╭──────────────────────────────────────────────────────╮    │\n" +
				"│  │ flow-v1-api                                          │    │\n" +
				"│  ╰──────────────────────────────────────────────────────╯    │\n" +
				"│                                                              │\n" +
				"│  ALIASES                                                     │\n" +
				"│  ┌──────┐ ┌────┐                                             │\n" +
				"│  │ fapi │ │ v1 │ + add                                       │\n" +
				"│  └──────┘ └────┘                                             │\n" +
				"│                                                              │\n" +
				"│  TAGS                                                        │\n" +
				"│  ┌────────┐ ┌─────┐                                          │\n" +
				"│  │ Fabric │ │ api │ + add                                    │\n" +
				"│  └────────┘ └─────┘                                          │\n" +
				"├──────────────────────────────────────────────────────────────┤\n" +
				"│  ⏎/e edit · x remove · ←→ move · ⇥ next field · esc close    │\n" +
				"╰──────────────────────────────────────────────────────────────╯",
		},
		{
			name: "editing-tag-chip",
			setup: func(m *Model) {
				m.editFocus = editFieldTags
				m.editMode = editModeEdit
				m.editTagCursor = 0
				m.editBuffer = "Fabric"
				m.editCursor = len([]rune("Fabric"))
			},
			want: "╭──────────────────────────────────────────────────────────────╮\n" +
				"│  Edit Project flow-v1-api                       ◉ EDIT MODE  │\n" +
				"├──────────────────────────────────────────────────────────────┤\n" +
				"│  NAME                                                        │\n" +
				"│  ╭──────────────────────────────────────────────────────╮    │\n" +
				"│  │ flow-v1-api                                          │    │\n" +
				"│  ╰──────────────────────────────────────────────────────╯    │\n" +
				"│                                                              │\n" +
				"│  ALIASES                                                     │\n" +
				"│  ┌──────┐ ┌────┐                                             │\n" +
				"│  │ fapi │ │ v1 │ + add                                       │\n" +
				"│  └──────┘ └────┘                                             │\n" +
				"│                                                              │\n" +
				"│  TAGS                                                        │\n" +
				"│  ┌─────────┐ ┌─────┐                                         │\n" +
				"│  │ Fabric  │ │ api │ + add                                   │\n" +
				"│  └─────────┘ └─────┘                                         │\n" +
				"├──────────────────────────────────────────────────────────────┤\n" +
				"│  ⏎ save · esc discard · ←→ cursor    empty on save = delete  │\n" +
				"╰──────────────────────────────────────────────────────────────╯",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				modal:       modalEditProject,
				editProject: project.Project{Name: "flow-v1-api"},
				editMode:    editModeNavigate,
				editFocus:   editFieldName,
				editName:    "flow-v1-api",
				editAliases: []string{"fapi", "v1"},
				editTags:    []string{"Fabric", "api"},
			}
			tc.setup(&m)
			got := ansi.Strip(m.renderEditProjectContent())
			if got != tc.want {
				t.Errorf("render mismatch\n got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}
