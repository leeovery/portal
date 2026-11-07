package capture

import (
	"fmt"
	"slices"
	"testing"
)

// sweepRuleName is a rule of the size sweep a drawing breaks, as the sweep
// finds it.
type sweepRuleName string

const (
	// ruleFits (R1) is a draw of other than the terminal's rows, or a row of
	// other than its width.
	ruleFits sweepRuleName = "fitting its terminal"
	// ruleChrome (R2) is a header's first row or a footer row other than the
	// tall draw's, from chromeFloor up.
	ruleChrome sweepRuleName = "keeping its chrome"
	// ruleCursor (R3) is the cursor's row, as the tall draw shows it, not
	// shown.
	ruleCursor sweepRuleName = "showing the cursor's session"
	// ruleCutWithRoom (R4) is a known name cut, as "kiln-rel…", on a row with
	// more blank cells at its end than the cut took.
	ruleCutWithRoom sweepRuleName = "a cut name with room"
	// rulePaging (R6) is a list's pagination dots drawn, or not, otherwise
	// than as its items outgrow the rows it has.
	rulePaging sweepRuleName = "paging exactly when its items outgrow its rows"
)

// breakClass is what decision 3 owes a break: a fix, where it falls at one
// of the owner's sizes or an ordinary size, 80x24 or larger; nothing, where
// the design draws it so; or nothing yet, where it falls only at an extreme
// size, under extremeWidth wide or extremeHeight tall.
type breakClass string

const (
	owedAFix breakClass = "owed a fix"
	byDesign breakClass = "by design"
	extreme  breakClass = "extreme"
)

// A size is extreme under extremeWidth cells wide or extremeHeight rows
// tall: a break only there may stay listed.
const (
	extremeWidth  = 52
	extremeHeight = 12
)

// knownBreak is a break the sweep finds that the list holds: the drawing
// named breaks the rule at each size, as "73x61", for the reason why, of the
// class decision 3 gives it. A cut the design draws names its cuts, as
// "kiln-rel…", the only cuts it passes over; any other break passes over
// the rule at those sizes.
type knownBreak struct {
	fixture string
	sizes   []string
	rule    sweepRuleName
	class   breakClass
	why     string
	cuts    []string
}

// knownBreaks are the breaks the sweep finds that are listed rather than
// fixed. One that no longer breaks where it's named fails the sweep, so the
// list holds only what's drawn.
var knownBreaks = slices.Concat(owedBreaks(), designedBreaks(), extremeBreaks())

// owedBreaks are the breaks decision 3 owes a fix, each falling at one of
// the owner's sizes, listed until the fix lands.
func owedBreaks() []knownBreak {
	return nil
}

// Why a name is cut with room on a session's row: the row keeps its
// indicator slot and right margin whether or not the session shows an
// indicator, so the indicator column holds still down the list.
const indicatorSlotKept = "the row keeps its indicator slot and margin, shown or not, so the indicator column holds still down the list"

// designedBreaks are what the design draws that the sweep takes for a break.
func designedBreaks() []knownBreak {
	return []knownBreak{
		{
			fixture: "loading-screen", rule: ruleChrome, class: byDesign, sizes: slices.Concat(sizesOf(sweptWidths, 12), sizesOf(sweptWidths[2:], 15)),
			why: "the loading page has no header: it centres its block, which reaches the header's row once the terminal is shorter than it",
		},
		{
			fixture: "projects", rule: ruleChrome, class: byDesign, sizes: sizesOf(sweptWidths, 12),
			why: "below 13 rows the Projects page can't hold one two-line project row, its pagination dots and its footer; a project row is two lines by design",
		},
		{
			fixture: "loading-error", rule: ruleChrome, class: byDesign, sizes: []string{"138x15"},
			why: "the loading page has no header: it centres its error block, which reaches the header's row once the terminal is shorter than it",
		},
		{
			fixture: "theme-panel-adaptive-pair", rule: ruleChrome, class: byDesign, sizes: sizesOf(sweptWidths[1:], 12),
			why: "below the page-aligned floor the panel takes its compact shape, its rule closing up onto the page's header row",
		},
		{
			fixture: "theme-panel-adaptive-pair", rule: rulePaging, class: byDesign, sizes: sizesOf([]int{40}, 12, 15, 18),
			why: "40 cells wide, the panel, an opaque layer over the page, covers the middle of the page's list, where its pagination dots are drawn",
		},
		{fixture: "sessions-flat", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"loom-p8…"}, sizes: sizesOf([]int{20}, sweptHeights[3:]...)},
		{fixture: "sessions-flat", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"quill-U…"}, sizes: sizesOf([]int{20}, sweptHeights[5:]...)},
		{fixture: "sessions-paged", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"session…"}, sizes: sizesOf([]int{20}, sweptHeights[3:]...)},
		{fixture: "sessions-by-project", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"lumen webhooks and watche…"}, sizes: sizesOf([]int{40}, sweptHeights[4:]...)},
		{fixture: "sessions-by-tag", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"lumen webhooks and watche…"}, sizes: sizesOf([]int{40}, sweptHeights[5:]...)},
		{fixture: "theme-panel-adaptive-pair", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"loom-p8…"}, sizes: sizesOf([]int{20}, sweptHeights[5:]...)},
		{fixture: "theme-panel-adaptive-pair", rule: ruleCutWithRoom, class: byDesign, why: indicatorSlotKept, cuts: []string{"quill-U…"}, sizes: sizesOf([]int{20}, sweptHeights[6:]...)},
	}
}

