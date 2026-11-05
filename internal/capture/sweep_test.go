package capture

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// The sweep's grid: widths either side of those a layout changes at and the
// owner's widths, and heights from where a page first has rows for a list to
// the owner's tallest.
var (
	sweptWidths  = []int{20, 40, 52, 63, 73, 80, 100, 108, 120, 138, 160, 173, 230}
	sweptHeights = []int{8, 10, 12, 15, 18, 24, 30, 36, 40, 44, 53, 61}
)

// ownersSizes are the terminals every drawing is swept at, as the owner uses
// them: a phone upright, tall and short, and on its side, tall and short; an
// iPad, tall and short; a desktop; and a MacBook's side screen.
var ownersSizes = []Size{
	{Width: 73, Height: 61}, {Width: 73, Height: 36}, {Width: 138, Height: 30}, {Width: 138, Height: 15},
	{Width: 108, Height: 44}, {Width: 108, Height: 24}, {Width: 173, Height: 53}, {Width: 109, Height: 53},
}

// sweptFully are the drawings swept across the whole grid, one for each kind
// of page the picker draws and one for each resume renderer, by the kind each
// stands for. Every other drawing is swept at the owner's sizes alone.
var sweptFully = map[string]string{
	"sessions-flat":             "the flat list",
	"sessions-by-project":       "the list by project",
	"sessions-by-tag":           "the list by tag",
	"sessions-paged":            "the paged list",
	"sessions-search-results":   "a session search",
	"projects":                  "the Projects page",
	"preview-screen":            "the preview",
	"loading-screen":            "the loading page",
	"theme-panel-adaptive-pair": "the theme panel",
	resumeWaitingSurface:        "the resume panel",
	resumeDiscardSurface:        "the discard confirmation",
}

// tallest is the height every shorter draw's chrome and cursor are held to:
// tall enough that every page fits.
const tallest = 60

// chromeFloor is the least height whose page has room for its header, a row
// of its list and its footer, so its chrome is held to the tall draw's: below
// it a page gives its chrome up to whatever it can show.
const chromeFloor = 12

// swept is one drawing as the sweep draws it: its name, the names its world
// gives that a row may cut, its reading at each size it's swept at, and, of a
// page of the picker, its reading as tall as everything fits at each of those
// widths.
type swept struct {
	name string
	// names are what a row may cut: sessions' names and labels, their
	// directories' last parts, and projects' names, paths' last parts and tags.
	names []string
	sizes []Size
	at    map[Size]reading
	// picker is a page of the picker, with a header, a footer and a cursor; a
	// resume surface is a card drawn into a pane, with none of them.
	picker bool
	tall   map[int]reading
	// fresh are, of a drawing sweptFresh names, its draws built at each size
	// rather than resized to it.
	fresh map[Size]reading
}

// sweptFresh are the drawings whose list is filtered as they're built, read
// built at each size as well as resized to it: a filter's matches are paged
// on a path a resize doesn't take.
var sweptFresh = []string{"sessions-filtering", "sessions-search-results", "sessions-search-home"}

// reading is a draw as the rules read it: how it misfits its terminal, ""
// where it fits; its rows as plain text; and its cursor's row, as cursorOf
// reads it.
type reading struct {
	misfit string
	rows   []string
	cursor string
}

// theSweep is every drawing FixtureNames lists but the contrast swatch, read
// at each size it's swept at: drawn once, for every rule to read.
var theSweep = sync.OnceValues(func() ([]swept, error) {
	th, err := FramesTheme()
	if err != nil {
		return nil, err
	}
	var all []swept
	for _, name := range FixtureNames() {
		if name == ContrastValidationFixture {
			continue
		}
		d, err := sweptDrawing(name, th)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		all = append(all, d)
	}
	return all, nil
})

func sweptDrawings(t *testing.T) []swept {
	t.Helper()
	all, err := theSweep()
	switch {
	case err != nil:
		t.Fatalf("the sweep: %v", err)
	case len(all) == 0:
		t.Fatal("the sweep draws nothing")
	}
	return all
}

