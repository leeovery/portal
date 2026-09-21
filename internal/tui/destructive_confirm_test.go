package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	goldenKillModalDarkCol    = "\x1b[38;2;41;46;66m╭─────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20m▲\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20mKill session?\x1b[m\x1b[48;2;11;12;20m                                  \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20maviva-proxy-qNyfEO\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m· 3 windows\x1b[m\x1b[48;2;11;12;20m                  \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[48;2;11;12;20m\x1b[m\x1b[48;2;11;12;20m                                                 \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mEnds the tmux session and all its panes. Can't be\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mundone.\x1b[m\x1b[48;2;11;12;20m                                          \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mkill\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m\x1b[48;2;11;12;20m                              \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m╰─────────────────────────────────────────────────────╯\x1b[m"
	goldenDeleteModalDarkCol  = "\x1b[38;2;41;46;66m╭────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20m▲\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20mDelete project?\x1b[m\x1b[48;2;11;12;20m                               \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;247;118;142;48;2;11;12;20mflow-v1-api\x1b[m\x1b[48;2;11;12;20m                                     \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m/Users/leeovery/Code/fabric/flow-v1-api\x1b[m\x1b[48;2;11;12;20m         \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[48;2;11;12;20m\x1b[m\x1b[48;2;11;12;20m                                                \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mRemoves this project from Portal (name, aliases,\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mtags). Your sessions and files are untouched.\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;41;46;66m│\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mdelete\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m\x1b[48;2;11;12;20m                           \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;41;46;66m│\x1b[m\n\x1b[38;2;41;46;66m╰────────────────────────────────────────────────────╯\x1b[m"
	goldenKillModalLightCol   = "\x1b[38;2;201;205;219m╭─────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231m▲\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231mKill session?\x1b[m\x1b[48;2;225;226;231m                                  \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231maviva-proxy-qNyfEO\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m· 3 windows\x1b[m\x1b[48;2;225;226;231m                  \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[48;2;225;226;231m\x1b[m\x1b[48;2;225;226;231m                                                 \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mEnds the tmux session and all its panes. Can't be\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mundone.\x1b[m\x1b[48;2;225;226;231m                                          \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├─────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mkill\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m\x1b[48;2;225;226;231m                              \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m╰─────────────────────────────────────────────────────╯\x1b[m"
	goldenDeleteModalLightCol = "\x1b[38;2;201;205;219m╭────────────────────────────────────────────────────╮\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231m▲\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231mDelete project?\x1b[m\x1b[48;2;225;226;231m                               \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;189;37;69;48;2;225;226;231mflow-v1-api\x1b[m\x1b[48;2;225;226;231m                                     \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m/Users/leeovery/Code/fabric/flow-v1-api\x1b[m\x1b[48;2;225;226;231m         \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[48;2;225;226;231m\x1b[m\x1b[48;2;225;226;231m                                                \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mRemoves this project from Portal (name, aliases,\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mtags). Your sessions and files are untouched.\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m├────────────────────────────────────────────────────┤\x1b[m\n\x1b[38;2;201;205;219m│\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mdelete\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m\x1b[48;2;225;226;231m                           \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;201;205;219m│\x1b[m\n\x1b[38;2;201;205;219m╰────────────────────────────────────────────────────╯\x1b[m"
	goldenKillModalNoCol      = "╭─────────────────────────────────────────────────────╮\n│  \x1b[1m▲\x1b[m \x1b[1mKill session?\x1b[m                                    │\n├─────────────────────────────────────────────────────┤\n│  \x1b[1maviva-proxy-qNyfEO\x1b[m  · 3 windows                    │\n│                                                     │\n│  Ends the tmux session and all its panes. Can't be  │\n│  undone.                                            │\n├─────────────────────────────────────────────────────┤\n│  y kill   esc cancel                                │\n╰─────────────────────────────────────────────────────╯"
	goldenDeleteModalNoCol    = "╭────────────────────────────────────────────────────╮\n│  \x1b[1m▲\x1b[m \x1b[1mDelete project?\x1b[m                                 │\n├────────────────────────────────────────────────────┤\n│  \x1b[1mflow-v1-api\x1b[m                                       │\n│  /Users/leeovery/Code/fabric/flow-v1-api           │\n│                                                    │\n│  Removes this project from Portal (name, aliases,  │\n│  tags). Your sessions and files are untouched.     │\n├────────────────────────────────────────────────────┤\n│  y delete   esc cancel                             │\n╰────────────────────────────────────────────────────╯"
)

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
			th         theme.Theme
			colourless bool
			want       string
		}{
			{"dark", testDarkTheme(t), false, goldenKillModalDarkCol},
			{"light", testLightTheme(t), false, goldenKillModalLightCol},
			{"dark colourless", testDarkTheme(t), true, goldenKillModalNoCol},
			{"light colourless", testLightTheme(t), true, goldenKillModalNoCol},
		}
		for _, tc := range cases {
			t.Run(tc.label, func(t *testing.T) {
				got := renderKillModalContent(killName, killWindows, tc.th, tc.colourless)
				if got != tc.want {
					t.Errorf("kill modal drift\n got: %q\nwant: %q", got, tc.want)
				}
			})
		}
	})

	t.Run("delete", func(t *testing.T) {
		cases := []struct {
			label      string
			th         theme.Theme
			colourless bool
			want       string
		}{
			{"dark", testDarkTheme(t), false, goldenDeleteModalDarkCol},
			{"light", testLightTheme(t), false, goldenDeleteModalLightCol},
			{"dark colourless", testDarkTheme(t), true, goldenDeleteModalNoCol},
			{"light colourless", testLightTheme(t), true, goldenDeleteModalNoCol},
		}
		for _, tc := range cases {
			t.Run(tc.label, func(t *testing.T) {
				got := renderDeleteModalContent(delName, delPath, tc.th, tc.colourless)
				if got != tc.want {
					t.Errorf("delete modal drift\n got: %q\nwant: %q", got, tc.want)
				}
			})
		}
	})
}

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
		th         theme.Theme
		colourless bool
		want       string
	}{
		{"dark", testDarkTheme(t), false, goldenKillModalDarkCol},
		{"light", testLightTheme(t), false, goldenKillModalLightCol},
		{"dark colourless", testDarkTheme(t), true, goldenKillModalNoCol},
		{"light colourless", testLightTheme(t), true, goldenKillModalNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := renderDestructiveConfirm(spec, tc.th, tc.colourless)
			if got != tc.want {
				t.Errorf("kill spec drift\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestRenderDestructiveConfirm_DeleteSpec(t *testing.T) {
	spec := destructiveConfirmSpec{
		title:         deleteTitle,
		targetName:    "flow-v1-api",
		extraBodyRows: []string{deleteModalPathRow("/Users/leeovery/Code/fabric/flow-v1-api", testDarkTheme(t), false)},
		consequence:   deleteConsequence,
		confirmKey:    deleteKeyConfirm,
		confirmLabel:  deleteLabelConfirm,
	}
	cases := []struct {
		label      string
		th         theme.Theme
		colourless bool
		want       string
	}{
		{"dark", testDarkTheme(t), false, goldenDeleteModalDarkCol},
		{"light", testLightTheme(t), false, goldenDeleteModalLightCol},
		{"dark colourless", testDarkTheme(t), true, goldenDeleteModalNoCol},
		{"light colourless", testLightTheme(t), true, goldenDeleteModalNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			spec.extraBodyRows = []string{deleteModalPathRow("/Users/leeovery/Code/fabric/flow-v1-api", tc.th, tc.colourless)}
			got := renderDestructiveConfirm(spec, tc.th, tc.colourless)
			if got != tc.want {
				t.Errorf("delete spec drift\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

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
			rows := destructiveConsequenceRows(tc.text, destructiveBodyWidth, testDarkTheme(t), false)
			if len(rows) != len(tc.want) {
				t.Fatalf("want %d wrapped lines, got %d: %v", len(tc.want), len(rows), rows)
			}
			for i, row := range rows {
				if got := ansi.Strip(row); got != tc.want[i] {
					t.Errorf("line %d: got %q, want %q", i, got, tc.want[i])
				}
			}
			for i, row := range rows {
				if w := len([]rune(ansi.Strip(row))); w > destructiveBodyWidth {
					t.Errorf("line %d width %d exceeds body width %d: %q", i, w, destructiveBodyWidth, ansi.Strip(row))
				}
			}
		})
	}
}

func TestDestructiveConfirmSpec_TargetAndReportRows(t *testing.T) {
	th := testDarkTheme(t)
	base := destructiveConfirmSpec{
		title:        "Discard resume?",
		targetName:   "the-single-name-row",
		consequence:  killConsequence,
		confirmKey:   "y",
		confirmLabel: "discard",
	}

	t.Run("it renders targetRows in place of the single name row", func(t *testing.T) {
		spec := base
		spec.targetRows = []string{
			headerStyle(th.StateDestructive, th, true).Render("first target row"),
			headerStyle(th.StateDestructive, th, true).Render("second target row"),
		}
		body := destructiveConfirmCompartments(spec, destructiveBodyWidth, th, true)[1]
		if got := []string{ansi.Strip(body[0]), ansi.Strip(body[1])}; got[0] != "first target row" || got[1] != "second target row" {
			t.Errorf("the body opens with %q, want the two target rows", got)
		}
		for _, row := range body {
			if strings.Contains(ansi.Strip(row), base.targetName) {
				t.Errorf("the body still carries the single name row %q", ansi.Strip(row))
			}
		}
	})

	t.Run("it appends reportRows after the consequence rows", func(t *testing.T) {
		spec := base
		spec.reportRows = []string{headerStyle(th.AccentAttention, th, true).Render("the store refused")}
		body := destructiveConfirmCompartments(spec, destructiveBodyWidth, th, true)[1]
		if got := ansi.Strip(body[len(body)-1]); got != "the store refused" {
			t.Errorf("the body ends with %q, want the report row", got)
		}
		if got, want := ansi.Strip(body[len(body)-2]), "undone."; got != want {
			t.Errorf("the row above the report reads %q, want the consequence's last row %q", got, want)
		}
		quiet := destructiveConfirmCompartments(base, destructiveBodyWidth, th, true)[1]
		if len(body) != len(quiet)+1 {
			t.Errorf("the report adds %d body rows, want exactly 1", len(body)-len(quiet))
		}
	})

	t.Run("it wraps the consequence at the width it is handed", func(t *testing.T) {
		body := destructiveConfirmCompartments(base, 24, th, true)[1]
		var wrapped []string
		for _, row := range body[2:] {
			wrapped = append(wrapped, ansi.Strip(row))
		}
		want := []string{"Ends the tmux session", "and all its panes. Can't", "be undone."}
		if !reflect.DeepEqual(wrapped, want) {
			t.Errorf("the consequence wraps\n got: %q\nwant: %q", wrapped, want)
		}
	})
}