// extremeBreaks are the breaks that fall only at extreme sizes.
func extremeBreaks() []knownBreak {
	return []knownBreak{
		{
			fixture: "sessions-by-project", rule: ruleFits, class: extreme, sizes: sizesOf([]int{20}, sweptHeights...),
			why: "20 cells wide, a group's heading row draws wider than the terminal",
		},
		{
			fixture: "sessions-by-tag", rule: ruleFits, class: extreme, sizes: sizesOf([]int{20}, sweptHeights...),
			why: "20 cells wide, a group's heading row draws wider than the terminal",
		},
		{
			fixture: "theme-panel-adaptive-pair", rule: ruleChrome, class: extreme, sizes: []string{"20x12", "20x15"},
			why: "20 cells wide, the notice band that closes the panel wraps to four rows the list's budget doesn't give up, so the footer goes off the foot",
		},
		{
			fixture: "theme-panel-adaptive-pair", rule: ruleCursor, class: extreme, sizes: append(sizesOf(sweptWidths, 8), "20x10", "20x12"),
			why: "the notice band that closes the panel, too short or too narrow, takes every row the list had, the cursor's among them",
		},
	}
}

// sizesOf names each of the widths given at each of the heights given, as
// "73x61".
func sizesOf(widths []int, heights ...int) []string {
	var sizes []string
	for _, w := range widths {
		for _, h := range heights {
			sizes = append(sizes, Size{Width: w, Height: h}.String())
		}
	}
	return sizes
}

// sweepBreak is a rule a drawing breaks, and what shows it does.
type sweepBreak struct {
	rule  sweepRuleName
	shown string
}

// designedAt are the cuts the design draws that breaks has the drawing named
// cut with room at the size named.
func designedAt(breaks []knownBreak, name, at string) []knownBreak {
	var designed []knownBreak
	for _, k := range breaks {
		if len(k.cuts) > 0 && k.fixture == name && slices.Contains(k.sizes, at) {
			designed = append(designed, k)
		}
	}
	return designed
}

// cutsOf are the cuts of each of the design's given.
func cutsOf(designed []knownBreak) []string {
	var cuts []string
	for _, k := range designed {
		cuts = append(cuts, k.cuts...)
	}
	return cuts
}

// undrawn are the design's cuts given none of whose cuts is among those
// drawn.
func undrawn(designed []knownBreak, drawn []nameCut) []knownBreak {
	return slices.DeleteFunc(slices.Clone(designed), func(k knownBreak) bool {
		return slices.ContainsFunc(drawn, func(d nameCut) bool { return slices.Contains(k.cuts, d.cut) })
	})
}

// brokenAt are the rules breaks has the drawing named break at the size
// named, but the design's cuts.
func brokenAt(breaks []knownBreak, name, at string) []sweepRuleName {
	var rules []sweepRuleName
	for _, k := range breaks {
		if len(k.cuts) == 0 && k.fixture == name && slices.Contains(k.sizes, at) {
			rules = append(rules, k.rule)
		}
	}
	return rules
}

// judged are the breaks found of none of the rules known, and the rules
// known none of them is of.
func judged(found []sweepBreak, known []sweepRuleName) (unknown []sweepBreak, cleared []sweepRuleName) {
	for _, b := range found {
		if !slices.Contains(known, b.rule) {
			unknown = append(unknown, b)
		}
	}
	for _, rule := range known {
		if !slices.ContainsFunc(found, func(b sweepBreak) bool { return b.rule == rule }) {
			cleared = append(cleared, rule)
		}
	}
	return unknown, cleared
}

// isExtreme reports whether the size named is extreme.
func isExtreme(at string) bool {
	var w, h int
	if _, err := fmt.Sscanf(at, "%dx%d", &w, &h); err != nil {
		return false
	}
	return w < extremeWidth || h < extremeHeight
}