func sweptDrawing(name string, th theme.Theme) (swept, error) {
	d := swept{name: name, sizes: sweepSizes(sweptFully[name] != ""), at: make(map[Size]reading)}
	draw, err := drawerOf(&d, th)
	if err != nil {
		return swept{}, err
	}
	built, err := builderOf(&d, th)
	if err != nil {
		return swept{}, err
	}
	for _, at := range d.sizes {
		if d.at[at], err = readingOf(draw, at, th, d.names); err != nil {
			return swept{}, err
		}
		if built != nil {
			if d.fresh[at], err = readingOf(built, at, th, d.names); err != nil {
				return swept{}, err
			}
		}
		if _, drawn := d.tall[at.Width]; d.picker && !drawn {
			if d.tall[at.Width], err = readingOf(draw, Size{Width: at.Width, Height: tallest}, th, d.names); err != nil {
				return swept{}, err
			}
		}
	}
	return d, nil
}

// drawerOf is the drawing's draw at a size, as View's raw content, filling in
// what its world gives the sweep.
func drawerOf(d *swept, th theme.Theme) (func(w, h int) string, error) {
	if s, ok := SurfaceByName(d.name); ok {
		return func(w, h int) string { return surfaceAt(s, th, w, h) }, nil
	}
	f, err := FixtureByName(d.name)
	if err != nil {
		return nil, err
	}
	d.names, d.picker, d.tall = knownNames(f), true, make(map[int]reading)
	return drawnAt(f, th), nil
}

// drawnAt draws the fixture as captured at its frame's size, then its
// terminal resized to the size given, its declared size passed over, so a
// geometry fixture is swept like any other. Each size is resized to from the
// captured model, never from the last size drawn, so no size's draw hangs on
// the order they're drawn in.
func drawnAt(f *Fixture, th theme.Theme) func(w, h int) string {
	var captured tea.Model = f.modelAt(f.Deps(th), cmp.Or(f.frame.Width, harnessSize.Width), cmp.Or(f.frame.Height, harnessSize.Height))
	return func(w, h int) string {
		sized, _ := captured.Update(tea.WindowSizeMsg{Width: w, Height: h})
		return sized.View().Content
	}
}

// builderOf is the draw of a drawing sweptFresh names built at a size, nil
// for any other.
func builderOf(d *swept, th theme.Theme) (func(w, h int) string, error) {
	if !slices.Contains(sweptFresh, d.name) {
		return nil, nil
	}
	f, err := FixtureByName(d.name)
	if err != nil {
		return nil, err
	}
	d.fresh = make(map[Size]reading)
	return func(w, h int) string { return f.modelAt(f.Deps(th), w, h).View().Content }, nil
}

// surfaceAt is the resume surface drawn by its renderer at the size given,
// its pinned size passed over.
func surfaceAt(s *Surface, th theme.Theme, w, h int) string {
	seed := s.WithTheme(th, false).screen
	seed.Width, seed.Height = w, h
	return s.draw(seed)
}

// sweepSizes are the whole grid and the owner's sizes, where a drawing is
// swept fully; else the owner's sizes alone.
func sweepSizes(fully bool) []Size {
	sizes := slices.Clone(ownersSizes)
	if !fully {
		return sizes
	}
	for _, w := range sweptWidths {
		for _, h := range sweptHeights {
			if s := (Size{Width: w, Height: h}); !slices.Contains(sizes, s) {
				sizes = append(sizes, s)
			}
		}
	}
	return sizes
}

// knownNames are the names the fixture's world gives that a row may cut.
func knownNames(f *Fixture) []string {
	var names []string
	for _, s := range f.Lister.sessions {
		names = append(names, s.Name, s.Label, path.Base(s.Dir))
	}
	for _, p := range f.projectStore.projects {
		names = append(names, p.Name, path.Base(p.Path))
		names = append(names, p.Tags...)
	}
	names = slices.DeleteFunc(names, func(n string) bool { return n == "" || n == "." || n == "/" })
	slices.Sort(names)
	return slices.Compact(names)
}

// readingOf is the draw at the size given, read.
func readingOf(draw func(w, h int) string, at Size, th theme.Theme, names []string) (reading, error) {
	content := draw(at.Width, at.Height)
	cursor, err := cursorOf(content, th, names)
	if err != nil {
		return reading{}, fmt.Errorf("at %s: %w", at, err)
	}
	return reading{misfit: misfitOf(content, at), rows: plainRows(content, at.Height), cursor: cursor}, nil
}

