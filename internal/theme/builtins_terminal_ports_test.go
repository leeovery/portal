package theme_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/theme"
)

// The three built-ins ported from sixteen-colour terminal schemes (the
// iTerm2-Color-Schemes set Ghostty ships). Each pins its 19 values and the
// derivation every value not taken from its scheme carries beside it.
type terminalPort struct {
	slug        string
	header      []string
	tokens      []theme.Token
	corrections []derivationRecord
	inventions  []derivationRecord
	direct      int
}

var terminalPorts = []terminalPort{
	{
		slug:   "nord-wave",
		header: []string{"Nord Wave", "https://github.com/mbadolato/iTerm2-Color-Schemes", "https://www.nordtheme.com/", "corrected for Portal's contrast floors", "state.destructive"},
		tokens: []theme.Token{
			{Name: "text.primary", Value: "#ECEFF4"},
			{Name: "text.secondary", Value: "#E5E9F0"},
			{Name: "text.tertiary", Value: "#D8DEE9"},
			{Name: "text.muted", Value: "#7D8AA2"},
			{Name: "text.subtle", Value: "#63708A"},
			{Name: "text.faint", Value: "#4C566A"},
			{Name: "text.on-selection", Value: "#FFFFFF"},
			{Name: "accent.primary", Value: "#B48EAD"},
			{Name: "accent.key", Value: "#81A1C1"},
			{Name: "accent.mode", Value: "#88C0D0"},
			{Name: "accent.attention", Value: "#EBCB8B"},
			{Name: "state.positive", Value: "#A3BE8C"},
			{Name: "state.destructive", Value: "#CB6B74"},
			{Name: "canvas", Value: "#212121"},
			{Name: "bg.selection", Value: "#3B4252"},
			{Name: "bg.attention", Value: "#2F2D28"},
			{Name: "bg.subtle", Value: "#2E3440"},
			{Name: "border", Value: "#434C5E"},
			{Name: "text.on-attention", Value: "#ECEFF4"},
		},
		corrections: []derivationRecord{
			{
				kind: "correction", marker: correctionMarker,
				token: "state.destructive", shipped: "#CB6B74",
				figures: []string{"#BF616A", "3.94", "4.52", "100.8%", "Oklab"},
			},
		},
		inventions: []derivationRecord{
			{kind: "invention", marker: inventionMarker, token: "text.muted", shipped: "#7D8AA2", figures: []string{"nord3", "4.62"}},
			{kind: "invention", marker: inventionMarker, token: "text.subtle", shipped: "#63708A", figures: []string{"nord3", "3.23"}},
			{kind: "invention", marker: inventionMarker, token: "bg.attention", shipped: "#2F2D28", figures: []string{"nord13", "7%", "1.17"}},
		},
		direct: 14,
	},
	{
		slug:   "gitlab-dark-grey",
		header: []string{"GitLab Dark Grey", "https://github.com/mbadolato/iTerm2-Color-Schemes", "nothing is corrected"},
		tokens: []theme.Token{
			{Name: "text.primary", Value: "#FFFFFF"},
			{Name: "text.secondary", Value: "#E0E0E0"},
			{Name: "text.tertiary", Value: "#BFBFBF"},
			{Name: "text.muted", Value: "#8A8A8A"},
			{Name: "text.subtle", Value: "#707070"},
			{Name: "text.faint", Value: "#515151"},
			{Name: "text.on-selection", Value: "#FFFFFF"},
			{Name: "accent.primary", Value: "#AD95E9"},
			{Name: "accent.key", Value: "#7FB6ED"},
			{Name: "accent.mode", Value: "#32C5D2"},
			{Name: "accent.attention", Value: "#D99530"},
			{Name: "state.positive", Value: "#52B87A"},
			{Name: "state.destructive", Value: "#F57F6C"},
			{Name: "canvas", Value: "#222222"},
			{Name: "bg.selection", Value: "#3A3644"},
			{Name: "bg.attention", Value: "#312B23"},
			{Name: "bg.subtle", Value: "#333333"},
			{Name: "border", Value: "#474747"},
			{Name: "text.on-attention", Value: "#E9BE74"},
		},
		inventions: []derivationRecord{
			{kind: "invention", marker: inventionMarker, token: "text.secondary", shipped: "#E0E0E0", figures: []string{"12.05"}},
			{kind: "invention", marker: inventionMarker, token: "text.muted", shipped: "#8A8A8A", figures: []string{"4.61"}},
			{kind: "invention", marker: inventionMarker, token: "text.subtle", shipped: "#707070", figures: []string{"3.21", "#666666", "2.77"}},
			{kind: "invention", marker: inventionMarker, token: "text.faint", shipped: "#515151", figures: []string{"2.00"}},
			{kind: "invention", marker: inventionMarker, token: "bg.selection", shipped: "#3A3644", figures: []string{"17%", "1.36"}},
			{kind: "invention", marker: inventionMarker, token: "bg.attention", shipped: "#312B23", figures: []string{"8%", "1.14"}},
			{kind: "invention", marker: inventionMarker, token: "bg.subtle", shipped: "#333333", figures: []string{"1.26"}},
			{kind: "invention", marker: inventionMarker, token: "border", shipped: "#474747", figures: []string{"1.71"}},
		},
		direct: 10,
	},
	{
		slug:   "horizon",
		header: []string{"Horizon", "https://github.com/mbadolato/iTerm2-Color-Schemes", "nothing is corrected", "#6C6F93"},
		tokens: []theme.Token{
			{Name: "text.primary", Value: "#E5E5E5"},
			{Name: "text.secondary", Value: "#D5D8DA"},
			{Name: "text.tertiary", Value: "#B7B9CA"},
			{Name: "text.muted", Value: "#8285A3"},
			{Name: "text.subtle", Value: "#6C6F93"},
			{Name: "text.faint", Value: "#4B4D65"},
			{Name: "text.on-selection", Value: "#FFFFFF"},
			{Name: "accent.primary", Value: "#EE64AC"},
			{Name: "accent.key", Value: "#26BBD9"},
			{Name: "accent.mode", Value: "#59E1E3"},
			{Name: "accent.attention", Value: "#FAB795"},
			{Name: "state.positive", Value: "#29D398"},
			{Name: "state.destructive", Value: "#E95678"},
			{Name: "canvas", Value: "#1C1E26"},
			{Name: "bg.selection", Value: "#36394A"},
			{Name: "bg.attention", Value: "#2C292E"},
			{Name: "bg.subtle", Value: "#2E303E"},
			{Name: "border", Value: "#414358"},
			{Name: "text.on-attention", Value: "#FBC3A7"},
		},
		inventions: []derivationRecord{
			{kind: "invention", marker: inventionMarker, token: "text.tertiary", shipped: "#B7B9CA", figures: []string{"#6C6F93", "8.56"}},
			{kind: "invention", marker: inventionMarker, token: "text.muted", shipped: "#8285A3", figures: []string{"4.62"}},
			{kind: "invention", marker: inventionMarker, token: "text.faint", shipped: "#4B4D65", figures: []string{"2.02"}},
			{kind: "invention", marker: inventionMarker, token: "bg.selection", shipped: "#36394A", figures: []string{"#6C6F93", "33%", "1.46"}},
			{kind: "invention", marker: inventionMarker, token: "bg.attention", shipped: "#2C292E", figures: []string{"7%", "1.16"}},
			{kind: "invention", marker: inventionMarker, token: "bg.subtle", shipped: "#2E303E", figures: []string{"1.27"}},
			{kind: "invention", marker: inventionMarker, token: "border", shipped: "#414358", figures: []string{"#6C6F93", "46%", "1.72"}},
		},
		direct: 11,
	},
}

