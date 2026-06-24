package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Pre-refactor goldens — captured from the ORIGINAL renderKillModalContent /
// renderDeleteModalContent (before the destructiveConfirmSpec consolidation), so the
// byte-identical regression below genuinely proves the refactor produced zero drift.
// The colourless (NoCol) goldens are mode-independent (all hue dropped), so dark and
// light share one literal each.
const (
	goldenKillModalDarkCol    = "\x1b[38;2;41;46;66m╭─────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20m▲\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20mKill session?\x1b[m\x1b[48;2;11;12;20m                                  \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20maviva-proxy-qNyfEO\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m· 3 windows\x1b[m\x1b[48;2;11;12;20m                  \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[48;2;11;12;20m\x1b[m\x1b[48;2;11;12;20m                                                 \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mEnds the tmux session and all its panes. Can't be\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mundone.\x1b[m\x1b[48;2;11;12;20m                                          \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mkill\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m\x1b[48;2;11;12;20m                              \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m╰─────────────────────────────────────────────────────╯\x1b[m"
	goldenDeleteModalDarkCol  = "\x1b[38;2;41;46;66m╭────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20m▲\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20mDelete project?\x1b[m\x1b[48;2;11;12;20m                               \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20mflow-v1-api\x1b[m\x1b[48;2;11;12;20m                                     \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m/Users/leeovery/Code/fabric/flow-v1-api\x1b[m\x1b[48;2;11;12;20m         \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[48;2;11;12;20m\x1b[m\x1b[48;2;11;12;20m                                                \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mRemoves this project from Portal (name, aliases,\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mtags). Your sessions and files are untouched.\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mdelete\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m\x1b[48;2;11;12;20m                           \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m╰────────────────────────────────────────────────────╯\x1b[m"
	goldenKillModalLightCol   = "\x1b[38;2;201;205;219m╭─────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231m▲\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231mKill session?\x1b[m\x1b[48;2;225;226;231m                                  \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231maviva-proxy-qNyfEO\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m· 3 windows\x1b[m\x1b[48;2;225;226;231m                  \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[48;2;225;226;231m\x1b[m\x1b[48;2;225;226;231m                                                 \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mEnds the tmux session and all its panes. Can't be\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mundone.\x1b[m\x1b[48;2;225;226;231m                                          \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mkill\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m\x1b[48;2;225;226;231m                              \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m╰─────────────────────────────────────────────────────╯\x1b[m"
	goldenDeleteModalLightCol = "\x1b[38;2;201;205;219m╭────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231m▲\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231mDelete project?\x1b[m\x1b[48;2;225;226;231m                               \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231mflow-v1-api\x1b[m\x1b[48;2;225;226;231m                                     \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m/Users/leeovery/Code/fabric/flow-v1-api\x1b[m\x1b[48;2;225;226;231m         \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[48;2;225;226;231m\x1b[m\x1b[48;2;225;226;231m                                                \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mRemoves this project from Portal (name, aliases,\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mtags). Your sessions and files are untouched.\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mdelete\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m\x1b[48;2;225;226;231m                           \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m╰────────────────────────────────────────────────────╯\x1b[m"
	goldenKillModalNoCol      = "╭─────────────────────────────────────────────────────╮\n│  \x1b[1m▲\x1b[m \x1b[1mKill session?\x1b[m                                    │\n├─────────────────────────────────────────────────────┤\n│  \x1b[1maviva-proxy-qNyfEO\x1b[m  · 3 windows                    │\n│                                                     │\n│  Ends the tmux session and all its panes. Can't be  │\n│  undone.                                            │\n├─────────────────────────────────────────────────────┤\n│  y kill   esc cancel                                │\n╰─────────────────────────────────────────────────────╯"
	goldenDeleteModalNoCol    = "╭────────────────────────────────────────────────────╮\n│  \x1b[1m▲\x1b[m \x1b[1mDelete project?\x1b[m                                 │\n├────────────────────────────────────────────────────┤\n│  \x1b[1mflow-v1-api\x1b[m                                       │\n│  /Users/leeovery/Code/fabric/flow-v1-api           │\n│                                                    │\n│  Removes this project from Portal (name, aliases,  │\n│  tags). Your sessions and files are untouched.     │\n├────────────────────────────────────────────────────┤\n│  y delete   esc cancel                             │\n╰────────────────────────────────────────────────────╯"
)