// misfitOf is how the content misfits a terminal of the size given: other
// than its rows, or a row other than its width; "" where it fits.
func misfitOf(content string, at Size) string {
	rows := strings.Split(content, "\n")
	if len(rows) != at.Height {
		return fmt.Sprintf("%d rows", len(rows))
	}
	for i, row := range rows {
		if w := ansi.StringWidth(row); w != at.Width {
			return fmt.Sprintf("row %d is %d cells", i+1, w)
		}
	}
	return ""
}

// plainRows are a frame's rows as plain text, as many as it's tall.
func plainRows(content string, height int) []string {
	rows := strings.Split(ansi.Strip(content), "\n")
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// cursorOf is the text the picker's cursor row shows, as selectedRun reads
// it, where it shows a name the world gives; "" where no row does.
func cursorOf(content string, th theme.Theme, names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	g, err := parse(content)
	if err != nil {
		return "", err
	}
	selection := colourOf(th.BgSelection.Color())
	for _, row := range g {
		if text := selectedRun(row, selection); text != "" && showsAName(text, names) {
			return text, nil
		}
	}
	return "", nil
}

// selectedRun is the row's first run of cells on the selection's background,
// as text, trimmed; or, where none is, as under NO_COLOR, the row from its
// selector bar on.
func selectedRun(row []cell, selection colour) string {
	var run strings.Builder
	for _, c := range row {
		if c.bg != selection {
			if run.Len() > 0 {
				break
			}
			continue
		}
		if c.glyph != continuation {
			run.WriteRune(c.glyph)
		}
	}
	if run.Len() > 0 {
		return strings.TrimSpace(run.String())
	}
	if _, bar, ok := strings.Cut(grid{row}.text()[0], "▌"); ok {
		return strings.TrimSpace("▌" + bar)
	}
	return ""
}

// showsAName reports whether the text shows one of the names given, whole or
// cut to a rune at least and an ellipsis.
func showsAName(text string, names []string) bool {
	return slices.ContainsFunc(names, func(name string) bool {
		if strings.Contains(text, name) {
			return true
		}
		_, cut := cutOf(text, name)
		return cut
	})
}

// Every drawing sweptFully names is one the sweep draws, so a fixture
// renamed can't leave its page out of the full grid unnoticed.
func TestEveryDrawingSweptFullyIsOneTheSweepDraws(t *testing.T) {
	all := sweptDrawings(t)
	for name := range sweptFully {
		d := slices.IndexFunc(all, func(d swept) bool { return d.name == name })
		if d < 0 || len(all[d].sizes) <= len(ownersSizes) {
			t.Errorf("sweptFully names %s, which the sweep doesn't draw across the grid", name)
		}
	}
}

// A fixture's draw at a size is the same whatever size was drawn before it,
// as each is resized to from the fixture as captured.
func TestASweptDrawIsTheSameWhateverWasDrawnBefore(t *testing.T) {
	th := framesTheme(t)
	sizes := []Size{{Width: 138, Height: 15}, {Width: 20, Height: 8}, {Width: 230, Height: 61}, {Width: 73, Height: 36}}
	for name := range sweptFully {
		f, err := FixtureByName(name)
		if err != nil {
			continue
		}
		forward, backward := drawnAt(f, th), drawnAt(f, th)
		drawn := make(map[Size]string)
		for _, at := range sizes {
			drawn[at] = forward(at.Width, at.Height)
		}
		for _, at := range slices.Backward(sizes) {
			if backward(at.Width, at.Height) != drawn[at] {
				t.Errorf("%s at %s draws otherwise for the sizes drawn before it", name, at)
			}
		}
	}
}

// R1: every drawing, at every size it's swept at, draws exactly as many rows
// as the terminal is tall, each exactly as many cells as it is wide.
func TestEveryDrawingFitsItsTerminalAtEverySweptSize(t *testing.T) {
	sweepRule(t, ruleFits, func(d swept, at Size) string { return d.at[at].misfit })
}

// R2: every page of the picker, at every height it's swept at from
// chromeFloor up, draws its header's first row and its footer row as it draws
// them as tall as everything fits, as the height changes neither: no list
// draws on them.
func TestNoPageDrawsOnItsChromeAtAnySweptHeight(t *testing.T) {
	sweepRule(t, ruleChrome, func(d swept, at Size) string {
		if !d.picker || at.Height < chromeFloor {
			return ""
		}
		rows, tall := d.at[at].rows, d.tall[at.Width].rows
		header, footer, tallFooter := tui.Vinset, at.Height-1-tui.Vinset, tallest-1-tui.Vinset
		switch {
		case rows[header] != tall[header]:
			return fmt.Sprintf("the header reads %q, want %q", rows[header], tall[header])
		case rows[footer] != tall[tallFooter]:
			return fmt.Sprintf("the footer reads %q, want %q", rows[footer], tall[tallFooter])
		}
		return ""
	})
}

// R6: every page of the picker whose list fits on one page as tall as
// everything fits draws its pagination dots, at every height it's swept at
// from chromeFloor up, exactly when its items outgrow the rows its list has
// there: resized, and for the drawings sweptFresh names built at the size
// too.
func TestEveryListPagesExactlyWhenItsItemsOutgrowItsRows(t *testing.T) {
	judged := 0
	sweepRule(t, rulePaging, func(d swept, at Size) string {
		if !d.picker || at.Height < chromeFloor {
			return ""
		}
		tall := d.tall[at.Width].rows
		for _, draw := range []struct {
			how  string
			read map[Size]reading
		}{{"resized", d.at}, {"built", d.fresh}} {
			r, ok := draw.read[at]
			if !ok {
				continue
			}
			problem, read := pagingMisdrawn(r.rows, tall, at.Height, d.names)
			if read {
				judged++
			}
			if problem != "" {
				return draw.how + ", " + problem
			}
		}
		return ""
	})
	if judged == 0 {
		t.Error("no drawing has a list the rule can read")
	}
}

// pagingMisdrawn is how a page h rows tall, as rows reads it, draws its
// pagination dots other than exactly when the items the tall draw shows
// outgrow the rows its list has: the rows from the items' first to the
// footer's rule, but the one bubbles/list keeps under them. read is whether
// the page has a list the rule can read: a footer's rule in both draws, and
// in the tall one a block of rows over the rule showing a known name, with no
// dots under it.
func pagingMisdrawn(rows, tall []string, h int, names []string) (problem string, read bool) {
	tallRule, rule := tallest-2-tui.Vinset, h-2-tui.Vinset
	if !isFooterRule(pageOf(tall[tallRule])) || !isFooterRule(pageOf(rows[rule])) {
		return "", false
	}
	first, last, ok := itemBlock(tall[:tallRule], names)
	if !ok || slices.ContainsFunc(tall[first:tallRule], isDotsRow) {
		return "", false
	}
	items, room := last-first+1, rule-1-first
	if dots := slices.ContainsFunc(rows[first:rule], isDotsRow); dots != (items > room) {
		return fmt.Sprintf("%d rows of items, %d rows for them, draws dots %t", items, room, dots), true
	}
	return "", true
}

// itemBlock is the first and last of the last run of rows that aren't blank,
// where one of them shows a known name.
func itemBlock(rows []string, names []string) (first, last int, ok bool) {
	last = len(rows) - 1
	for last >= 0 && pageOf(rows[last]) == "" {
		last--
	}
	if last < 0 {
		return 0, 0, false
	}
	first = last
	for first > 0 && pageOf(rows[first-1]) != "" {
		first--
	}
	return first, last, slices.ContainsFunc(rows[first:last+1], func(r string) bool { return showsAName(pageOf(r), names) })
}

// pageOf is the row's text the page beneath the theme panel shows, trimmed:
// all of it, or what's left of the panel's border.
func pageOf(row string) string {
	page, _, _ := strings.Cut(row, "│")
	return strings.TrimSpace(page)
}

func isFooterRule(text string) bool {
	return text != "" && strings.Trim(text, "▔") == ""
}

// isDotsRow reports whether the row is a list's pagination: dots, or the
// count bubbles/list draws where they don't fit, as "2/5".
func isDotsRow(row string) bool {
	text := pageOf(row)
	if text != "" && strings.Trim(text, "•") == "" {
		return true
	}
	var page, pages int
	n, err := fmt.Sscanf(text, "%d/%d", &page, &pages)
	return err == nil && n == 2 && fmt.Sprintf("%d/%d", page, pages) == text
}

// R3: every page of the picker with a session or project under its cursor as
// tall as everything fits shows that row, as drawn there, at every height
// it's swept at.
func TestTheCursorsSessionIsShownAtEverySweptHeight(t *testing.T) {
	held := 0
	sweepRule(t, ruleCursor, func(d swept, at Size) string {
		want := d.tall[at.Width].cursor
		if want == "" {
			return ""
		}
		held++
		if got := d.at[at].cursor; got != want {
			return fmt.Sprintf("the cursor's row reads %q, want %q", got, want)
		}
		return ""
	})
	if held == 0 {
		t.Error("no drawing has a cursor on a session or project to hold")
	}
}

// R4: no row cuts a name its drawing's world gives with "…" while more blank
// cells end the row's content, inside the gutter, than the cut took, but the
// cuts the design draws.
func TestNoKnownNameIsCutWithRoom(t *testing.T) {
	for _, d := range sweptDrawings(t) {
		t.Run(d.name, func(t *testing.T) {
			for _, at := range d.sizes {
				name := at.String()
				designed := designedAt(knownBreaks, d.name, name)
				cuts := cutsWithRoom(d.at[at].rows, at.Width, d.names)
				rest := slices.DeleteFunc(slices.Clone(cuts), func(c nameCut) bool { return slices.Contains(cutsOf(designed), c.cut) })
				var found []sweepBreak
				if len(rest) > 0 {
					found = append(found, sweepBreak{rule: ruleCutWithRoom, shown: rest[0].shown})
				}
				reportJudged(t, d.name, name, found, rulesAt(brokenAt(knownBreaks, d.name, name), ruleCutWithRoom))
				for _, k := range undrawn(designed, cuts) {
					t.Errorf("%s at %s no longer draws %q with room: delete it from knownBreaks", d.name, name, k.cuts)
				}
			}
		})
	}
}

// sweepRule holds every drawing, at every size it's swept at, to the rule
// given, as breaks reads it: what breaks it, "" where nothing does. A break no
// known break names fails, as does a known break of the rule that no longer
// breaks it.
func sweepRule(t *testing.T, rule sweepRuleName, breaks func(swept, Size) string) {
	t.Helper()
	for _, d := range sweptDrawings(t) {
		t.Run(d.name, func(t *testing.T) {
			for _, at := range d.sizes {
				var found []sweepBreak
				if shown := breaks(d, at); shown != "" {
					found = append(found, sweepBreak{rule: rule, shown: shown})
				}
				reportJudged(t, d.name, at.String(), found, rulesAt(brokenAt(knownBreaks, d.name, at.String()), rule))
			}
		})
	}
}

// reportJudged fails the test for each break found no known break names, and
// each known break no break found is of.
func reportJudged(t *testing.T, name, at string, found []sweepBreak, known []sweepRuleName) {
	t.Helper()
	unknown, cleared := judged(found, known)
	for _, b := range unknown {
		t.Errorf("%s at %s breaks the rule of %s: %s", name, at, b.rule, b.shown)
	}
	for _, rule := range cleared {
		t.Errorf("%s at %s no longer breaks the rule of %s: delete it from knownBreaks", name, at, rule)
	}
}

// rulesAt are the rules given that are the one named.
func rulesAt(rules []sweepRuleName, rule sweepRuleName) []sweepRuleName {
	return slices.DeleteFunc(rules, func(r sweepRuleName) bool { return r != rule })
}

// nameCut is a known name cut short as drawn, as "kiln-relay…", and what
// shows it had room.
type nameCut struct {
	cut   string
	shown string
}

// cutsWithRoom are where the rows cut one of the names given, two runes left
// at least, with more blank cells at the end of the row's content, inside the
// gutter, than the cut took: of the names the cut may be of, the longest's.
func cutsWithRoom(rows []string, width int, names []string) []nameCut {
	var cuts []nameCut
	for _, row := range rows {
		spare := width - tui.Hinset - ansi.StringWidth(strings.TrimRight(row, " "))
		took, left := 0, ""
		for _, name := range names {
			cut, ok := cutOf(row, name)
			if !ok || utf8.RuneCountInString(cut) < 2 {
				continue
			}
			if n := utf8.RuneCountInString(name) - utf8.RuneCountInString(cut); n > took {
				took, left = n, cut
			}
		}
		if took > 0 && spare > took {
			cuts = append(cuts, nameCut{cut: left + "…", shown: fmt.Sprintf("%q, %d cells cut, with %d blank at its row's end:\n%s", left+"…", took, spare, row)})
		}
	}
	return cuts
}

// cutOf is the longest start of the name, short of it whole, that the text
// shows with an ellipsis after it, where the start begins a word: at the
// text's start, or after a space, a rule, a slash or a bracket.
func cutOf(text, name string) (string, bool) {
	runes := []rune(name)
	for n := len(runes) - 1; n >= 1; n-- {
		cut := string(runes[:n])
		for at, rest := 0, text; ; {
			i := strings.Index(rest, cut+"…")
			if i < 0 {
				break
			}
			if at+i == 0 || strings.ContainsRune(" │/(", lastRune(text[:at+i])) {
				return cut, true
			}
			at, rest = at+i+1, rest[i+1:]
		}
	}
	return "", false
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

// sweptKeys are the keys the panic sweep presses in turn: every key a page of
// the picker or a surface answers to.
var sweptKeys = []tea.KeyPressMsg{
	{Code: tea.KeyUp}, {Code: tea.KeyDown}, {Code: tea.KeyUp, Mod: tea.ModCtrl}, {Code: tea.KeyDown, Mod: tea.ModCtrl},
	keyEnter(), keyRune('/'), {Code: tea.KeySpace, Text: " "}, keyRune('s'), keyRune('n'), keyRune('r'), keyRune('k'),
	keyRune('q'), keyRune('x'), keyRune('t'), keyRune('m'), keyRune('?'), keyRune('e'), keyRune('d'), keyRune('l'),
	keyRune('y'), {Code: tea.KeyEscape},
}

// R5: every drawing but the swatch takes each key in turn without panicking.
// One swept fully draws at every width from 1 to 240 at its frame's height,
// resized from one to the next; then, from where it's captured, takes a key
// at every fourth width. The rest take each key at their frame's size. A
// key's command is never run: every seam is a fake, and the loading
// receivers block by design.
func TestNoFixtureKeyOrWidthPanics(t *testing.T) {
	th := framesTheme(t)
	for _, name := range FixtureNames() {
		if name == ContrastValidationFixture {
			continue
		}
		t.Run(name, func(t *testing.T) {
			m, size := capturedAt(t, name, th)
			if sweptFully[name] == "" {
				for _, k := range sweptKeys {
					m = takenAt(t, m, size, k)
				}
				return
			}
			for width := 1; width <= 240; width++ {
				m = takenAt(t, m, Size{Width: width, Height: size.Height})
			}
			m, _ = capturedAt(t, name, th)
			for i, width := 0, 1; width <= 240; i, width = i+1, width+4 {
				m = takenAt(t, m, Size{Width: width, Height: size.Height}, sweptKeys[i%len(sweptKeys)])
			}
		})
	}
}

// capturedAt is the drawing named as it's captured, and the size its frame
// is kept at.
func capturedAt(t *testing.T, name string, th theme.Theme) (tea.Model, Size) {
	t.Helper()
	if s, ok := SurfaceByName(name); ok {
		return s.WithTheme(th, false), s.drawing().Size()
	}
	f, err := FixtureByName(name)
	if err != nil {
		t.Fatalf("FixtureByName(%s): %v", name, err)
	}
	size := f.drawing().Size()
	return f.ModelAt(th, size.Width, size.Height), size
}

// takenAt is m once the keys given are taken and the terminal resized to
// size, drawn, failing the test where taking them, resizing or drawing
// panics. A surface is drawn by its renderer at the size, past its pin.
func takenAt(t *testing.T, m tea.Model, size Size, keys ...tea.KeyPressMsg) tea.Model {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("at %s, taking %v: panicked: %v", size, keys, r)
		}
	}()
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: size.Width, Height: size.Height})
	if s, ok := m.(*Surface); ok {
		seed := s.screen
		seed.Width, seed.Height = size.Width, size.Height
		s.draw(seed)
		return m
	}
	m.View()
	return m
}