// Each known break names a drawing the sweep draws, at sizes it draws that
// drawing at, each drawing, size and rule, or cut, once.
func TestEveryKnownBreakNamesAFixtureAndSizeTheSweepDraws(t *testing.T) {
	sizes := make(map[string][]string)
	for _, d := range sweptDrawings(t) {
		for _, s := range d.sizes {
			sizes[d.name] = append(sizes[d.name], s.String())
		}
	}
	named := make(map[string]bool)
	for _, k := range knownBreaks {
		drawn, ok := sizes[k.fixture]
		if !ok {
			t.Errorf("knownBreaks names %s, which the sweep doesn't draw", k.fixture)
			continue
		}
		if len(k.sizes) == 0 {
			t.Errorf("knownBreaks names %s breaking %s at no size", k.fixture, k.rule)
		}
		for _, at := range k.sizes {
			if !slices.Contains(drawn, at) {
				t.Errorf("knownBreaks names %s at %s, a size the sweep doesn't draw it at", k.fixture, at)
			}
			what := []string{string(k.rule)}
			if len(k.cuts) > 0 {
				what = k.cuts
			}
			for _, w := range what {
				if once := k.fixture + " " + at + " " + w; named[once] {
					t.Errorf("knownBreaks name %s at %s breaking %s twice", k.fixture, at, w)
				} else {
					named[once] = true
				}
			}
		}
	}
}

// Each known break says why, and is of one of decision 3's classes: an
// extreme one only at extreme sizes, and a cut the design draws only as by
// design, of the rule of a cut name with room.
func TestEveryKnownBreakSaysWhy(t *testing.T) {
	for _, k := range knownBreaks {
		if k.why == "" {
			t.Errorf("knownBreaks names %s breaking %s at %q without saying why", k.fixture, k.rule, k.sizes)
		}
		if problem := k.misclassed(); problem != "" {
			t.Errorf("knownBreaks names %s breaking %s at %q: %s", k.fixture, k.rule, k.sizes, problem)
		}
	}
}

// misclassed is what's wrong with the break's class under decision 3, ""
// where nothing is.
func (k knownBreak) misclassed() string {
	switch {
	case !slices.Contains([]breakClass{owedAFix, byDesign, extreme}, k.class):
		return "its class is none of decision 3's"
	case len(k.cuts) > 0 && (k.class != byDesign || k.rule != ruleCutWithRoom):
		return "a cut the design draws is by design, of the rule of a cut name with room"
	case k.class == extreme && !slices.ContainsFunc(k.sizes, isExtreme):
		return "an extreme break falls at extreme sizes"
	case k.class == extreme && slices.ContainsFunc(k.sizes, func(at string) bool { return !isExtreme(at) }):
		return "an extreme break falls only at extreme sizes"
	case k.class == owedAFix && !slices.ContainsFunc(k.sizes, func(at string) bool { return !isExtreme(at) }):
		return "a break only at extreme sizes owes no fix: it's extreme"
	}
	return ""
}