// TestKillDeleteModalContent_ByteIdenticalGolden is the heart of the consolidation:
// it pins renderKillModalContent and renderDeleteModalContent against goldens captured
// from the PRE-refactor render, proving the destructiveConfirmSpec consolidation
// produced zero output drift for both modals in both modes and the colourless carve-out.
func TestKillDeleteModalContent_ByteIdenticalGolden(t *testing.T) {
	const (
		killName    = "aviva-proxy-qNyfEO"
		killWindows = 3
		delName     = "flow-v1-api"
		delPath     = "/Users/leeovery/Code/fabric/flow-v1-api"
	)

	t.Run("kill", func(t *testing.T) {
		cases := []struct {
			label      string
			mode       theme.Mode
			colourless bool
			want       string
		}{
			{"dark", theme.Dark, false, goldenKillModalDarkCol},
			{"light", theme.Light, false, goldenKillModalLightCol},
			{"dark colourless", theme.Dark, true, goldenKillModalNoCol},
			{"light colourless", theme.Light, true, goldenKillModalNoCol},
		}
		for _, tc := range cases {
			t.Run(tc.label, func(t *testing.T) {
				got := renderKillModalContent(killName, killWindows, tc.mode, tc.colourless)
				if got != tc.want {
					t.Errorf("kill modal drift\n got: %q\nwant: %q", got, tc.want)
				}
			})
		}
	})

	t.Run("delete", func(t *testing.T) {
		cases := []struct {
			label      string
			mode       theme.Mode
			colourless bool
			want       string
		}{
			{"dark", theme.Dark, false, goldenDeleteModalDarkCol},
			{"light", theme.Light, false, goldenDeleteModalLightCol},
			{"dark colourless", theme.Dark, true, goldenDeleteModalNoCol},
			{"light colourless", theme.Light, true, goldenDeleteModalNoCol},
		}
		for _, tc := range cases {
			t.Run(tc.label, func(t *testing.T) {
				got := renderDeleteModalContent(delName, delPath, tc.mode, tc.colourless)
				if got != tc.want {
					t.Errorf("delete modal drift\n got: %q\nwant: %q", got, tc.want)
				}
			})
		}
	})
}

// TestRenderDestructiveConfirm_KillSpec asserts the shared renderer reproduces the kill
// modal output byte-for-byte when fed a kill spec (no extra body rows — the count rides
// the name row via nameTrailer), in both modes and with colourless true/false.
func TestRenderDestructiveConfirm_KillSpec(t *testing.T) {
	spec := destructiveConfirmSpec{
		title:        killTitle,
		targetName:   "aviva-proxy-qNyfEO",
		nameTrailer:  killWindowCount(3),
		consequence:  killConsequence,
		confirmKey:   killKeyConfirm,
		confirmLabel: killLabelConfirm,
	}
	cases := []struct {
		label      string
		mode       theme.Mode
		colourless bool
		want       string
	}{
		{"dark", theme.Dark, false, goldenKillModalDarkCol},
		{"light", theme.Light, false, goldenKillModalLightCol},
		{"dark colourless", theme.Dark, true, goldenKillModalNoCol},
		{"light colourless", theme.Light, true, goldenKillModalNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := renderDestructiveConfirm(spec, tc.mode, tc.colourless)
			if got != tc.want {
				t.Errorf("kill spec drift\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestRenderDestructiveConfirm_DeleteSpec asserts the shared renderer reproduces the
// delete modal output byte-for-byte when fed a delete spec WITH the project-path
// extra-body row, in both modes and with colourless true/false.
func TestRenderDestructiveConfirm_DeleteSpec(t *testing.T) {
	spec := destructiveConfirmSpec{
		title:         deleteTitle,
		targetName:    "flow-v1-api",
		extraBodyRows: []string{deleteModalPathRow("/Users/leeovery/Code/fabric/flow-v1-api", theme.Dark, false)},
		consequence:   deleteConsequence,
		confirmKey:    deleteKeyConfirm,
		confirmLabel:  deleteLabelConfirm,
	}
	// extraBodyRows are already-styled rows; re-style per mode/colourless inside each case.
	cases := []struct {
		label      string
		mode       theme.Mode
		colourless bool
		want       string
	}{
		{"dark", theme.Dark, false, goldenDeleteModalDarkCol},
		{"light", theme.Light, false, goldenDeleteModalLightCol},
		{"dark colourless", theme.Dark, true, goldenDeleteModalNoCol},
		{"light colourless", theme.Light, true, goldenDeleteModalNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			spec.extraBodyRows = []string{deleteModalPathRow("/Users/leeovery/Code/fabric/flow-v1-api", tc.mode, tc.colourless)}
			got := renderDestructiveConfirm(spec, tc.mode, tc.colourless)
			if got != tc.want {
				t.Errorf("delete spec drift\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestDestructiveConsequenceRows_WordWrapAt52 asserts the shared consequence word-wrap
// at the single body-width const (52) matches the prior per-modal line-splitting for a
// multi-line consequence — the kill and delete copies both wrap to their known line
// shapes, proving the factored wrap loop preserves the §8.3/§8.6 break points.
func TestDestructiveConsequenceRows_WordWrapAt52(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "kill consequence",
			text: killConsequence,
			want: []string{
				"Ends the tmux session and all its panes. Can't be",
				"undone.",
			},
		},
		{
			name: "delete consequence",
			text: deleteConsequence,
			want: []string{
				"Removes this project from Portal (name, aliases,",
				"tags). Your sessions and files are untouched.",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := destructiveConsequenceRows(tc.text, theme.Dark, false)
			if len(rows) != len(tc.want) {
				t.Fatalf("want %d wrapped lines, got %d: %v", len(tc.want), len(rows), rows)
			}
			for i, row := range rows {
				if got := ansi.Strip(row); got != tc.want[i] {
					t.Errorf("line %d: got %q, want %q", i, got, tc.want[i])
				}
			}
			// Every wrapped line stays within the body width.
			for i, row := range rows {
				if w := len([]rune(ansi.Strip(row))); w > destructiveBodyWidth {
					t.Errorf("line %d width %d exceeds body width %d: %q", i, w, destructiveBodyWidth, ansi.Strip(row))
				}
			}
		})
	}
}