func TestLoadBuiltin_TerminalPortsAreValid(t *testing.T) {
	for _, port := range terminalPorts {
		t.Run(port.slug, func(t *testing.T) {
			got, rejection, found := theme.Loader{}.LoadBuiltin(port.slug)

			if rejection != nil {
				t.Fatalf("LoadBuiltin(%q) rejected the embedded file: %v", port.slug, rejection)
			}
			if !found {
				t.Fatalf("LoadBuiltin(%q) reported not found, want the embedded built-in", port.slug)
			}
			if got.Slug != port.slug {
				t.Errorf("slug = %q, want %q", got.Slug, port.slug)
			}
			if tokens := got.Theme.All(); !slices.Equal(tokens, port.tokens) {
				t.Errorf("theme = %+v, want %+v", tokens, port.tokens)
			}
		})
	}
}

func TestTerminalPortFiles_CorrectionsAndInventionsCarryComments(t *testing.T) {
	for _, port := range terminalPorts {
		t.Run(port.slug, func(t *testing.T) {
			// Every value is either taken from the scheme, corrected, invented, or
			// text.on-selection's contrast choice; the header states the sum.
			accounted := port.direct + len(port.corrections) + len(port.inventions) + 1
			if accounted != len(theme.TokenNames()) {
				t.Fatalf("%d direct + %d corrected + %d invented + text.on-selection = %d, want all %d roles",
					port.direct, len(port.corrections), len(port.inventions), accounted, len(theme.TokenNames()))
			}

			marked := fmt.Sprintf("the port is %d values taken directly, %d corrections and %d inventions",
				port.direct, len(port.corrections), len(port.inventions))
			assertDerivationRecords(t, readBuiltinFile(t, port.slug),
				slices.Concat(port.corrections, port.inventions), marked)
		})
	}
}

func TestTerminalPortFiles_HeaderAttributesThePalette(t *testing.T) {
	for _, port := range terminalPorts {
		t.Run(port.slug, func(t *testing.T) {
			header := leadingCommentBlock(t, builtinPath(port.slug), readBuiltinFile(t, port.slug))

			for _, phrase := range port.header {
				if !strings.Contains(header, phrase) {
					t.Errorf("the header omits %q:\n%s", phrase, header)
				}
			}
		})
	}
}