func TestAKnownBreakIsClassedAsDecision3ClassesIt(t *testing.T) {
	for _, tt := range []struct {
		name string
		k    knownBreak
		ok   bool
	}{
		{name: "owed a fix at the owner's size", k: knownBreak{class: owedAFix, rule: ruleFits, sizes: []string{"73x61"}}, ok: true},
		{name: "owed a fix at the owner's size and an extreme one", k: knownBreak{class: owedAFix, rule: ruleFits, sizes: []string{"20x8", "73x61"}}, ok: true},
		{name: "owed a fix, only at an extreme size", k: knownBreak{class: owedAFix, rule: ruleFits, sizes: []string{"20x8"}}},
		{name: "extreme, at no size", k: knownBreak{class: extreme, rule: ruleFits}},
		{name: "extreme, narrow", k: knownBreak{class: extreme, rule: ruleFits, sizes: []string{"40x24"}}, ok: true},
		{name: "extreme, short", k: knownBreak{class: extreme, rule: ruleFits, sizes: []string{"120x10"}}, ok: true},
		{name: "extreme, at the owner's size too", k: knownBreak{class: extreme, rule: ruleFits, sizes: []string{"40x24", "73x61"}}},
		{name: "extreme, at the least width and height", k: knownBreak{class: extreme, rule: ruleFits, sizes: []string{"52x12"}}},
		{name: "the design's cuts", k: knownBreak{class: byDesign, rule: ruleCutWithRoom, cuts: []string{"kiln-…"}, sizes: []string{"73x61"}}, ok: true},
		{name: "cuts, owed a fix", k: knownBreak{class: owedAFix, rule: ruleCutWithRoom, cuts: []string{"kiln-…"}, sizes: []string{"73x61"}}},
		{name: "cuts, of another rule", k: knownBreak{class: byDesign, rule: ruleChrome, cuts: []string{"kiln-…"}, sizes: []string{"73x61"}}},
		{name: "no class", k: knownBreak{rule: ruleFits, sizes: []string{"73x61"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if problem := tt.k.misclassed(); (problem == "") != tt.ok {
				t.Errorf("misclassed = %q, want ok %t", problem, tt.ok)
			}
		})
	}
}

// A break the list names passes where it breaks, a break it doesn't name
// fails, and a break it names that no longer breaks fails.
func TestAKnownBreakPassesWhereItBreaksAndFailsWhereItsCleared(t *testing.T) {
	known := []knownBreak{
		{fixture: "sessions-flat", rule: ruleFits, class: extreme, sizes: []string{"20x8"}},
		{fixture: "sessions-flat", rule: ruleChrome, class: owedAFix, sizes: []string{"73x61"}},
		{fixture: "projects", rule: ruleCursor, class: owedAFix, sizes: []string{"73x61"}},
	}
	fits, chrome, cursor := sweepBreak{rule: ruleFits}, sweepBreak{rule: ruleChrome}, sweepBreak{rule: ruleCursor}
	for _, tt := range []struct {
		name, fixture, at string
		found             []sweepBreak
		unknown           []sweepBreak
		cleared           []sweepRuleName
	}{
		{name: "breaking what's known", fixture: "sessions-flat", at: "20x8", found: []sweepBreak{fits}},
		{name: "breaking what's known and more", fixture: "sessions-flat", at: "73x61", found: []sweepBreak{chrome, cursor}, unknown: []sweepBreak{cursor}},
		{name: "breaking what's known elsewhere", fixture: "sessions-flat", at: "108x24", found: []sweepBreak{fits}, unknown: []sweepBreak{fits}},
		{name: "breaking what another drawing is known for", fixture: "sessions-flat", at: "73x61", found: []sweepBreak{chrome, cursor}, unknown: []sweepBreak{cursor}},
		{name: "breaking another rule than what's known", fixture: "projects", at: "73x61", found: []sweepBreak{fits}, unknown: []sweepBreak{fits}, cleared: []sweepRuleName{ruleCursor}},
		{name: "no longer breaking what's known", fixture: "sessions-flat", at: "20x8", cleared: []sweepRuleName{ruleFits}},
		{name: "breaking nothing, with nothing known", fixture: "sessions-flat", at: "108x24"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			unknown, cleared := judged(tt.found, brokenAt(known, tt.fixture, tt.at))
			if !slices.Equal(unknown, tt.unknown) || !slices.Equal(cleared, tt.cleared) {
				t.Errorf("judged %v unknown and %q cleared, want %v and %q", unknown, cleared, tt.unknown, tt.cleared)
			}
		})
	}
}

// The design's cuts are passed over at the sizes it names them at alone,
// and fail where none of them is drawn with room any longer.
func TestADesignedCutPassesWhereItsDrawnAndFailsWhereItsNot(t *testing.T) {
	designed := knownBreak{fixture: "sessions-flat", rule: ruleCutWithRoom, class: byDesign, why: "names at their most", cuts: []string{"kiln-relay…", "orchard-…"}, sizes: []string{"138x30"}}
	known := []knownBreak{designed, {fixture: "sessions-flat", rule: ruleChrome, class: owedAFix, sizes: []string{"138x30"}}}
	drawn := func(cuts ...string) []nameCut {
		var drawn []nameCut
		for _, c := range cuts {
			drawn = append(drawn, nameCut{cut: c})
		}
		return drawn
	}
	for _, tt := range []struct {
		name, at string
		drawn    []nameCut
		excused  []string
		cleared  int
	}{
		{name: "both drawn", at: "138x30", drawn: drawn("orchard-…", "kiln-relay…"), excused: designed.cuts},
		{name: "one drawn", at: "138x30", drawn: drawn("kiln-relay…", "lumen…"), excused: designed.cuts},
		{name: "neither drawn", at: "138x30", drawn: drawn("lumen…"), excused: designed.cuts, cleared: 1},
		{name: "at a size it names none", at: "73x61", drawn: drawn("kiln-relay…")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at := designedAt(known, "sessions-flat", tt.at)
			if got, cleared := cutsOf(at), undrawn(at, tt.drawn); !slices.Equal(got, tt.excused) || len(cleared) != tt.cleared {
				t.Errorf("excused %q and %d cleared, want %q and %d", got, len(cleared), tt.excused, tt.cleared)
			}
		})
	}
}
